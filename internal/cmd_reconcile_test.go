package internal

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestReconcileCmdUsesRunnerValidation(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := ReconcileCmd(cmd, []string{t.TempDir()})
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}

	if !strings.Contains(err.Error(), "manifest file could not be found") {
		t.Fatalf("expected runner manifest validation error, got %v", err)
	}
}
