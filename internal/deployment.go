package internal

import (
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
	// TODO: Validate deployhandler
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
