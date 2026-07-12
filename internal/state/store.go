package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store manages the observer-local reconcile state file.
type Store struct {
	path string
}

// NewStore constructs a state store for one absolute or relative file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Path returns the store file path.
func (s *Store) Path() string {
	return s.path
}

// Load reads the current state file or returns an empty state when absent.
func (s *Store) Load() (File, error) {
	content, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyFile(), nil
		}
		return File{}, fmt.Errorf("reading state file %q: %w", s.path, err)
	}

	var file File
	if err := json.Unmarshal(content, &file); err != nil {
		return File{}, fmt.Errorf("decoding state file %q: %w", s.path, err)
	}

	if file.Version == 0 || file.Version == 1 {
		// Version 1 did not record whether SecretTargets was a complete
		// inventory. Preserve its entries, but mark the file as current so
		// successful reconciles can rewrite discovered applications. Retained
		// entries remain untrusted because their marker is false.
		file.Version = CurrentVersion
	}
	if file.Version != CurrentVersion {
		return File{}, fmt.Errorf("unsupported state file version %d in %q", file.Version, s.path)
	}
	if file.Applications == nil {
		file.Applications = map[string]Entry{}
	}

	return file, nil
}

// Save writes the full state file atomically.
func (s *Store) Save(file File) error {
	if file.Version == 0 {
		file.Version = CurrentVersion
	}
	if file.Applications == nil {
		file.Applications = map[string]Entry{}
	}

	content, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state file %q: %w", s.path, err)
	}
	content = append(content, '\n')

	parentDir := filepath.Dir(s.path)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("creating state directory %q: %w", parentDir, err)
	}

	tempFile, err := os.CreateTemp(parentDir, filepath.Base(s.path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp state file for %q: %w", s.path, err)
	}
	tempPath := tempFile.Name()

	cleanup := func() {
		_ = os.Remove(tempPath)
	}

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("writing temp state file %q: %w", tempPath, err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("syncing temp state file %q: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("closing temp state file %q: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, s.path); err != nil {
		cleanup()
		return fmt.Errorf("replacing state file %q: %w", s.path, err)
	}

	return nil
}

func emptyFile() File {
	return File{
		Version:      CurrentVersion,
		Applications: map[string]Entry{},
	}
}
