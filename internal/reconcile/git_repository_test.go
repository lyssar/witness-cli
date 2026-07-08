package reconcile

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitRepositorySyncTargetRevisions(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for repository sync tests")
	}

	fixture := newGitFixture(t)

	testCases := []struct {
		name           string
		targetRevision string
		expectedSHA    string
	}{
		{name: "head checks out remote default branch", targetRevision: "HEAD", expectedSHA: fixture.mainSHA},
		{name: "branch checks out tracked branch", targetRevision: "feature", expectedSHA: fixture.featureSHA},
		{name: "tag checks out detached tag", targetRevision: "v1.0.0", expectedSHA: fixture.tagSHA},
		{name: "commit checks out detached commit", targetRevision: fixture.featureSHA, expectedSHA: fixture.featureSHA},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			repoPath := filepath.Join(t.TempDir(), "repo")

			repo, err := newGitRepository(repoPath, fixture.remotePath)
			if err != nil {
				t.Fatalf("new git repository: %v", err)
			}

			operation, err := repo.Sync(context.Background(), testCase.targetRevision)
			if err != nil {
				t.Fatalf("sync repository: %v", err)
			}

			if operation != "clone" {
				t.Fatalf("expected clone operation, got %q", operation)
			}

			gotSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
			if gotSHA != testCase.expectedSHA {
				t.Fatalf("expected HEAD %q, got %q", testCase.expectedSHA, gotSHA)
			}
		})
	}
}

func TestNewGitRepositoryRejectsOptionLikeRemoteURL(t *testing.T) {
	t.Parallel()

	_, err := newGitRepository(filepath.Join(t.TempDir(), "repo"), "-cprotocol.file.allow=always")
	if err == nil {
		t.Fatalf("expected option-like remote url to be rejected")
	}

	if !strings.Contains(err.Error(), "repository remote url must not start with '-'") {
		t.Fatalf("expected option-like remote url validation error, got %v", err)
	}
}

func TestGitRepositoryCloneErrorIncludesSanitizedStderrAndWrappedError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for repository sync tests")
	}

	repoPath := filepath.Join(t.TempDir(), "repo")
	repo, err := newGitRepository(repoPath, "https://user:super-secret@example.invalid/repo.git")
	if err != nil {
		t.Fatalf("new git repository: %v", err)
	}

	err = repo.clone(context.Background())
	if err == nil {
		t.Fatalf("expected clone to fail for invalid remote")
	}

	message := err.Error()
	if strings.Contains(message, "super-secret") {
		t.Fatalf("expected clone error to redact credentials, got %q", message)
	}

	if !strings.Contains(message, "https://example.invalid/repo.git") {
		t.Fatalf("expected clone error to include sanitized remote url, got %q", message)
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected clone error to wrap exec.ExitError, got %T", err)
	}
}

func TestGitRepositoryRunErrorSanitizesStderrAndWrapsError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for repository sync tests")
	}

	repoPath := t.TempDir()
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "remote", "add", "origin", "https://user:super-secret@example.invalid/repo.git")

	repo, err := newGitRepository(repoPath, "https://example.invalid/repo.git")
	if err != nil {
		t.Fatalf("new git repository: %v", err)
	}

	_, err = repo.run(context.Background(), "fetch", "--prune", "--tags", "origin")
	if err == nil {
		t.Fatalf("expected fetch to fail for invalid remote")
	}

	message := err.Error()
	if strings.Contains(message, "super-secret") {
		t.Fatalf("expected run error to redact credentials, got %q", message)
	}

	if !strings.Contains(message, "https://example.invalid/repo.git") {
		t.Fatalf("expected run error to include sanitized remote url, got %q", message)
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected run error to wrap exec.ExitError, got %T", err)
	}
}

