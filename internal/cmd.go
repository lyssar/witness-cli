package internal

import (
	"log/slog"

	"github.com/lyssar/witness-cli/internal/reconcile"
	"github.com/lyssar/witness-cli/utils"
	"github.com/spf13/cobra"
)

func InitCmd(cmd *cobra.Command, args []string) error {
	if err := CheckPrerequisites(cmd); err != nil {
		return err
	}

	local, _ := cmd.Flags().GetBool("local")

	observer := NewObserver(cmd)

	// In --local mode, skip the age key prompt; WriteConfigRoot generates a new key.
	if local {
		observer.AgeKeyFile = "generated"
	}

	observer.Configure()

	if local {
		return observer.WriteConfigRoot()
	}

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
	err := reconcile.NewRunner(args[0]).Run(cmd.Context())
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

	utils.LogSuccess("Deploy completed successfully", "host", deployHandler.SSH.Host, "project", observer.Spec.Project)
	return nil
}
