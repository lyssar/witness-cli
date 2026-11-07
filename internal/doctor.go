package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/lyssar/witness-cli/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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

type hostFixStatus string

const (
	hostFixOK       hostFixStatus = "OK"
	hostFixRepaired hostFixStatus = "REPAIRED"
	hostFixSkipped  hostFixStatus = "SKIP"
	hostFixFailed   hostFixStatus = "FAILED"
)

const (
	defaultServiceUser   = "witness"
	observerConfigMode   = "700"
	binaryCapability     = "cap_chown+ep"
	installedBinaryPath  = "/usr/local/bin/witness"
	observerDiscoveryCmd = "find /home -maxdepth 5 -type f -path '*/.config/witness/*/manifest.yaml' -print0 2>/dev/null || true"
)

type observerConfig struct {
	Root string
	User string
}

type observerManifest struct {
	Metadata struct {
		User *string `yaml:"user"`
	} `yaml:"metadata"`
}

func logHostFix(status hostFixStatus, subject string, args ...any) {
	slog.Info(fmt.Sprintf("%s %s", status, subject), args...)
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

	var prereqErr error
	if len(missing) > 0 {
		if dh.CheckOnly {
			prereqErr = dh.reportMissing(missing)
		} else {
			prereqErr = dh.installMissing(missing)
		}
	}

	hostErr := dh.applyHostFixes()
	if err := errors.Join(prereqErr, hostErr); err != nil {
		return err
	}

	utils.LogSuccess("All prerequisites satisfied")
	return nil
}

