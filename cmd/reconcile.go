package cmd

import (
	"github.com/lyssar/witness-cli/internal"
	"github.com/spf13/cobra"
)

// reconcileCmd represents the observer creation command
var reconcileCmd = &cobra.Command{
	Use:   "reconcile [OBSERVER_CONFIG_ROOT]",
	Short: "Run one reconciliation pass for an observer config root",
	Long: `Executes a single reconciliation pass for the observer defined at the given
config root directory. The directory must contain:
  - manifest.yaml    Observer manifest with source repo and destination settings
  - age.key          Age private key for decrypting app secrets

Workflow:
  1. Sync the configured git repository
  2. Discover applications under the configured source path
  3. Build filesets, stage files, decrypt secrets
  4. Detect drift against the destination root
  5. Apply changes for drifted apps, delete removed apps
  6. Persist per-application state to state.json`,
	Args: cobra.ExactArgs(1),
	RunE: internal.ReconcileCmd,
}

func init() {
	rootCmd.AddCommand(reconcileCmd)
}
