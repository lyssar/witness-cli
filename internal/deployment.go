package internal

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/lyssar/witness-cli/templates"
	"github.com/lyssar/witness-cli/utils"
	"github.com/spf13/cobra"
)

var (
	deployUnixUsernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	deployProjectNamePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
)

const deploySudoPasswordEnv = "WITNESS_SUDO_PASSWORD"

func validateDeploymentInputs(projectName string, username string, destination string) error {
	if !deployProjectNamePattern.MatchString(strings.ToLower(projectName)) {
		return fmt.Errorf("invalid project name: %s", projectName)
	}
	if !deployUnixUsernamePattern.MatchString(username) {
		return fmt.Errorf("invalid username: %s", username)
	}
	if !path.IsAbs(destination) {
		return fmt.Errorf("destination path must be absolute: %s", destination)
	}
	return nil
}

func randomRemoteTmpDir(projectName string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return "", fmt.Errorf("random tmp dir suffix: %w", err)
	}
	return fmt.Sprintf("/tmp/witness-%s-%d", strings.ToLower(projectName), n.Int64()), nil
}

type SSHConfig struct {
	User string
	Key  *string
	Host string
}

type DeployHandler struct {
	AgeFilePath string
	Manifest    string
	Sudoer      string
	BinaryPath  string
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

	binaryPath, err := cmd.Flags().GetString("binary-path")
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
		BinaryPath:  binaryPath,
		SSH:         sshConfig,
	}
}

func (dh *DeployHandler) AskForSudoer() {
	if dh.Sudoer == "" {
		dh.Sudoer = strings.TrimSpace(os.Getenv(deploySudoPasswordEnv))
	}

	if dh.Sudoer == "" {
		err := huh.NewInput().
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
		utils.CheckErr(err)
	}
}

func (dh *DeployHandler) runSudo(client *utils.RemoteClient, command string, user *string) ([]byte, error) {
	out, err := client.RunSudo(command, dh.Sudoer, user)
	if err == nil {
		return out, nil
	}

	if dh.Sudoer != "" || !sudoPasswordPromptRequired(out) {
		return out, err
	}

	dh.AskForSudoer()
	return client.RunSudo(command, dh.Sudoer, user)
}

func sudoPasswordPromptRequired(out []byte) bool {
	message := strings.ToLower(string(out))
	return strings.Contains(message, "password is required") || strings.Contains(message, "a terminal is required")
}

func (dh *DeployHandler) Validate() error {
	if dh.AgeFilePath == "" {
		err := huh.NewInput().
			Title("Age file key file").
			Value(&dh.AgeFilePath).
			Validate(func(ageKeyFile string) error {
				if !utils.FileExists(ageKeyFile) {
					return errors.New("you must enter an existing age key file")
				}
				return nil
			}).
			Run()
		if err != nil {
			return fmt.Errorf("prompting for age key file: %w", err)
		}
	}

	remoteClient, err := utils.NewRemote(dh.SSH.Host, dh.SSH.User, *dh.SSH.Key)
	if err != nil {
		return err
	}

	observer, err := NewObserverFromManifest(dh.Manifest, dh.AgeFilePath)
	if err != nil {
		return err
	}

	if err := validateDeploymentInputs(observer.Spec.Project, observer.Metadata.User, observer.Spec.Destination); err != nil {
		return err
	}

	stdOut, stdErr := remoteClient.SSH.Run(fmt.Sprintf("id -u %s", utils.ShellQuote(observer.Metadata.User)))

	if stdErr != nil {
		if strings.Contains(string(stdOut), "no such user") {
			return fmt.Errorf("user [%s] is missing on the remote system", observer.Metadata.User)
		}
		return fmt.Errorf("%s (%s)", string(stdOut), stdErr.Error())
	}

	serviceName := strings.ToLower(observer.Metadata.Name)
	stdOut, stdErr = dh.runSudo(remoteClient, fmt.Sprintf("S=%s.service; T=${S%%.service}.timer; systemctl show -p LoadState --value \"$T\"", utils.ShellQuote(serviceName)), nil)

	serviceState := strings.TrimSpace(string(stdOut))
	slog.Debug("service state", "service", serviceName, "state", serviceState)

	// "not-found" means the timer doesn't exist yet — this is expected on first deploy
	if serviceState != "not-found" {
		if stdErr != nil {
			return fmt.Errorf("checkup failed: %s (%s)", string(stdOut), stdErr)
		}
		overrideIt := false
		err := huh.NewConfirm().
			Title(fmt.Sprintf("Service already present, override it? (%s)", serviceState)).Value(&overrideIt).Run()
		if err != nil {
			return fmt.Errorf("prompting for service override: %w", err)
		}
		if !overrideIt {
			return fmt.Errorf("service already present stop deploy")
		}
	}

	// witness binary is uploaded and installed by DeployToHost — no pre-check needed

	// Check age is installed — output contains path if found, ignore SSH session EOF errors
	stdOut, _ = dh.runSudo(remoteClient, "command -v age", nil)
	agePath := strings.TrimSpace(string(stdOut))
	if agePath == "" || strings.Contains(agePath, "not found") {
		return fmt.Errorf("age not installed on target host — install it first: https://github.com/FiloSottile/age#installation")
	}
	slog.Debug("age found on target", "path", agePath)

	utils.LogInfo("Validating deployment")

	return nil
}

