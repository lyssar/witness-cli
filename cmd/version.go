package cmd

import (
	"fmt"

	"github.com/lyssar/witness-cli/version"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version and exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Printf("witness %s (commit %s)\n", version.Version, version.Commit())
		return err
	},
}
