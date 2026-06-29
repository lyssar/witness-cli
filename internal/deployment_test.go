package internal

import "testing"

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
