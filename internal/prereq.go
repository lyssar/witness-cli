package internal

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	if ok, _ := utils.BinaryExists("git"); !ok {
		missing = append(missing, "git")
	}

	if ok, _ := utils.BinaryExists("systemctl"); !ok {
		missing = append(missing, "systemctl")
	} else if v, err := systemdMajorVersion(); err != nil || v < 249 {
		missing = append(missing, fmt.Sprintf("systemctl must be equal or newer then 249 got %d", v))
	}

	dockerOK, _ := utils.BinaryExists("docker")
	podmanOK, _ := utils.BinaryExists("podman")
	if !dockerOK && !podmanOK {
		missing = append(missing, "docker or podman")
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
	os.MkdirAll(fmt.Sprintf("%s/%s", userPath, utils.APP_NAME), 0755)
}

func isFolderFeasable(workingPath string) bool {
	if !utils.FolderExists(workingPath) {
		slog.Debug("folder does not exist", "folder", workingPath)
		return false
	}

	if !utils.DirIsEmpty(workingPath) {
		slog.Debug("folder seems not be empty", "folder", workingPath)
		return false
	}

	return true
}

func evaluateWorkingPath(cmd *cobra.Command) (string, error) {
	workingPath, err := cmd.Flags().GetString("working-path")
	if err != nil {
		return "", err
	}

	if workingPath == "" {
		if userPath, err := os.UserConfigDir(); err == nil {
			workingPath = userPath
		} else {
			workingPath = filepath.Join(os.Getenv("HOME"), ".config")
		}
		workingPath = fmt.Sprintf("%s/%s", workingPath, utils.APP_NAME)
	}
	return os.ExpandEnv(workingPath), nil

}

func systemdMajorVersion() (int, error) {
	out, err := exec.Command("systemctl", "--version").Output()
	slog.Debug("checking systemd major version", "version-out", string(out), "error", err)

	if err != nil {
		return 0, err
	}
	line := strings.SplitN(string(out), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		if v, err := strconv.Atoi(fields[1]); err == nil {
			return v, nil
		}
	}
	return 0, fmt.Errorf("couldn't parse systemd version: %q", line)
}
