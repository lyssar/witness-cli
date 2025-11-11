package cmd

import (
	"github.com/lyssar/skuld-cli/internal"
	"github.com/spf13/cobra"
)

// addCmd represents the observer creation command
var addCmd = &cobra.Command{
	Use:   "add [SERVICE_NAME] [REPO-URL]",
	Short: "Will init the repository skuld recopnsiler as a service with name [SERVICE_NAME] for repo [REPO-URL] as main source of truth",
	Long: `Creates a .service with the given SERVICE_NAME and configures the Skuld reconciler
to reconcile against REPO-URL (the source of truth) every few minutes.

The command installs the service unit and ensures continuous, periodic
reconciliation of state from the repository.

Credentials and trust material are provided via environment variables (not flags):
  - SKULD_AGE_KEY          : AGE private key used to decrypt repository secrets
  - SKULD_SSH_PRVT_KEY     : SSH private key for Git access
  - SKULD_KNOWN_HOST_FILE  : Path to a known_hosts file for host key verification`,
	Args: cobra.ExactArgs(2),
	RunE: internal.ObserverAddCmd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}
