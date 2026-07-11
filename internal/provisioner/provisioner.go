package provisioner

import (
	"context"

	"github.com/lyssar/witness-cli/internal/application"
)

// RuntimeContext provides stable reconcile inputs to provisioners.
type RuntimeContext struct {
	OperationalID        string
	RuntimeSlug          string
	LiveDir              string
	SourceDir            string
	RegistryPasswordPath string // path to decrypted registry password file
}

// Provisioner defines the runtime contract used by reconcile orchestration.
type Provisioner interface {
	Name() string
	Validate(ctx context.Context, runtime RuntimeContext, app application.Application) error
	Apply(ctx context.Context, runtime RuntimeContext, app application.Application) error
	Delete(ctx context.Context, runtime RuntimeContext, app application.Application) error
	CleanupMissing(ctx context.Context, runtime RuntimeContext) error
}
