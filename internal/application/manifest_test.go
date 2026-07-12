package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		content        string
		expectErr      bool
		expectContains string
	}{
		{
			name: "valid minimal manifest",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
`,
		},
		{
			name: "unknown field rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
  unknownField: true
`,
			expectErr:      true,
			expectContains: "field unknownField not found",
		},
		{
			name: "wrong apiVersion rejected",
			content: `apiVersion: witness.dev/v1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
`,
			expectErr:      true,
			expectContains: "apiVersion must be",
		},
		{
			name: "wrong kind rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: App
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
`,
			expectErr:      true,
			expectContains: "kind must be",
		},
		{
			name: "missing metadata name rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata: {}
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
`,
			expectErr:      true,
			expectContains: "metadata.name is required",
		},
		{
			name: "missing compose files rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
`,
			expectErr:      true,
			expectContains: "spec.composeFiles must contain at least one entry",
		},
		{
			name: "duplicate compose files rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
    - ./docker-compose.yaml
`,
			expectErr:      true,
			expectContains: "duplicate path",
		},
		{
			name: "missing declared compose file rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.missing.yaml
`,
			expectErr:      true,
			expectContains: "does not exist",
		},
		{
			name: "duplicate secret source rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
  secrets:
    - source: secret.env.age
      target: .env
      decryptor: age
    - source: ./secret.env.age
      target: .env.local
      decryptor: age
`,
			expectErr:      true,
			expectContains: "duplicate secret source",
		},
		{
			name: "duplicate secret target rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
  secrets:
    - source: a.env.age
      target: .env
      decryptor: age
    - source: b.env.age
      target: ./.env
      decryptor: age
`,
			expectErr:      true,
			expectContains: "duplicate secret target",
		},
		{
			name: "secret target collides with compose file rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
  secrets:
    - source: secret.env.age
      target: docker-compose.yaml
      decryptor: age
`,
			expectErr:      true,
			expectContains: "secret target collides with compose file",
		},
		{
			name: "secret target collides with secret source rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
  secrets:
    - source: a.env.age
      target: .env
      decryptor: age
    - source: b.env.age
      target: a.env.age
      decryptor: age
`,
			expectErr:      true,
			expectContains: "secret target collides with secret source",
		},
		{
			name: "secret target collides with later secret source rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
  secrets:
    - source: first.env.age
      target: a.env.age
      decryptor: age
    - source: a.env.age
      target: .env
      decryptor: age
`,
			expectErr:      true,
			expectContains: "secret target collides with secret source",
		},
		{
			name: "unsupported provisioner rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: shell
  composeFiles:
    - docker-compose.yaml
`,
			expectErr:      true,
			expectContains: "spec.provisioner must be",
		},
		{
			name: "unsupported decryptor rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
  secrets:
    - source: secret.env.enc
      target: .env
      decryptor: sops
`,
			expectErr:      true,
			expectContains: "decryptor must be",
		},
		{
			name: "unsafe compose path rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - ../docker-compose.yaml
`,
			expectErr:      true,
			expectContains: "escapes root",
		},
		{
			name: "multiple yaml documents rejected",
			content: `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
---
apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: another
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
`,
			expectErr:      true,
			expectContains: "multiple YAML documents are not allowed",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			manifestPath := filepath.Join(dir, ManifestFileName)
			if err := os.WriteFile(filepath.Join(dir, "docker-compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
				t.Fatalf("write compose file: %v", err)
			}
			if err := os.WriteFile(manifestPath, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			_, err := ParseManifest(manifestPath)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tc.expectContains != "" && !strings.Contains(err.Error(), tc.expectContains) {
					t.Fatalf("expected error containing %q, got %q", tc.expectContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
		})
	}
}

func TestParseManifestSecretModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mode      string
		want      os.FileMode
		wantError bool
	}{
		{name: "missing defaults to 0600", want: 0o600},
		{name: "empty double quoted mode rejected", mode: `mode: ""`, wantError: true},
		{name: "empty single quoted mode rejected", mode: "mode: ''", wantError: true},
		{name: "quoted container readable mode", mode: `mode: "0444"`, want: 0o444},
		{name: "unquoted mode rejected", mode: "mode: 0600", wantError: true},
		{name: "non canonical mode rejected", mode: `mode: "600"`, wantError: true},
		{name: "non octal mode rejected", mode: `mode: "0680"`, wantError: true},
		{name: "special bits rejected", mode: `mode: "1600"`, wantError: true},
		{name: "group write rejected", mode: `mode: "0660"`, wantError: true},
		{name: "other execute rejected", mode: `mode: "0601"`, wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
				t.Fatalf("write compose file: %v", err)
			}
			manifest := `apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: app
spec:
  provisioner: docker-compose
  composeFiles:
    - compose.yaml
  secrets:
    - source: secret.age
      target: secret
      decryptor: age
`
			if tc.mode != "" {
				manifest += "      " + tc.mode + "\n"
			}
			manifestPath := filepath.Join(dir, ManifestFileName)
			if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			app, err := ParseManifest(manifestPath)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected parse error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parse manifest: %v", err)
			}
			got, err := app.Spec.Secrets[0].ResolvedMode()
			if err != nil {
				t.Fatalf("resolve mode: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected resolved mode %04o, got %04o", tc.want, got)
			}
		})
	}
}