func (dh *DeployHandler) WithManifest(manifest string) DeployHandler {
	newDeployHandler := *dh
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
	projectName := strings.ToLower(observer.Spec.Project)
	if !deployProjectNamePattern.MatchString(projectName) {
		return fmt.Errorf("invalid project name: %s", observer.Spec.Project)
	}

	utils.LogInfo("Verify systemd service", "service", systemdService)
	analyzeOut, err := dh.runSudo(client, fmt.Sprintf("systemd-analyze verify %s", utils.ShellQuote(systemdService)), nil)
	if string(analyzeOut) != "" || err != nil {
		return fmt.Errorf("error during systemd analyzation: %s (%s)", string(analyzeOut), err)
	}

	reloadOut, err := dh.runSudo(client, "systemctl daemon-reload", nil)
	if string(reloadOut) != "" || err != nil {
		return fmt.Errorf("error during daemon-reload: %s (%s)", string(analyzeOut), err)
	}

	restartOut, err := dh.runSudo(client, fmt.Sprintf("systemctl reload-or-restart %s", utils.ShellQuote(projectName+".timer")), nil)
	if err != nil {
		return fmt.Errorf("error during service restart: %s (%s)", string(restartOut), err)
	}

	enableOut, err := dh.runSudo(client, fmt.Sprintf("systemctl enable --quiet --no-warn %s", utils.ShellQuote(projectName)), nil)
	if err != nil {
		return fmt.Errorf("error during service enable: %s (%s)", string(enableOut), err)
	}

	return nil
}

