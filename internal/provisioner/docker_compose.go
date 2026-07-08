package provisioner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lyssar/skuld-cli/internal/application"
)

// CommandRunner executes provisioner commands.
type CommandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) error
}

// DockerCompose applies and deletes docker-compose applications.
type DockerCompose struct {
	runner CommandRunner
}

// NewDockerCompose constructs a docker-compose provisioner.
func NewDockerCompose(runner CommandRunner) *DockerCompose {
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &DockerCompose{runner: runner}
}

// Name returns the manifest-facing provisioner identifier.
func (p *DockerCompose) Name() string {
	return application.ProvisionerDockerCompose
}

// Validate validates compose inputs without mutating runtime state.
func (p *DockerCompose) Validate(ctx context.Context, runtime RuntimeContext, app application.Application) error {
	if err := p.validateRuntime(runtime, app); err != nil {
		return err
	}
	return p.runner.Run(ctx, runtime.LiveDir, "docker", composeArgs(runtime, app, "config", "--quiet")...)
}

// Apply converges the compose project inside the live app directory.
func (p *DockerCompose) Apply(ctx context.Context, runtime RuntimeContext, app application.Application) error {
	if err := p.validateRuntime(runtime, app); err != nil {
		return err
	}
	return p.runner.Run(ctx, runtime.LiveDir, "docker", composeArgs(runtime, app, "up", "--detach", "--remove-orphans")...)
}

// Delete tears down the compose project using the archived or live app directory.
func (p *DockerCompose) Delete(ctx context.Context, runtime RuntimeContext, app application.Application) error {
	if strings.TrimSpace(runtime.LiveDir) == "" {
		return fmt.Errorf("docker-compose delete: live dir is required")
	}
	if strings.TrimSpace(runtime.RuntimeSlug) == "" {
		return fmt.Errorf("docker-compose delete: runtime slug is required")
	}
	if len(app.Spec.ComposeFiles) == 0 {
		return fmt.Errorf("docker-compose delete: at least one compose file is required")
	}
	if _, err := os.Stat(runtime.LiveDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("docker-compose delete: stat %q: %w", runtime.LiveDir, err)
	}
	return p.runner.Run(ctx, runtime.LiveDir, "docker", composeArgs(runtime, app, "down", "--remove-orphans")...)
}

// CleanupMissing is a no-op because archive-first deletion keeps archived files on disk.
func (p *DockerCompose) CleanupMissing(ctx context.Context, runtime RuntimeContext) error {
	_ = ctx
	_ = runtime
	return nil
}

func (p *DockerCompose) validateRuntime(runtime RuntimeContext, app application.Application) error {
	if strings.TrimSpace(runtime.LiveDir) == "" {
		return fmt.Errorf("docker-compose: live dir is required")
	}
	if strings.TrimSpace(runtime.RuntimeSlug) == "" {
		return fmt.Errorf("docker-compose: runtime slug is required")
	}
	if len(app.Spec.ComposeFiles) == 0 {
		return fmt.Errorf("docker-compose: at least one compose file is required")
	}
	for _, composeFile := range app.Spec.ComposeFiles {
		composePath := filepath.Join(runtime.LiveDir, filepath.FromSlash(composeFile))
		if _, err := os.Stat(composePath); err != nil {
			return fmt.Errorf("docker-compose: stat compose file %q: %w", composePath, err)
		}
	}
	return nil
}

func composeArgs(runtime RuntimeContext, app application.Application, tail ...string) []string {
	args := []string{"compose", "--project-name", runtime.RuntimeSlug}
	for _, composeFile := range app.Spec.ComposeFiles {
		args = append(args, "--file", filepath.Join(runtime.LiveDir, filepath.FromSlash(composeFile)))
	}
	args = append(args, tail...)
	return args
}
