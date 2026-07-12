package provisioner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyssar/witness-cli/internal/application"
)

func TestDockerComposeCommands(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	composePath := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	runner := &recordingRunner{}
	provisioner := NewDockerCompose(runner)
	app := application.Application{Spec: application.Spec{ComposeFiles: []string{"compose.yaml"}}}
	runtime := RuntimeContext{RuntimeSlug: "apps-hello", LiveDir: root, ComposeFilesChanged: true}

	if err := provisioner.Validate(context.Background(), runtime, app); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := provisioner.Apply(context.Background(), runtime, app); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := provisioner.Delete(context.Background(), runtime, app); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 command calls, got %d", len(runner.calls))
	}
	if runner.calls[0].args[0] != "compose" || runner.calls[0].args[len(runner.calls[0].args)-2] != "config" {
		t.Fatalf("unexpected validate args: %#v", runner.calls[0].args)
	}
	if runner.calls[1].args[len(runner.calls[1].args)-3] != "up" {
		t.Fatalf("unexpected apply args: %#v", runner.calls[1].args)
	}
	if runner.calls[2].args[len(runner.calls[2].args)-2] != "down" {
		t.Fatalf("unexpected delete args: %#v", runner.calls[2].args)
	}
}

type recordingRunner struct {
	calls    []runnerCall
	stdinErr error
}

type runnerCall struct {
	dir   string
	name  string
	args  []string
	stdin string
}

func (r *recordingRunner) Run(_ context.Context, dir string, name string, args ...string) error {
	r.calls = append(r.calls, runnerCall{dir: dir, name: name, args: append([]string(nil), args...)})
	return nil
}

func (r *recordingRunner) RunWithStdin(_ context.Context, dir string, stdin string, name string, args ...string) error {
	r.calls = append(r.calls, runnerCall{dir: dir, name: name, args: append([]string(nil), args...), stdin: stdin})
	return r.stdinErr
}

func TestDockerComposeSecretOnlyDrift(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	composePath := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	runner := &recordingRunner{}
	p := NewDockerCompose(runner)
	app := application.Application{Spec: application.Spec{ComposeFiles: []string{"compose.yaml"}}}
	runtime := RuntimeContext{RuntimeSlug: "apps-hello", LiveDir: root, SecretsChanged: true}

	if err := p.Apply(context.Background(), runtime, app); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(runner.calls))
	}
	args := runner.calls[0].args
	assertContains(t, args, "up")
	assertContains(t, args, "--detach")
	assertContains(t, args, "--force-recreate")
	assertContains(t, args, "--remove-orphans")
}

func TestDockerComposeComposeOnlyDrift(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	composePath := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	runner := &recordingRunner{}
	p := NewDockerCompose(runner)
	app := application.Application{Spec: application.Spec{ComposeFiles: []string{"compose.yaml"}}}
	runtime := RuntimeContext{RuntimeSlug: "apps-hello", LiveDir: root, ComposeFilesChanged: true}

	if err := p.Apply(context.Background(), runtime, app); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(runner.calls))
	}
	args := runner.calls[0].args
	assertContains(t, args, "up")
	assertNotContains(t, args, "--force-recreate")
	assertContains(t, args, "--remove-orphans")
}

func TestDockerComposeNoDriftNoApply(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	composePath := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	runner := &recordingRunner{}
	p := NewDockerCompose(runner)
	app := application.Application{Spec: application.Spec{ComposeFiles: []string{"compose.yaml"}}}
	runtime := RuntimeContext{RuntimeSlug: "apps-hello", LiveDir: root}

	if err := p.Apply(context.Background(), runtime, app); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no command calls for no drift, got %d: %#v", len(runner.calls), runner.calls)
	}
}

func TestDockerComposeBothDriftForceRecreate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	composePath := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	runner := &recordingRunner{}
	p := NewDockerCompose(runner)
	app := application.Application{Spec: application.Spec{ComposeFiles: []string{"compose.yaml"}}}
	runtime := RuntimeContext{RuntimeSlug: "apps-hello", LiveDir: root, ComposeFilesChanged: true, SecretsChanged: true}

	if err := p.Apply(context.Background(), runtime, app); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(runner.calls))
	}
	args := runner.calls[0].args
	assertContains(t, args, "up")
	assertContains(t, args, "--force-recreate")
	assertContains(t, args, "--remove-orphans")
}

func TestDockerComposeRegistryPasswordRemovedAfterLogin(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		loginErr error
	}{
		{name: "success"},
		{name: "login failure", loginErr: errors.New("registry rejected super-secret-password")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			composePath := filepath.Join(root, "compose.yaml")
			passwordPath := filepath.Join(root, ".registry-password")
			if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
				t.Fatalf("write compose file: %v", err)
			}
			if err := os.WriteFile(passwordPath, []byte("super-secret-password\n"), 0o600); err != nil {
				t.Fatalf("write registry password: %v", err)
			}

			runner := &recordingRunner{stdinErr: test.loginErr}
			p := NewDockerCompose(runner)
			app := application.Application{Spec: application.Spec{
				ComposeFiles: []string{"compose.yaml"},
				RegistryCredentials: &application.RegistryCredentials{
					Registry: "registry.example.com",
					Username: "robot",
				},
			}}
			runtime := RuntimeContext{
				RuntimeSlug:          "apps-hello",
				LiveDir:              root,
				RegistryPasswordPath: passwordPath,
				ComposeFilesChanged:  true,
			}

			err := p.Apply(context.Background(), runtime, app)
			if test.loginErr != nil {
				if err == nil {
					t.Fatal("expected login failure")
				}
				if strings.Contains(err.Error(), "super-secret-password") {
					t.Fatalf("password leaked in error: %v", err)
				}
			} else if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if _, err := os.Stat(passwordPath); !os.IsNotExist(err) {
				t.Fatalf("expected registry password removal, stat err=%v", err)
			}
		})
	}
}

func TestDockerComposeRegistryPasswordRemovedAfterReadFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	composePath := filepath.Join(root, "compose.yaml")
	passwordPath := filepath.Join(root, ".registry-password")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	if err := os.Mkdir(passwordPath, 0o700); err != nil {
		t.Fatalf("create unreadable registry password path: %v", err)
	}

	runner := &recordingRunner{}
	p := NewDockerCompose(runner)
	app := application.Application{Spec: application.Spec{
		ComposeFiles: []string{"compose.yaml"},
		RegistryCredentials: &application.RegistryCredentials{
			Registry: "registry.example.com",
			Username: "robot",
		},
	}}
	runtime := RuntimeContext{
		RuntimeSlug:          "apps-hello",
		LiveDir:              root,
		RegistryPasswordPath: passwordPath,
		ComposeFilesChanged:  true,
	}

	err := p.Apply(context.Background(), runtime, app)
	if err == nil {
		t.Fatal("expected registry password read failure")
	}
	if strings.Contains(err.Error(), "super-secret-password") {
		t.Fatalf("password leaked in error: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("read failure invoked docker: %#v", runner.calls)
	}
	if _, err := os.Stat(passwordPath); !os.IsNotExist(err) {
		t.Fatalf("expected registry password cleanup after read failure, stat err=%v", err)
	}
}

func assertContains(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Fatalf("expected %q in args, got %#v", want, slice)
}

func assertNotContains(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			t.Fatalf("unexpected %q in args, got %#v", want, slice)
		}
	}
}
