package reconcile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/internal/provisioner"
	"github.com/lyssar/witness-cli/internal/state"
)

func promoteStagingToLive(destinationRoot string, app application.DiscoveredApplication, staging AppStaging) (string, error) {
	destination, err := openDestinationFS(destinationRoot)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := destination.Close(); closeErr != nil {
			return
		}
	}()
	ref, err := promoteRootedStagingToLive(destination, app, staging)
	if err != nil {
		return "", err
	}
	return destination.absolute(ref), nil
}

func promoteRootedStagingToLive(destination *destinationFS, app application.DiscoveredApplication, staging AppStaging) (string, error) {
	live, err := liveRef(app.OperationalID)
	if err != nil {
		return "", err
	}
	if err := destination.ensureNoSymlinkAncestry(live); err != nil {
		return "", err
	}
	stage := ".witness-stage-" + filepath.Base(live) + "-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if err := destination.copyExternalTree(staging.Root, stage); err != nil {
		return "", fmt.Errorf("copying staging tree into destination: %w", err)
	}
	backup := live + ".previous"
	if err := destination.root.RemoveAll(backup); err != nil {
		return "", fmt.Errorf("removing prior backup: %w", err)
	}
	if _, err := destination.root.Lstat(live); err == nil {
		if err := destination.root.Rename(live, backup); err != nil {
			return "", fmt.Errorf("moving live app aside: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat live app: %w", err)
	}
	if err := destination.root.Rename(stage, live); err != nil {
		if _, restoreErr := destination.root.Lstat(backup); restoreErr == nil {
			if rollbackErr := destination.root.Rename(backup, live); rollbackErr != nil {
				return "", errors.Join(
					fmt.Errorf("promoting rooted staging tree: %w", err),
					fmt.Errorf("restoring prior live app after failed promotion: %w", rollbackErr),
				)
			}
		} else if !os.IsNotExist(restoreErr) {
			return "", errors.Join(
				fmt.Errorf("promoting rooted staging tree: %w", err),
				fmt.Errorf("checking prior live app for rollback: %w", restoreErr),
			)
		}
		return "", fmt.Errorf("promoting rooted staging tree: %w", err)
	}
	if err := destination.root.Chmod(live, 0o700); err != nil {
		return "", fmt.Errorf("hardening live app root: %w", err)
	}
	if err := destination.root.RemoveAll(backup); err != nil {
		return "", fmt.Errorf("removing live backup: %w", err)
	}
	return live, nil
}

func reconcileDeletedApp(ctx context.Context, destination *destinationFS, operationalID string, entry state.Entry, p provisioner.Provisioner, now time.Time) (state.Entry, bool, error) {
	app, err := appFromState(operationalID, entry)
	if err != nil {
		return entry, false, err
	}
	archive, valid := canonicalArchiveRef(operationalID, entry.ArchivePath)
	if entry.ArchivePath != "" && !valid {
		return deletingStateEntry(now, entry, "", fmt.Errorf("state entry archive path is invalid")), false, fmt.Errorf("state entry archive path is invalid")
	}
	if archive == "" {
		live, err := liveRef(operationalID)
		if err != nil {
			return entry, false, err
		}
		if err := destination.ensureNoSymlinkAncestry(live); err != nil {
			return entry, false, err
		}
		if _, err := destination.root.Lstat(live); os.IsNotExist(err) {
			archive, err = latestArchivedAppRef(destination, operationalID)
			if err != nil {
				return deletingStateEntry(now, entry, "", err), false, err
			}
			if archive != "" {
				if err := deleteAndScrub(ctx, destination, archive, operationalID, entry, p, app); err != nil {
					return deletingStateEntry(now, entry, archive, err), false, err
				}
			}
			if scrubErr := scrubOrphanedArchivedSecrets(destination, operationalID, entry.SecretTargets); scrubErr != nil {
				return deletingStateEntry(now, entry, archive, scrubErr), false, scrubErr
			}
			if err := p.CleanupMissing(ctx, provisioner.RuntimeContext{OperationalID: operationalID, RuntimeSlug: entry.RuntimeSlug, LiveDir: destination.absolute(live)}); err != nil {
				return deletingStateEntry(now, entry, "", err), false, err
			}
			return state.Entry{}, true, nil
		} else if err != nil {
			return entry, false, fmt.Errorf("stat rooted live app: %w", err)
		}
		archive, err = archiveRef(operationalID, now)
		if err != nil {
			return entry, false, err
		}
		if err := destination.ensureNoSymlinkAncestry(archive); err != nil {
			return entry, false, err
		}
		if err := destination.root.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
			return entry, false, err
		}
		if err := destination.root.Rename(live, archive); err != nil {
			return entry, false, fmt.Errorf("archiving rooted live app: %w", err)
		}
	}
	if err := deleteAndScrub(ctx, destination, archive, operationalID, entry, p, app); err != nil {
		return deletingStateEntry(now, entry, archive, err), false, err
	}
	if err := scrubOrphanedArchivedSecrets(destination, operationalID, entry.SecretTargets); err != nil {
		return deletingStateEntry(now, entry, archive, err), false, err
	}
	return state.Entry{}, true, nil
}

