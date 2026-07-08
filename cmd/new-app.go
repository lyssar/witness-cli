package cmd

import (
	"github.com/lyssar/witness-cli/internal"
	"github.com/spf13/cobra"
)

var newAppCmd = &cobra.Command{
	Use:   "new-app",
	Short: "Create a new Skuld application manifest",
	Long: `Creates an Application manifest YAML file interactively.

Prompts for the app name, provisioner type, compose files,
and secrets. The app inherits its source repository from
the Observer manifest at reconcile time.`,
	RunE: internal.NewAppCmd,
}

func init() {
	newAppCmd.Flags().StringP("age-key", "a", "", "path to the age private key")
	rootCmd.AddCommand(newAppCmd)
}
