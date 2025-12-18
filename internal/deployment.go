package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
)

type SSHConfig struct {
	User string
	Key  *string
	Host string
}

type DeployHandler struct {
	AgeFilePath string
	Manifest    string
	Sudoer      string
	SSH         SSHConfig
}

func NewDeployHandler(cmd *cobra.Command) *DeployHandler {
	sshUser, err := cmd.Flags().GetString("ssh-user")
	utils.CheckErr(err)

	sshKey, err := cmd.Flags().GetString("ssh-key")
	utils.CheckErr(err)

	host, err := cmd.Flags().GetString("host")
	utils.CheckErr(err)

	ageFilePath, err := cmd.Flags().GetString("age-key")
	utils.CheckErr(err)

	sshConfig := SSHConfig{
		User: sshUser,
		Key:  &sshKey,
		Host: host,
	}

	return &DeployHandler{
		AgeFilePath: ageFilePath,
		Manifest:    "",
		Sudoer:      "",
		SSH:         sshConfig,
	}
}

func (dh *DeployHandler) AskForSudoer() {
	if dh.Sudoer == "" {
		huh.NewInput().
			Title("Enter sudoer password for deploy user").
			EchoMode(huh.EchoModePassword).
			Value(&dh.Sudoer).
			Validate(func(pass string) error {
				if len(strings.TrimSpace(pass)) <= 0 {
					return errors.New("you must enter a sudoer password")
				}
				return nil
			}).
			Run()
	}
}

func (dh *DeployHandler) Validate() error {
	if dh.AgeFilePath == "" {
		huh.NewInput().
			Title("Age file key file").
			Value(&dh.AgeFilePath).
			Validate(func(ageKeyFile string) error {
				if !utils.FileExists(ageKeyFile) {
					return errors.New("you must enter an existing age key file")
				}
				return nil
			}).
			Run()
	}

	dh.AskForSudoer()

	remoteClient, err := utils.NewRemote(dh.SSH.Host, dh.SSH.User, *dh.SSH.Key)
	if err != nil {
		return err
	}

	observer, err := NewObserverFromManifest(dh.Manifest, dh.AgeFilePath)
	if err != nil {
		return err
	}

	stdOut, stdErr := remoteClient.SSH.Run(fmt.Sprintf("id -u '%s'", observer.Metadata.User))

	if stdErr != nil {
		if strings.Contains(string(stdOut), "no such user") {
			return fmt.Errorf("User [%s] is missing on the remote system", observer.Metadata.User)
		}
		return fmt.Errorf("%s (%s)", string(stdOut), stdErr.Error())
	}

	stdOut, stdErr = remoteClient.RunSudo(fmt.Sprintf("S=%s.service; T=${S%%.service}.timer; systemctl show -p LoadState --value \"$T\"", observer.Metadata.Name), dh.Sudoer, nil)
	if stdErr != nil {
		return fmt.Errorf("Checkup failed: %s (%s)", string(stdOut), stdErr)
	}

	slog.Debug(string(stdOut))

	if strings.TrimSpace(string(stdOut)) != "not-found" {
		overrideIt := false
		huh.NewConfirm().
			Title(fmt.Sprintf("Service already present, override it? (%s)", string(stdOut))).Value(&overrideIt).Run()
		if !overrideIt {
			return fmt.Errorf("Service already present. Stop deploy.")
		}
	}

	stdOut, stdErr = remoteClient.RunSudo("command -v skuld-cli", dh.Sudoer, &observer.Metadata.User)

	if stdErr != nil {
		return fmt.Errorf("Skuld-cli not on target host found %s. (error %s)", string(stdOut), stdErr)
	}

	stdOut, stdErr = remoteClient.RunSudo("command -v age", dh.Sudoer, &observer.Metadata.User)

	if stdErr != nil {
		return fmt.Errorf("age not on target host found %s. (error %s)", string(stdOut), stdErr)
	}

	utils.LogInfo("Validating deployment")

	return nil
}

func (dh *DeployHandler) WithManifest(manifest string) DeployHandler {
	var newDeployHandler DeployHandler
	newDeployHandler = *dh
	newDeployHandler.Manifest = manifest

	return newDeployHandler
}

func (dh *DeployHandler) DeployToHost() error {
	utils.LogInfo("Deploying to host", "host", dh.SSH.Host)
	client, err := utils.NewRemote(dh.SSH.Host, dh.SSH.User, *dh.SSH.Key)
	if err != nil {
		return err
	}

	observer, err := NewObserverFromManifest(dh.Manifest, dh.AgeFilePath)
	if err != nil {
		return err
	}

	dh.AskForSudoer()

	utils.LogInfo("Creating destination for user")
	stdOut, stdErr := client.RunSudo(fmt.Sprintf("mkdir -p %s", observer.Spec.Destination), dh.Sudoer, nil)

	if stdErr != nil {
		return fmt.Errorf("Error while destination folder: %s", stdOut)
	}

	utils.LogInfo("Change owner")

	stdOut, stdErr = client.RunSudo(fmt.Sprintf("chown %s:%[1]s %s", observer.Metadata.User, observer.Spec.Destination), dh.Sudoer, nil)

	if stdErr != nil {
		return fmt.Errorf("Error while changing folder: %s", stdOut)
	}

	// TODO create service file
	//      create timer file
	//

	return nil
}
