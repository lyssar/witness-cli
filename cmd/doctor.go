package cmd

import (
	"github.com/lyssar/witness-cli/internal"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check and install OS/tool prerequisites on a target host",
	Long: `Checks the target host for required tools (git, age, libcap/setcap, docker, docker compose)
and reports missing ones. Without --check-only, prompts interactively before installing anything.
With --check-only, prints missing tools and install commands and exits non-zero if any tool is missing.`,
	RunE: internal.DoctorCmd,
}

func init() {
	doctorCmd.Flags().StringP("ssh-user", "u", "", "The ssh user to check")
	doctorCmd.Flags().StringP("ssh-key", "k", "", "The ssh key to use, optional. Make sure to have a ssh config for the host if ommited.")
	doctorCmd.Flags().String("host", "", "The host to check")
	doctorCmd.Flags().Bool("local", false, "Check the local machine instead of a remote host")
	doctorCmd.Flags().Bool("check-only", false, "Report missing tools and install commands without installing; exit non-zero if any tool is missing")
	rootCmd.AddCommand(doctorCmd)
}
