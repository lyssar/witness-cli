package reconcile

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/internal/state"
)

func statePathForConfigRoot(configRoot string) string {
	return filepath.Join(configRoot, "state.json")
}

func desiredStateEntry(now time.Time, app application.DiscoveredApplication, resolvedCommit string) state.Entry {
	entry := state.Entry{
		Status:                       state.StatusHealthy,
		LastAttemptedReconcileAt:     timePointer(now),
		LastSuccessfulReconcileAt:    timePointer(now),
		LastSuccessfulResolvedCommit: resolvedCommit,
		RuntimeSlug:                  app.RuntimeSlug,
		ApplicationName:              app.Application.Metadata.Name,
		Provisioner:                  app.Application.Spec.Provisioner,
		ComposeFiles:                 append([]string(nil), app.Application.Spec.ComposeFiles...),
		SecretTargets:                secretTargetsForState(app),
		SecretTargetsKnown:           true,
	}
	return entry
}

func failedStateEntry(now time.Time, existing state.Entry, app application.DiscoveredApplication, resolvedCommit string, err error) state.Entry {
	entry := existing
	entry.Status = state.StatusFailed
	entry.LastError = err.Error()
	entry.LastAttemptedReconcileAt = timePointer(now)
	entry.RuntimeSlug = app.RuntimeSlug
	entry.ApplicationName = app.Application.Metadata.Name
	entry.Provisioner = app.Application.Spec.Provisioner
	entry.ComposeFiles = append([]string(nil), app.Application.Spec.ComposeFiles...)
	entry.SecretTargets = secretTargetsForState(app)
	if resolvedCommit != "" {
		entry.LastSuccessfulResolvedCommit = resolvedCommit
	}
	return entry
}

func deletingStateEntry(now time.Time, existing state.Entry, archivePath string, err error) state.Entry {
	entry := existing
	entry.Status = state.StatusDeleting
	entry.LastAttemptedReconcileAt = timePointer(now)
	entry.ArchivePath = archivePath
	if err != nil {
		entry.LastError = err.Error()
	} else {
		entry.LastError = ""
	}
	return entry
}

func appFromState(operationalID string, entry state.Entry) (application.Application, error) {
	app, err := appFromStateWithoutSecrets(operationalID, entry)
	if err != nil {
		return application.Application{}, err
	}
	if !entry.SecretTargetsKnown {
		return application.Application{}, fmt.Errorf("state entry secret target inventory is unavailable")
	}
	secretTargets, err := canonicalStatePaths("secret targets", entry.SecretTargets, false)
	if err != nil {
		return application.Application{}, err
	}
	app.Spec.Secrets = secretsFromTargets(secretTargets)
	return app, nil
}

// appFromStateWithoutSecrets validates the operational fields needed to stop a
// runtime, deliberately excluding an untrusted secret-target inventory.
func appFromStateWithoutSecrets(operationalID string, entry state.Entry) (application.Application, error) {
	expectedRuntimeSlug, err := application.SlugFromIdentityPath(operationalID)
	if err != nil {
		return application.Application{}, fmt.Errorf("validating operational identity %q: %w", operationalID, err)
	}
	if entry.RuntimeSlug != expectedRuntimeSlug {
		return application.Application{}, fmt.Errorf("state runtime slug %q does not match operational identity %q", entry.RuntimeSlug, operationalID)
	}
	if entry.Provisioner == "" {
		return application.Application{}, fmt.Errorf("state entry provisioner is required")
	}
	composeFiles, err := canonicalStatePaths("compose files", entry.ComposeFiles, true)
	if err != nil {
		return application.Application{}, err
	}

	return application.Application{
		Metadata: application.Metadata{Name: entry.ApplicationName},
		Spec: application.Spec{
			Provisioner:  entry.Provisioner,
			ComposeFiles: composeFiles,
		},
	}, nil
}

func canonicalStatePaths(name string, paths []string, required bool) ([]string, error) {
	if required && len(paths) == 0 {
		return nil, fmt.Errorf("state entry %s are required", name)
	}
	canonical := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for i, path := range paths {
		normalized, err := application.CanonicalRelativePath(path)
		if err != nil {
			return nil, fmt.Errorf("state entry %s[%d]: %w", name, i, err)
		}
		if _, ok := seen[normalized]; ok {
			return nil, fmt.Errorf("state entry %s[%d]: duplicate path %q", name, i, path)
		}
		seen[normalized] = struct{}{}
		canonical = append(canonical, normalized)
	}
	return canonical, nil
}

func secretTargetsForState(app application.DiscoveredApplication) []string {
	if len(app.Application.Spec.Secrets) == 0 {
		return nil
	}
	targets := make([]string, 0, len(app.Application.Spec.Secrets))
	for _, secret := range app.Application.Spec.Secrets {
		targets = append(targets, secret.Target)
	}
	return targets
}

func secretsFromTargets(targets []string) []application.Secret {
	if len(targets) == 0 {
		return nil
	}
	secrets := make([]application.Secret, 0, len(targets))
	for _, target := range targets {
		secrets = append(secrets, application.Secret{Target: target})
	}
	return secrets
}

func timePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}
