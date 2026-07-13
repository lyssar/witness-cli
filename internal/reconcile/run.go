package reconcile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/internal/decryptor"
	"github.com/lyssar/witness-cli/internal/provisioner"
	"github.com/lyssar/witness-cli/internal/state"
	"gopkg.in/yaml.v3"
)

// Runner executes the reconcile workflow from an observer config root.
type Runner struct {
	logger         *slog.Logger
	configRoot     string
	decryptors     map[string]decryptor.Decryptor
	provisioners   map[string]provisioner.Provisioner
	stateStoreFor  func(string) *state.Store
	stagingBuilder func(context.Context, string, map[string]decryptor.Decryptor, application.DiscoveredApplication, application.FileSet) (AppStaging, func() error, error)
}

// RunnerOption customizes runner dependencies.
type RunnerOption func(*Runner)

// NewRunner creates a new reconcile runner.
func NewRunner(configRoot string, options ...RunnerOption) *Runner {
	runner := &Runner{
		logger:     slog.Default(),
		configRoot: configRoot,
		decryptors: map[string]decryptor.Decryptor{
			application.DecryptorAge: decryptor.Age{},
		},
		provisioners: map[string]provisioner.Provisioner{
			application.ProvisionerDockerCompose: provisioner.NewDockerCompose(nil),
		},
		stateStoreFor: func(root string) *state.Store {
			return state.NewStore(statePathForConfigRoot(root))
		},
		stagingBuilder: buildAppStaging,
	}

	for _, option := range options {
		option(runner)
	}

	return runner
}

// WithDecryptor overrides one decryptor dependency.
func WithDecryptor(d decryptor.Decryptor) RunnerOption {
	return func(r *Runner) {
		if d != nil {
			r.decryptors[d.Name()] = d
		}
	}
}

// WithProvisioner overrides one provisioner dependency.
func WithProvisioner(p provisioner.Provisioner) RunnerOption {
	return func(r *Runner) {
		if p != nil {
			r.provisioners[p.Name()] = p
		}
	}
}

// WithStateStoreFactory overrides state store creation.
func WithStateStoreFactory(factory func(string) *state.Store) RunnerOption {
	return func(r *Runner) {
		if factory != nil {
			r.stateStoreFor = factory
		}
	}
}

// WithLogger sets the logger for structured output.
func WithLogger(logger *slog.Logger) RunnerOption {
	return func(r *Runner) {
		if logger != nil {
			r.logger = logger
		}
	}
}

type discoveredAppSet map[string]application.DiscoveredApplication

type appRuntime struct {
	app                    application.DiscoveredApplication
	fileset                application.FileSet
	staging                AppStaging
	stagingDone            func() error
	drift                  DriftResult
	volumeClaimsForcedDrift bool
}

