package internal

import (
	"errors"
	"fmt"
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
		SSH:         sshConfig,
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

	remoteClient, err := utils.NewRemote(dh.SSH.Host, dh.SSH.User, *dh.SSH.Key)
	if err != nil {
		return err
	}
	defer remoteClient.SSH.Close()

	sshSession, err := remoteClient.SSH.NewSession()
	if err != nil {
		return fmt.Errorf("Couldn't connect to remote system: %+v", err)
	}

	defer sshSession.Close()

	// TODO: parse manifest file to get User.

	observer, err := NewObserverFromManifest(dh.Manifest, dh.AgeFilePath)
	if err != nil {
		return err
	}

	stdOut, stdErr := sshSession.CombinedOutput(fmt.Sprintf("id -u '%s'", observer.Metadata.User))

	if stdErr != nil {
		if strings.Contains(string(stdOut), "no such user") {
			return errors.New("User is missing on the remote system")
		}
		return fmt.Errorf("%s (%s)", string(stdOut), stdErr.Error())
	}

	// - try read destination of manifest

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
	return nil
}