func (dh *DoctorHandler) applyHostFixes() error {
	configs, discoveryErr := dh.discoverObserverConfigs()
	users := serviceUsers(configs)
	if len(users) == 0 {
		users = []string{defaultServiceUser}
	}

	var errs []error
	if discoveryErr != nil {
		errs = append(errs, discoveryErr)
	}
	if err := dh.fixAppGroup(); err != nil {
		errs = append(errs, err)
	}
	for _, user := range users {
		if err := dh.fixGroupMembership(user); err != nil {
			errs = append(errs, err)
		}
	}
	for _, cfg := range configs {
		if err := dh.fixObserverConfigRoot(cfg); err != nil {
			errs = append(errs, err)
		}
	}
	if err := dh.fixBinaryCapability(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (dh *DoctorHandler) runRead(command string) ([]byte, error) {
	if dh.CheckOnly {
		if dh.Local {
			return runLocalSudo(command, "")
		}
		return dh.client.RunSudo(command, "", nil)
	}
	return dh.runSudo(command)
}

func (dh *DoctorHandler) discoverObserverConfigs() ([]observerConfig, error) {
	out, err := dh.runRead(observerDiscoveryCmd)
	if err != nil {
		logHostFix(hostFixFailed, "observer config discovery", "error", err.Error())
		return nil, fmt.Errorf("discovering observer configs: %w", err)
	}

	configs := []observerConfig{}
	var errs []error
	for _, manifestPath := range parseManifestPaths(string(out)) {
		root := filepath.Dir(manifestPath)
		pathOwner, ok := parseObserverConfigRoot(root)
		if !ok {
			logHostFix(hostFixFailed, fmt.Sprintf("observer config root %q", root), "reason", "path does not match /home/<user>/.config/witness/<app>")
			errs = append(errs, fmt.Errorf("rejecting observer config root %q: path does not match /home/<user>/.config/witness/<app>", root))
			continue
		}
		content, readErr := dh.runRead("cat " + utils.ShellQuote(manifestPath))
		if readErr != nil {
			logHostFix(hostFixFailed, fmt.Sprintf("observer manifest %q", manifestPath), "error", readErr.Error())
			errs = append(errs, fmt.Errorf("reading observer manifest %q: %w", manifestPath, readErr))
			continue
		}
		manifestUser, parseErr := parseObserverManifestUser(content)
		if parseErr != nil {
			logHostFix(hostFixFailed, fmt.Sprintf("observer manifest %q", manifestPath), "error", parseErr.Error())
			errs = append(errs, parseErr)
			continue
		}
		user, ok := observerUserForConfigRoot(pathOwner, manifestUser)
		if !ok {
			logHostFix(hostFixFailed, fmt.Sprintf("observer config root %q", root), "reason", "metadata.user does not match path owner")
			errs = append(errs, fmt.Errorf("rejecting observer config root %q: metadata.user %q does not match path owner %q", root, manifestUser, pathOwner))
			continue
		}
		configs = append(configs, observerConfig{Root: root, User: user})
	}
	return configs, errors.Join(errs...)
}

func (dh *DoctorHandler) fixAppGroup() error {
	subject := fmt.Sprintf("group %q", appGroupName)
	out, err := dh.runner.Run(fmt.Sprintf("getent group %s", utils.ShellQuote(appGroupName)))
	if err == nil && strings.TrimSpace(string(out)) != "" {
		logHostFix(hostFixOK, subject)
		return nil
	}

	command := fmt.Sprintf("groupadd --system %s", utils.ShellQuote(appGroupName))
	if dh.CheckOnly {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("group %q is missing: run %s", appGroupName, command)
	}

	out, err = dh.runSudo(command)
	if err != nil {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("creating group %q: %w (%s)", appGroupName, err, strings.TrimSpace(string(out)))
	}

	out, err = dh.runner.Run(fmt.Sprintf("getent group %s", utils.ShellQuote(appGroupName)))
	if err != nil || strings.TrimSpace(string(out)) == "" {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("group %q is still missing after %s", appGroupName, command)
	}
	logHostFix(hostFixRepaired, subject, "command", command)
	return nil
}

func (dh *DoctorHandler) fixGroupMembership(user string) error {
	subject := fmt.Sprintf("group membership of %q in %q", user, appGroupName)
	passwd, err := dh.runner.Run(fmt.Sprintf("getent passwd %s", utils.ShellQuote(user)))
	if err != nil || strings.TrimSpace(string(passwd)) == "" {
		logHostFix(hostFixSkipped, subject, "reason", "service user does not exist yet")
		return nil
	}

	groups, err := dh.runner.Run(fmt.Sprintf("id -nG %s", utils.ShellQuote(user)))
	if err != nil {
		logHostFix(hostFixFailed, subject, "error", err.Error())
		return fmt.Errorf("reading group membership of %q: %w", user, err)
	}
	if validateAppGroupMembership(appGroupName, user, string(groups)) == nil {
		logHostFix(hostFixOK, subject)
		return nil
	}

	command := fmt.Sprintf("usermod -aG %s %s", utils.ShellQuote(appGroupName), utils.ShellQuote(user))
	if dh.CheckOnly {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("user %q is not a member of group %q: run %s", user, appGroupName, command)
	}

	if out, err := dh.runSudo(command); err != nil {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("adding %q to group %q: %w (%s)", user, appGroupName, err, strings.TrimSpace(string(out)))
	}

	groups, err = dh.runner.Run(fmt.Sprintf("id -nG %s", utils.ShellQuote(user)))
	if err != nil || validateAppGroupMembership(appGroupName, user, string(groups)) != nil {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("user %q is still not a member of group %q after %s", user, appGroupName, command)
	}
	logHostFix(hostFixRepaired, subject, "command", command)
	return nil
}

func (dh *DoctorHandler) fixObserverConfigRoot(cfg observerConfig) error {
	subject := fmt.Sprintf("config root %q", cfg.Root)
	command := observerConfigRootGuard(cfg.Root) + fmt.Sprintf("install -d -m %s -o %s -g %s %s",
		observerConfigMode,
		utils.ShellQuote(cfg.User),
		utils.ShellQuote(cfg.User),
		utils.ShellQuote(cfg.Root),
	)

	matches, statErr := dh.observerConfigRootStatus(cfg)
	if statErr == nil && matches {
		logHostFix(hostFixOK, subject)
		return nil
	}
	if dh.CheckOnly {
		if statErr != nil {
			logHostFix(hostFixFailed, subject, "error", statErr.Error())
			return fmt.Errorf("inspecting config root %q: %w", cfg.Root, statErr)
		}
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("config root %q does not match mode %s and owner %q: run %s", cfg.Root, observerConfigMode, cfg.User, command)
	}

	if out, err := dh.runSudo(command); err != nil {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("repairing config root %q: %w (%s)", cfg.Root, err, strings.TrimSpace(string(out)))
	}
	if matches, statErr = dh.observerConfigRootStatus(cfg); statErr != nil || !matches {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("config root %q still does not match mode %s and owner %q after %s", cfg.Root, observerConfigMode, cfg.User, command)
	}
	logHostFix(hostFixRepaired, subject, "command", command)
	return nil
}

func observerConfigRootGuard(root string) string {
	clean := filepath.Clean(root)
	parts := strings.Split(clean, "/")
	checks := []string{}
	current := ""
	for i := 1; i < len(parts)-1; i++ {
		current += "/" + parts[i]
		quoted := utils.ShellQuote(current)
		checks = append(checks, "test -d "+quoted+" && test ! -L "+quoted)
	}
	quotedRoot := utils.ShellQuote(clean)
	checks = append(checks, "( if test -e "+quotedRoot+" || test -L "+quotedRoot+"; then test -d "+quotedRoot+" && test ! -L "+quotedRoot+"; fi )")
	return "set -e; " + strings.Join(checks, " && ") + "; "
}

func (dh *DoctorHandler) observerConfigRootStatus(cfg observerConfig) (bool, error) {
	out, err := dh.runRead(observerConfigRootGuard(cfg.Root) + "stat -c '%a %U' " + utils.ShellQuote(cfg.Root))
	if err != nil {
		return false, err
	}
	return configRootMatches(out, cfg.User), nil
}

func configRootMatches(statOutput []byte, user string) bool {
	mode, owner := parseStatModeOwner(string(statOutput))
	return mode == observerConfigMode && owner == user
}

func (dh *DoctorHandler) fixBinaryCapability() error {
	binaryPath, err := dh.witnessBinaryPath()
	if err != nil {
		logHostFix(hostFixFailed, "binary capability", "error", err.Error())
		return err
	}
	if binaryPath == "" {
		logHostFix(hostFixSkipped, "binary capability", "reason", "witness binary is not installed yet")
		return nil
	}

	subject := fmt.Sprintf("binary capability %q on %q", binaryCapability, binaryPath)
	statOut, err := dh.runRead("stat -c '%a %U %F' " + utils.ShellQuote(binaryPath))
	if err != nil {
		logHostFix(hostFixFailed, subject, "error", err.Error())
		return fmt.Errorf("inspecting binary %q: %w", binaryPath, err)
	}
	if !binaryPathSafe(string(statOut)) {
		logHostFix(hostFixFailed, subject, "reason", "binary must be a regular root-owned file without group or world write permission")
		return fmt.Errorf("binary %q must be a regular root-owned file without group or world write permission", binaryPath)
	}

	getcap := dh.resolveTool("getcap")
	if getcap == "" {
		logHostFix(hostFixSkipped, subject, "reason", "getcap is unavailable")
		return nil
	}
	out, err := dh.runner.Run(fmt.Sprintf("%s %s", getcap, utils.ShellQuote(binaryPath)))
	if err == nil && hasBinaryCapability(string(out)) {
		logHostFix(hostFixOK, subject)
		return nil
	}

	setcap := dh.resolveTool("setcap")
	if setcap == "" {
		logHostFix(hostFixFailed, subject, "reason", "setcap is unavailable")
		return fmt.Errorf("setcap is unavailable; install libcap and rerun witness doctor")
	}
	command := fmt.Sprintf("%s %s %s", setcap, binaryCapability, utils.ShellQuote(binaryPath))
	if dh.CheckOnly {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("binary %q lacks %s: run %s", binaryPath, binaryCapability, command)
	}

	if out, err := dh.runSudo(command); err != nil {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("setting capabilities on %q: %w (%s)", binaryPath, err, strings.TrimSpace(string(out)))
	}

	out, err = dh.runner.Run(fmt.Sprintf("%s %s", getcap, utils.ShellQuote(binaryPath)))
	if err != nil || !hasBinaryCapability(string(out)) {
		logHostFix(hostFixFailed, subject, "command", command)
		return fmt.Errorf("binary %q still lacks %s after %s", binaryPath, binaryCapability, command)
	}
	logHostFix(hostFixRepaired, subject, "command", command)
	return nil
}

func (dh *DoctorHandler) witnessBinaryPath() (string, error) {
	quoted := utils.ShellQuote(installedBinaryPath)
	out, err := dh.runRead("test -e " + quoted + " && echo " + quoted + " || true")
	if err != nil {
		return "", fmt.Errorf("inspecting witness binary %q: %w", installedBinaryPath, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return "", nil
	}
	return installedBinaryPath, nil
}

func binaryPathSafe(statOutput string) bool {
	fields := strings.Fields(statOutput)
	if len(fields) < 3 {
		return false
	}
	mode, err := strconv.ParseUint(fields[0], 8, 32)
	if err != nil || mode&0o022 != 0 {
		return false
	}
	if fields[1] != "root" {
		return false
	}
	fileType := strings.Join(fields[2:], " ")
	return fileType == "regular file" || fileType == "regular empty file"
}

func (dh *DoctorHandler) resolveTool(name string) string {
	for _, candidate := range []string{"/sbin/" + name, "/usr/sbin/" + name} {
		quoted := utils.ShellQuote(candidate)
		out, err := dh.runner.Run("test -x " + quoted + " && echo " + quoted + " || true")
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return quoted
		}
	}
	return ""
}

func parseManifestPaths(output string) []string {
	paths := []string{}
	for _, path := range strings.Split(output, "\x00") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func parseObserverConfigRoot(root string) (string, bool) {
	parts := strings.Split(filepath.Clean(root), "/")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "home" || parts[3] != ".config" || parts[4] != utils.APP_NAME {
		return "", false
	}
	user, app := parts[2], parts[5]
	if !deployUnixUsernamePattern.MatchString(user) || !deployProjectNamePattern.MatchString(app) {
		return "", false
	}
	return user, true
}

func observerUserForConfigRoot(pathOwner string, manifestUser string) (string, bool) {
	manifestUser = strings.TrimSpace(manifestUser)
	if pathOwner == "" {
		return "", false
	}
	if manifestUser == "" || manifestUser == pathOwner {
		return pathOwner, true
	}
	return "", false
}

func parseObserverManifestUser(content []byte) (string, error) {
	manifest := observerManifest{}
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return "", fmt.Errorf("parsing observer manifest: %w", err)
	}
	if manifest.Metadata.User == nil {
		return "", nil
	}
	return strings.TrimSpace(*manifest.Metadata.User), nil
}

func parseStatModeOwner(output string) (string, string) {
	fields := strings.Fields(output)
	if len(fields) < 2 {
		return "", ""
	}
	return fields[0], fields[1]
}

func hasBinaryCapability(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		for _, group := range fields[1:] {
			capabilities, flags, ok := strings.Cut(group, "=")
			if !ok {
				continue
			}
			for _, capability := range strings.Split(capabilities, ",") {
				if capability == "cap_chown" {
					return flags == "ep"
				}
			}
		}
	}
	return false
}

func serviceUsers(configs []observerConfig) []string {
	seen := map[string]struct{}{}
	users := []string{}
	for _, cfg := range configs {
		if cfg.User == "" {
			continue
		}
		if _, ok := seen[cfg.User]; ok {
			continue
		}
		seen[cfg.User] = struct{}{}
		users = append(users, cfg.User)
	}
	sort.Strings(users)
	return users
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
