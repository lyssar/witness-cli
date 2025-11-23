package cmd

import (
	"github.com/lyssar/skuld-cli/internal"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:     "init [OBSERVER_NAME]",
	Short:   "init will create an skuld observer config for your reconilation",
	Long:    "TBD",
	Example: "TBD",
	RunE:    internal.InitCmd,
}

func init() {
	initCmd.Flags().StringP("age-key", "k", "", "path to the age private key")
	rootCmd.AddCommand(initCmd)
}
