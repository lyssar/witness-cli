package reconcile

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerRunValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("nil context", func(t *testing.T) {
		var nilCtx context.Context
		err := NewRunner(t.TempDir()).Run(nilCtx)
		if err == nil || !strings.Contains(err.Error(), "context is required") {
			t.Fatalf("expected context error, got %v", err)
		}
	})

	t.Run("empty config root", func(t *testing.T) {
		err := NewRunner(" \n\t ").Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "config root is required") {
			t.Fatalf("expected config root error, got %v", err)
		}
	})

	t.Run("missing manifest", func(t *testing.T) {
		configRoot := t.TempDir()
		writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "manifest file could not be found") {
			t.Fatalf("expected missing manifest error, got %v", err)
		}
	})

	t.Run("missing age key", func(t *testing.T) {
		configRoot := t.TempDir()
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest("https://example.com/repo.git", "HEAD", "/", "/tmp/witness"))

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "age file could not be found") {
			t.Fatalf("expected missing age key error, got %v", err)
		}
	})

	t.Run("age key must not be group readable", func(t *testing.T) {
		configRoot := t.TempDir()
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest("https://example.com/repo.git", "HEAD", "/", "/tmp/witness"))
		agePath := filepath.Join(configRoot, "age.key")
		writeFile(t, agePath, "dummy")
		if err := os.Chmod(agePath, 0o644); err != nil {
			t.Fatalf("chmod age key: %v", err)
		}

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "age file permissions must not grant group/other access") {
			t.Fatalf("expected age key permission error, got %v", err)
		}
	})

	t.Run("age key must parse valid identities", func(t *testing.T) {
		configRoot := t.TempDir()
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest("https://example.com/repo.git", "HEAD", "/", "/tmp/witness"))
		writeFile(t, filepath.Join(configRoot, "age.key"), "not-an-age-key")

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "age file must contain valid age identities") {
			t.Fatalf("expected invalid age key parse error, got %v", err)
		}
	})

	t.Run("repo url required", func(t *testing.T) {
		configRoot := t.TempDir()
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest("  ", "HEAD", "/", "/tmp/witness"))
		writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "manifest spec.source.repoURL is required") {
			t.Fatalf("expected repo url error, got %v", err)
		}
	})

	t.Run("repo url must not be option-like", func(t *testing.T) {
		configRoot := t.TempDir()
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest("--upload-pack=/tmp/pwn", "HEAD", "/", "/tmp/witness"))
		writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "manifest spec.source.repoURL must not start with '-'") {
			t.Fatalf("expected option-like repo url error, got %v", err)
		}
	})

	t.Run("target revision required", func(t *testing.T) {
		configRoot := t.TempDir()
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest("https://example.com/repo.git", "  ", "/", "/tmp/witness"))
		writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "manifest spec.source.targetRevision is required") {
			t.Fatalf("expected target revision error, got %v", err)
		}
	})

	t.Run("destination required", func(t *testing.T) {
		configRoot := t.TempDir()
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest("https://example.com/repo.git", "HEAD", "/", "  "))
		writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "manifest spec.destination is required") {
			t.Fatalf("expected destination required error, got %v", err)
		}
	})

	t.Run("destination absolute", func(t *testing.T) {
		configRoot := t.TempDir()
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest("https://example.com/repo.git", "HEAD", "/", "relative/path"))
		writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "manifest spec.destination must be absolute") {
			t.Fatalf("expected destination absolute error, got %v", err)
		}
	})

	t.Run("ssh key required", func(t *testing.T) {
		configRoot := t.TempDir()
		manifest := "apiVersion: witness/v1alpha1\n" +
			"kind: Observer\n" +
			"spec:\n" +
			"  destination: /tmp/witness\n" +
			"  source:\n" +
			"    repoURL: https://example.com/repo.git\n" +
			"    targetRevision: HEAD\n" +
			"    path: /\n"
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), manifest)
		writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

		err := NewRunner(configRoot).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "manifest spec.source.sshKey is required") {
			t.Fatalf("expected ssh key required error, got %v", err)
		}
	})
}

