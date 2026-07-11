package reconcile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
)

type runnerValidation struct {
	configRoot   string
	manifestPath string
	agePath      string
}

type validatedManifestSource struct {
	repoURL        string
	targetRevision string
	path           string
	destination    string
	sshKey         string
}

func (r *Runner) validateRun(ctx context.Context) (runnerValidation, error) {
	if ctx == nil {
		return runnerValidation{}, errors.New("context is required")
	}

	configRoot := strings.TrimSpace(r.configRoot)
	if configRoot == "" {
		return runnerValidation{}, errors.New("config root is required")
	}

	manifestPath := filepath.Join(configRoot, "manifest.yaml")
	if !fileExists(manifestPath) {
		return runnerValidation{}, fmt.Errorf("manifest file could not be found in config root (%s)", manifestPath)
	}

	agePath := filepath.Join(configRoot, "age.key")
	if !fileExists(agePath) {
		return runnerValidation{}, fmt.Errorf("age file could not be found in config root (%s)", agePath)
	}
	if err := validateAgeKeyPath(agePath); err != nil {
		return runnerValidation{}, err
	}

	return runnerValidation{
		configRoot:   configRoot,
		manifestPath: manifestPath,
		agePath:      agePath,
	}, nil
}

func validateAgeKeyPath(agePath string) error {
	info, err := os.Lstat(agePath)
	if err != nil {
		return fmt.Errorf("inspect age file in config root (%s): %w", agePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("age file must not be a symlink (%s)", agePath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("age file must be a regular file (%s)", agePath)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("age file permissions must not grant group/other access (%s)", agePath)
	}
	keyFile, err := os.Open(agePath)
	if err != nil {
		return fmt.Errorf("open age file for validation (%s): %w", agePath, err)
	}
	defer func() {
		_ = keyFile.Close()
	}()
	identities, err := age.ParseIdentities(keyFile)
	if err != nil {
		return fmt.Errorf("age file must contain valid age identities (%s): %w", agePath, err)
	}
	if len(identities) == 0 {
		return fmt.Errorf("age file must contain at least one age identity (%s)", agePath)
	}
	return nil
}

func validateObserverManifest(manifest observerManifest) (validatedManifestSource, error) {
	repoURL := strings.TrimSpace(manifest.Spec.Source.RepoURL)
	if repoURL == "" {
		return validatedManifestSource{}, errors.New("manifest spec.source.repoURL is required")
	}
	if strings.HasPrefix(repoURL, "-") {
		return validatedManifestSource{}, fmt.Errorf("manifest spec.source.repoURL must not start with '-' (got %q)", sanitizeGitSourceForLog(repoURL))
	}

	targetRevision := strings.TrimSpace(manifest.Spec.Source.TargetRevision)
	if targetRevision == "" {
		return validatedManifestSource{}, errors.New("manifest spec.source.targetRevision is required")
	}

	destination := strings.TrimSpace(manifest.Spec.Destination)
	if destination == "" {
		return validatedManifestSource{}, errors.New("manifest spec.destination is required")
	}
	if !filepath.IsAbs(destination) {
		return validatedManifestSource{}, errors.New("manifest spec.destination must be absolute")
	}

	sshKey := strings.TrimSpace(manifest.Spec.Source.SSHKey)
	if sshKey == "" {
		return validatedManifestSource{}, errors.New("manifest spec.source.sshKey is required")
	}

	return validatedManifestSource{
		repoURL:        repoURL,
		targetRevision: targetRevision,
		path:           manifest.Spec.Source.Path,
		destination:    filepath.Clean(destination),
		sshKey:         sshKey,
	}, nil
}
