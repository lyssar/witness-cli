package internal

import (
	"log/slog"

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

func NewAppCmd(cmd *cobra.Command, args []string) error {
	if err := CheckPrerequisites(cmd); err != nil {
		return err
	}

	app := NewApp(cmd)
	app.Configure()
	app.WriteConfig()

	utils.DebugStruct(app)

	return nil
}

func ReconcileCmd(cmd *cobra.Command, args []string) error {
	slog.Info("Start reconcilation run")
	reconcileRun := NewReconcileRun(args[0])
	if ok, err := reconcileRun.Validate(); !ok {
		return err
	}

	err := reconcileRun.LoadManifest()
	if err != nil {
		return err
	}

	err = reconcileRun.Reconcile()
	if err != nil {
		return err
	}

	slog.Info("Finished reconcilation run")
	return nil
}

func DeployCmd(cmd *cobra.Command, args []string) error {
	deployHandler := NewDeployHandler(cmd).WithManifest(args[0])

	err := deployHandler.Validate()
	utils.CheckErr(err)

	observer, err := NewObserverFromManifest(deployHandler.Manifest, deployHandler.AgeFilePath)
	utils.CheckErr(err)

	err = deployHandler.DeployToHost(observer)
	utils.CheckErr(err)

	err = deployHandler.ReloadSystemD(observer)
	utils.CheckErr(err)

	//   - Create Service and Timer on the system, named by the app of apps name
	utils.LogSuccess("deploy successfull", "state", "NOT_IMPLEMENTED")
	return nil
}
