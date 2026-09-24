package reconcile

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lyssar/witness-cli/internal/application"
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

// preserveVolumeDirs moves directories found by composeVolumeSources from old
// live into the stage tree so Docker bind-mount data survives a promotion
// rename cycle. Volume source paths are resolved relative to each compose
// file's directory, matching Docker Compose semantics. Moving (rather than
// copying) preserves container-owned ownership and modes without requiring
// read access to the data. A committed non-empty directory in the stage tree
// wins over the old live data; an absent or empty stage directory is replaced
// by the move. Volume claim ownership is applied after moving. The returned
// slice lists the relative directories actually moved, in order, so callers
// can reverse the moves for rollback.
func preserveVolumeDirs(d *destinationFS, oldLive, stage string, composeFiles []string, volumeClaims []application.VolumeClaim) ([]string, error) {
	if oldLive == "" || stage == "" {
		return nil, nil
	}
	// Build claim lookup: relative dir → (uid, gid)
	claimMap := volumeClaimMap(volumeClaims)
	var claimErrs []error
	var moved []string
	for _, composeFile := range composeFiles {
		composeRef := filepath.Join(stage, filepath.FromSlash(composeFile))
		sources, err := composeVolumeSources(d, composeRef)
		if err != nil {
			return moved, err
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
			info, err := d.root.Lstat(oldDir)
			if err != nil || !info.IsDir() {
				// Missing or single-file bind-mount (e.g. ./Caddyfile):
				// nothing to move.
				continue
			}
			newDir := filepath.Join(stage, relDir)
			if err := d.ensureNoSymlinkAncestry(oldDir); err != nil {
				return moved, err
			}
			if err := d.ensureNoSymlinkAncestry(newDir); err != nil {
				return moved, err
			}
			// Collision handling: a committed non-empty directory in the
			// stage tree wins; an absent or empty newDir is replaced by the
			// move.
			shouldMove := true
			applyClaim := true
			newInfo, newErr := d.root.Lstat(newDir)
			switch {
			case os.IsNotExist(newErr):
				// Absent — move directly.
			case newErr != nil:
				return moved, newErr
			case !newInfo.IsDir():
				// Committed single-file bind-mount (e.g. ./Caddyfile)
				// shadows the old directory; the committed seed wins and no
				// directory claim applies.
				shouldMove = false
				applyClaim = false
			default:
				entries, err := d.readDir(newDir)
				if err != nil {
					return moved, err
				}
				if len(entries) > 0 {
					// Non-empty committed seed wins; do not move. The old
					// live data is discarded with the backup, so make the
					// discard observable.
					shouldMove = false
					slog.Warn("Committed volume directory wins over old live data", "dir", relDir)
				} else {
					// Empty pre-created directory (ensureVolumeDirs): remove
					// it so the rename can replace it (os.Root.Rename refuses
					// to rename onto an existing directory).
					if err := d.root.Remove(newDir); err != nil {
						return moved, err
					}
				}
			}
			if shouldMove {
				if err := d.root.Rename(oldDir, newDir); err != nil {
					return moved, fmt.Errorf("preserving volume directory %q from old live: %w", src, err)
				}
				moved = append(moved, relDir)
			}
			if applyClaim {
				if err := applyVolumeClaim(d, claimMap, relDir, newDir, src); err != nil {
					claimErrs = append(claimErrs, err)
				}
			}
		}
	}
	return moved, errorsJoin(claimErrs)
}