func TestSanitizeAndTruncateGitCommandOutput(t *testing.T) {
	t.Run("redacts credentials", func(t *testing.T) {
		message := "fatal: unable to access 'https://user:token@example.com/repo.git/': could not resolve host"
		got := sanitizeAndTruncateGitCommandOutput(message)
		if strings.Contains(got, "token") {
			t.Fatalf("expected credentials to be redacted, got %q", got)
		}

		if !strings.Contains(got, "https://example.com/repo.git/") {
			t.Fatalf("expected sanitized url in output, got %q", got)
		}
	})

	t.Run("redacts credentials from all urls", func(t *testing.T) {
		message := "fatal: failed to access https://user-one:token-one@example.com/repo.git and https://user-two:token-two@example.org/another.git"
		got := sanitizeAndTruncateGitCommandOutput(message)

		if strings.Contains(got, "token-one") || strings.Contains(got, "token-two") {
			t.Fatalf("expected all credentials to be redacted, got %q", got)
		}

		if !strings.Contains(got, "https://example.com/repo.git") {
			t.Fatalf("expected first url to be sanitized, got %q", got)
		}

		if !strings.Contains(got, "https://example.org/another.git") {
			t.Fatalf("expected second url to be sanitized, got %q", got)
		}
	})

	t.Run("truncates oversized output", func(t *testing.T) {
		longMessage := strings.Repeat("x", maxGitCommandErrorOutputLen+32)
		got := sanitizeAndTruncateGitCommandOutput(longMessage)

		if len([]rune(got)) != maxGitCommandErrorOutputLen+1 {
			t.Fatalf("expected truncated output length %d, got %d", maxGitCommandErrorOutputLen+1, len([]rune(got)))
		}

		if !strings.HasSuffix(got, "…") {
			t.Fatalf("expected truncated output to end with ellipsis, got %q", got)
		}
	})
}

type gitFixture struct {
	remotePath string
	mainSHA    string
	featureSHA string
	tagSHA     string
}

func newGitFixture(t *testing.T) gitFixture {
	t.Helper()

	root := t.TempDir()
	seedPath := filepath.Join(root, "seed")
	remotePath := filepath.Join(root, "remote.git")

	if err := os.MkdirAll(seedPath, 0o755); err != nil {
		t.Fatalf("mkdir seed repository: %v", err)
	}

	runGit(t, seedPath, "init", "-b", "main")
	runGit(t, seedPath, "config", "user.email", "test@example.com")
	runGit(t, seedPath, "config", "user.name", "test")

	writeFile(t, filepath.Join(seedPath, "README.md"), "main\n")
	runGit(t, seedPath, "add", "README.md")
	runGit(t, seedPath, "commit", "-m", "main commit")
	_ = strings.TrimSpace(runGit(t, seedPath, "rev-parse", "HEAD"))

	tagSHA := strings.TrimSpace(runGit(t, seedPath, "rev-parse", "HEAD"))
	runGit(t, seedPath, "tag", "v1.0.0")
	runGit(t, seedPath, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(seedPath, "apps", "app-one", "witness.yaml"), appManifest("app-one"))
	writeFile(t, filepath.Join(seedPath, "apps", "app-one", "compose.yaml"), "services: {}\n")
	runGit(t, seedPath, "add", "apps/app-one/witness.yaml", "apps/app-one/compose.yaml")
	runGit(t, seedPath, "commit", "-m", "feature commit")
	featureSHA := strings.TrimSpace(runGit(t, seedPath, "rev-parse", "HEAD"))

	runGit(t, seedPath, "checkout", "main")
	writeFile(t, filepath.Join(seedPath, "apps", "app-main", "witness.yaml"), appManifest("app-main"))
	writeFile(t, filepath.Join(seedPath, "apps", "app-main", "compose.yaml"), "services: {}\n")
	runGit(t, seedPath, "add", "apps/app-main/witness.yaml", "apps/app-main/compose.yaml")
	runGit(t, seedPath, "commit", "-m", "add app manifest")
	mainSHA := strings.TrimSpace(runGit(t, seedPath, "rev-parse", "HEAD"))

	runGit(t, root, "init", "--bare", remotePath)
	runGit(t, seedPath, "remote", "add", "origin", remotePath)
	runGit(t, seedPath, "push", "--all", "origin")
	runGit(t, seedPath, "push", "--tags", "origin")
	runGit(t, root, "--git-dir", remotePath, "symbolic-ref", "HEAD", "refs/heads/main")

	return gitFixture{remotePath: remotePath, mainSHA: mainSHA, featureSHA: featureSHA, tagSHA: tagSHA}
}

func appManifest(name string) string {
	return "apiVersion: witness.dev/v1alpha1\n" +
		"kind: Application\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"spec:\n" +
		"  provisioner: docker-compose\n" +
		"  composeFiles:\n" +
		"    - compose.yaml\n"
}

func runGit(t *testing.T, workdir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = workdir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}

	return string(output)
}
