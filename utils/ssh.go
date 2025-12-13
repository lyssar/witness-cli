package utils

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/charmbracelet/huh"
	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	sshConfig        *ssh_config.Config
	sshConfigOnce    sync.Once
	remoteClient     *RemoteClient
	remoteClientErr  error
	remoteClientOnce sync.Once
)

type RemoteClient struct {
	SSH *ssh.Client
	Scp *scp.Client
}

func NewRemote(host string, sshUser string, sshKey string) (*RemoteClient, error) {
	remoteClientOnce.Do(func() {
		sshClient, err := newSSHClient(host, sshUser, sshKey)
		if err != nil {
			remoteClientErr = err
			return
		}

		scpClient, err := scp.NewClientBySSH(sshClient)
		if err != nil {
			remoteClientErr = err
			return
		}

		remoteClient = &RemoteClient{
			SSH: sshClient,
			Scp: &scpClient,
		}
	})
	return remoteClient, nil
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

func newSSHClient(host string, sshUser string, sshKey string) (*ssh.Client, error) {
	var err error
	sshConfig := loadSSHConfig()

	configuredHost, _ := sshConfig.Get(host, "HostName")
	if configuredHost == "" {
		configuredHost = host
	}

	configuredPort, _ := sshConfig.Get(host, "Port")

	if sshKey == "" {
		sshKey, err = sshConfig.Get(host, "IdentityFile")
		if err != nil {
			return nil, err
		}
	}

	slog.Debug("IdentityFile", "sshKey", sshKey, "host", host)

	if sshUser == "" {
		sshUser, _ = sshConfig.Get(host, "User")
	}

	parsedSshKeySigner, err := getKeySigner(sshKey)
	if err != nil {
		return nil, err
	}

	hostKeyCallback, err := knownhosts.New(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts"))
	if err != nil {
		return nil, err
	}

	clientConf := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(*parsedSshKeySigner),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	serverAddr := net.JoinHostPort(configuredHost, configuredPort)

	client, err := ssh.Dial("tcp", serverAddr, clientConf)

	CheckErr(err)

	return client, nil
}

func getKeySigner(sshKey string) (*ssh.Signer, error) {
	var (
		signer ssh.Signer
		err    error
	)

	normalizedKeyPath, err := NormalizeHomePath(sshKey)

	if err != nil || !FileExists(sshKey) {
		return nil, errors.New("ssh key not found")
	}

	sshKeyPemBytes, err := os.ReadFile(normalizedKeyPath)
	if err != nil {
		return nil, err
	}

	var passErr *ssh.PassphraseMissingError
	signer, err = ssh.ParsePrivateKey(sshKeyPemBytes)

	if errors.Is(err, passErr) {
		var passphrase string
		huh.NewInput().Title("SSH key passphrase").EchoMode(huh.EchoModePassword).Value(&passphrase).Run()
		signer, err = ssh.ParsePrivateKeyWithPassphrase(sshKeyPemBytes, []byte(passphrase))
	}

	if err != nil {
		return nil, err
	}

	return &signer, nil
}