func (dh *DeployHandler) DeployToHost(observer Observer) (retErr error) {
	utils.LogInfo("Deploying to host", "host", dh.SSH.Host)
	client, err := utils.NewRemote(dh.SSH.Host, dh.SSH.User, *dh.SSH.Key)
	if err != nil {
		return err
	}

	if err := validateDeploymentInputs(observer.Spec.Project, observer.Metadata.User, observer.Spec.Destination); err != nil {
		return err
	}

	observerUserHomeConfigDir := fmt.Sprintf("/home/%s/.config/%s", observer.Metadata.User, utils.APP_NAME)
	remoteAgeFilePath := fmt.Sprintf("%s/%s/age.key", observerUserHomeConfigDir, strings.ToLower(observer.Spec.Project))
	remoteManifestPath := fmt.Sprintf("%s/%s/manifest.yaml", observerUserHomeConfigDir, strings.ToLower(observer.Spec.Project))
	utils.LogInfo("Creating destination for user")
	dh.runSudo(client, fmt.Sprintf("mkdir -p %s %s", utils.ShellQuote(observer.Spec.Destination), utils.ShellQuote(filepath.Dir(remoteManifestPath))), nil)

	// Verify directory was created (ignore SSH session EOF errors)
	verifyOut, _ := dh.runSudo(client, fmt.Sprintf("test -d %s && echo ok", utils.ShellQuote(filepath.Dir(remoteManifestPath))), nil)
	if strings.TrimSpace(string(verifyOut)) != "ok" {
		return fmt.Errorf("failed to create config directory %s", filepath.Dir(remoteManifestPath))
	}

	utils.LogInfo("Change owner")

	dh.runSudo(client, fmt.Sprintf("chown %s:%[1]s %s %s", utils.ShellQuote(observer.Metadata.User), utils.ShellQuote(observerUserHomeConfigDir), utils.ShellQuote(observer.Spec.Destination), utils.ShellQuote(filepath.Dir(remoteManifestPath))), nil)

	renderer, err := templates.NewRenderer()
	if err != nil {
		return err
	}

	sftp, err := client.SSH.NewSftp()
	if err != nil {
		return err
	}

	templateData := map[string]any{
		"Observer":           observer,
		"ObserverConfigPath": observerUserHomeConfigDir,
		"AgeFile":            remoteAgeFilePath,
	}

	ageKeyData, err := os.ReadFile(dh.AgeFilePath)
	if err != nil {
		return fmt.Errorf("error get age key file content: %s", err)
	}

	manifestData, err := os.ReadFile(dh.Manifest)
	if err != nil {
		return fmt.Errorf("error get manifest file content: %s", err)
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

	tmpRemoteDir, err := randomRemoteTmpDir(observer.Spec.Project)
	if err != nil {
		return err
	}
	stdOut, stdErr := dh.runSudo(client, fmt.Sprintf("mkdir %s && chmod 700 %s", utils.ShellQuote(tmpRemoteDir), utils.ShellQuote(tmpRemoteDir)), nil)
	if stdErr != nil {
		return fmt.Errorf("error while creating temp directory: %s", stdOut)
	}
	defer func() {
		cleanupOut, cleanupErr := dh.runSudo(client, fmt.Sprintf("rm -rf %s", utils.ShellQuote(tmpRemoteDir)), nil)
		if cleanupErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("error cleaning remote temp directory %s: %s", tmpRemoteDir, cleanupOut))
		}
	}()

	// Upload witness binary to remote host
	binaryPath := dh.BinaryPath
	if binaryPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot auto-detect binary path: %w", err)
		}
		binaryPath = exePath
	}
	remoteBinaryTmpPath := path.Join(tmpRemoteDir, "witness")
	utils.LogInfo("Uploading witness binary", "src", binaryPath, "dst", remoteBinaryTmpPath)
	if err := client.TransferFile(binaryPath, remoteBinaryTmpPath); err != nil {
		return fmt.Errorf("uploading binary: %w", err)
	}
	stdOut, stdErr = dh.runSudo(client, fmt.Sprintf("mv %s /usr/local/bin/witness && chmod 755 /usr/local/bin/witness", utils.ShellQuote(remoteBinaryTmpPath)), nil)
	if stdErr != nil {
		return fmt.Errorf("installing binary on remote: %s", string(stdOut))
	}

	for _, deployFile := range deployFileList {
		tmpRemoteFilePath := path.Join(tmpRemoteDir, filepath.Base(deployFile.RemoteFilePath))
		remoteFile, err := sftp.Create(tmpRemoteFilePath)
		if err != nil {
			return fmt.Errorf("error creating tmp remote file [%s]: %s", tmpRemoteFilePath, err)
		}

		if deployFile.TemplateName == "direct" {
			_, err = remoteFile.Write(deployFile.Content)
			if err != nil {
				if closeErr := remoteFile.Close(); closeErr != nil {
					return fmt.Errorf("writing direct file %s: %w (close failed: %v)", deployFile.RemoteFilePath, err, closeErr)
				}
				return fmt.Errorf("writing direct file %s: %w", deployFile.RemoteFilePath, err)
			}
		} else {
			err = renderer.Render(deployFile.TemplateName, deployFile.TemplateData, remoteFile)
			if err != nil {
				if closeErr := remoteFile.Close(); closeErr != nil {
					return fmt.Errorf("rendering template %s: %w (close failed: %v)", deployFile.TemplateName, err, closeErr)
				}
				return fmt.Errorf("rendering template %s: %w", deployFile.TemplateName, err)
			}
		}

		err = remoteFile.Close()
		if err != nil {
			return fmt.Errorf("closing tmp remote file [%s]: %w", tmpRemoteFilePath, err)
		}

		stdOut, stdErr = dh.runSudo(client, fmt.Sprintf("mv %s %s", utils.ShellQuote(tmpRemoteFilePath), utils.ShellQuote(deployFile.RemoteFilePath)), nil)
		if stdErr != nil {
			return fmt.Errorf("error while moving remote file do location: %s", stdOut)
		}

		if filepath.Base(deployFile.RemoteFilePath) == "age.key" {
			stdOut, stdErr = dh.runSudo(client, fmt.Sprintf("chmod 600 %s", utils.ShellQuote(deployFile.RemoteFilePath)), nil)
			if stdErr != nil {
				return fmt.Errorf("error while setting remote file permissions: %s", stdOut)
			}
		}

		stdOut, stdErr = dh.runSudo(client, fmt.Sprintf("chown %s:%[1]s %s", utils.ShellQuote(deployFile.RemoteFileOwner), utils.ShellQuote(deployFile.RemoteFilePath)), nil)
		if stdErr != nil {
			return fmt.Errorf("error while changing remote file owner: %s", stdOut)
		}
	}

	return nil
}
