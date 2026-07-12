package reconcile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeRelativeBindMount(t *testing.T) {
	tests := []struct {
		source      string
		want        string
		wantErr     bool
		errContains string
	}{
		{source: "./data", want: "data"},
		{source: "./config/app", want: "config/app"},
		{source: "./data/../config", want: "config"}, // cleaned
		{source: "/absolute/path", want: ""},
		{source: "named-volume", want: ""},
		{source: "data", want: ""}, // no ./ prefix
		{source: "./../escape", wantErr: true, errContains: "must be inside"},
		{source: "./", wantErr: true, errContains: "must be inside"},
		{source: "../escape", want: ""}, // no ./ prefix, treated as named
		{source: ".", want: ""},         // bare dot, no ./ prefix
		{source: "..", want: ""},        // bare dot-dot, no ./ prefix
		{source: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got, err := normalizeRelativeBindMount(tt.source)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestComposeVolumeSourcesShortSyntax(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n      - ./config:/config\n      - /abs:/abs\n      - named:/target\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	sources, err := composeVolumeSources(dest, "compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0] != "data" || sources[1] != "config" {
		t.Fatalf("expected [data config], got %#v", sources)
	}
}

func TestComposeVolumeSourcesLongSyntax(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	compose := "services:\n  app:\n    volumes:\n      - type: bind\n        source: ./data\n        target: /data\n      - type: volume\n        source: named\n        target: /target\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	sources, err := composeVolumeSources(dest, "compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0] != "data" {
		t.Fatalf("expected [data], got %#v", sources)
	}
}

func TestComposeVolumeSourcesDeduplicates(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n      - ./data:/data-again\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	sources, err := composeVolumeSources(dest, "compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0] != "data" {
		t.Fatalf("expected [data], got %#v", sources)
	}
}

func TestCopyDirRootedRejectsSymlinkInSource(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	// Create a source directory with a symlink inside.
	oldDir := filepath.Join(dir, "old")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(oldDir, "link")); err != nil {
		t.Fatal(err)
	}

	err = copyDirRooted(dest, "old", "new")
	if err == nil {
		t.Fatal("expected symlink rejection, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestCopyDirRootedRejectsSymlinkAncestry(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	// Create a symlink as an ancestor of the target.
	if err := os.MkdirAll(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	err = copyDirRooted(dest, "real/child", "link/nested")
	if err == nil {
		t.Fatal("expected symlink ancestry rejection, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestCopyDirRootedCopiesNestedFiles(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	oldDir := filepath.Join(dir, "old")
	if err := os.MkdirAll(filepath.Join(oldDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "top.txt"), []byte("top"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "nested", "deep.txt"), []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyDirRooted(dest, "old", "new"); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(dir, "new", "top.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "top" {
		t.Fatalf("expected 'top', got %q", string(b))
	}
	b, err = os.ReadFile(filepath.Join(dir, "new", "nested", "deep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "deep" {
		t.Fatalf("expected 'deep', got %q", string(b))
	}
}
