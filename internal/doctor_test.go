package internal

import (
	"errors"
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
