package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/lyssar/witness-cli/utils"
	"github.com/spf13/cobra"
)

type OSFamily string

const (
	OSFamilyDebian OSFamily = "debian"
	OSFamilyRHEL   OSFamily = "rhel"
)

type OSInfo struct {
	ID              string
	IDLike          string
	Version         string
	VersionCodename string
	Family          OSFamily
}

type Tool struct {
	Name    string
	Check   string
	Install func(pkgMgr string, osInfo OSInfo) string
}

var doctorTools = []Tool{
	{
		Name:  "git",
		Check: "command -v git",
		Install: func(pkgMgr string, osInfo OSInfo) string {
			return fmt.Sprintf("%s install -y git", pkgMgr)
		},
	},
	{
		Name:  "age",
		Check: "command -v age",
		Install: func(pkgMgr string, osInfo OSInfo) string {
			return fmt.Sprintf("%s install -y age", pkgMgr)
		},
	},
	{
		Name:  "libcap (setcap)",
		Check: "test -x /sbin/setcap && echo /sbin/setcap || command -v setcap || true",
		Install: func(pkgMgr string, osInfo OSInfo) string {
			if pkgMgr == "apt-get" {
				return "apt-get install -y libcap2-bin"
			}
			return fmt.Sprintf("%s install -y libcap", pkgMgr)
		},
	},
	{
		Name:  "docker",
		Check: "command -v docker",
		Install: func(pkgMgr string, osInfo OSInfo) string {
			return dockerInstallCmd(pkgMgr, osInfo)
		},
	},
	{
		Name:  "docker compose",
		Check: "docker compose version >/dev/null 2>&1 && echo compose-plugin || command -v docker-compose || true",
		Install: func(pkgMgr string, osInfo OSInfo) string {
			return dockerInstallCmd(pkgMgr, osInfo)
		},
	},
}

func dockerInstallCmd(pkgMgr string, osInfo OSInfo) string {
	if pkgMgr == "apt-get" {
		return dockerAptInstallCmd(osInfo)
	}
	return dockerRHELInstallCmd(pkgMgr, osInfo)
}

func dockerAptInstallCmd(osInfo OSInfo) string {
	if osInfo.VersionCodename == "" {
		return "echo 'docker install requires VERSION_CODENAME in /etc/os-release' >&2 && exit 1"
	}
	distro := "debian"
	if osInfo.ID == "ubuntu" {
		distro = "ubuntu"
	}
	return strings.Join([]string{
		"install -m 0755 -d /etc/apt/keyrings",
		fmt.Sprintf("curl -fsSL https://download.docker.com/linux/%s/gpg -o /etc/apt/keyrings/docker.asc", distro),
		"chmod a+r /etc/apt/keyrings/docker.asc",
		fmt.Sprintf("echo \"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/%s %s stable\" > /etc/apt/sources.list.d/docker.list", distro, osInfo.VersionCodename),
		"apt-get update",
		"apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin",
	}, " && ")
}

func dockerRHELInstallCmd(pkgMgr string, osInfo OSInfo) string {
	repo := "centos"
	if osInfo.ID == "rhel" {
		repo = "rhel"
	}
	cmds := []string{}
	if pkgMgr == "dnf" {
		cmds = append(cmds, "dnf install -y dnf-plugins-core")
	}
	cmds = append(cmds,
		fmt.Sprintf("%s config-manager --add-repo https://download.docker.com/linux/%s/docker-ce.repo", pkgMgr, repo),
		fmt.Sprintf("%s install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin", pkgMgr),
	)
	return strings.Join(cmds, " && ")
}

type commandRunner interface {
	Run(command string) ([]byte, error)
}

type localRunner struct{}

func (localRunner) Run(command string) ([]byte, error) {
	return exec.Command("bash", "-c", command).CombinedOutput()
}

type remoteRunner struct {
	client *utils.RemoteClient
}

func (r remoteRunner) Run(command string) ([]byte, error) {
	return r.client.SSH.Run(command)
}

