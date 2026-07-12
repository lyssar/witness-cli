package reconcile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lyssar/witness-cli/internal/application"
)

// destinationFS confines all destination filesystem access to one os.Root.
type destinationFS struct {
	root *os.Root
}

func openDestinationFS(destination string) (*destinationFS, error) {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return nil, fmt.Errorf("creating destination root: %w", err)
	}
	info, err := os.Lstat(destination)
	if err != nil {
		return nil, fmt.Errorf("inspect destination root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("destination root must be a non-symlink directory: %q", destination)
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return nil, fmt.Errorf("open destination root: %w", err)
	}
	return &destinationFS{root: root}, nil
}

func (d *destinationFS) Close() error { return d.root.Close() }

func liveRef(operationalID string) (string, error) {
	if err := application.ValidateIdentityPath(operationalID); err != nil {
		return "", fmt.Errorf("validating operational identity %q: %w", operationalID, err)
	}
	return filepath.FromSlash(operationalID), nil
}

func archiveRef(operationalID string, now time.Time) (string, error) {
	live, err := liveRef(operationalID)
	if err != nil {
		return "", err
	}
	return filepath.Join("archive", live, now.UTC().Format("20060102T150405.000000000Z")), nil
}

// canonicalArchiveRef accepts only the state representation produced by archiveRef.
func canonicalArchiveRef(operationalID, value string) (string, bool) {
	if value == "" || filepath.IsAbs(value) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	parts := strings.Split(filepath.ToSlash(clean), "/")
	wantID := strings.Split(operationalID, "/")
	if len(parts) != len(wantID)+2 || parts[0] != "archive" {
		return "", false
	}
	for i := range wantID {
		if parts[i+1] != wantID[i] {
			return "", false
		}
	}
	if _, err := time.Parse("20060102T150405.000000000Z", parts[len(parts)-1]); err != nil {
		return "", false
	}
	return filepath.FromSlash(strings.Join(parts, "/")), true
}

func (d *destinationFS) ensureNoSymlinkAncestry(ref string) error {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(ref)), "/")
	for i := 1; i < len(parts); i++ {
		ancestor := filepath.FromSlash(strings.Join(parts[:i], "/"))
		info, err := d.root.Lstat(ancestor)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination path ancestor %q is a symlink", ancestor)
		}
	}
	return nil
}

// validateArchiveDirectory verifies that a canonical archive reference resolves
// inside the destination to a real directory, without a symlink in its path.
func (d *destinationFS) validateArchiveDirectory(ref string) error {
	if err := d.ensureNoSymlinkAncestry(ref); err != nil {
		return err
	}
	info, err := d.root.Lstat(ref)
	if err != nil {
		return fmt.Errorf("stat archive directory %q: %w", ref, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("archive path must be a non-symlink directory: %q", ref)
	}
	return nil
}

func (d *destinationFS) copyExternalTree(source, target string) error {
	if err := d.ensureNoSymlinkAncestry(target); err != nil {
		return err
	}
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dst := target
		if rel != "." {
			dst = filepath.Join(target, rel)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("staging tree contains symlink %q", path)
		}
		if info.IsDir() {
			return d.root.MkdirAll(dst, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staging tree contains unsupported file %q", path)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := d.root.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			if closeErr := in.Close(); closeErr != nil {
				return fmt.Errorf("open rooted destination %q: %w (closing staging source: %v)", dst, err, closeErr)
			}
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		inCloseErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return inCloseErr
	})
}

func (d *destinationFS) absolute(ref string) string { return filepath.Join(d.root.Name(), ref) }

func (d *destinationFS) readDir(ref string) ([]os.DirEntry, error) {
	dir, err := d.root.Open(ref)
	if err != nil {
		return nil, err
	}
	entries, readErr := dir.ReadDir(-1)
	closeErr := dir.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return entries, nil
}
