package internal

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    OSInfo
	}{
		{
			name:    "debian",
			content: "PRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\nID=debian\nVERSION_ID=\"12\"\n",
			want:    OSInfo{ID: "debian", Version: "12", Family: OSFamilyDebian},
		},
		{
			name:    "ubuntu with id_like",
			content: "ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"24.04\"\n",
			want:    OSInfo{ID: "ubuntu", IDLike: "debian", Version: "24.04", Family: OSFamilyDebian},
		},
		{
			name:    "ubuntu with codename",
			content: "ID=ubuntu\nVERSION_CODENAME=noble\n",
			want:    OSInfo{ID: "ubuntu", VersionCodename: "noble", Family: OSFamilyDebian},
		},
		{
			name:    "rocky with quoted id",
			content: "ID=\"rocky\"\nID_LIKE=\"rhel fedora\"\nVERSION_ID=\"9.4\"\n",
			want:    OSInfo{ID: "rocky", IDLike: "rhel fedora", Version: "9.4", Family: OSFamilyRHEL},
		},
		{
			name:    "almalinux",
			content: "ID=\"almalinux\"\nID_LIKE=\"rhel centos fedora\"\n",
			want:    OSInfo{ID: "almalinux", IDLike: "rhel centos fedora", Family: OSFamilyRHEL},
		},
		{
			name:    "unsupported alpine",
			content: "ID=alpine\n",
			want:    OSInfo{ID: "alpine", Family: ""},
		},
		{
			name:    "comments and blank lines",
			content: "# comment\n\nID=ubuntu\n",
			want:    OSInfo{ID: "ubuntu", Family: OSFamilyDebian},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOSRelease(tt.content)
			if got != tt.want {
				t.Fatalf("expected %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestDoctorToolInstallCommands(t *testing.T) {
	ubuntu := OSInfo{ID: "ubuntu", VersionCodename: "noble", Family: OSFamilyDebian}
	debian := OSInfo{ID: "debian", VersionCodename: "bookworm", Family: OSFamilyDebian}
	rhel := OSInfo{ID: "rhel", Family: OSFamilyRHEL}
	rocky := OSInfo{ID: "rocky", Family: OSFamilyRHEL}
	noCodename := OSInfo{ID: "ubuntu", Family: OSFamilyDebian}

	aptDocker := "install -m 0755 -d /etc/apt/keyrings && curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc && chmod a+r /etc/apt/keyrings/docker.asc && echo \"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu noble stable\" > /etc/apt/sources.list.d/docker.list && apt-get update && apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"
	debianDocker := "install -m 0755 -d /etc/apt/keyrings && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc && chmod a+r /etc/apt/keyrings/docker.asc && echo \"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian bookworm stable\" > /etc/apt/sources.list.d/docker.list && apt-get update && apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"
	rhelDocker := "dnf install -y dnf-plugins-core && dnf config-manager --add-repo https://download.docker.com/linux/rhel/docker-ce.repo && dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"
	rockyDocker := "dnf install -y dnf-plugins-core && dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo && dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"

	tests := []struct {
		name   string
		tool   string
		pkgMgr string
		osInfo OSInfo
		want   string
	}{
		{name: "git apt", tool: "git", pkgMgr: "apt-get", want: "apt-get install -y git"},
		{name: "git dnf", tool: "git", pkgMgr: "dnf", want: "dnf install -y git"},
		{name: "age apt", tool: "age", pkgMgr: "apt-get", want: "apt-get install -y age"},
		{name: "libcap apt", tool: "libcap (setcap)", pkgMgr: "apt-get", want: "apt-get install -y libcap2-bin"},
		{name: "libcap dnf", tool: "libcap (setcap)", pkgMgr: "dnf", want: "dnf install -y libcap"},
		{name: "docker ubuntu", tool: "docker", pkgMgr: "apt-get", osInfo: ubuntu, want: aptDocker},
		{name: "docker debian", tool: "docker", pkgMgr: "apt-get", osInfo: debian, want: debianDocker},
		{name: "docker rhel", tool: "docker", pkgMgr: "dnf", osInfo: rhel, want: rhelDocker},
		{name: "docker rocky", tool: "docker", pkgMgr: "dnf", osInfo: rocky, want: rockyDocker},
		{name: "docker missing codename", tool: "docker", pkgMgr: "apt-get", osInfo: noCodename, want: "echo 'docker install requires VERSION_CODENAME in /etc/os-release' >&2 && exit 1"},
		{name: "compose ubuntu", tool: "docker compose", pkgMgr: "apt-get", osInfo: ubuntu, want: aptDocker},
		{name: "compose rocky", tool: "docker compose", pkgMgr: "dnf", osInfo: rocky, want: rockyDocker},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := doctorToolByName(tt.tool)
			if !ok {
				t.Fatalf("tool %q not found", tt.tool)
			}
			if got := tool.Install(tt.pkgMgr, tt.osInfo); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestResolvePkgMgr(t *testing.T) {
	tests := []struct {
		name    string
		osInfo  OSInfo
		out     string
		err     error
		want    string
		wantErr bool
	}{
		{name: "debian hardcoded", osInfo: OSInfo{Family: OSFamilyDebian}, want: "apt-get"},
		{name: "dnf full path", osInfo: OSInfo{Family: OSFamilyRHEL}, out: "/usr/bin/dnf\n", want: "dnf"},
		{name: "yum full path", osInfo: OSInfo{Family: OSFamilyRHEL}, out: "/usr/sbin/yum\n", want: "yum"},
		{name: "reject unknown", osInfo: OSInfo{Family: OSFamilyRHEL}, out: "/usr/bin/zypper\n", wantErr: true},
		{name: "reject empty", osInfo: OSInfo{Family: OSFamilyRHEL}, out: "", wantErr: true},
		{name: "reject command error", osInfo: OSInfo{Family: OSFamilyRHEL}, err: errors.New("not found"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dh := &DoctorHandler{runner: fakeRunner{out: tt.out, err: tt.err}}
			got, err := dh.resolvePkgMgr(tt.osInfo)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

type fakeRunner struct {
	out string
	err error
}

func (f fakeRunner) Run(command string) ([]byte, error) {
	return []byte(f.out), f.err
}

func doctorToolByName(name string) (Tool, bool) {
	for _, tool := range doctorTools {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

func TestParseManifestPaths(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{name: "empty", output: "", want: []string{}},
		{name: "single nul terminated", output: "/home/witness/.config/witness/app/manifest.yaml\x00", want: []string{"/home/witness/.config/witness/app/manifest.yaml"}},
		{name: "multiple nul separated", output: "/home/a/.config/witness/x/manifest.yaml\x00/home/b/.config/witness/y/manifest.yaml\x00", want: []string{"/home/a/.config/witness/x/manifest.yaml", "/home/b/.config/witness/y/manifest.yaml"}},
		{name: "newline stays inside one record", output: "/home/a/.config/witness/x/manifest.yaml\n/home/b/.config/witness/y/manifest.yaml\x00", want: []string{"/home/a/.config/witness/x/manifest.yaml\n/home/b/.config/witness/y/manifest.yaml"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseManifestPaths(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("expected %v, got %v", tt.want, got)
				}
			}
		})
	}
}

func TestParseObserverConfigRoot(t *testing.T) {
	tests := []struct {
		name     string
		root     string
		wantUser string
		wantOK   bool
	}{
		{name: "canonical", root: "/home/witness/.config/witness/app", wantUser: "witness", wantOK: true},
		{name: "trailing slash", root: "/home/deploy/.config/witness/app/", wantUser: "deploy", wantOK: true},
		{name: "newline injection", root: "/home/witness/.config/witness/app\n/home/root/.config/witness/app", wantOK: false},
		{name: "nested project", root: "/home/witness/.config/witness/app/sub", wantOK: false},
		{name: "wrong app name", root: "/home/witness/.config/other/app", wantOK: false},
		{name: "outside home", root: "/srv/witness/.config/witness/app", wantOK: false},
		{name: "invalid username", root: "/home/Witness/.config/witness/app", wantOK: false},
		{name: "invalid app name", root: "/home/witness/.config/witness/App", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, ok := parseObserverConfigRoot(tt.root)
			if ok != tt.wantOK || user != tt.wantUser {
				t.Fatalf("expected user %q ok %t, got user %q ok %t", tt.wantUser, tt.wantOK, user, ok)
			}
		})
	}
}

func TestParseObserverManifestUser(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{name: "explicit user", content: "metadata:\n  user: deploy\n", want: "deploy"},
		{name: "quoted user", content: "metadata:\n  name: obs\n  user: \"ops\"\n", want: "ops"},
		{name: "missing user stays empty", content: "metadata:\n  name: obs\n", want: ""},
		{name: "empty metadata stays empty", content: "kind: Observer\n", want: ""},
		{name: "trims explicit user", content: "metadata:\n  user: \" deploy \"\n", want: "deploy"},
		{name: "rejects invalid yaml", content: "metadata: [", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseObserverManifestUser([]byte(tt.content))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestObserverUserForConfigRoot(t *testing.T) {
	tests := []struct {
		name         string
		pathOwner    string
		manifestUser string
		want         string
		wantOK       bool
	}{
		{name: "manifest user matches owner", pathOwner: "deploy", manifestUser: "deploy", want: "deploy", wantOK: true},
		{name: "manifest user mismatch", pathOwner: "deploy", manifestUser: "ops", wantOK: false},
		{name: "missing manifest user uses owner", pathOwner: "deploy", manifestUser: "", want: "deploy", wantOK: true},
		{name: "blank manifest user uses owner", pathOwner: "deploy", manifestUser: "  ", want: "deploy", wantOK: true},
		{name: "no path owner", pathOwner: "", manifestUser: "deploy", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := observerUserForConfigRoot(tt.pathOwner, tt.manifestUser)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("expected user %q ok %t, got user %q ok %t", tt.want, tt.wantOK, got, ok)
			}
		})
	}
}

func TestParseStatModeOwner(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantMode  string
		wantOwner string
	}{
		{name: "canonical", output: "700 witness\n", wantMode: "700", wantOwner: "witness"},
		{name: "whitespace", output: " 755\troot \n", wantMode: "755", wantOwner: "root"},
		{name: "empty", output: "", wantMode: "", wantOwner: ""},
		{name: "owner missing", output: "700", wantMode: "", wantOwner: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, owner := parseStatModeOwner(tt.output)
			if mode != tt.wantMode || owner != tt.wantOwner {
				t.Fatalf("expected mode %q owner %q, got mode %q owner %q", tt.wantMode, tt.wantOwner, mode, owner)
			}
		})
	}
}

func TestHasBinaryCapability(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "cap_chown ep", output: "/usr/local/bin/witness cap_chown=ep\n", want: true},
		{name: "cap_chown among others", output: "/usr/local/bin/witness cap_chown,cap_net_bind_service=ep\n", want: true},
		{name: "cap_chown group with separate flags", output: "/usr/local/bin/witness cap_chown=ep cap_net_bind_service=p\n", want: true},
		{name: "permitted only", output: "/usr/local/bin/witness cap_chown=p\n", want: false},
		{name: "inheritable only", output: "/usr/local/bin/witness cap_chown=i\n", want: false},
		{name: "effective and inheritable", output: "/usr/local/bin/witness cap_chown=eip\n", want: false},
		{name: "other capability", output: "/usr/local/bin/witness cap_dac_override=ep\n", want: false},
		{name: "no output", output: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasBinaryCapability(tt.output); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestServiceUsers(t *testing.T) {
	tests := []struct {
		name    string
		configs []observerConfig
		want    []string
	}{
		{name: "none", configs: nil, want: []string{}},
		{name: "dedupe and sort", configs: []observerConfig{{User: "witness"}, {User: "deploy"}, {User: "witness"}}, want: []string{"deploy", "witness"}},
		{name: "skips empty", configs: []observerConfig{{User: ""}, {User: "witness"}}, want: []string{"witness"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serviceUsers(tt.configs)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("expected %v, got %v", tt.want, got)
				}
			}
		})
	}
}

func TestConfigRootMatches(t *testing.T) {
	tests := []struct {
		name string
		out  string
		user string
		want bool
	}{
		{name: "matching", out: "700 witness\n", user: "witness", want: true},
		{name: "wrong mode", out: "755 witness\n", user: "witness", want: false},
		{name: "wrong owner", out: "700 root\n", user: "witness", want: false},
		{name: "empty output", out: "", user: "witness", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configRootMatches([]byte(tt.out), tt.user); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestObserverConfigRootGuard(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "home", "deploy", ".config", "witness", "app")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("creating test tree: %v", err)
	}

	runGuard := func() error {
		return exec.Command("bash", "-c", observerConfigRootGuard(root)).Run()
	}

	if err := runGuard(); err != nil {
		t.Fatalf("expected guard to accept real tree: %v", err)
	}

	configDir := filepath.Join(base, "home", "deploy", ".config")
	if err := os.RemoveAll(configDir); err != nil {
		t.Fatalf("removing config dir: %v", err)
	}
	target := filepath.Join(base, "elsewhere")
	if err := os.MkdirAll(filepath.Join(target, "witness", "app"), 0o700); err != nil {
		t.Fatalf("creating symlink target: %v", err)
	}
	if err := os.Symlink(target, configDir); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}
	if err := runGuard(); err == nil {
		t.Fatal("expected guard to reject symlinked config ancestor")
	}

	if err := os.Remove(configDir); err != nil {
		t.Fatalf("removing config symlink: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
		t.Fatalf("recreating config dir: %v", err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("removing config root: %v", err)
	}
	if err := runGuard(); err != nil {
		t.Fatalf("expected guard to allow missing config root: %v", err)
	}

	if err := os.Symlink(target, root); err != nil {
		t.Fatalf("creating root symlink: %v", err)
	}
	if err := runGuard(); err == nil {
		t.Fatal("expected guard to reject symlinked config root")
	}
}

func TestBinaryPathSafe(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "root owned 755", output: "755 root regular file\n", want: true},
		{name: "root owned 700 empty", output: "700 root regular empty file\n", want: true},
		{name: "group writable", output: "775 root regular file\n", want: false},
		{name: "world writable", output: "757 root regular file\n", want: false},
		{name: "not root owned", output: "755 witness regular file\n", want: false},
		{name: "directory", output: "755 root directory\n", want: false},
		{name: "symlink", output: "777 root symbolic link\n", want: false},
		{name: "empty", output: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := binaryPathSafe(tt.output); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestInstalledBinaryPathIsAbsolute(t *testing.T) {
	if !filepath.IsAbs(installedBinaryPath) {
		t.Fatalf("expected absolute installed binary path, got %q", installedBinaryPath)
	}
}
