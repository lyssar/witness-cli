package cmd

import (
	"github.com/lyssar/witness-cli/internal"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [OBSERVER_NAME]",
	Short: "Initialize a Skuld observer configuration",
	Long: `Creates an Observer manifest YAML file interactively.

Prompts for the observer name, execution user, destination path,
git repository URL, target revision, credentials, and age key path.
The access token is encrypted with age in the output manifest.

When --local is set, creates a complete config root directory containing
manifest.yaml and a generated age.key, ready for use with reconcile.`,
	RunE: internal.InitCmd,
}

func init() {
	initCmd.Flags().StringP("age-key", "k", "", "path to the age private key")
	initCmd.Flags().Bool("local", false, "create a complete config root directory with manifest.yaml and generated age.key")
	rootCmd.AddCommand(initCmd)
}
