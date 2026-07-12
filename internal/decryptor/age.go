package decryptor

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
)

// Age decrypts base64-encoded age ciphertext files into runtime targets.
type Age struct{}

// Name returns the manifest-facing decryptor identifier.
func (Age) Name() string {
	return "age"
}

// DecryptFile decrypts one base64-encoded age file into the requested target path.
func (Age) DecryptFile(ctx context.Context, request Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(request.KeyPath) == "" {
		return fmt.Errorf("decrypt %q: key path is required", request.OperationalID)
	}

	identities, err := loadIdentities(request.KeyPath)
	if err != nil {
		return fmt.Errorf("decrypt %q: %w", request.OperationalID, err)
	}

	encodedCiphertext, err := os.ReadFile(request.SourcePath)
	if err != nil {
		return fmt.Errorf("decrypt %q source %q: %w", request.OperationalID, request.SourcePath, err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedCiphertext)))
	if err != nil {
		return fmt.Errorf("decrypt %q source %q: base64 decode: %w", request.OperationalID, request.SourcePath, err)
	}

	reader, err := age.Decrypt(bytes.NewReader(ciphertext), identities...)
	if err != nil {
		return fmt.Errorf("decrypt %q source %q: age decrypt: %w", request.OperationalID, request.SourcePath, err)
	}

	if err := os.MkdirAll(filepath.Dir(request.TargetPath), 0o700); err != nil {
		return fmt.Errorf("decrypt %q target %q: create parent: %w", request.OperationalID, request.TargetPath, err)
	}

	targetFile, err := os.OpenFile(request.TargetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, request.Mode)
	if err != nil {
		return fmt.Errorf("decrypt %q target %q: open target: %w", request.OperationalID, request.TargetPath, err)
	}
	defer func() {
		_ = targetFile.Close()
	}()

	if _, err := io.Copy(targetFile, reader); err != nil {
		return fmt.Errorf("decrypt %q target %q: write plaintext: %w", request.OperationalID, request.TargetPath, err)
	}
	if err := targetFile.Chmod(request.Mode); err != nil {
		return fmt.Errorf("decrypt %q target %q: set mode: %w", request.OperationalID, request.TargetPath, err)
	}

	return nil
}

func loadIdentities(keyPath string) ([]age.Identity, error) {
	keyFile, err := os.Open(keyPath)
	if err != nil {
		return nil, fmt.Errorf("open age key %q: %w", keyPath, err)
	}
	defer func() {
		_ = keyFile.Close()
	}()

	identities, err := age.ParseIdentities(keyFile)
	if err != nil {
		return nil, fmt.Errorf("parse age key %q: %w", keyPath, err)
	}
	if len(identities) == 0 {
		return nil, fmt.Errorf("parse age key %q: no identities found", keyPath)
	}

	return identities, nil
}
