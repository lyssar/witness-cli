package reconcile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	// Always ensure claimed directories exist — covers first deploy and
	// recovery after manual deletion of volume directories on updates.
	// On updates this pre-creates empty directories that the volume-dir
	// moves below replace.
	if err := ensureVolumeDirs(destination, stage, app.Application.Spec.ComposeFiles, app.Application.Spec.VolumeClaims); err != nil {
		if errors.Is(err, errVolumeClaim) {
			// Volume claim chown failed — log and continue, don't abort.
			slog.Warn("Volume claim ownership not applied", "operationalID", app.OperationalID, "error", err)
		} else {
			return "", fmt.Errorf("creating docker volume directories: %w", err)
		}
	}
	if _, statErr := destination.root.Lstat(live); os.IsNotExist(statErr) {
		// First deploy — fall through to promotion below
	} else if statErr != nil {
		return "", fmt.Errorf("stat live app: %w", statErr)
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

	// On updates (live directory existed), move Docker bind-mount volume
	// directories from the old live tree (now in backup) into the stage tree
	// so persistent data survives the promotion rename cycle. Moving rather
	// than copying preserves container-owned ownership and modes without
	// requiring read access to the data. A failure reverses the moves and
	// leaves the old live tree intact in backup.
	var moved []string
	if _, statErr := destination.root.Lstat(backup); statErr == nil {
		var preserveErr error
		moved, preserveErr = preserveVolumeDirs(destination, backup, stage, app.Application.Spec.ComposeFiles, app.Application.Spec.VolumeClaims)
		if preserveErr != nil {
			if errors.Is(preserveErr, errVolumeClaim) {
				slog.Warn("Volume claim ownership not applied during preserve", "operationalID", app.OperationalID, "error", preserveErr)
			} else {
				if rollbackErr := reverseVolumeMoves(destination, stage, backup, moved); rollbackErr != nil {
					return "", errors.Join(
						fmt.Errorf("preserving docker volume directories: %w", preserveErr),
						fmt.Errorf("reversing volume directory moves after preserve failure: %w", rollbackErr),
					)
				}
				// Restore the old live tree so the app returns to its prior
				// healthy state; otherwise the next reconcile would treat the
				// app as first-deploy and RemoveAll(backup) would discard the
				// preserved container data.
				if _, restoreErr := destination.root.Lstat(backup); restoreErr == nil {
					if rollbackErr := destination.root.Rename(backup, live); rollbackErr != nil {
						return "", errors.Join(
							fmt.Errorf("preserving docker volume directories: %w", preserveErr),
							fmt.Errorf("restoring prior live app after preserve failure: %w", rollbackErr),
						)
					}
				} else if !os.IsNotExist(restoreErr) {
					return "", errors.Join(
						fmt.Errorf("preserving docker volume directories: %w", preserveErr),
						fmt.Errorf("checking prior live app for rollback: %w", restoreErr),
					)
				}
				return "", fmt.Errorf("preserving docker volume directories: %w", preserveErr)
			}
		}
	}
	if err := destination.root.Rename(stage, live); err != nil {
		if rollbackErr := reverseVolumeMoves(destination, stage, backup, moved); rollbackErr != nil {
			return "", errors.Join(
				fmt.Errorf("promoting rooted staging tree: %w", err),
				fmt.Errorf("reversing volume directory moves after failed promotion: %w", rollbackErr),
			)
		}
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

// reverseVolumeMoves moves each previously moved volume directory back from
// the stage tree to the backup tree, in reverse order, so rollback restores
// the old live layout after a failed promotion.
func reverseVolumeMoves(d *destinationFS, stage, backup string, moved []string) error {
	var errs []error
	for i := len(moved) - 1; i >= 0; i-- {
		rel := moved[i]
		from := filepath.Join(stage, rel)
		to := filepath.Join(backup, rel)
		if err := d.root.Rename(from, to); err != nil {
			errs = append(errs, fmt.Errorf("reversing volume directory move %q: %w", rel, err))
		}
	}
	return errors.Join(errs...)
}

func reconcileDeletedApp(ctx context.Context, destination *destinationFS, operationalID string, entry state.Entry, p provisioner.Provisioner, now time.Time) (state.Entry, bool, error) {
	if !entry.SecretTargetsKnown {
		return reconcileUntrustedDeletedApp(ctx, destination, operationalID, entry, p, now)
	}
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

// reconcileUntrustedDeletedApp is a safe recovery path for legacy state that
// lacks a complete secret inventory. It only removes a live tree after the
// runtime has stopped; it never archives or inspects untrusted secret targets.
func reconcileUntrustedDeletedApp(ctx context.Context, destination *destinationFS, operationalID string, entry state.Entry, p provisioner.Provisioner, now time.Time) (state.Entry, bool, error) {
	app, err := appFromStateWithoutSecrets(operationalID, entry)
	if err != nil {
		return entry, false, err
	}
	if entry.ArchivePath != "" {
		err := fmt.Errorf("untrusted deletion state has an archive path and cannot be recovered safely")
		return entry, false, err
	}
	archives, err := validatedArchivedAppRefs(destination, operationalID)
	if err != nil {
		return entry, false, err
	}
	if len(archives) != 0 {
		err := fmt.Errorf("untrusted deletion state has recoverable archive %q and cannot be recovered safely", archives[len(archives)-1])
		return entry, false, err
	}
	live, err := liveRef(operationalID)
	if err != nil {
		return entry, false, err
	}
	if err := destination.ensureNoSymlinkAncestry(live); err != nil {
		return entry, false, err
	}
	runtime := provisioner.RuntimeContext{OperationalID: operationalID, RuntimeSlug: entry.RuntimeSlug, LiveDir: destination.absolute(live)}
	if _, err := destination.root.Lstat(live); os.IsNotExist(err) {
		if err := p.CleanupMissing(ctx, runtime); err != nil {
			return deletingStateEntry(now, entry, "", err), false, err
		}
		return state.Entry{}, true, nil
	} else if err != nil {
		return entry, false, fmt.Errorf("stat rooted live app: %w", err)
	}
	if err := destination.validateLiveDirectory(live); err != nil {
		return entry, false, err
	}
	if err := p.Delete(ctx, runtime, app); err != nil {
		return deletingStateEntry(now, entry, "", err), false, err
	}
	if err := destination.validateLiveDirectory(live); err != nil {
		return entry, false, err
	}
	if err := p.CleanupMissing(ctx, runtime); err != nil {
		return deletingStateEntry(now, entry, "", err), false, err
	}
	if err := destination.root.RemoveAll(live); err != nil {
		return deletingStateEntry(now, entry, "", fmt.Errorf("removing rooted live app: %w", err)), false, fmt.Errorf("removing rooted live app: %w", err)
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
	refs, err := archivedAppRefs(destination, operationalID, false)
	if err != nil || len(refs) == 0 {
		return "", err
	}
	return refs[len(refs)-1], nil
}

// validatedArchivedAppRefs lists every canonical archive artifact only after
// proving that each is a real directory. It is used by untrusted recovery,
// where a symlink or non-directory timestamp artifact must block recovery
// rather than be silently skipped.
func validatedArchivedAppRefs(destination *destinationFS, operationalID string) ([]string, error) {
	return archivedAppRefs(destination, operationalID, true)
}

func archivedAppRefs(destination *destinationFS, operationalID string, validateDirectories bool) ([]string, error) {
	prefix, err := liveRef(operationalID)
	if err != nil {
		return nil, err
	}
	root := filepath.Join("archive", prefix)
	if err := destination.ensureNoSymlinkAncestry(root); err != nil {
		return nil, err
	}
	entries, err := destination.readDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, entry := range entries {
		ref, ok := canonicalArchiveRef(operationalID, filepath.Join(root, entry.Name()))
		if !ok {
			continue
		}
		if validateDirectories {
			if err := destination.validateArchiveDirectory(ref); err != nil {
				return nil, fmt.Errorf("validating recovered archive artifact %q: %w", ref, err)
			}
		} else if !entry.IsDir() {
			continue
		}
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs, nil
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