type DoctorHandler struct {
	Local     bool
	CheckOnly bool
	SSH       SSHConfig
	Sudoer    string
	pkgMgr    string
	osInfo    OSInfo
	runner    commandRunner
	client    *utils.RemoteClient
}

func NewDoctorHandler(cmd *cobra.Command) *DoctorHandler {
	local := flagBool(cmd, "local")
	checkOnly := flagBool(cmd, "check-only")

	sshUser, err := cmd.Flags().GetString("ssh-user")
	utils.CheckErr(err)

	sshKey, err := cmd.Flags().GetString("ssh-key")
	utils.CheckErr(err)

	host, err := cmd.Flags().GetString("host")
	utils.CheckErr(err)

	if !local && host == "" {
		utils.CheckErr(errors.New("specify --host or --local"))
	}

	dh := &DoctorHandler{
		Local:     local,
		CheckOnly: checkOnly,
		SSH: SSHConfig{
			User: sshUser,
			Key:  &sshKey,
			Host: host,
		},
	}

	if local {
		dh.runner = localRunner{}
	} else {
		client, err := utils.NewRemote(host, sshUser, sshKey)
		utils.CheckErr(err)
		dh.client = client
		dh.runner = remoteRunner{client: client}
	}

	return dh
}

func flagBool(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	value, err := cmd.Flags().GetBool(name)
	utils.CheckErr(err)
	return value
}

func DoctorCmd(cmd *cobra.Command, args []string) error {
	return NewDoctorHandler(cmd).Run()
}

func (dh *DoctorHandler) Run() error {
	osInfo, err := dh.detectOS()
	if err != nil {
		return err
	}
	dh.osInfo = osInfo
	if osInfo.Family == "" {
		return fmt.Errorf("unsupported OS %q — supported: Debian/Ubuntu (apt), RHEL-family (dnf/yum)", osInfo.ID)
	}

	pkgMgr, err := dh.resolvePkgMgr(osInfo)
	if err != nil {
		return err
	}
	dh.pkgMgr = pkgMgr

	target := "local machine"
	if !dh.Local {
		target = dh.SSH.Host
	}
	utils.LogInfo("Checking prerequisites", "target", target, "os", osInfo.ID, "version", osInfo.Version)

	var missing []Tool
	for _, tool := range doctorTools {
		present, detail := dh.checkTool(tool)
		if present {
			slog.Info(fmt.Sprintf("%s: present", tool.Name), "path", detail)
		} else {
			slog.Info(fmt.Sprintf("%s: missing", tool.Name))
			missing = append(missing, tool)
		}
	}

	if len(missing) == 0 {
		utils.LogSuccess("All prerequisites satisfied")
		return nil
	}

	if dh.CheckOnly {
		return dh.reportMissing(missing)
	}

	return dh.installMissing(missing)
}

func (dh *DoctorHandler) detectOS() (OSInfo, error) {
	out, err := dh.runner.Run("cat /etc/os-release")
	if err != nil {
		return OSInfo{}, fmt.Errorf("reading /etc/os-release: %w", err)
	}
	return parseOSRelease(string(out)), nil
}

func parseOSRelease(content string) OSInfo {
	info := OSInfo{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch key {
		case "ID":
			info.ID = value
		case "ID_LIKE":
			info.IDLike = value
		case "VERSION_ID":
			info.Version = value
		case "VERSION_CODENAME":
			info.VersionCodename = value
		}
	}
	info.Family = detectOSFamily(info)
	return info
}

func detectOSFamily(info OSInfo) OSFamily {
	switch info.ID {
	case "debian", "ubuntu":
		return OSFamilyDebian
	case "rhel", "rocky", "alma", "almalinux", "centos", "fedora", "ol", "amzn":
		return OSFamilyRHEL
	}
	for _, like := range strings.Fields(info.IDLike) {
		switch like {
		case "debian":
			return OSFamilyDebian
		case "rhel", "fedora":
			return OSFamilyRHEL
		}
	}
	return ""
}

