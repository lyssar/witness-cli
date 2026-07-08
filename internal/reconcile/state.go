package reconcile

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/lyssar/skuld-cli/internal/application"
	"github.com/lyssar/skuld-cli/internal/state"
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

func appFromState(entry state.Entry) (application.Application, error) {
	if entry.Provisioner == "" {
		return application.Application{}, fmt.Errorf("state entry provisioner is required")
	}
	if len(entry.ComposeFiles) == 0 {
		return application.Application{}, fmt.Errorf("state entry compose files are required")
	}

	return application.Application{
		Metadata: application.Metadata{Name: entry.ApplicationName},
		Spec: application.Spec{
			Provisioner:  entry.Provisioner,
			ComposeFiles: append([]string(nil), entry.ComposeFiles...),
			Secrets:      secretsFromTargets(entry.SecretTargets),
		},
	}, nil
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
