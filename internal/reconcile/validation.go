package reconcile

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

	return runnerValidation{
		configRoot:   configRoot,
		manifestPath: manifestPath,
		agePath:      agePath,
	}, nil
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

	return validatedManifestSource{
		repoURL:        repoURL,
		targetRevision: targetRevision,
		path:           manifest.Spec.Source.Path,
		destination:    filepath.Clean(destination),
	}, nil
}
