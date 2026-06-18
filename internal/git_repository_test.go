package internal

import (
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
		{name: "tag checks out detached tag", targetRevision: "v1.0.0", expectedSHA: fixture.mainSHA},
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

			operation, err := repo.Sync(testCase.targetRevision)
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

func TestGitRepositorySyncUpdateExistingRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for repository sync tests")
	}

	fixture := newGitFixture(t)
	repoPath := filepath.Join(t.TempDir(), "repo")

	repo, err := newGitRepository(repoPath, fixture.remotePath)
	if err != nil {
		t.Fatalf("new git repository: %v", err)
	}

	if _, err := repo.Sync("main"); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	operation, err := repo.Sync("feature")
	if err != nil {
		t.Fatalf("update sync: %v", err)
	}

	if operation != "update" {
		t.Fatalf("expected update operation, got %q", operation)
	}

	gotSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
	if gotSHA != fixture.featureSHA {
		t.Fatalf("expected feature HEAD %q, got %q", fixture.featureSHA, gotSHA)
	}
}

func TestGitRepositorySyncErrors(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for repository sync tests")
	}

	fixture := newGitFixture(t)

	t.Run("empty target revision is invalid", func(t *testing.T) {
		repoPath := filepath.Join(t.TempDir(), "repo")
		repo, err := newGitRepository(repoPath, fixture.remotePath)
		if err != nil {
			t.Fatalf("new git repository: %v", err)
		}

		if _, err := repo.Sync("  "); err == nil {
			t.Fatal("expected empty target revision to fail")
		}
	})

	t.Run("ambiguous branch and tag revision is rejected", func(t *testing.T) {
		repoPath := filepath.Join(t.TempDir(), "repo")
		repo, err := newGitRepository(repoPath, fixture.remotePath)
		if err != nil {
			t.Fatalf("new git repository: %v", err)
		}

		_, err = repo.Sync("release")
		if err == nil {
			t.Fatal("expected ambiguous target revision error")
		}

		if !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("expected ambiguous error, got %v", err)
		}
	})

	t.Run("unexpected existing remote is rejected", func(t *testing.T) {
		repoPath := filepath.Join(t.TempDir(), "repo")
		repo, err := newGitRepository(repoPath, fixture.remotePath)
		if err != nil {
			t.Fatalf("new git repository: %v", err)
		}

		if _, err := repo.Sync("main"); err != nil {
			t.Fatalf("initial sync: %v", err)
		}

		otherRemotePath := filepath.Join(t.TempDir(), "other-remote.git")
		runGit(t, t.TempDir(), "init", "--bare", otherRemotePath)
		runGit(t, repoPath, "remote", "set-url", "origin", otherRemotePath)

		_, err = repo.Sync("main")
		if err == nil {
			t.Fatal("expected remote mismatch to fail")
		}

		if !strings.Contains(err.Error(), "unexpected origin remote") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

type gitFixture struct {
	remotePath string
	mainSHA    string
	featureSHA string
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
	mainSHA := strings.TrimSpace(runGit(t, seedPath, "rev-parse", "HEAD"))

	runGit(t, seedPath, "tag", "v1.0.0")
	runGit(t, seedPath, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(seedPath, "feature.txt"), "feature\n")
	runGit(t, seedPath, "add", "feature.txt")
	runGit(t, seedPath, "commit", "-m", "feature commit")
	featureSHA := strings.TrimSpace(runGit(t, seedPath, "rev-parse", "HEAD"))

	runGit(t, seedPath, "checkout", "main")
	runGit(t, seedPath, "checkout", "-b", "release")
	runGit(t, seedPath, "checkout", "main")
	runGit(t, seedPath, "tag", "release")

	runGit(t, root, "init", "--bare", remotePath)
	runGit(t, seedPath, "remote", "add", "origin", remotePath)
	runGit(t, seedPath, "push", "--all", "origin")
	runGit(t, seedPath, "push", "--tags", "origin")
	runGit(t, root, "--git-dir", remotePath, "symbolic-ref", "HEAD", "refs/heads/main")

	return gitFixture{
		remotePath: remotePath,
		mainSHA:    mainSHA,
		featureSHA: featureSHA,
	}
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}
