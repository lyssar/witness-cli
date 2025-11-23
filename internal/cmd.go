package internal

import (
	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
)

func InitCmd(cmd *cobra.Command, args []string) error {
	if err := CheckPrerequisites(cmd); err != nil {
		return err
	}

	observer := NewObserver(cmd)
	observer.Configure()
	observer.WriteConfig()

	utils.DebugStruct(observer)

	return nil
}

func ReconcileCmd(cmd *cobra.Command, args []string) error {
	utils.LogSuccess("reconcilation finished", "state", "NOT_IMPLEMENTED")

	return nil
}

func DeployCmd(cmd *cobra.Command, args []string) error {
	utils.LogSuccess("deploy successfull", "state", "NOT_IMPLEMENTED")
	return nil
}
