package reconcile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractSubmodulePaths(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitmodules"), []byte("[submodule \"dep\"]\n\tpath = third_party/dep\n\turl = https://example.com/dep.git\n"), 0o600); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}

	paths, err := extractSubmodulePaths(context.Background(), repoRoot)
	if err != nil {
		t.Fatalf("extract submodule paths: %v", err)
	}
	if len(paths) != 1 || paths[0] != "third_party/dep" {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}

func TestExtractSubmodulePathsInvalidFails(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitmodules"), []byte("[submodule \"dep\"]\n\tpath = ../../etc\n\turl = https://example.com/dep.git\n"), 0o600); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}

	_, err := extractSubmodulePaths(context.Background(), repoRoot)
	if err == nil || !strings.Contains(err.Error(), "submodule") {
		t.Fatalf("expected submodule error, got %v", err)
	}
}
