package reconcile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lyssar/skuld-cli/internal/application"
	"github.com/lyssar/skuld-cli/internal/provisioner"
	"github.com/lyssar/skuld-cli/internal/state"
)

func liveAppDir(destinationRoot string, operationalID string) (string, error) {
	return safeJoinUnder(filepath.Join(destinationRoot, "apps"), filepath.FromSlash(operationalID))
}

func archiveAppDir(destinationRoot string, operationalID string, now time.Time) (string, error) {
	archiveRoot, err := archiveAppRoot(destinationRoot, operationalID)
	if err != nil {
		return "", err
	}
	archivePath := filepath.Join(archiveRoot, now.UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return "", fmt.Errorf("creating archive parent for %q: %w", archivePath, err)
	}
	return archivePath, nil
}

func archiveAppRoot(destinationRoot string, operationalID string) (string, error) {
	return safeJoinUnder(filepath.Join(destinationRoot, "archive"), filepath.FromSlash(operationalID))
}

func promoteStagingToLive(destinationRoot string, app application.DiscoveredApplication, staging AppStaging) (string, error) {
	liveDir, err := liveAppDir(destinationRoot, app.OperationalID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(liveDir), 0o755); err != nil {
		return "", fmt.Errorf("creating live app parent for %q: %w", liveDir, err)
	}

	backupDir := liveDir + ".previous"
	_ = os.RemoveAll(backupDir)

	if _, err := os.Stat(liveDir); err == nil {
		if err := os.Rename(liveDir, backupDir); err != nil {
			return "", fmt.Errorf("moving existing live dir %q aside: %w", liveDir, err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat live dir %q: %w", liveDir, err)
	}

	if err := os.Rename(staging.Root, liveDir); err != nil {
		if _, restoreErr := os.Stat(backupDir); restoreErr == nil {
			_ = os.Rename(backupDir, liveDir)
		}
		return "", fmt.Errorf("promoting staging dir %q to %q: %w", staging.Root, liveDir, err)
	}

	_ = os.RemoveAll(backupDir)
	return liveDir, nil
}

func reconcileDeletedApp(ctx context.Context, destinationRoot string, operationalID string, entry state.Entry, p provisioner.Provisioner, now time.Time) (state.Entry, bool, error) {
	app, err := appFromState(entry)
	if err != nil {
		return entry, false, err
	}

	deleteDir := entry.ArchivePath
	if deleteDir == "" {
		liveDir, err := liveAppDir(destinationRoot, operationalID)
		if err != nil {
			return entry, false, err
		}

		if _, err := os.Stat(liveDir); err != nil {
			if os.IsNotExist(err) {
				orphanArchiveDir, err := latestArchivedAppDir(destinationRoot, operationalID)
				if err != nil {
					return deletingStateEntry(now, entry, "", err), false, err
				}
				if orphanArchiveDir != "" {
					runtime := provisioner.RuntimeContext{
						OperationalID: operationalID,
						RuntimeSlug:   entry.RuntimeSlug,
						LiveDir:       orphanArchiveDir,
					}
					if err := p.Delete(ctx, runtime, app); err != nil {
						// Best-effort cleanup: scrub secrets even if delete fails,
						// so decrypted material is not left in the orphaned archive.
						_ = scrubArchivedSecrets(orphanArchiveDir, entry.SecretTargets)
						return deletingStateEntry(now, entry, orphanArchiveDir, err), false, err
					}
				}
				if err := scrubOrphanedArchivedSecrets(destinationRoot, operationalID, entry.SecretTargets); err != nil {
					return deletingStateEntry(now, entry, orphanArchiveDir, err), false, err
				}
				runtime := provisioner.RuntimeContext{
					OperationalID: operationalID,
					RuntimeSlug:   entry.RuntimeSlug,
					LiveDir:       liveDir,
				}
				if err := p.CleanupMissing(ctx, runtime); err != nil {
					return deletingStateEntry(now, entry, "", err), false, err
				}
				return state.Entry{}, true, nil
			}
			return entry, false, fmt.Errorf("stat live dir %q: %w", liveDir, err)
		}

		deleteDir, err = archiveAppDir(destinationRoot, operationalID, now)
		if err != nil {
			return entry, false, err
		}
		if err := os.Rename(liveDir, deleteDir); err != nil {
			return entry, false, fmt.Errorf("archiving live dir %q to %q: %w", liveDir, deleteDir, err)
		}
	}

	runtime := provisioner.RuntimeContext{
		OperationalID: operationalID,
		RuntimeSlug:   entry.RuntimeSlug,
		LiveDir:       deleteDir,
	}
	if err := p.Delete(ctx, runtime, app); err != nil {
		scrubErr := scrubArchivedSecrets(deleteDir, entry.SecretTargets)
		if scrubErr != nil {
			return deletingStateEntry(now, entry, deleteDir, fmt.Errorf("delete failed: %w; scrub archived secrets: %v", err, scrubErr)), false, err
		}
		return deletingStateEntry(now, entry, deleteDir, err), false, err
	}
	if err := p.CleanupMissing(ctx, runtime); err != nil {
		scrubErr := scrubArchivedSecrets(deleteDir, entry.SecretTargets)
		if scrubErr != nil {
			combinedErr := fmt.Errorf("cleanup missing failed: %w; scrub archived secrets: %v", err, scrubErr)
			return deletingStateEntry(now, entry, deleteDir, combinedErr), false, combinedErr
		}
		return deletingStateEntry(now, entry, deleteDir, err), false, err
	}
	if err := scrubArchivedSecrets(deleteDir, entry.SecretTargets); err != nil {
		return deletingStateEntry(now, entry, deleteDir, err), false, err
	}

	return state.Entry{}, true, nil
}

func scrubArchivedSecrets(archiveDir string, secretTargets []string) error {
	cleanArchiveDir := filepath.Clean(archiveDir)
	for _, target := range secretTargets {
		archivePath := filepath.Clean(filepath.Join(cleanArchiveDir, filepath.FromSlash(target)))

		// Resolve symlinks to prevent path traversal attacks:
		// a symlinked parent (e.g. "secrets -> /etc") under the archive dir
		// MUST NOT cause deletion outside the archive root.
		resolvedPath, err := filepath.EvalSymlinks(archivePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // already gone, nothing to scrub
			}
			return fmt.Errorf("resolving archived secret target %q under %q: %w", target, archiveDir, err)
		}

		// Verify that the resolved path is inside the archive directory.
		// Note: there is a TOCTOU window between EvalSymlinks and Remove;
		// a fully robust fix would use os.Remove on an O_NOFOLLOW fd.
		parentPrefix := cleanArchiveDir + string(filepath.Separator)
		if !strings.HasPrefix(resolvedPath, parentPrefix) && resolvedPath != cleanArchiveDir {
			return fmt.Errorf("archived secret target %q resolved outside archive dir: %q -> %q", target, archivePath, resolvedPath)
		}

		if err := os.Remove(resolvedPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove archived secret target %q: %w", target, err)
		}
	}
	return nil
}

func latestArchivedAppDir(destinationRoot string, operationalID string) (string, error) {
	archiveRoot, err := archiveAppRoot(destinationRoot, operationalID)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(archiveRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading archive root %q: %w", archiveRoot, err)
	}
	archiveDirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		archiveDirs = append(archiveDirs, filepath.Join(archiveRoot, entry.Name()))
	}
	if len(archiveDirs) == 0 {
		return "", nil
	}
	sort.Strings(archiveDirs)
	return archiveDirs[len(archiveDirs)-1], nil
}

func scrubOrphanedArchivedSecrets(destinationRoot string, operationalID string, secretTargets []string) error {
	archiveRoot, err := archiveAppRoot(destinationRoot, operationalID)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(archiveRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading archive root %q: %w", archiveRoot, err)
	}
	archiveDirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		archiveDirs = append(archiveDirs, filepath.Join(archiveRoot, entry.Name()))
	}
	sort.Strings(archiveDirs)
	for _, archiveDir := range archiveDirs {
		if err := scrubArchivedSecrets(archiveDir, secretTargets); err != nil {
			return err
		}
	}
	return nil
}
