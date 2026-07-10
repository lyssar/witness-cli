package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/huh"
	"github.com/kevinburke/ssh_config"
	"github.com/melbahja/goph"
	"golang.org/x/crypto/ssh"
)

var unixUsernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

var (
	sshConfig        *ssh_config.Config
	sshConfigOnce    sync.Once
	remoteClient     *RemoteClient
	remoteClientErr  error
	remoteClientOnce sync.Once
)

type RemoteClient struct {
	SSH *goph.Client
}

func NewRemote(host string, sshUser string, sshKey string) (*RemoteClient, error) {
	remoteClientOnce.Do(func() {
		sshClient, err := newSSHClient(host, sshUser, sshKey)
		if err != nil {
			remoteClientErr = err
			return
		}

		remoteClient = &RemoteClient{
			SSH: sshClient,
		}
	})
	return remoteClient, remoteClientErr
}

func (r *RemoteClient) RunSudo(command string, pass string, user *string) ([]byte, error) {
	userSwitch := ""
	if user != nil {
		if !unixUsernamePattern.MatchString(*user) {
			return nil, fmt.Errorf("invalid sudo user: %s", *user)
		}
		userSwitch = fmt.Sprintf(" -u %s", ShellQuote(*user))
	}

	remoteCmd := fmt.Sprintf("sudo -n%s -- bash -c %s", userSwitch, ShellQuote(command))
	var stdin *strings.Reader
	if pass != "" {
		remoteCmd = fmt.Sprintf("sudo -S -p ''%s -- bash -c %s", userSwitch, ShellQuote(command))
		stdin = strings.NewReader(pass + "\n")
	}

	sess, err := r.SSH.NewSession()
	if err != nil {
		return nil, err
	}

	if stdin != nil {
		sess.Stdin = stdin
	}
	out, runErr := sess.CombinedOutput(remoteCmd)
	closeErr := sess.Close()

	// SSH sessions often return EOF on close even when the command succeeded.
	// If the command itself returned output and no command error, treat close errors as non-fatal.
	if runErr != nil {
		return out, runErr
	}
	if closeErr != nil && len(out) == 0 {
		return out, closeErr
	}

	return out, nil
}

func (r RemoteClient) TransferFile(srcFile string, dstFile string) error {
	slog.Debug("Transferring file to remote", "src", srcFile, "dst", dstFile)
	return r.SSH.Upload(srcFile, dstFile)
}

func loadSSHConfig() *ssh_config.Config {
	sshConfigOnce.Do(func() {
		sshConfigPath := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
		sshConfigB, err := os.ReadFile(sshConfigPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				sshConfig = &ssh_config.Config{}
				return
			}

			slog.Warn("Failed to read SSH config, continuing with explicit flags only", "path", sshConfigPath, "error", err)
			sshConfig = &ssh_config.Config{}
			return
		}

		sshConfig, err = ssh_config.DecodeBytes(sshConfigB)
		if err != nil {
			slog.Warn("Failed to parse SSH config, continuing with explicit flags only", "path", sshConfigPath, "error", err)
			sshConfig = &ssh_config.Config{}
		}
	})

	return sshConfig
}

func newSSHClient(host string, sshUser string, sshKey string) (*goph.Client, error) {
	var err error
	sshConfig := loadSSHConfig()
	originalHost := host
	configuredPort := uint(22)

	configuredHost, hostPort, err := splitSSHHostPort(host)
	if err != nil {
		return nil, err
	}
	if hostPort != 0 {
		configuredPort = hostPort
	}

	configHostName, _ := sshConfig.Get(originalHost, "HostName")
	if configHostName != "" {
		configuredHost = configHostName
	}

	configPort, _ := sshConfig.Get(originalHost, "Port")
	if configuredPort == 22 && strings.TrimSpace(configPort) != "" {
		parsedPort, convErr := strconv.ParseUint(strings.TrimSpace(configPort), 10, 16)
		if convErr != nil {
			return nil, fmt.Errorf("invalid ssh config port for %q: %w", originalHost, convErr)
		}
		configuredPort = uint(parsedPort)
	}

	if sshKey == "" {
		sshKey, err = sshConfig.Get(originalHost, "IdentityFile")
		if err != nil {
			return nil, err
		}
	}

	normalizedSSHKey, passphrase, err := getNormaluedSSHKey(sshKey)

	if err != nil {
		return nil, err
	}

	slog.Debug("IdentityFile", "sshKey", normalizedSSHKey, "host", host)

	if sshUser == "" {
		sshUser, _ = sshConfig.Get(originalHost, "User")
	}

	auth, err := goph.Key(normalizedSSHKey, passphrase)

	if err != nil {
		return nil, err
	}

	callback, err := goph.DefaultKnownHosts()
	if err != nil {
		return nil, err
	}

	return goph.NewConn(&goph.Config{
		User:     sshUser,
		Addr:     configuredHost,
		Port:     configuredPort,
		Auth:     auth,
		Timeout:  goph.DefaultTimeout,
		Callback: callback,
	})
}

func splitSSHHostPort(host string) (string, uint, error) {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return "", 0, errors.New("ssh host is empty")
	}

	if parsedHost, parsedPort, err := net.SplitHostPort(trimmed); err == nil {
		portValue, convErr := strconv.ParseUint(parsedPort, 10, 16)
		if convErr != nil {
			return "", 0, fmt.Errorf("invalid ssh port %q: %w", parsedPort, convErr)
		}
		return parsedHost, uint(portValue), nil
	}

	if strings.Count(trimmed, ":") == 1 && !strings.Contains(trimmed, "]") {
		hostPart, portPart, ok := strings.Cut(trimmed, ":")
		if ok {
			portValue, convErr := strconv.ParseUint(portPart, 10, 16)
			if convErr != nil {
				return "", 0, fmt.Errorf("invalid ssh port %q: %w", portPart, convErr)
			}
			return hostPart, uint(portValue), nil
		}
	}

	return trimmed, 0, nil
}

func getNormaluedSSHKey(sshKey string) (string, string, error) {
	var err error
	passphrase := ""

	normalizedKeyPath, err := NormalizeHomePath(sshKey)

	if err != nil || !FileExists(sshKey) {
		return "", "", errors.New("ssh key not found")
	}

	sshKeyPemBytes, err := os.ReadFile(normalizedKeyPath)
	if err != nil {
		return "", "", err
	}

	_, err = ssh.ParsePrivateKey(sshKeyPemBytes)

	if isPassphraseMissingError(err) {
		err = huh.NewInput().Title("SSH key passphrase").EchoMode(huh.EchoModePassword).Value(&passphrase).Run()
		if err != nil {
			return "", "", fmt.Errorf("asking for ssh key passphrase: %w", err)
		}
		_, err = ssh.ParsePrivateKeyWithPassphrase(sshKeyPemBytes, []byte(passphrase))
	}

	if err != nil {
		return "", "", err
	}

	return normalizedKeyPath, passphrase, nil
}

func isPassphraseMissingError(err error) bool {
	if err == nil {
		return false
	}

	var passErr *ssh.PassphraseMissingError
	return errors.As(err, &passErr)
}