// Run executes a reconcile pass for the configured root.
func (r *Runner) Run(ctx context.Context) error {
	runStart := time.Now()

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

	r.logger.Info(
		"Starting reconcile run",
		"configRoot", validatedRun.configRoot,
		"repoSource", sanitizeGitSourceForLog(manifestSource.repoURL),
		"targetRevision", manifestSource.targetRevision,
	)

	repoSyncStart := time.Now()
	repoOperation, err := repo.Sync(ctx, manifestSource.targetRevision)
	if err != nil {
		return err
	}
	resolvedCommit, err := repo.CurrentCommit(ctx)
	if err != nil {
		return fmt.Errorf("resolving synced commit: %w", err)
	}

	discoveryRoot, err := resolveDiscoveryRoot(repoRoot, manifestSource.path)
	if err != nil {
		return err
	}

	r.logger.Info("Repository prepared",
		"repoOperation", repoOperation,
		"discoveryRoot", discoveryRoot,
		"duration", time.Since(repoSyncStart).String(),
	)

	submodulePaths, err := extractSubmodulePaths(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("extracting submodule paths: %w", err)
	}
	submodulePaths, err = submodulePathsUnderDiscoveryRoot(submodulePaths, discoveryRoot, repoRoot)
	if err != nil {
		return fmt.Errorf("normalizing submodule paths for discovery root: %w", err)
	}

	var discoveredApplications []application.DiscoveredApplication
	if _, err := os.Stat(discoveryRoot); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat discovery root %q: %w", discoveryRoot, err)
		}
		r.logger.Info("Discovery root missing after sync; treating as zero discovered applications", "discoveryRoot", discoveryRoot)
	} else {
		discoveredApplications, err = application.DiscoverApplications(discoveryRoot)
		if err != nil {
			return fmt.Errorf("discovering applications: %w", err)
		}
	}
	sort.Slice(discoveredApplications, func(i, j int) bool {
		return discoveredApplications[i].OperationalID < discoveredApplications[j].OperationalID
	})

	stateStore := r.stateStoreFor(validatedRun.configRoot)
	stateFile, err := stateStore.Load()
	if err != nil {
		return err
	}
	if err := validateOperationalIDNamespace(discoveredApplications, stateFile); err != nil {
		return err
	}
	destination, err := openDestinationFS(manifestSource.destination)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := destination.Close(); closeErr != nil {
			r.logger.Warn("Closing destination root failed", "error", closeErr)
		}
	}()

	discoveredByID := make(discoveredAppSet, len(discoveredApplications))

	appErrors := make([]string, 0)
	now := time.Now().UTC()
	for _, app := range discoveredApplications {
		discoveredByID[app.OperationalID] = app
		r.logger.Debug(
			"Discovered application",
			"operationalID", app.OperationalID,
			"runtimeSlug", app.RuntimeSlug,
			"sourceDir", app.SourceDir,
			"manifestPath", app.ManifestPath,
			"metadataName", app.Application.Metadata.Name,
		)

		appEntry, appErr := r.prepareAppRuntime(ctx, validatedRun.agePath, destination, submodulePaths, app)
		if appErr != nil {
			stateFile.Applications[app.OperationalID] = failedStateEntry(now, stateFile.Applications[app.OperationalID], app, "", appErr)
			if saveErr := stateStore.Save(stateFile); saveErr != nil {
				return saveErr
			}
			appErrors = append(appErrors, fmt.Sprintf("%s: %v", app.OperationalID, appErr))
			r.logger.Error("App preparation failed", "operationalID", app.OperationalID, "error", appErr)
			continue
		}

		for _, warning := range appEntry.drift.Warnings {
			r.logger.Warn(
				"App fileset warning",
				"operationalID", app.OperationalID,
				"code", warning.Code,
				"path", warning.Path,
				"message", warning.Message,
			)
		}

		r.logger.Info("App drift analyzed",
			"operationalID", app.OperationalID,
			"hasDrift", appEntry.drift.HasDrift,
			"changed", len(appEntry.drift.Changed),
			"missing", len(appEntry.drift.Missing),
			"typeMismatch", len(appEntry.drift.TypeMismatch),
			"deferredSecretTargets", len(appEntry.drift.DeferredSecretTargets),
		)

		currentState := stateFile.Applications[app.OperationalID]
		// If a volume claim directory is missing from the live tree, force
		// apply so ensureVolumeDirs recreates it.
		if !appEntry.drift.HasDrift && len(app.Application.Spec.VolumeClaims) > 0 {
			if missing, err := volumeClaimDirsNeedFix(destination, app); err != nil {
				r.logger.Warn("Checking volume claim directories failed", "operationalID", app.OperationalID, "error", err)
			} else if missing {
				appEntry.drift.HasDrift = true
				appEntry.volumeClaimsForcedDrift = true
				r.logger.Info("Volume claim directory missing or wrong ownership — forcing apply", "operationalID", app.OperationalID)
			}
		}
		needsApply := appEntry.drift.HasDrift || currentState.Status == state.StatusFailed || currentState.Status == state.StatusDeleting
		if !needsApply {
			currentState = desiredStateEntry(now, app, resolvedCommit)
			currentState.LastError = ""
			stateFile.Applications[app.OperationalID] = currentState
			if saveErr := stateStore.Save(stateFile); saveErr != nil {
				return errors.Join(saveErr, cleanupAppStaging(app, appEntry.staging, appEntry.stagingDone))
			}
			if cleanupErr := cleanupAppStaging(app, appEntry.staging, appEntry.stagingDone); cleanupErr != nil {
				r.logger.Warn("App staging cleanup failed", "operationalID", app.OperationalID, "error", cleanupErr)
			}
			continue
		}

		if err := r.applyApp(ctx, destination, appEntry); err != nil {
			if cleanupErr := appEntry.stagingDone(); cleanupErr != nil {
				r.logger.Warn("App staging cleanup failed", "operationalID", app.OperationalID, "error", cleanupErr)
			}
			stateFile.Applications[app.OperationalID] = failedStateEntry(now, currentState, app, currentState.LastSuccessfulResolvedCommit, err)
			if saveErr := stateStore.Save(stateFile); saveErr != nil {
				return saveErr
			}
			appErrors = append(appErrors, fmt.Sprintf("%s: %v", app.OperationalID, err))
			r.logger.Error("App apply failed", "operationalID", app.OperationalID, "error", err)
			continue
		}

		stateFile.Applications[app.OperationalID] = desiredStateEntry(now, app, resolvedCommit)
		if saveErr := stateStore.Save(stateFile); saveErr != nil {
			return errors.Join(saveErr, cleanupAppStaging(app, appEntry.staging, appEntry.stagingDone))
		}
		if cleanupErr := cleanupAppStaging(app, appEntry.staging, appEntry.stagingDone); cleanupErr != nil {
			appErrors = append(appErrors, fmt.Sprintf("%s: %v", app.OperationalID, cleanupErr))
			r.logger.Error("App staging cleanup failed after successful apply", "operationalID", app.OperationalID, "error", cleanupErr)
		}
	}

	for _, operationalID := range sortedDeletedOperationalIDs(stateFile, discoveredByID) {
		entry := stateFile.Applications[operationalID]
		p := r.provisioners[entry.Provisioner]
		if p == nil {
			appErrors = append(appErrors, fmt.Sprintf("%s: unsupported state provisioner %q", operationalID, entry.Provisioner))
			continue
		}

		updatedEntry, removeEntry, err := reconcileDeletedApp(ctx, destination, operationalID, entry, p, now)
		if err != nil {
			stateFile.Applications[operationalID] = updatedEntry
			if saveErr := stateStore.Save(stateFile); saveErr != nil {
				return saveErr
			}
			appErrors = append(appErrors, fmt.Sprintf("%s: %v", operationalID, err))
			r.logger.Error("App deletion failed", "operationalID", operationalID, "error", err)
			continue
		}
		if removeEntry {
			delete(stateFile.Applications, operationalID)
		} else {
			stateFile.Applications[operationalID] = updatedEntry
		}
		if saveErr := stateStore.Save(stateFile); saveErr != nil {
			return saveErr
		}
	}

	r.logger.Info("Finished reconcile run",
		"discoveredApplications", len(discoveredApplications),
		"duration", time.Since(runStart).String(),
		"appErrors", len(appErrors),
	)
	if len(appErrors) > 0 {
		return errors.New("reconcile app failures (" + fmt.Sprint(len(appErrors)) + "): " + strings.Join(appErrors, "; "))
	}

	return nil
}

