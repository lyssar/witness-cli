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
	// TODO:
	//   - Create DeployCmdHandler struct

	deployHandler := NewDeployHandler(cmd).WithManifest(args[0])

	err := deployHandler.Validate()
	utils.CheckErr(err)

	err = deployHandler.DeployToHost()
	utils.CheckErr(err)

	utils.DebugStruct(deployHandler)
	//   - Implement validation for needed inputs and ask if something is missing or wrong
	//   - Test SSH connection
	//   - Test if skuld-cli is present on host system (needed to run reconcilation)
	//   - Do we need AGE as system requirement?
	//   - Test if service already exists on host, if yes ask to override
	//   - Test if configured destination path exists or is creatable (to prevent startup fail of the observer service)
	//   - Test if the given SSH user has root privlieges (needed to create the service)
	//   - Test if the given app of apps has all it needs, if not fail deployment
	//   - Test if the user configured in app of apps is present
	//   - Create Service and Timer on the system, named by the app of apps name
	utils.LogSuccess("deploy successfull", "state", "NOT_IMPLEMENTED")
	return nil
}
