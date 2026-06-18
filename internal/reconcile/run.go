package reconcile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/lyssar/skuld-cli/internal/application"
	"gopkg.in/yaml.v3"
)

// Runner executes the reconcile workflow from an observer config root.
type Runner struct {
	configRoot string
}

// NewRunner creates a new reconcile runner.
func NewRunner(configRoot string) *Runner {
	return &Runner{configRoot: configRoot}
}

// Run executes a reconcile pass for the configured root.
func (r *Runner) Run(ctx context.Context) error {
	validatedRun, err := r.validateRun(ctx)
	if err != nil {
		return err
	}

	manifest, err := loadManifest(validatedRun.manifestPath)
	if err != nil {
		return err
	}

	manifestSource, err := validateObserverManifest(manifest)
	if err != nil {
		return err
	}

	repoRoot := filepath.Join(validatedRun.configRoot, "repo")
	repo, err := newGitRepository(repoRoot, manifestSource.repoURL)
	if err != nil {
		return err
	}

	slog.Info(
		"Starting reconcile run",
		"configRoot", validatedRun.configRoot,
		"repoSource", sanitizeGitSourceForLog(manifestSource.repoURL),
		"targetRevision", manifestSource.targetRevision,
	)

	repoOperation, err := repo.Sync(ctx, manifestSource.targetRevision)
	if err != nil {
		return err
	}

	discoveryRoot, err := resolveDiscoveryRoot(repoRoot, manifestSource.path)
	if err != nil {
		return err
	}

	slog.Info("Repository prepared", "repoOperation", repoOperation, "discoveryRoot", discoveryRoot)

	submodulePaths, err := extractSubmodulePaths(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("extracting submodule paths: %w", err)
	}
	submodulePaths, err = submodulePathsUnderDiscoveryRoot(submodulePaths, discoveryRoot, repoRoot)
	if err != nil {
		return fmt.Errorf("normalizing submodule paths for discovery root: %w", err)
	}

	discoveredApplications, err := application.DiscoverApplications(discoveryRoot)
	if err != nil {
		return fmt.Errorf("discovering applications: %w", err)
	}

	appErrors := make([]string, 0)
	for _, app := range discoveredApplications {
		slog.Debug(
			"Discovered application",
			"operationalID", app.OperationalID,
			"runtimeSlug", app.RuntimeSlug,
			"sourceDir", app.SourceDir,
			"manifestPath", app.ManifestPath,
			"metadataName", app.Application.Metadata.Name,
		)

		fileset, err := application.BuildFileSet(app, submodulePaths)
		if err != nil {
			appErrors = append(appErrors, fmt.Sprintf("%s: %v", app.OperationalID, err))
			slog.Error("App fileset failed", "operationalID", app.OperationalID, "error", err)
			continue
		}

		staging, cleanup, err := buildAppStaging(app, fileset)
		if err != nil {
			appErrors = append(appErrors, fmt.Sprintf("%s: %v", app.OperationalID, err))
			slog.Error("App staging failed", "operationalID", app.OperationalID, "error", err)
			continue
		}

		drift, driftErr := detectDrift(manifestSource.destination, app, fileset, staging)
		cleanupErr := cleanup()
		if cleanupErr != nil {
			slog.Warn("App staging cleanup failed", "operationalID", app.OperationalID, "error", cleanupErr)
		}
		if driftErr != nil {
			appErrors = append(appErrors, fmt.Sprintf("%s: %v", app.OperationalID, driftErr))
			slog.Error("App drift check failed", "operationalID", app.OperationalID, "error", driftErr)
			continue
		}

		for _, warning := range drift.Warnings {
			slog.Warn(
				"App fileset warning",
				"operationalID", app.OperationalID,
				"code", warning.Code,
				"path", warning.Path,
				"message", warning.Message,
			)
		}

		slog.Info("App drift analyzed",
			"operationalID", app.OperationalID,
			"hasDrift", drift.HasDrift,
			"changed", len(drift.Changed),
			"missing", len(drift.Missing),
			"typeMismatch", len(drift.TypeMismatch),
			"deferredSecretTargets", len(drift.DeferredSecretTargets),
		)
	}

	slog.Info("Finished reconcile run", "discoveredApplications", len(discoveredApplications))
	if len(appErrors) > 0 {
		return errors.New("reconcile app failures (" + fmt.Sprint(len(appErrors)) + "): " + strings.Join(appErrors, "; "))
	}

	return nil
}

func submodulePathsUnderDiscoveryRoot(submodulePaths []string, discoveryRoot string, repoRoot string) ([]string, error) {
	relDiscoveryRoot, err := filepath.Rel(repoRoot, discoveryRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve discovery root relative path: %w", err)
	}
	relDiscoveryRoot = filepath.Clean(filepath.ToSlash(relDiscoveryRoot))
	if relDiscoveryRoot == "." {
		return append([]string(nil), submodulePaths...), nil
	}

	normalized := make([]string, 0, len(submodulePaths))
	prefix := relDiscoveryRoot + "/"
	for _, submodulePath := range submodulePaths {
		if submodulePath == relDiscoveryRoot {
			normalized = append(normalized, ".")
			continue
		}
		if strings.HasPrefix(submodulePath, prefix) {
			normalized = append(normalized, strings.TrimPrefix(submodulePath, prefix))
		}
	}

	return normalized, nil
}

type observerManifest struct {
	Spec observerSpec `yaml:"spec"`
}

type observerSpec struct {
	Destination string         `yaml:"destination"`
	Source      observerSource `yaml:"source"`
}

type observerSource struct {
	RepoURL        string `yaml:"repoURL"`
	TargetRevision string `yaml:"targetRevision"`
	Path           string `yaml:"path"`
}

func loadManifest(manifestPath string) (observerManifest, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return observerManifest{}, fmt.Errorf("reading observer manifest %q: %w", manifestPath, err)
	}

	var manifest observerManifest
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return observerManifest{}, fmt.Errorf("parsing observer manifest %q: %w", manifestPath, err)
	}

	return manifest, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
