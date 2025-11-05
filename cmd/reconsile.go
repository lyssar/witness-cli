package cmd

import (
	"github.com/lyssar/skuld-cli/internal"
	"github.com/spf13/cobra"
)

// reconsileCmd represents the observer creation command
var reconsileCmd = &cobra.Command{
	Use:   "reconsile [SERVICE_NAME] [REPO-URL]",
	Short: "TBD",
	Long:  `TBD`,
	Args:  cobra.ExactArgs(2),
	RunE:  internal.ObserveAddCmd,
}

func init() {
	rootCmd.AddCommand(reconsileCmd)
}
