package cmd

import (
	"github.com/lyssar/skuld-cli/internal"
	"github.com/spf13/cobra"
)

// reconcileCmd represents the observer creation command
var reconcileCmd = &cobra.Command{
	Use:   "reconcile [MANIFEST] [AGE_FILE]",
	Short: "TBD",
	Long:  `TBD`,
	Args:  cobra.ExactArgs(2),
	RunE:  internal.ReconcileCmd,
}

func init() {
	rootCmd.AddCommand(reconcileCmd)
}