func (dh *DoctorHandler) resolvePkgMgr(osInfo OSInfo) (string, error) {
	if osInfo.Family == OSFamilyDebian {
		return "apt-get", nil
	}
	out, err := dh.runner.Run("command -v dnf || command -v yum")
	if err != nil {
		return "", fmt.Errorf("detecting package manager: %w", err)
	}
	pkgMgr := strings.TrimSpace(string(out))
	if pkgMgr == "" {
		return "", fmt.Errorf("no supported package manager (dnf/yum) found")
	}
	pkgMgr = filepath.Base(pkgMgr)
	switch pkgMgr {
	case "dnf", "yum":
		return pkgMgr, nil
	}
	return "", fmt.Errorf("unsupported package manager %q (expected dnf or yum)", pkgMgr)
}

func (dh *DoctorHandler) checkTool(tool Tool) (bool, string) {
	out, err := dh.runner.Run(tool.Check)
	if err != nil {
		return false, ""
	}
	detail := strings.TrimSpace(string(out))
	return detail != "", detail
}

func (dh *DoctorHandler) reportMissing(missing []Tool) error {
	names := make([]string, 0, len(missing))
	for _, tool := range missing {
		names = append(names, tool.Name)
	}
	fmt.Println("Missing prerequisites:")
	for _, tool := range missing {
		fmt.Printf("  %s\n", tool.Name)
	}
	fmt.Println("Install commands:")
	for _, tool := range missing {
		fmt.Printf("  %s\n", tool.Install(dh.pkgMgr, dh.osInfo))
	}
	return fmt.Errorf("missing prerequisites: %s", strings.Join(names, ", "))
}

func (dh *DoctorHandler) installMissing(missing []Tool) error {
	var remaining []Tool
	for _, tool := range missing {
		install := false
		err := huh.NewConfirm().
			Title(fmt.Sprintf("Install %s?", tool.Name)).
			Description(tool.Install(dh.pkgMgr, dh.osInfo)).
			Value(&install).
			Run()
		if err != nil {
			return fmt.Errorf("prompting to install %s: %w", tool.Name, err)
		}
		if !install {
			remaining = append(remaining, tool)
			continue
		}
		utils.LogInfo("Installing", "tool", tool.Name)
		out, err := dh.runSudo(tool.Install(dh.pkgMgr, dh.osInfo))
		if err != nil {
			return fmt.Errorf("installing %s: %s (%s)", tool.Name, err, strings.TrimSpace(string(out)))
		}
		utils.LogSuccess("Installed", "tool", tool.Name)
	}

	if len(remaining) > 0 {
		names := make([]string, 0, len(remaining))
		for _, tool := range remaining {
			names = append(names, tool.Name)
		}
		return fmt.Errorf("missing prerequisites: %s", strings.Join(names, ", "))
	}
	return nil
}

func (dh *DoctorHandler) AskForSudoer() {
	if dh.Sudoer == "" {
		dh.Sudoer = strings.TrimSpace(os.Getenv(deploySudoPasswordEnv))
	}

	if dh.Sudoer == "" {
		err := huh.NewInput().
			Title("Enter sudoer password").
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

func (dh *DoctorHandler) runSudo(command string) ([]byte, error) {
	run := func() ([]byte, error) {
		if dh.Local {
			return runLocalSudo(command, dh.Sudoer)
		}
		return dh.client.RunSudo(command, dh.Sudoer, nil)
	}

	out, err := run()
	if err == nil {
		return out, nil
	}

	if dh.Sudoer != "" || !sudoPasswordPromptRequired(out) {
		return out, err
	}

	dh.AskForSudoer()
	return run()
}

func runLocalSudo(command, pass string) ([]byte, error) {
	args := []string{"-n", "--", "bash", "-c", command}
	if pass != "" {
		args = []string{"-S", "-p", "", "--", "bash", "-c", command}
	}
	cmd := exec.Command("sudo", args...)
	if pass != "" {
		cmd.Stdin = strings.NewReader(pass + "\n")
	}
	return cmd.CombinedOutput()
}
