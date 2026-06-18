package internal

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/lyssar/skuld-cli/internal/application"
	"github.com/lyssar/skuld-cli/utils"
)

type ReconcileRun struct {
	configRoot string
	manifest   Observer
}

func NewReconcileRun(configRoot string) *ReconcileRun {
	reconcileRun := &ReconcileRun{
		configRoot: configRoot,
	}

	return reconcileRun
}

func (rr ReconcileRun) getManifestFilePath() string {
	return fmt.Sprintf("%s/manifest.yaml", rr.configRoot)
}

func (rr ReconcileRun) getAgeFilePath() string {
	return fmt.Sprintf("%s/age.key", rr.configRoot)
}

func (rr ReconcileRun) Validate() (bool, error) {
	manifestFile := rr.getManifestFilePath()
	if !utils.FileExists(manifestFile) {
		return false, fmt.Errorf("manifest file could not be found in config root (%s)", manifestFile)
	}

	ageFile := rr.getAgeFilePath()
	if !utils.FileExists(ageFile) {
		return false, fmt.Errorf("age file could not be found in config root (%s)", ageFile)
	}

	return true, nil
}

func (rr *ReconcileRun) LoadManifest() error {
	manifestFile := rr.getManifestFilePath()
	ageFile := rr.getAgeFilePath()
	observer, err := NewObserverFromManifest(manifestFile, ageFile)
	if err != nil {
		return err
	}
	rr.manifest = observer
	return nil
}

func (rr ReconcileRun) LoadState() error {
	// stateFile := fmt.Sprintf("%s/state.yaml", rr.configRoot)
	return nil
}

func (rr *ReconcileRun) Reconcile() error {
	repoURL := strings.TrimSpace(rr.manifest.Spec.Source.RepoURL)
	if repoURL == "" {
		return fmt.Errorf("manifest spec.source.repoURL is required")
	}

	targetRevision := strings.TrimSpace(rr.manifest.Spec.Source.TargetRevision)
	if targetRevision == "" {
		return fmt.Errorf("manifest spec.source.targetRevision is required")
	}

	repoRoot := filepath.Join(rr.configRoot, "repo")
	repo, err := newGitRepository(repoRoot, repoURL)
	if err != nil {
		return err
	}

	slog.Info(
		"Starting reconcile run",
		"configRoot", rr.configRoot,
		"repoSource", sanitizeGitSourceForLog(repoURL),
		"targetRevision", targetRevision,
	)

	repoOperation, err := repo.Sync(targetRevision)
	if err != nil {
		return err
	}

	discoveryRoot, err := resolveDiscoveryRoot(repoRoot, rr.manifest.Spec.Source.Path)
	if err != nil {
		return err
	}

	slog.Info(
		"Repository prepared",
		"repoOperation", repoOperation,
		"discoveryRoot", discoveryRoot,
	)

	discoveredApplications, err := application.DiscoverApplications(discoveryRoot)
	if err != nil {
		return fmt.Errorf("discovering applications: %w", err)
	}

	for _, app := range discoveredApplications {
		slog.Debug(
			"Discovered application",
			"operationalID", app.OperationalID,
			"runtimeSlug", app.RuntimeSlug,
			"sourceDir", app.SourceDir,
			"manifestPath", app.ManifestPath,
			"metadataName", app.Application.Metadata.Name,
		)
	}

	slog.Info("Finished reconcile run", "discoveredApplications", len(discoveredApplications))

	return nil
}

func resolveDiscoveryRoot(repoRoot, sourcePath string) (string, error) {
	trimmed := strings.TrimSpace(sourcePath)
	if trimmed == "" || trimmed == "." || trimmed == "/" {
		return repoRoot, nil
	}

	trimmed = strings.TrimPrefix(trimmed, "/")

	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if cleaned == "" || cleaned == "." {
		return repoRoot, nil
	}

	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("manifest spec.source.path %q resolves to a host absolute path", sourcePath)
	}

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("manifest spec.source.path %q escapes repository root", sourcePath)
	}

	discoveryRoot := filepath.Join(repoRoot, cleaned)
	relativeToRepo, err := filepath.Rel(repoRoot, discoveryRoot)
	if err != nil {
		return "", fmt.Errorf("resolving discovery path %q: %w", sourcePath, err)
	}

	if relativeToRepo == ".." || strings.HasPrefix(relativeToRepo, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("manifest spec.source.path %q escapes repository root", sourcePath)
	}

	return discoveryRoot, nil
}

func sanitizeGitSourceForLog(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return ""
	}

	schemeIndex := strings.Index(trimmed, "://")
	if schemeIndex <= 0 {
		return trimmed
	}

	authorityStart := schemeIndex + len("://")
	authorityEndOffset := strings.IndexAny(trimmed[authorityStart:], "/?#")
	authorityEnd := len(trimmed)
	if authorityEndOffset >= 0 {
		authorityEnd = authorityStart + authorityEndOffset
	}

	authority := trimmed[authorityStart:authorityEnd]
	atIndex := strings.LastIndex(authority, "@")
	if atIndex < 0 {
		return trimmed
	}

	sanitizedAuthority := authority[atIndex+1:]
	return trimmed[:authorityStart] + sanitizedAuthority + trimmed[authorityEnd:]
}
