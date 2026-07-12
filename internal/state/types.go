package state

import "time"

const (
	// CurrentVersion is the current on-disk state schema version.
	CurrentVersion = 2
)

// Status represents the persisted reconcile status for one application.
type Status string

const (
	// StatusHealthy means the last reconcile for the app succeeded.
	StatusHealthy Status = "healthy"
	// StatusFailed means the last reconcile attempt for the app failed.
	StatusFailed Status = "failed"
	// StatusDeleting means the app is currently in deletion workflow.
	StatusDeleting Status = "deleting"
)

// Entry is the persisted per-application state payload.
type Entry struct {
	Status                       Status     `json:"status"`
	LastError                    string     `json:"lastError,omitempty"`
	LastAttemptedReconcileAt     *time.Time `json:"lastAttemptedReconcileAt,omitempty"`
	LastSuccessfulReconcileAt    *time.Time `json:"lastSuccessfulReconcileAt,omitempty"`
	LastSuccessfulResolvedCommit string     `json:"lastSuccessfulResolvedCommit,omitempty"`
	RuntimeSlug                  string     `json:"runtimeSlug,omitempty"`
	ApplicationName              string     `json:"applicationName,omitempty"`
	Provisioner                  string     `json:"provisioner,omitempty"`
	ComposeFiles                 []string   `json:"composeFiles,omitempty"`
	SecretTargets                []string   `json:"secretTargets,omitempty"`
	SecretTargetsKnown           bool       `json:"secretTargetsKnown,omitempty"`
	ArchivePath                  string     `json:"archivePath,omitempty"`
}

// File is the aggregate observer-local state file.
type File struct {
	Version      int              `json:"version"`
	Applications map[string]Entry `json:"applications"`
}
