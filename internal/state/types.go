package state

import "time"

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
}

// File is the aggregate observer-local state file.
type File struct {
	Version      int              `json:"version"`
	Applications map[string]Entry `json:"applications"`
}
