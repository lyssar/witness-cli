package provisioner

import (
	"context"
	"os"
	"path/filepath"
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
	runtime := RuntimeContext{RuntimeSlug: "apps-hello", LiveDir: root}

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
	calls []runnerCall
}

type runnerCall struct {
	dir    string
	name   string
	args   []string
	stdin  string
}

func (r *recordingRunner) Run(_ context.Context, dir string, name string, args ...string) error {
	r.calls = append(r.calls, runnerCall{dir: dir, name: name, args: append([]string(nil), args...)})
	return nil
}

func (r *recordingRunner) RunWithStdin(_ context.Context, dir string, stdin string, name string, args ...string) error {
	r.calls = append(r.calls, runnerCall{dir: dir, name: name, args: append([]string(nil), args...), stdin: stdin})
	return nil
}
