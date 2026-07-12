package reconcile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/internal/decryptor"
)

// AppStaging holds one app staging tree.
type AppStaging struct {
	Root                 string
	RegistryPasswordPath string // path to decrypted registry password
}

func buildAppStaging(ctx context.Context, ageKeyPath string, decryptors map[string]decryptor.Decryptor, app application.DiscoveredApplication, fileset application.FileSet) (AppStaging, func() error, error) {
	root, err := os.MkdirTemp("", "witness-stage-*")
	if err != nil {
		return AppStaging{}, nil, fmt.Errorf("creating staging root: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return AppStaging{}, nil, fmt.Errorf("hardening staging root %q: %w", root, err)
	}

	cleanup := func() error {
		return os.RemoveAll(root)
	}

	for _, managed := range fileset.ManagedFiles {
		targetPath := filepath.Join(root, filepath.FromSlash(managed.RelativePath))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			// Best-effort cleanup: staging build already failed and cleanup failure should not mask root cause.
			_ = cleanup()
			return AppStaging{}, nil, fmt.Errorf("creating staging parent for %q: %w", managed.RelativePath, err)
		}

		if err := copyFileWithMode(managed.SourcePath, targetPath); err != nil {
			// Best-effort cleanup: staging build already failed and cleanup failure should not mask root cause.
			_ = cleanup()
			return AppStaging{}, nil, fmt.Errorf("staging %q: %w", managed.RelativePath, err)
		}
	}

	if err := stageSecrets(ctx, ageKeyPath, decryptors, root, app); err != nil {
		_ = cleanup()
		return AppStaging{}, nil, err
	}

	// Decrypt registry credentials if present
	var registryPasswordPath string
	if app.Application.Spec.RegistryCredentials != nil && app.Application.Spec.RegistryCredentials.Password != "" {
		path, err := stageRegistryPassword(ctx, ageKeyPath, decryptors, root, app)
		if err != nil {
			_ = cleanup()
			return AppStaging{}, nil, err
		}
		registryPasswordPath = path
	}

	return AppStaging{Root: root, RegistryPasswordPath: registryPasswordPath}, cleanup, nil
}

func stageSecrets(ctx context.Context, ageKeyPath string, decryptors map[string]decryptor.Decryptor, root string, app application.DiscoveredApplication) error {
	for _, secret := range app.Application.Spec.Secrets {
		mode, err := secret.ResolvedMode()
		if err != nil {
			return fmt.Errorf("resolving mode for secret %q: %w", secret.Target, err)
		}
		decrypt := decryptors[secret.Decryptor]
		if decrypt == nil {
			return fmt.Errorf("decryptor %q is required for app %q", secret.Decryptor, app.OperationalID)
		}
		sourcePath := filepath.Join(app.SourceDir, filepath.FromSlash(secret.Source))
		targetPath := filepath.Join(root, filepath.FromSlash(secret.Target))
		if err := decrypt.DecryptFile(ctx, decryptor.Request{
			OperationalID: app.OperationalID,
			KeyPath:       ageKeyPath,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			Mode:          mode,
		}); err != nil {
			return fmt.Errorf("decrypting secret %q to %q: %w", secret.Source, secret.Target, err)
		}
	}

	return nil
}

func stageRegistryPassword(ctx context.Context, ageKeyPath string, decryptors map[string]decryptor.Decryptor, root string, app application.DiscoveredApplication) (string, error) {
	return stageRegistryPasswordWithRemove(ctx, ageKeyPath, decryptors, root, app, os.Remove)
}

func stageRegistryPasswordWithRemove(ctx context.Context, ageKeyPath string, decryptors map[string]decryptor.Decryptor, root string, app application.DiscoveredApplication, removeFile func(string) error) (string, error) {
	creds := app.Application.Spec.RegistryCredentials
	if creds == nil || creds.Password == "" {
		return "", nil
	}

	decrypt := decryptors[application.DecryptorAge]
	if decrypt == nil {
		return "", fmt.Errorf("decryptor %q is required for registry credentials", application.DecryptorAge)
	}

	// Write encrypted password to temp file for decryption
	encryptedPath := filepath.Join(root, ".registry-password-encrypted")
	if err := os.WriteFile(encryptedPath, []byte(creds.Password), 0o600); err != nil {
		return "", fmt.Errorf("writing encrypted registry password: %w", err)
	}

	// Decrypt to target path
	targetPath := filepath.Join(root, ".registry-password")
	if err := decrypt.DecryptFile(ctx, decryptor.Request{
		OperationalID: app.OperationalID,
		KeyPath:       ageKeyPath,
		SourcePath:    encryptedPath,
		TargetPath:    targetPath,
		Mode:          0o600,
	}); err != nil {
		return "", fmt.Errorf("decrypting registry password: %w", err)
	}

	// The encrypted source is staging-only. Do not return a staging tree that
	// could later be promoted or archived when its removal fails.
	if err := removeFile(encryptedPath); err != nil {
		cleanupErrs := []error{fmt.Errorf("removing encrypted registry password: %w", err)}
		if cleanupErr := os.Remove(encryptedPath); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("retrying encrypted registry password cleanup: %w", cleanupErr))
		}
		if cleanupErr := os.Remove(targetPath); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("removing decrypted registry password after encrypted cleanup failure: %w", cleanupErr))
		}
		return "", errors.Join(cleanupErrs...)
	}

	return targetPath, nil
}

func copyFileWithMode(sourcePath, destinationPath string) error {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("stat source file %q: %w", sourcePath, err)
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", sourcePath, err)
	}
	defer func() {
		_ = sourceFile.Close()
	}()

	destinationFile, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, sourceInfo.Mode().Perm())
	if err != nil {
		return fmt.Errorf("open destination file %q: %w", destinationPath, err)
	}
	defer func() {
		_ = destinationFile.Close()
	}()

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return fmt.Errorf("copy file %q to %q: %w", sourcePath, destinationPath, err)
	}

	return nil
}
