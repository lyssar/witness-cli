package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/lyssar/skuld-cli/templates"
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

type DeployFile struct {
	TemplateName    string
	TemplateData    *map[string]any
	RemoteFilePath  string
	RemoteFileOwner string
	Content         []byte
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

	stdOut, stdErr = remoteClient.RunSudo(fmt.Sprintf("S=%s.service; T=${S%%.service}.timer; systemctl show -p LoadState --value \"$T\"", strings.ToLower(observer.Metadata.Name)), dh.Sudoer, nil)
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

func (dh *DeployHandler) ReloadSystemD(observer Observer) error {
	utils.LogInfo("Reloading systemd to apply changes", "host", dh.SSH.Host)
	client, err := utils.NewRemote(dh.SSH.Host, dh.SSH.User, *dh.SSH.Key)
	if err != nil {
		return err
	}

	fullSystemDPath := observer.FullServicePath()
	systemdService := strings.TrimSuffix(fullSystemDPath, path.Ext(fullSystemDPath)) + "*"

	utils.LogInfo("Verify systemd service", "service", systemdService)
	analyzeOut, err := client.RunSudo(fmt.Sprintf("systemd-analyze verify %s", systemdService), dh.Sudoer, nil)
	if string(analyzeOut) != "" || err != nil {
		return fmt.Errorf("Error during systemd analyzation: %s (%s)", string(analyzeOut), err)
	}

	reloadOut, err := client.RunSudo("systemctl daemon-reload", dh.Sudoer, nil)
	if string(reloadOut) != "" || err != nil {
		return fmt.Errorf("Error during daemon-reload: %s (%s)", string(analyzeOut), err)
	}

	restartOut, err := client.RunSudo(fmt.Sprintf("systemctl reload-or-restart %s.timer", strings.ToLower(observer.Spec.Project)), dh.Sudoer, nil)
	if err != nil {
		return fmt.Errorf("Error during service restart: %s (%s)", string(restartOut), err)
	}

	enableOut, err := client.RunSudo(fmt.Sprintf("systemctl enable --quiet --no-warn %s", strings.ToLower(observer.Spec.Project)), dh.Sudoer, nil)
	if err != nil {
		return fmt.Errorf("Error during service enable: %s (%s)", string(enableOut), err)
	}

	return nil
}

func (dh *DeployHandler) DeployToHost(observer Observer) error {
	utils.LogInfo("Deploying to host", "host", dh.SSH.Host)
	client, err := utils.NewRemote(dh.SSH.Host, dh.SSH.User, *dh.SSH.Key)
	if err != nil {
		return err
	}

	dh.AskForSudoer()

	remoteAgeFilePath := fmt.Sprintf("/home/%s/.config/skuld/%s/age.key", observer.Metadata.User, strings.ToLower(observer.Spec.Project))
	remoteManifestPath := fmt.Sprintf("/home/%s/.config/skuld/%s/manifest.yaml", observer.Metadata.User, strings.ToLower(observer.Spec.Project))
	utils.LogInfo("Creating destination for user")
	stdOut, stdErr := client.RunSudo(fmt.Sprintf("mkdir -p %s %s", observer.Spec.Destination, filepath.Dir(remoteManifestPath)), dh.Sudoer, nil)

	if stdErr != nil {
		return fmt.Errorf("Error while destination folder: %s", stdOut)
	}

	utils.LogInfo("Change owner")

	stdOut, stdErr = client.RunSudo(fmt.Sprintf("chown %s:%[1]s %s %s", observer.Metadata.User, observer.Spec.Destination, filepath.Dir(remoteManifestPath)), dh.Sudoer, nil)

	if stdErr != nil {
		return fmt.Errorf("Error while changing folder: %s", stdOut)
	}

	renderer, err := templates.NewRenderer()
	if err != nil {
		return err
	}

	sftp, err := client.SSH.NewSftp()
	if err != nil {
		return err
	}

	templateData := map[string]any{
		"Observer":     observer,
		"ManifestPath": remoteManifestPath,
		"AgeFile":      remoteAgeFilePath,
	}

	ageKeyData, err := os.ReadFile(dh.AgeFilePath)
	if err != nil {
		return fmt.Errorf("Error get age key file content: %s", err)
	}

	manifestData, err := os.ReadFile(dh.Manifest)
	if err != nil {
		return fmt.Errorf("Error get manifest file content: %s", err)
	}

	deployFileList := []DeployFile{
		{
			TemplateName:    "service",
			TemplateData:    &templateData,
			RemoteFilePath:  observer.FullServicePath(),
			RemoteFileOwner: "root",
		},
		{
			TemplateName:    "timer",
			TemplateData:    &templateData,
			RemoteFilePath:  observer.FullServiceTimerPath(),
			RemoteFileOwner: "root",
		},
		{
			TemplateName:    "direct",
			Content:         manifestData,
			RemoteFilePath:  remoteManifestPath,
			RemoteFileOwner: observer.Metadata.User,
		},
		{
			TemplateName:    "direct",
			Content:         ageKeyData,
			RemoteFilePath:  remoteAgeFilePath,
			RemoteFileOwner: observer.Metadata.User,
		},
	}

	for _, deployFile := range deployFileList {
		tmpRemoteFilePath := fmt.Sprintf("/tmp/%s", filepath.Base(deployFile.RemoteFilePath))
		remoteFile, err := sftp.Create(tmpRemoteFilePath)
		if err != nil {
			return fmt.Errorf("Error creating tmp remote file [%s]: %s", tmpRemoteFilePath, err)
		}

		if deployFile.TemplateName == "direct" {
			_, err = remoteFile.Write(deployFile.Content)
			if err != nil {
				return err
			}
		} else {
			renderer.Render(deployFile.TemplateName, deployFile.TemplateData, remoteFile)
		}

		remoteFile.Close()

		stdOut, stdErr = client.RunSudo(fmt.Sprintf("mv %s %s", tmpRemoteFilePath, deployFile.RemoteFilePath), dh.Sudoer, nil)
		if stdErr != nil {
			return fmt.Errorf("Error while moving remote file do location: %s", stdOut)
		}

		stdOut, stdErr = client.RunSudo(fmt.Sprintf("chown %s:%[1]s %s", deployFile.RemoteFileOwner, deployFile.RemoteFilePath), dh.Sudoer, nil)
		if stdErr != nil {
			return fmt.Errorf("Error while changing remote file owner: %s", stdOut)
		}
	}

	return nil
}
