package utils

import (
	"fmt"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestLoadSSHConfigMissingFileReturnsEmptyConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sshConfig = nil
	sshConfigOnce = sync.Once{}

	config := loadSSHConfig()
	if config == nil {
		t.Fatal("expected non-nil ssh config")
	}

	sshConfig = nil
	sshConfigOnce = sync.Once{}
}

func TestSplitSSHHostPort(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		wantHost string
		wantPort uint
		wantErr  bool
	}{
		{name: "host only", host: "127.0.0.1", wantHost: "127.0.0.1", wantPort: 0},
		{name: "host and port", host: "127.0.0.1:2222", wantHost: "127.0.0.1", wantPort: 2222},
		{name: "ipv6 and port", host: "[::1]:2200", wantHost: "::1", wantPort: 2200},
		{name: "invalid port", host: "127.0.0.1:not-a-port", wantErr: true},
		{name: "empty host", host: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort, err := splitSSHHostPort(tt.host)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotHost != tt.wantHost {
				t.Fatalf("expected host %q, got %q", tt.wantHost, gotHost)
			}
			if gotPort != tt.wantPort {
				t.Fatalf("expected port %d, got %d", tt.wantPort, gotPort)
			}
		})
	}
}

func TestIsPassphraseMissingError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "other error", err: fmt.Errorf("other"), want: false},
		{name: "direct passphrase error", err: &ssh.PassphraseMissingError{}, want: true},
		{name: "wrapped passphrase error", err: fmt.Errorf("wrapped: %w", &ssh.PassphraseMissingError{}), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPassphraseMissingError(tt.err); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
