package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
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

	remoteCmd := fmt.Sprintf("sudo -S -p ''%s -- bash -c %s", userSwitch, ShellQuote(command))

	sess, err := r.SSH.NewSession()
	if err != nil {
		return nil, err
	}

	sess.Stdin = strings.NewReader(pass + "\n")
	out, runErr := sess.CombinedOutput(remoteCmd)
	closeErr := sess.Close()
	if runErr != nil {
		return out, runErr
	}
	if closeErr != nil {
		return out, closeErr
	}

	return out, nil
}

func (r RemoteClient) TransferFile(srcFile string, dstFile string) {
	// TODO
	slog.Debug("Copy file to server with SCP package")
}

func loadSSHConfig() *ssh_config.Config {
	sshConfigOnce.Do(func() {
		sshConfigB, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".ssh", "config"))
		CheckErr(err)
		sshConfig, err = ssh_config.DecodeBytes(sshConfigB)
		CheckErr(err)
	})

	return sshConfig
}

func newSSHClient(host string, sshUser string, sshKey string) (*goph.Client, error) {
	var err error
	sshConfig := loadSSHConfig()

	configuredHost, _ := sshConfig.Get(host, "HostName")
	if configuredHost == "" {
		configuredHost = host
	}

	// configuredPort, _ := sshConfig.Get(host, "Port")

	if sshKey == "" {
		sshKey, err = sshConfig.Get(host, "IdentityFile")
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
		sshUser, _ = sshConfig.Get(host, "User")
	}

	auth, err := goph.Key(normalizedSSHKey, passphrase)

	if err != nil {
		return nil, err
	}

	return goph.New(sshUser, configuredHost, auth)
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

	var passErr *ssh.PassphraseMissingError
	_, err = ssh.ParsePrivateKey(sshKeyPemBytes)

	if errors.Is(err, passErr) {
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