func TestResolveDiscoveryRoot(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()

	testCases := []struct {
		name       string
		sourcePath string
		expected   string
		errPart    string
	}{
		{name: "empty path resolves to repo root", sourcePath: "", expected: repoRoot},
		{name: "dot resolves to repo root", sourcePath: ".", expected: repoRoot},
		{name: "slash resolves to repo root", sourcePath: "/", expected: repoRoot},
		{name: "leading slash is logical repo path", sourcePath: "/apps/frontend", expected: filepath.Join(repoRoot, "apps", "frontend")},
		{name: "relative path resolves under repo root", sourcePath: "apps/backend", expected: filepath.Join(repoRoot, "apps", "backend")},
		{name: "dot dot is rejected", sourcePath: "..", errPart: "escapes repository root"},
		{name: "traversal is rejected", sourcePath: "apps/../../etc", errPart: "escapes repository root"},
		{name: "traversal with leading slash is rejected", sourcePath: "/../etc", errPart: "escapes repository root"},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			got, err := resolveDiscoveryRoot(repoRoot, testCase.sourcePath)
			if testCase.errPart != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", testCase.errPart)
				}

				if !strings.Contains(err.Error(), testCase.errPart) {
					t.Fatalf("expected error containing %q, got %v", testCase.errPart, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("resolve discovery root: %v", err)
			}

			if got != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestSanitizeGitSourceForLog(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		source   string
		expected string
	}{
		{name: "removes url user info", source: "https://user:token@example.com/org/repo.git", expected: "https://example.com/org/repo.git"},
		{name: "removes user info from multiple urls", source: "failed: https://u1:t1@example.com/r1.git and https://u2:t2@example.org/r2.git", expected: "failed: https://example.com/r1.git and https://example.org/r2.git"},
		{name: "keeps non url syntax as is", source: "git@github.com:lyssar/witness-cli.git", expected: "git@github.com:lyssar/witness-cli.git"},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			got := sanitizeGitSourceForLog(testCase.source)
			if got != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestRunnerRunDiscoversApplications(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for reconcile runner tests")
	}

	fixture := newGitFixture(t)

	t.Run("discovery root subtree", func(t *testing.T) {
		configRoot := t.TempDir()
		destinationRoot := filepath.Join(t.TempDir(), "dest")
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest(fixture.remotePath, "main", "apps", destinationRoot))
		writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))
		writeFile(t, filepath.Join(destinationRoot, "app-main", "witness.yaml"), appManifest("app-main"))
		writeFile(t, filepath.Join(destinationRoot, "app-main", "compose.yaml"), "services: {}\n")

		if err := NewRunner(configRoot).Run(context.Background()); err != nil {
			t.Fatalf("runner run: %v", err)
		}

		if _, err := os.Stat(filepath.Join(configRoot, "repo", "apps", "app-main", "witness.yaml")); err != nil {
			t.Fatalf("expected synced app manifest in runtime repo: %v", err)
		}
	})

	t.Run("missing discovery root is treated as zero apps", func(t *testing.T) {
		configRoot := t.TempDir()
		destinationRoot := filepath.Join(t.TempDir(), "dest")
		writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest(fixture.remotePath, "main", "missing", destinationRoot))
		writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

		if err := NewRunner(configRoot).Run(context.Background()); err != nil {
			t.Fatalf("runner run: %v", err)
		}
	})
}

func TestSubmodulePathsUnderDiscoveryRoot(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	discoveryRoot := filepath.Join(repoRoot, "apps")

	paths, err := submodulePathsUnderDiscoveryRoot([]string{"apps/app-main/vendor/dep", "third_party/lib"}, discoveryRoot, repoRoot)
	if err != nil {
		t.Fatalf("normalize submodule paths: %v", err)
	}
	if len(paths) != 1 || paths[0] != "app-main/vendor/dep" {
		t.Fatalf("unexpected normalized paths: %#v", paths)
	}
}

func minimalManifest(repoURL, targetRevision, sourcePath, destination string) string {
	return "apiVersion: witness/v1alpha1\n" +
		"kind: Observer\n" +
		"spec:\n" +
		"  destination: " + destination + "\n" +
		"  source:\n" +
		"    repoURL: " + repoURL + "\n" +
		"    targetRevision: " + targetRevision + "\n" +
		"    path: " + sourcePath + "\n" +
		"    sshKey: ssh-key-placeholder\n"
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %q: %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}
