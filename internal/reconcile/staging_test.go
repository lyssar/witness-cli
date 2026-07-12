package reconcile

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/internal/decryptor"
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

	// Staging root must be 0700.
	rootInfo, err := os.Stat(staging.Root)
	if err != nil {
		t.Fatalf("stat staging root: %v", err)
	}
	if got := rootInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected staging root mode 0700, got %o", got)
	}

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
			Mode:      application.NewSecretMode("0444"),
		}}}},
	}

	staging, cleanup, err := buildAppStaging(context.Background(), keyPath, map[string]decryptor.Decryptor{application.DecryptorAge: decryptor.Age{}}, app, application.FileSet{ManagedFiles: []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: source}}})
	if err != nil {
		t.Fatalf("build staging: %v", err)
	}
	defer func() { _ = cleanup() }()

	// Staging root must be 0700.
	rootInfo, err := os.Stat(staging.Root)
	if err != nil {
		t.Fatalf("stat staging root: %v", err)
	}
	if got := rootInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected staging root mode 0700, got %o", got)
	}

	secretTarget := filepath.Join(staging.Root, "secrets", ".env")
	content, err := os.ReadFile(secretTarget)
	if err != nil {
		t.Fatalf("read decrypted secret: %v", err)
	}
	if string(content) != "TOKEN=secret\n" {
		t.Fatalf("unexpected secret content: %q", string(content))
	}

	// Decrypted secret files must retain their resolved manifest mode.
	secretInfo, err := os.Stat(secretTarget)
	if err != nil {
		t.Fatalf("stat secret: %v", err)
	}
	if got := secretInfo.Mode().Perm(); got != 0o444 {
		t.Fatalf("expected secret mode 0444, got %o", got)
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

func TestStageRegistryPasswordFailsClosedWhenEncryptedSourceRemovalFails(t *testing.T) {
	root := t.TempDir()
	app := application.DiscoveredApplication{
		OperationalID: "apps/hello",
		Application: application.Application{Spec: application.Spec{RegistryCredentials: &application.RegistryCredentials{
			Password: "encrypted-password",
		}}},
	}
	decrypt := fakeDecryptor{content: "plaintext-password"}
	removeErr := errors.New("remove encrypted source")

	_, err := stageRegistryPasswordWithRemove(context.Background(), "age.key", map[string]decryptor.Decryptor{application.DecryptorAge: decrypt}, root, app, func(path string) error {
		if filepath.Base(path) == ".registry-password-encrypted" {
			return removeErr
		}
		return os.Remove(path)
	})
	if !errors.Is(err, removeErr) {
		t.Fatalf("stage registry password error = %v, want encrypted-source removal error", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".registry-password")); !os.IsNotExist(statErr) {
		t.Fatalf("decrypted registry password remained after failed encrypted cleanup: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".registry-password-encrypted")); !os.IsNotExist(statErr) {
		t.Fatalf("encrypted registry password remained after failed cleanup: %v", statErr)
	}
}

func TestPromoteStagingToLivePreservesSecretModeAndHardensAppRoot(t *testing.T) {
	t.Parallel()

	stagingRoot := t.TempDir()
	secretPath := filepath.Join(stagingRoot, "secrets", "token")
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o700); err != nil {
		t.Fatalf("create secret parent: %v", err)
	}
	if err := os.WriteFile(secretPath, []byte("secret"), 0o444); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.Chmod(secretPath, 0o444); err != nil {
		t.Fatalf("set secret mode: %v", err)
	}
	if err := os.Chmod(stagingRoot, 0o755); err != nil {
		t.Fatalf("relax staging root for promotion assertion: %v", err)
	}

	liveDir, err := promoteStagingToLive(filepath.Join(t.TempDir(), "apps"), application.DiscoveredApplication{OperationalID: "hello"}, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("promote staging: %v", err)
	}

	liveInfo, err := os.Stat(liveDir)
	if err != nil {
		t.Fatalf("stat live app root: %v", err)
	}
	if got := liveInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected live app root mode 0700, got %o", got)
	}
	secretInfo, err := os.Stat(filepath.Join(liveDir, "secrets", "token"))
	if err != nil {
		t.Fatalf("stat promoted secret: %v", err)
	}
	if got := secretInfo.Mode().Perm(); got != 0o444 {
		t.Fatalf("expected promoted secret mode 0444, got %o", got)
	}
}
