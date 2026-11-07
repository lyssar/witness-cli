package cmd

import (
	"github.com/lyssar/witness-cli/internal"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check and repair host prerequisites on a target host",
	Long: `Checks the target host for required tools (git, age, libcap/setcap, docker, docker compose)
and reports missing ones. Without --check-only, prompts interactively before installing anything.
It then verifies and repairs host state: the witness group, group membership of every service user
found under /home/*/.config/witness/*/manifest.yaml, the observer config roots (mode 0700, owned by
the service user), and CAP_CHOWN on the installed witness binary. Repairs run through sudo.
With --check-only, prints missing tools and the repair commands and exits non-zero if anything is
missing; it never prompts for a sudo password.`,
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
