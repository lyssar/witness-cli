package internal

import (
	"os"

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
	utils.LogInfo("user executing this", "user_id", os.Geteuid(), "group_id", os.Getegid())
	utils.LogSuccess("reconcilation finished", "state", "NOT_IMPLEMENTED")
	return nil
}

func DeployCmd(cmd *cobra.Command, args []string) error {
	deployHandler := NewDeployHandler(cmd).WithManifest(args[0])

	err := deployHandler.Validate()
	utils.CheckErr(err)

	err = deployHandler.DeployToHost()
	utils.CheckErr(err)

	//   - Create Service and Timer on the system, named by the app of apps name
	utils.LogSuccess("deploy successfull", "state", "NOT_IMPLEMENTED")
	return nil
}
