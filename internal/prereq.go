package internal

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
)

func CheckPrerequisites(cmd *cobra.Command) error {
	var missing []string

	ensureCliConfigDir()

	if ok, _ := utils.BinaryExists("age"); !ok {
		missing = append(missing, "age")
	}

	if ok, _ := utils.BinaryExists("ssh"); ok {
		cmd := exec.Command("ssh", "-V")
		out, err := cmd.CombinedOutput()
		if err != nil {
			slog.Debug("ssh version check failed", "error", err.Error(), "output", strings.TrimSpace(string(out)))
			missing = append(missing, "ssh (failed to execute)")
		} else {
			slog.Debug("ssh version detected", "output", strings.TrimSpace(string(out)))
		}
	} else {
		missing = append(missing, "ssh")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing prerequisites: %s", strings.Join(missing, ", "))
	}

	return nil
}

func ensureCliConfigDir() {
	userPath, err := os.UserConfigDir()
	utils.CheckErr(err)
	err = os.MkdirAll(fmt.Sprintf("%s/%s", userPath, utils.APP_NAME), 0700)
	utils.CheckErr(err)
}
