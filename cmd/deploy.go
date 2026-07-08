package cmd

import (
	"github.com/lyssar/skuld-cli/internal"
	"github.com/spf13/cobra"
)

// deployCmd represents the observer creation command
var deployCmd = &cobra.Command{
	Use:   "deploy [APP_OF_APPS_MANIFEST]",
	Short: "Will deploy the given app of apps to the [host]",
	Long: `Creates a .service with the app of apps name and configures the Skuld reconciler
to reconcile against configured source repo (the source of truth) every few minutes.

The command installs the service unit and ensures continuous, periodic
reconciliation of state from the repository`,
	Args: cobra.ExactArgs(1),
	RunE: internal.DeployCmd,
}

func init() {
	deployCmd.Flags().StringP("age-key", "a", "", "The path to the age key to deploy with the observer app")
	deployCmd.Flags().StringP("ssh-user", "u", "", "The ssh user to deploy the app of apps to")
	deployCmd.Flags().StringP("ssh-key", "k", "", "The ssh key to use, optional. Make sure to have a ssh config for the host if ommited.")
	deployCmd.Flags().String("host", "", "The host to deploy the observer to")
	deployCmd.Flags().String("binary-path", "", "Path to the skuld-cli binary to upload (default: auto-detect)")
	rootCmd.AddCommand(deployCmd)
}
