package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/charmbracelet/huh"
	"github.com/kevinburke/ssh_config"
	"github.com/melbahja/goph"
	"golang.org/x/crypto/ssh"
)

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
		userSwitch = fmt.Sprintf(" -u %s ", *user)
	}
	return r.SSH.Run(fmt.Sprintf("echo '%s' | sudo -S -p ''%s bash -c '%s'", pass, userSwitch, command))
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

	client, err := goph.New(sshUser, configuredHost, auth)

	return client, nil
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
		huh.NewInput().Title("SSH key passphrase").EchoMode(huh.EchoModePassword).Value(&passphrase).Run()
		_, err = ssh.ParsePrivateKeyWithPassphrase(sshKeyPemBytes, []byte(passphrase))
	}

	if err != nil {
		return "", "", err
	}

	return normalizedKeyPath, passphrase, nil
}
