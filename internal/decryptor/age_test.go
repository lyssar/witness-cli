package decryptor

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestAgeDecryptFile(t *testing.T) {
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

	var cipher bytes.Buffer
	writer, err := age.Encrypt(&cipher, identity.Recipient())
	if err != nil {
		t.Fatalf("encrypt writer: %v", err)
	}
	if _, err := io.WriteString(writer, "hello secret\n"); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	sourcePath := filepath.Join(root, "secret.age")
	if err := os.WriteFile(sourcePath, []byte(base64.StdEncoding.EncodeToString(cipher.Bytes())), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	targetPath := filepath.Join(root, "runtime", ".env")
	if err := (Age{}).DecryptFile(context.Background(), Request{
		OperationalID: "apps/hello",
		KeyPath:       keyPath,
		SourcePath:    sourcePath,
		TargetPath:    targetPath,
	}); err != nil {
		t.Fatalf("decrypt file: %v", err)
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "hello secret\n" {
		t.Fatalf("unexpected plaintext: %q", string(content))
	}
}
