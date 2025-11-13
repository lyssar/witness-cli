package internal

import (
	"fmt"

	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
)

func ObserverAddCmd(cmd *cobra.Command, args []string) error {
	if err := CheckPrerequisites(cmd); err != nil {
		return err
	}

	observer := NewObserver()
	observer.Configure()
	observer.WriteConfig()

	utils.LogInfo("observer", "struct", fmt.Sprintf("%+v", observer))

	utils.LogSuccess("Add new service", "project", observer.Spec.Project)
	return nil
}
