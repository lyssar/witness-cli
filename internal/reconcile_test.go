package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcileRunLoadManifestRetainsManifest(t *testing.T) {
	t.Parallel()

	configRoot := t.TempDir()
	manifestPath := filepath.Join(configRoot, "manifest.yaml")

	manifest := "apiVersion: skuld/v1alpha1\n" +
		"kind: Observer\n" +
		"metadata:\n" +
		"  name: observer\n" +
		"spec:\n" +
		"  project: observer\n" +
		"  destination: /tmp\n" +
		"  source:\n" +
		"    repoURL: https://example.com/repo.git\n" +
		"    targetRevision: HEAD\n" +
		"    path: /\n" +
		"  timeout:\n" +
		"    reconciliation: 180s\n"

	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	run := NewReconcileRun(configRoot)
	if err := run.LoadManifest(); err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if run.manifest.Spec.Source.RepoURL != "https://example.com/repo.git" {
		t.Fatalf("expected loaded manifest repo url to be retained, got %q", run.manifest.Spec.Source.RepoURL)
	}
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
		{
			name:     "removes url user info",
			source:   "https://user:token@example.com/org/repo.git",
			expected: "https://example.com/org/repo.git",
		},
		{
			name:     "keeps non url syntax as is",
			source:   "git@github.com:lyssar/skuld-cli.git",
			expected: "git@github.com:lyssar/skuld-cli.git",
		},
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
