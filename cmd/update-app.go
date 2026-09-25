package cmd

import (
	"github.com/lyssar/witness-cli/internal"
	"github.com/spf13/cobra"
)

var updateAppCmd = &cobra.Command{
	Use:   "update-app",
	Short: "Update an existing Witness application manifest",
	Long: `Updates an existing Application manifest YAML file interactively.

Can add or update registry credentials, secrets, and compose files.
The manifest is updated in-place.`,
	RunE: internal.UpdateAppCmd,
}

func init() {
	updateAppCmd.Flags().StringP("age-key", "a", "", "path to the age private key")
	rootCmd.AddCommand(updateAppCmd)
}
