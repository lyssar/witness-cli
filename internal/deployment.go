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

	if stdErr != nil || strings.Contains(string(stdOut), "no such user") {
		utils.LogInfo("Creating user on remote host", "user", observer.Metadata.User)
		// Create user with docker group membership in one command
		_, _ = dh.runSudo(remoteClient, fmt.Sprintf(
			"useradd --create-home --shell /bin/bash -G docker %s",
			utils.ShellQuote(observer.Metadata.User),
		), nil)
	} else {
		// User exists — ensure docker group membership
		utils.LogInfo("Adding user to docker group", "user", observer.Metadata.User)
		_, _ = dh.runSudo(remoteClient, fmt.Sprintf("usermod -aG docker %s", utils.ShellQuote(observer.Metadata.User)), nil)
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

	projectName := strings.ToLower(observer.Spec.Project)
	if !deployProjectNamePattern.MatchString(projectName) {
		return fmt.Errorf("invalid project name: %s", observer.Spec.Project)
	}

	utils.LogInfo("Verify systemd service", "service", projectName+".service")
	analyzeOut, err := dh.runSudo(client, fmt.Sprintf("systemd-analyze verify %s.service %s.timer", utils.ShellQuote(projectName), utils.ShellQuote(projectName)), nil)
	if err != nil {
		slog.Debug("systemd-analyze verify", "output", string(analyzeOut), "error", err)
	}

	// daemon-reload, restart, enable — ignore SSH EOF errors (commands succeed silently)
	_, _ = dh.runSudo(client, "systemctl daemon-reload", nil)
	_, _ = dh.runSudo(client, fmt.Sprintf("systemctl reload-or-restart %s.timer", utils.ShellQuote(projectName)), nil)
	_, _ = dh.runSudo(client, fmt.Sprintf("systemctl enable --quiet --no-warn %s", utils.ShellQuote(projectName)), nil)

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
	_, _ = dh.runSudo(client, fmt.Sprintf("mkdir -p %s %s", utils.ShellQuote(observer.Spec.Destination), utils.ShellQuote(filepath.Dir(remoteManifestPath))), nil)

	// Verify directory was created (ignore SSH session EOF errors)
	verifyOut, _ := dh.runSudo(client, fmt.Sprintf("test -d %s && echo ok", utils.ShellQuote(filepath.Dir(remoteManifestPath))), nil)
	if strings.TrimSpace(string(verifyOut)) != "ok" {
		return fmt.Errorf("failed to create config directory %s", filepath.Dir(remoteManifestPath))
	}

	utils.LogInfo("Change owner")

	_, _ = dh.runSudo(client, fmt.Sprintf("chown -R %s:%[1]s %s %s %s", utils.ShellQuote(observer.Metadata.User), utils.ShellQuote(observerUserHomeConfigDir), utils.ShellQuote(observer.Spec.Destination), utils.ShellQuote(filepath.Dir(remoteManifestPath))), nil)

	// Pre-install GitHub host key — witness user needs it for git clone
	utils.LogInfo("Installing GitHub host key")
	_, _ = dh.runSudo(client, fmt.Sprintf("mkdir -p /home/%s/.ssh && ssh-keyscan github.com >> /home/%s/.ssh/known_hosts && chown -R %s:%s /home/%s/.ssh", observer.Metadata.User, observer.Metadata.User, observer.Metadata.User, observer.Metadata.User, observer.Metadata.User), nil)

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
		"ObserverConfigPath": filepath.Join(observerUserHomeConfigDir, strings.ToLower(observer.Spec.Project)),
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

	// Decrypt SSH key and prepare for deployment
	sshKeyContent, err := utils.DecryptSecret(observer.Spec.Source.SSHKey, dh.AgeFilePath)
	if err != nil {
		return fmt.Errorf("decrypting SSH key: %w", err)
	}

	// Determine SSH key filename from original path or use default
	sshKeyFilename := "id_ed25519"
	if observer.Spec.Source.SSHKey != "" {
		// Try to get filename from the original path stored in manifest
		// Since we stored content, we need to use a default name
		sshKeyFilename = "id_ed25519"
	}
	remoteSSHKeyPath := fmt.Sprintf("/home/%s/.ssh/%s", observer.Metadata.User, sshKeyFilename)

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
		{
			TemplateName:    "direct",
			Content:         []byte(sshKeyContent),
			RemoteFilePath:  remoteSSHKeyPath,
			RemoteFileOwner: observer.Metadata.User,
		},
	}

	tmpRemoteDir, err := randomRemoteTmpDir(observer.Spec.Project)
	if err != nil {
		return err
	}
	// Create temp dir without sudo — SFTP upload runs as SSH user
	if _, err := client.SSH.Run("mkdir -p " + utils.ShellQuote(tmpRemoteDir)); err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() {
		_, _ = client.SSH.Run("rm -rf " + utils.ShellQuote(tmpRemoteDir))
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
	_, _ = dh.runSudo(client, fmt.Sprintf("mv %s /usr/local/bin/witness && chmod 755 /usr/local/bin/witness", utils.ShellQuote(remoteBinaryTmpPath)), nil)

	// Grant CAP_CHOWN so witness can apply volume claim ownership during reconcile.
	// The witness daemon runs unprivileged but needs to chown bind-mount directories
	// to container UIDs declared in volumeClaims. setcap may fail on systems without
	// libcap2-bin; this is non-fatal — volume claims degrade to a no-op without it.
	if out, err := dh.runSudo(client, "setcap cap_chown+ep /usr/local/bin/witness", nil); err != nil {
		utils.LogInfo("Failed to set CAP_CHOWN on witness binary — volume claims will not apply ownership (non-fatal)", "error", err, "output", string(out))
	}

	// Verify binary was installed
	binCheck, _ := dh.runSudo(client, "command -v witness", nil)
	if strings.TrimSpace(string(binCheck)) == "" {
		return fmt.Errorf("failed to install witness binary on remote host")
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

		_, _ = dh.runSudo(client, fmt.Sprintf("mv %s %s", utils.ShellQuote(tmpRemoteFilePath), utils.ShellQuote(deployFile.RemoteFilePath)), nil)

		if filepath.Base(deployFile.RemoteFilePath) == "age.key" {
			_, _ = dh.runSudo(client, fmt.Sprintf("chmod 600 %s", utils.ShellQuote(deployFile.RemoteFilePath)), nil)
		}

		if filepath.Base(deployFile.RemoteFilePath) == sshKeyFilename {
			_, _ = dh.runSudo(client, fmt.Sprintf("chmod 600 %s", utils.ShellQuote(deployFile.RemoteFilePath)), nil)
		}

		_, _ = dh.runSudo(client, fmt.Sprintf("chown %s:%[1]s %s", utils.ShellQuote(deployFile.RemoteFileOwner), utils.ShellQuote(deployFile.RemoteFilePath)), nil)
	}

	// Create SSH config for GitHub
	sshConfigContent := fmt.Sprintf(`Host github.com
  IdentityFile /home/%s/.ssh/%s
  User git
`, observer.Metadata.User, sshKeyFilename)
	remoteSSHConfigPath := fmt.Sprintf("/home/%s/.ssh/config", observer.Metadata.User)
	sshConfigTmpPath := path.Join(tmpRemoteDir, "config")
	sshConfigFile, err := sftp.Create(sshConfigTmpPath)
	if err != nil {
		return fmt.Errorf("creating temp SSH config: %w", err)
	}
	if _, err := sshConfigFile.Write([]byte(sshConfigContent)); err != nil {
		_ = sshConfigFile.Close()
		return fmt.Errorf("writing SSH config: %w", err)
	}
	_ = sshConfigFile.Close()
	_, _ = dh.runSudo(client, fmt.Sprintf("mv %s %s", utils.ShellQuote(sshConfigTmpPath), utils.ShellQuote(remoteSSHConfigPath)), nil)
	_, _ = dh.runSudo(client, fmt.Sprintf("chmod 600 %s", utils.ShellQuote(remoteSSHConfigPath)), nil)
	_, _ = dh.runSudo(client, fmt.Sprintf("chown %s:%[1]s %s", utils.ShellQuote(observer.Metadata.User), utils.ShellQuote(remoteSSHConfigPath)), nil)

	// Final recursive chown — mv via sudo creates root-owned dirs
	_, _ = dh.runSudo(client, fmt.Sprintf("chown -R %s:%[1]s %s", utils.ShellQuote(observer.Metadata.User), utils.ShellQuote(observerUserHomeConfigDir)), nil)

	return nil
}
