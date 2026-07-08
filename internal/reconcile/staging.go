package reconcile

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/internal/decryptor"
)

// AppStaging holds one app staging tree.
type AppStaging struct {
	Root string
}

func buildAppStaging(ctx context.Context, ageKeyPath string, decryptors map[string]decryptor.Decryptor, app application.DiscoveredApplication, fileset application.FileSet) (AppStaging, func() error, error) {
	root, err := os.MkdirTemp("", "witness-stage-*")
	if err != nil {
		return AppStaging{}, nil, fmt.Errorf("creating staging root: %w", err)
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

	return AppStaging{Root: root}, cleanup, nil
}

func stageSecrets(ctx context.Context, ageKeyPath string, decryptors map[string]decryptor.Decryptor, root string, app application.DiscoveredApplication) error {
	for _, secret := range app.Application.Spec.Secrets {
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
		}); err != nil {
			return fmt.Errorf("decrypting secret %q to %q: %w", secret.Source, secret.Target, err)
		}
	}

	return nil
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
