package local

import (
	"errors"
	"fmt"
	"os"

	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
)

type Observer struct {
	Name           string
	repo           string
	sshPrivateKey  string
	ageKey         string
	knownHostsFile string
}

func NewObserver(gitRepo string, cmd *cobra.Command) (Observer, error) {
	name, _ := cmd.Flags().GetString("name")
	sshPrivateKey := os.Getenv("SKULD_SSH_PRVT_KEY")
	ageKey := os.Getenv("SKULD_AGE_KEY")
	knownHostsFile := os.Getenv("SKULD_KNOWN_HOST_FILE")

	observer := Observer{
		Name:           name,
		repo:           gitRepo,
		sshPrivateKey:  sshPrivateKey,
		ageKey:         ageKey,
		knownHostsFile: knownHostsFile,
	}
	return observer, observer.Validate()
}

func (observer Observer) Validate() error {
	// FIXME: change validation for new meta data, check only if systemctl has the service, unit files will be created later
	servicePath := fmt.Sprintf("/etc/systemd/system/%s-%s.service", utils.APP_NAME, observer.Name)
	if utils.FileExists(servicePath) {
		return fmt.Errorf("systemd service already exists: %s", servicePath)
	}

	if observer.sshPrivateKey == "" || !utils.FileExists(observer.sshPrivateKey) {
		return errors.New("You must set --git-secret to a valid age based secret env file with the keys MCL_GIT_TOKEN and MCL_GIT_USER set")
	}

	if observer.ageKey == "" || !utils.FileExists(observer.ageKey) {
		return errors.New("You must set --private-key to an existing age private key")
	}

	return nil
}
