package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLoadMissingReturnsEmptyFile(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	file, err := store.Load()
	if err != nil {
		t.Fatalf("load missing state: %v", err)
	}
	if file.Version != CurrentVersion {
		t.Fatalf("expected version %d, got %d", CurrentVersion, file.Version)
	}
	if len(file.Applications) != 0 {
		t.Fatalf("expected empty applications, got %#v", file.Applications)
	}
}

func TestStoreSaveAndLoad(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state", "state.json")
	store := NewStore(path)

	file := File{
		Applications: map[string]Entry{
			"apps/hello": {Status: StatusHealthy, RuntimeSlug: "apps-hello", ComposeFiles: []string{"compose.yaml"}},
		},
	}
	if err := store.Save(file); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	entry, ok := loaded.Applications["apps/hello"]
	if !ok {
		t.Fatalf("expected apps/hello entry, got %#v", loaded.Applications)
	}
	if entry.RuntimeSlug != "apps-hello" {
		t.Fatalf("expected runtime slug apps-hello, got %q", entry.RuntimeSlug)
	}
	if len(entry.ComposeFiles) != 1 || entry.ComposeFiles[0] != "compose.yaml" {
		t.Fatalf("unexpected compose files: %#v", entry.ComposeFiles)
	}
}

func TestStoreSaveIsAtomic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := NewStore(path)

	if err := store.Save(File{Applications: map[string]Entry{"a": {Status: StatusHealthy}}}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("expected only final state file, got %#v", entries)
	}
}
