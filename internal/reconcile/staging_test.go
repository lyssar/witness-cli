package reconcile

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/lyssar/skuld-cli/internal/application"
	"github.com/lyssar/skuld-cli/internal/decryptor"
)

func TestBuildAppStaging(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(source, []byte("services: {}"), 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}

	staging, cleanup, err := buildAppStaging(context.Background(), filepath.Join(root, "age.key"), nil, application.DiscoveredApplication{}, application.FileSet{ManagedFiles: []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: source}}})
	if err != nil {
		t.Fatalf("build staging: %v", err)
	}
	defer func() {
		_ = cleanup()
	}()

	staged := filepath.Join(staging.Root, "compose.yaml")
	content, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("read staged: %v", err)
	}
	if string(content) != "services: {}" {
		t.Fatalf("unexpected staged content: %q", string(content))
	}

	stagedInfo, err := os.Stat(staged)
	if err != nil {
		t.Fatalf("stat staged: %v", err)
	}
	if stagedInfo.Mode().Perm() != 0o640 {
		t.Fatalf("expected staged mode 0640, got %o", stagedInfo.Mode().Perm())
	}
}

func TestBuildAppStagingDecryptsSecrets(t *testing.T) {
	t.Parallel()

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	root := t.TempDir()
	keyPath := filepath.Join(root, "age.key")
	if err := os.WriteFile(keyPath, []byte(identity.String()+"\n"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	source := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(source, []byte("services: {}"), 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}

	var cipher bytes.Buffer
	writer, err := age.Encrypt(&cipher, identity.Recipient())
	if err != nil {
		t.Fatalf("encrypt writer: %v", err)
	}
	if _, err := writer.Write([]byte("TOKEN=secret\n")); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	secretSource := filepath.Join(root, "secret.age")
	if err := os.WriteFile(secretSource, []byte(base64.StdEncoding.EncodeToString(cipher.Bytes())), 0o600); err != nil {
		t.Fatalf("write secret source: %v", err)
	}

	app := application.DiscoveredApplication{
		OperationalID: "apps/hello",
		SourceDir:     root,
		Application: application.Application{Spec: application.Spec{Secrets: []application.Secret{{
			Source:    "secret.age",
			Target:    "secrets/.env",
			Decryptor: application.DecryptorAge,
		}}}},
	}

	staging, cleanup, err := buildAppStaging(context.Background(), keyPath, map[string]decryptor.Decryptor{application.DecryptorAge: decryptor.Age{}}, app, application.FileSet{ManagedFiles: []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: source}}})
	if err != nil {
		t.Fatalf("build staging: %v", err)
	}
	defer func() { _ = cleanup() }()

	secretTarget := filepath.Join(staging.Root, "secrets", ".env")
	content, err := os.ReadFile(secretTarget)
	if err != nil {
		t.Fatalf("read decrypted secret: %v", err)
	}
	if string(content) != "TOKEN=secret\n" {
		t.Fatalf("unexpected secret content: %q", string(content))
	}
}

func TestBuildAppStagingRejectsUnknownDecryptor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(source, []byte("services: {}"), 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}
	secretSource := filepath.Join(root, "secret.age")
	if err := os.WriteFile(secretSource, []byte("unused"), 0o600); err != nil {
		t.Fatalf("write secret source: %v", err)
	}

	app := application.DiscoveredApplication{
		OperationalID: "apps/hello",
		SourceDir:     root,
		Application: application.Application{Spec: application.Spec{Secrets: []application.Secret{{
			Source:    "secret.age",
			Target:    "secrets/.env",
			Decryptor: application.DecryptorAge,
		}}}},
	}

	_, cleanup, err := buildAppStaging(context.Background(), filepath.Join(root, "age.key"), map[string]decryptor.Decryptor{}, app, application.FileSet{ManagedFiles: []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: source}}})
	if cleanup != nil {
		defer func() { _ = cleanup() }()
	}
	if err == nil {
		t.Fatal("expected missing decryptor error")
	}
	if got := err.Error(); !bytes.Contains([]byte(got), []byte("decryptor \"age\" is required")) {
		t.Fatalf("unexpected error: %v", err)
	}
}
