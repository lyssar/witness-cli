package cmd

import (
	"github.com/lyssar/skuld-cli/internal"
	"github.com/spf13/cobra"
)

var newAppCmd = &cobra.Command{
	Use:     "new-app",
	Short:   "add a new skuld app",
	Long:    "TBD",
	Example: "TBD",
	RunE:    internal.NewAppCmd,
}

func init() {
	newAppCmd.Flags().StringP("age-key", "a", "", "path to the age private key")
	rootCmd.AddCommand(newAppCmd)
}