func (r *Runner) prepareAppRuntime(ctx context.Context, agePath string, destination *destinationFS, submodulePaths []string, app application.DiscoveredApplication) (appRuntime, error) {
	fileset, err := application.BuildFileSet(app, submodulePaths)
	if err != nil {
		return appRuntime{}, err
	}

	staging, cleanup, err := r.stagingBuilder(ctx, agePath, r.decryptors, app, fileset)
	if err != nil {
		return appRuntime{}, err
	}

	drift, err := detectRootedDrift(destination, app, fileset, staging)
	if err != nil {
		cleanupErr := cleanup()
		if cleanupErr != nil {
			r.logger.Warn("App staging cleanup failed", "operationalID", app.OperationalID, "error", cleanupErr)
		}
		return appRuntime{}, err
	}

	return appRuntime{
		app:         app,
		fileset:     fileset,
		staging:     staging,
		stagingDone: cleanup,
		drift:       drift,
	}, nil
}

// cleanupAppStaging removes the temporary plaintext tree and retains its path in
// any error so an operator can remediate a failed cleanup without exposing its
// contents in logs.
func cleanupAppStaging(app application.DiscoveredApplication, staging AppStaging, cleanup func() error) error {
	if err := cleanup(); err != nil {
		return fmt.Errorf("removing plaintext staging root %q for app %q: %w", staging.Root, app.OperationalID, err)
	}
	return nil
}

