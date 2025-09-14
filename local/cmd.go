package local

import (
	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
)

func ObserveCmd(cmd *cobra.Command, args []string) error {
	if err := CheckPrerequisites(cmd); err != nil {
		return err
	}

	runSync, err := cmd.Flags().GetBool("sync")
	utils.CheckErr(err)

	observer, err := NewObserver(args[0], cmd)
	if err != nil {
		return err
	}

	if runSync {
		return observer.Sync()
	}

	err = observer.Configure()
	if err != nil {
		return err
	}

	utils.LogSuccess("Add new service", "name", observer.Name)
	return nil
}