func deleteAndScrub(ctx context.Context, destination *destinationFS, archive, operationalID string, entry state.Entry, p provisioner.Provisioner, app application.Application) error {
	runtime := provisioner.RuntimeContext{OperationalID: operationalID, RuntimeSlug: entry.RuntimeSlug, LiveDir: destination.absolute(archive)}
	// This is intentionally immediately adjacent to the subprocess call. The
	// provisioner receives an absolute path, so an attacker with destination
	// write access can still replace it after this check and before the child
	// process opens it; os.Root cannot eliminate that external-process TOCTOU.
	if err := destination.validateArchiveDirectory(archive); err != nil {
		return err
	}
	if err := p.Delete(ctx, runtime, app); err != nil {
		if scrubErr := scrubArchivedSecrets(destination, archive, entry.SecretTargets); scrubErr != nil {
			return fmt.Errorf("delete failed: %w; scrub archived secrets: %v", err, scrubErr)
		}
		return err
	}
	if err := destination.validateArchiveDirectory(archive); err != nil {
		return err
	}
	if err := p.CleanupMissing(ctx, runtime); err != nil {
		if scrubErr := scrubArchivedSecrets(destination, archive, entry.SecretTargets); scrubErr != nil {
			return fmt.Errorf("cleanup missing failed: %w; scrub archived secrets: %v", err, scrubErr)
		}
		return err
	}
	return scrubArchivedSecrets(destination, archive, entry.SecretTargets)
}

func scrubArchivedSecrets(destination *destinationFS, archive string, targets []string) error {
	if err := destination.validateArchiveDirectory(archive); err != nil {
		return err
	}
	for _, target := range targets {
		if err := scrubArchivedFile(destination, archive, target); err != nil {
			return err
		}
	}
	return scrubArchivedFile(destination, archive, ".registry-password")
}

func scrubArchivedFile(destination *destinationFS, archive, target string) error {
	ref := filepath.Join(archive, filepath.FromSlash(target))
	if err := destination.ensureNoSymlinkAncestry(ref); err != nil {
		return fmt.Errorf("scrubbing archived sensitive file %q: %w", target, err)
	}
	if err := destination.root.Remove(ref); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove archived sensitive file %q: %w", target, err)
	}
	return nil
}

func latestArchivedAppRef(destination *destinationFS, operationalID string) (string, error) {
	prefix, err := liveRef(operationalID)
	if err != nil {
		return "", err
	}
	root := filepath.Join("archive", prefix)
	if err := destination.ensureNoSymlinkAncestry(root); err != nil {
		return "", err
	}
	entries, err := destination.readDir(root)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var refs []string
	for _, entry := range entries {
		if entry.IsDir() {
			if ref, ok := canonicalArchiveRef(operationalID, filepath.Join(root, entry.Name())); ok {
				refs = append(refs, ref)
			}
		}
	}
	sort.Strings(refs)
	if len(refs) == 0 {
		return "", nil
	}
	return refs[len(refs)-1], nil
}

func scrubOrphanedArchivedSecrets(destination *destinationFS, operationalID string, targets []string) error {
	prefix, err := liveRef(operationalID)
	if err != nil {
		return err
	}
	root := filepath.Join("archive", prefix)
	entries, err := destination.readDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if ref, ok := canonicalArchiveRef(operationalID, filepath.Join(root, entry.Name())); ok {
			if err := scrubArchivedSecrets(destination, ref, targets); err != nil {
				return err
			}
		}
	}
	return nil
}