// ensureVolumeDirs pre-creates Docker bind-mount directories found in compose
// files so they receive process-user ownership instead of root:root. Use on
// first deploy when no old live tree exists to preserve data from.
// Volume claim ownership is applied after creation.
func ensureVolumeDirs(d *destinationFS, stage string, composeFiles []string, volumeClaims []application.VolumeClaim) error {
	claimMap := volumeClaimMap(volumeClaims)
	var claimErrs []error
	for _, composeFile := range composeFiles {
		composeRef := filepath.Join(stage, filepath.FromSlash(composeFile))
		sources, err := composeVolumeSources(d, composeRef)
		if err != nil {
			return err
		}
		composeDir := filepath.Dir(filepath.FromSlash(composeFile))
		for _, src := range sources {
			relDir := src
			if composeDir != "." {
				relDir = filepath.Join(composeDir, src)
			}
			dir := filepath.Join(stage, relDir)
			// Bind-mount sources may be single files (e.g. ./Caddyfile) already
			// present in the stage tree. Leave them untouched; only pre-create
			// directories. Mirrors the file-vs-directory handling in
			// preserveVolumeDirs.
			if info, err := d.root.Lstat(dir); err == nil && !info.IsDir() {
				continue
			}
			if err := d.root.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("creating volume directory %q: %w", src, err)
			}
			if err := applyVolumeClaim(d, claimMap, relDir, dir, src); err != nil {
				claimErrs = append(claimErrs, err)
			}
		}
	}
	return errorsJoin(claimErrs)
}

// applyVolumeClaim sets ownership on a volume directory if a matching claim
// exists. Uses os.Lchown directly because os.Root.Chown can lose file
// capabilities (CAP_CHOWN) on Linux — see https://github.com/golang/go/issues/67002.
func applyVolumeClaim(d *destinationFS, claimMap map[string]application.VolumeClaim, relDir, absDir, src string) error {
	claim, ok := claimMap[filepath.ToSlash(relDir)]
	if !ok {
		return nil
	}
	if err := d.ensureNoSymlinkAncestry(absDir); err != nil {
		return err
	}
	hostPath := d.absolute(absDir)
	if _, err := os.Lstat(hostPath); err != nil {
		return err
	}
	return os.Lchown(hostPath, claim.UID, claim.GID)
}

// volumeClaimDirsNeedFix checks whether any volume claim directory is absent
// from the live tree or has incorrect ownership. Returns true if a fix is needed.
func volumeClaimDirsNeedFix(d *destinationFS, app application.DiscoveredApplication) (bool, error) {
	live, err := liveRef(app.OperationalID)
	if err != nil {
		return false, err
	}
	claimMap := volumeClaimMap(app.Application.Spec.VolumeClaims)
	for _, composeFile := range app.Application.Spec.ComposeFiles {
		composeRef := filepath.Join(live, filepath.FromSlash(composeFile))
		sources, err := composeVolumeSources(d, composeRef)
		if err != nil {
			return false, err
		}
		composeDir := filepath.Dir(filepath.FromSlash(composeFile))
		for _, src := range sources {
			relDir := src
			if composeDir != "." {
				relDir = filepath.Join(composeDir, src)
			}
			claim, ok := claimMap[filepath.ToSlash(relDir)]
			if !ok {
				continue
			}
			absDir := filepath.Join(live, relDir)
			if err := d.ensureNoSymlinkAncestry(absDir); err != nil {
				return false, err
			}
			hostPath := d.absolute(absDir)
			info, err := os.Lstat(hostPath)
			if os.IsNotExist(err) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return false, fmt.Errorf("volume claim path %q is a symlink", claim.Dir)
			}
			if !info.IsDir() {
				return false, fmt.Errorf("volume claim path %q is not a directory", claim.Dir)
			}
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				if stat.Uid != uint32(claim.UID) || stat.Gid != uint32(claim.GID) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// volumeClaimMap builds a lookup from normalized dir path to (uid, gid).
func volumeClaimMap(claims []application.VolumeClaim) map[string]application.VolumeClaim {
	m := make(map[string]application.VolumeClaim, len(claims))
	for _, c := range claims {
		m[filepath.ToSlash(filepath.Clean(c.Dir))] = c
	}
	return m
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

// volumeClaimError signals that a volume claim ownership operation failed.
// The caller may treat this as a warning rather than aborting the reconcile.
var errVolumeClaim = errors.New("volume claim")

func volumeClaimError(err error) error {
	return fmt.Errorf("%w: %w", errVolumeClaim, err)
}

func errorsJoin(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	wrapped := make([]error, len(errs))
	for i, e := range errs {
		wrapped[i] = volumeClaimError(e)
	}
	return errors.Join(wrapped...)
}
