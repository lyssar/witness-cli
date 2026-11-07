package internal

import (
	"errors"
	"strings"
	"testing"
)

type stubCommandRunner struct {
	outputs map[string]string
	errs    map[string]error
}

func (s stubCommandRunner) Run(command string) ([]byte, error) {
	if err, ok := s.errs[command]; ok {
		return nil, err
	}
	return []byte(s.outputs[command]), nil
}

func TestCheckAppGroup(t *testing.T) {
	const (
		groupCommand   = "getent group 'witness'"
		membersCommand = "id -nG 'witness'"
	)

	tests := []struct {
		name       string
		runner     stubCommandRunner
		wantErrMsg string
	}{
		{
			name: "deploy user is a member",
			runner: stubCommandRunner{outputs: map[string]string{
				groupCommand:   "witness:x:989:\n",
				membersCommand: "witness adm docker\n",
			}},
		},
		{
			name: "group missing",
			runner: stubCommandRunner{errs: map[string]error{
				groupCommand: errors.New("exit status 2"),
			}},
			wantErrMsg: `group "witness" is missing on target host`,
		},
		{
			name: "group lookup returns no entry",
			runner: stubCommandRunner{outputs: map[string]string{
				groupCommand: "\n",
			}},
			wantErrMsg: `group "witness" is missing on target host`,
		},
		{
			name: "deploy user is not a member",
			runner: stubCommandRunner{outputs: map[string]string{
				groupCommand:   "witness:x:989:\n",
				membersCommand: "shens adm docker\n",
			}},
			wantErrMsg: `deploy user "witness" is not a member of group "witness"`,
		},
		{
			name: "membership lookup fails",
			runner: stubCommandRunner{
				outputs: map[string]string{groupCommand: "witness:x:989:\n"},
				errs:    map[string]error{membersCommand: errors.New("exit status 1")},
			},
			wantErrMsg: `reading group membership of "witness"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkAppGroup(tt.runner, "witness")
			if tt.wantErrMsg == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErrMsg)
			}
			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErrMsg)
			}
		})
	}
}

func TestValidateAppGroupMembership(t *testing.T) {
	tests := []struct {
		name         string
		groupsOutput string
		wantErr      bool
	}{
		{name: "only member", groupsOutput: "witness\n"},
		{name: "member among groups", groupsOutput: "adm witness docker\n"},
		{name: "not a member", groupsOutput: "adm docker\n", wantErr: true},
		{name: "empty output", groupsOutput: "", wantErr: true},
		{name: "similar group name", groupsOutput: "witness-admins\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAppGroupMembership("witness", "witness", tt.groupsOutput)
			if tt.wantErr && err == nil {
				t.Fatal("expected membership error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected membership error: %v", err)
			}
		})
	}
}

func TestDeployHandlerAskForSudoerUsesEnvironment(t *testing.T) {
	t.Setenv(deploySudoPasswordEnv, "local-secret")

	handler := &DeployHandler{}
	handler.AskForSudoer()

	if handler.Sudoer != "local-secret" {
		t.Fatalf("expected sudo password from env, got %q", handler.Sudoer)
	}
}

func TestSudoPasswordPromptRequired(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{name: "password required", out: "sudo: a password is required", want: true},
		{name: "terminal required", out: "sudo: a terminal is required to read the password", want: true},
		{name: "other error", out: "sudo: command not found", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sudoPasswordPromptRequired([]byte(tt.out)); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