func (r *Runner) applyApp(ctx context.Context, destination *destinationFS, runtime appRuntime) error {
	p := r.provisioners[runtime.app.Application.Spec.Provisioner]
	if p == nil {
		return fmt.Errorf("unsupported provisioner %q", runtime.app.Application.Spec.Provisioner)
	}

	validationRuntime := provisioner.RuntimeContext{
		OperationalID: runtime.app.OperationalID,
		RuntimeSlug:   runtime.app.RuntimeSlug,
		LiveDir:       runtime.staging.Root,
		SourceDir:     runtime.app.SourceDir,
	}
	if err := p.Validate(ctx, validationRuntime, runtime.app.Application); err != nil {
		return fmt.Errorf("validating provisioner %q: %w", p.Name(), err)
	}

	liveRef, err := promoteRootedStagingToLive(destination, runtime.app, runtime.staging)
	if err != nil {
		return err
	}
	liveDir := destination.absolute(liveRef)
	runtime.staging.Root = liveDir

	// Update registry password path relative to new live dir
	var registryPasswordPath string
	if runtime.staging.RegistryPasswordPath != "" {
		registryPasswordPath = filepath.Join(liveDir, filepath.Base(runtime.staging.RegistryPasswordPath))
	}

	// Determine what changed — this decides the docker compose strategy:
	// - compose files changed → docker compose up --detach (Docker detects changes)
	// - secrets changed → docker compose up --detach --force-recreate (Docker can't detect secret file changes)
	// - neither → should not happen (apply only runs on drift)
	composeSet := make(map[string]struct{}, len(runtime.app.Application.Spec.ComposeFiles))
	for _, cf := range runtime.app.Application.Spec.ComposeFiles {
		composeSet[cf] = struct{}{}
	}

	composeFilesChanged := false
	allChanged := append(append(append([]string(nil), runtime.drift.Changed...), runtime.drift.Missing...), runtime.drift.TypeMismatch...)
	for _, f := range allChanged {
		if _, ok := composeSet[f]; ok {
			composeFilesChanged = true
			break
		}
	}

	secretsChanged := len(runtime.drift.SecretChanged) > 0

	runtimeRuntime := provisioner.RuntimeContext{
		OperationalID:         runtime.app.OperationalID,
		RuntimeSlug:           runtime.app.RuntimeSlug,
		LiveDir:               liveDir,
		SourceDir:             runtime.app.SourceDir,
		RegistryPasswordPath:  registryPasswordPath,
		ComposeFilesChanged:   composeFilesChanged,
		SecretsChanged:        secretsChanged,
		VolumeClaimsChanged:   runtime.volumeClaimsForcedDrift,
	}
	if err := p.Apply(ctx, runtimeRuntime, runtime.app.Application); err != nil {
		return fmt.Errorf("applying provisioner %q: %w", p.Name(), err)
	}

	return nil
}

func sortedDeletedOperationalIDs(stateFile state.File, discoveredByID discoveredAppSet) []string {
	deleted := make([]string, 0)
	for operationalID := range stateFile.Applications {
		if _, ok := discoveredByID[operationalID]; ok {
			continue
		}
		deleted = append(deleted, operationalID)
	}
	sort.Strings(deleted)
	return deleted
}

// validateOperationalIDNamespace rejects overlapping application trees and runtime
// slugs before any staging, destination mutation, or provisioner invocation.
func validateOperationalIDNamespace(discovered []application.DiscoveredApplication, stateFile state.File) error {
	ids := make(map[string]struct{}, len(discovered)+len(stateFile.Applications))
	for _, app := range discovered {
		ids[app.OperationalID] = struct{}{}
	}
	for id := range stateFile.Applications {
		ids[id] = struct{}{}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)

	slugs := make(map[string]string, len(ordered))
	for _, id := range ordered {
		if err := application.ValidateIdentityPath(id); err != nil {
			return fmt.Errorf("invalid operational identity %q in reconcile namespace: %w", id, err)
		}
		if id == "archive" || strings.HasPrefix(id, "archive/") {
			return fmt.Errorf("operational identity %q conflicts with reserved archive namespace", id)
		}
		slug, err := application.SlugFromIdentityPath(id)
		if err != nil {
			return fmt.Errorf("deriving runtime slug for operational identity %q: %w", id, err)
		}
		if otherID, ok := slugs[slug]; ok {
			return fmt.Errorf("runtime slug collision between %q and %q: %q", otherID, id, slug)
		}
		slugs[slug] = id
	}
	for i := 0; i < len(ordered)-1; i++ {
		if strings.HasPrefix(ordered[i+1], ordered[i]+"/") {
			return fmt.Errorf("operational identity collision between %q and %q", ordered[i], ordered[i+1])
		}
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
	SSHKey         string `yaml:"sshKey"`
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
