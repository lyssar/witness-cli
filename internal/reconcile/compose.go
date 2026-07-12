package reconcile

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// composeVolumeSources extracts relative bind-mount source directories from a
// compose file read through the destination root. Returns only paths starting
// with "./"; absolute paths and named volumes are ignored.
func composeVolumeSources(d *destinationFS, composeRef string) ([]string, error) {
	file, err := d.root.Open(composeRef)
	if err != nil {
		return nil, fmt.Errorf("open compose file %q: %w", composeRef, err)
	}
	defer func() { _ = file.Close() }()

	var raw struct {
		Services map[string]struct {
			Volumes yaml.Node `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.NewDecoder(file).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode compose file %q: %w", composeRef, err)
	}

	var sources []string
	for _, svc := range raw.Services {
		if svc.Volumes.Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range svc.Volumes.Content {
			source := volumeSourceFromNode(*item)
			if source == "" {
				continue
			}
			normalized, err := normalizeRelativeBindMount(source)
			if err != nil {
				return nil, fmt.Errorf("compose file %q volume source %q: %w", composeRef, source, err)
			}
			if normalized == "" {
				continue
			}
			sources = append(sources, normalized)
		}
	}
	return deduplicateStrings(sources), nil
}

// volumeSourceFromNode extracts the host-side path from a single compose volume
// entry. It handles short syntax ("/host:/container") and long syntax ({source: "./data"}).
func volumeSourceFromNode(node yaml.Node) string {
	// Short syntax: "./data:/data" or "volume_name:/data"
	if node.Kind == yaml.ScalarNode {
		parts := strings.SplitN(node.Value, ":", 2)
		if len(parts) > 0 {
			return parts[0]
		}
		return node.Value
	}
	// Long syntax: {type: bind, source: ./data, target: /data}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "source" {
				return node.Content[i+1].Value
			}
		}
	}
	return ""
}

// normalizeRelativeBindMount validates and normalizes a relative bind-mount
// source path. Returns an empty string for absolute paths or named volumes.
func normalizeRelativeBindMount(source string) (string, error) {
	if source == "" || filepath.IsAbs(source) {
		return "", nil
	}
	// Only handle paths relative to the compose file directory (./ prefix).
	// Paths without ./ are treated as named volumes and ignored.
	if !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "."+string(filepath.Separator)) {
		return "", nil
	}
	clean := filepath.Clean(source)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("volume source must be inside the app directory")
	}
	// Remove leading "./" or ".\" for use relative to the compose file.
	clean = strings.TrimPrefix(clean, "."+string(filepath.Separator))
	return clean, nil
}

// preserveVolumeDirs copies directories found by composeVolumeSources from old
// live into the stage tree so Docker bind-mount data survives a promotion
// rename cycle. Volume source paths are resolved relative to each compose
// file's directory, matching Docker Compose semantics. Existing directories
// in the stage tree are not overwritten.
func preserveVolumeDirs(d *destinationFS, oldLive, stage string, composeFiles []string) error {
	if oldLive == "" || stage == "" {
		return nil
	}
	for _, composeFile := range composeFiles {
		composeRef := filepath.Join(stage, filepath.FromSlash(composeFile))
		sources, err := composeVolumeSources(d, composeRef)
		if err != nil {
			return err
		}
		// Docker Compose resolves relative bind-mount sources against the
		// compose file's directory, so we must do the same.
		composeDir := filepath.Dir(filepath.FromSlash(composeFile))
		for _, src := range sources {
			relDir := src
			if composeDir != "." {
				relDir = filepath.Join(composeDir, src)
			}
			oldDir := filepath.Join(oldLive, relDir)
			if info, err := d.root.Lstat(oldDir); err != nil || !info.IsDir() {
				continue
			}
			newDir := filepath.Join(stage, relDir)
			if info, err := d.root.Lstat(newDir); err == nil && info.IsDir() {
				continue // already present in stage
			}
			if err := copyDirRooted(d, oldDir, newDir); err != nil {
				return fmt.Errorf("preserving volume directory %q from old live: %w", src, err)
			}
		}
	}
	return nil
}

// copyDirRooted copies the contents of oldDir to newDir using rooted
// destination operations. Symlinks are rejected.
func copyDirRooted(d *destinationFS, oldDir, newDir string) error {
	if err := d.ensureNoSymlinkAncestry(newDir); err != nil {
		return err
	}
	entries, err := d.readDir(oldDir)
	if err != nil {
		return err
	}
	if err := d.root.MkdirAll(newDir, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		oldPath := filepath.Join(oldDir, name)
		newPath := filepath.Join(newDir, name)
		info, err := d.root.Lstat(oldPath)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("volume directory contains symlink %q", oldPath)
		}
		if info.IsDir() {
			if err := copyDirRooted(d, oldPath, newPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := copyFileRooted(d, oldPath, newPath, info); err != nil {
			return err
		}
	}
	return nil
}

func copyFileRooted(d *destinationFS, oldPath, newPath string, info fs.FileInfo) error {
	in, err := d.root.Open(oldPath)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := d.root.OpenFile(newPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func deduplicateStrings(slice []string) []string {
	seen := make(map[string]struct{}, len(slice))
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		result = append(result, s)
	}
	return result
}
