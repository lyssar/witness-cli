package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lyssar/witness-cli/internal/application"
)

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}
	stagingRoot := t.TempDir()

	stagedPath := filepath.Join(stagingRoot, "compose.yaml")
	if err := os.WriteFile(stagedPath, []byte("a"), 0o600); err != nil {
		t.Fatalf("write staged: %v", err)
	}

	fileset := application.FileSet{ManagedFiles: []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: "/unused"}}}

	resultMissing, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("detect drift missing: %v", err)
	}
	if !resultMissing.HasDrift || len(resultMissing.Missing) != 1 {
		t.Fatalf("expected missing drift, got %#v", resultMissing)
	}

	livePath := filepath.Join(destination, "apps", "web")
	if err := os.MkdirAll(livePath, 0o755); err != nil {
		t.Fatalf("mkdir live: %v", err)
	}
	if err := os.WriteFile(filepath.Join(livePath, "compose.yaml"), []byte("b"), 0o600); err != nil {
		t.Fatalf("write live: %v", err)
	}

	resultChanged, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("detect drift changed: %v", err)
	}
	if !resultChanged.HasDrift || len(resultChanged.Changed) != 1 {
		t.Fatalf("expected changed drift, got %#v", resultChanged)
	}
}

func TestDetectDriftTypeMismatchAndSymlink(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}
	stagingRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingRoot, "compose.yaml"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	fileset := application.FileSet{ManagedFiles: []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: "/unused"}}}

	liveDir := filepath.Join(destination, "apps", "web")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir live: %v", err)
	}
	if err := os.Symlink("target", filepath.Join(liveDir, "compose.yaml")); err != nil {
		t.Fatalf("symlink live: %v", err)
	}

	result, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if len(result.TypeMismatch) != 1 || result.TypeMismatch[0] != "compose.yaml" {
		t.Fatalf("expected type mismatch for symlink, got %#v", result)
	}
	// TypeMismatch must count as drift
	if !result.HasDrift {
		t.Fatalf("expected HasDrift=true for type mismatch, got false")
	}
	// TypeMismatch must not appear in Changed or Missing
	if len(result.Changed) != 0 {
		t.Fatalf("expected no Changed entries for type mismatch, got %#v", result.Changed)
	}
	if len(result.Missing) != 0 {
		t.Fatalf("expected no Missing entries for type mismatch, got %#v", result.Missing)
	}
}

func TestDetectDriftLiveRootNonDirFails(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}
	stagingRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingRoot, "compose.yaml"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	fileset := application.FileSet{ManagedFiles: []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: "/unused"}}}

	livePath := filepath.Join(destination, "apps", "web")
	if err := os.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(livePath, []byte("not-dir"), 0o600); err != nil {
		t.Fatalf("write root file: %v", err)
	}

	if _, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot}); err == nil {
		t.Fatalf("expected non-dir root error")
	}
}

func TestDetectDriftMissingStagingFileFails(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}
	stagingRoot := t.TempDir()
	fileset := application.FileSet{ManagedFiles: []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: "/unused"}}}
	if err := os.MkdirAll(filepath.Join(destination, "apps", "web"), 0o755); err != nil {
		t.Fatalf("mkdir live: %v", err)
	}

	if _, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot}); err == nil {
		t.Fatalf("expected missing staging file error")
	}
}

func TestDetectDriftSecretTargetMissing(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}
	stagingRoot := t.TempDir()

	// Create a decrypted secret target in staging
	secretTarget := filepath.Join(stagingRoot, "secrets", ".env")
	if err := os.MkdirAll(filepath.Dir(secretTarget), 0o755); err != nil {
		t.Fatalf("mkdir staging secret: %v", err)
	}
	if err := os.WriteFile(secretTarget, []byte("TOKEN=secret"), 0o600); err != nil {
		t.Fatalf("write staging secret: %v", err)
	}

	fileset := application.FileSet{DeferredSecretTargets: []string{"secrets/.env"}}

	result, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if !result.HasDrift {
		t.Fatalf("expected drift for missing secret target, got %#v", result)
	}
	if len(result.SecretChanged) != 1 || result.SecretChanged[0] != "secrets/.env" {
		t.Fatalf("expected secret changed, got %#v", result)
	}
	if len(result.DeferredSecretTargets) != 1 {
		t.Fatalf("expected deferred secret targets in result, got %#v", result)
	}
}

func TestDetectDriftCarriesFilesetWarnings(t *testing.T) {
	t.Parallel()

	result, err := detectDrift(t.TempDir(), application.DiscoveredApplication{OperationalID: "apps/web"}, application.FileSet{
		Warnings: []application.FilesetWarning{{Code: "ignored_manifest_required_file", Path: "compose.yaml", Message: "warning"}},
	}, AppStaging{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "ignored_manifest_required_file" {
		t.Fatalf("expected warnings to be propagated, got %#v", result.Warnings)
	}
}

func TestDetectDriftSecretTargetChanged(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}

	// Create live app with secret target
	liveDir := filepath.Join(destination, "apps", "web")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir live app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "compose.yaml"), []byte("services: {}"), 0o600); err != nil {
		t.Fatalf("write live compose: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(liveDir, "secrets", ".env")), 0o755); err != nil {
		t.Fatalf("mkdir live secret: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "secrets", ".env"), []byte("OLD=value"), 0o600); err != nil {
		t.Fatalf("write live secret: %v", err)
	}

	// Create staging with different secret target
	stagingRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingRoot, "compose.yaml"), []byte("services: {}"), 0o600); err != nil {
		t.Fatalf("write staged compose: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(stagingRoot, "secrets", ".env")), 0o755); err != nil {
		t.Fatalf("mkdir staging secret: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingRoot, "secrets", ".env"), []byte("NEW=value"), 0o600); err != nil {
		t.Fatalf("write staged secret: %v", err)
	}

	fileset := application.FileSet{
		ManagedFiles:          []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: "/unused"}},
		DeferredSecretTargets: []string{"secrets/.env"},
	}

	result, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if !result.HasDrift {
		t.Fatalf("expected drift for changed secret target, got %#v", result)
	}
	if len(result.SecretChanged) != 1 || result.SecretChanged[0] != "secrets/.env" {
		t.Fatalf("expected secret changed, got %#v", result)
	}
	// Compose files should not show drift
	if len(result.Changed) != 0 {
		t.Fatalf("expected no compose drift, got %#v", result)
	}
}

func TestDetectDriftSecretTargetSame(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}

	// Create live app with secret target
	liveDir := filepath.Join(destination, "apps", "web")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir live app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "compose.yaml"), []byte("services: {}"), 0o600); err != nil {
		t.Fatalf("write live compose: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(liveDir, "secrets", ".env")), 0o755); err != nil {
		t.Fatalf("mkdir live secret: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "secrets", ".env"), []byte("TOKEN=secret"), 0o600); err != nil {
		t.Fatalf("write live secret: %v", err)
	}

	// Create staging with same secret target
	stagingRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingRoot, "compose.yaml"), []byte("services: {}"), 0o600); err != nil {
		t.Fatalf("write staged compose: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(stagingRoot, "secrets", ".env")), 0o755); err != nil {
		t.Fatalf("mkdir staging secret: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingRoot, "secrets", ".env"), []byte("TOKEN=secret"), 0o600); err != nil {
		t.Fatalf("write staged secret: %v", err)
	}

	fileset := application.FileSet{
		ManagedFiles:          []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: "/unused"}},
		DeferredSecretTargets: []string{"secrets/.env"},
	}

	result, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if result.HasDrift {
		t.Fatalf("expected no drift when secret target is same, got %#v", result)
	}
	if len(result.SecretChanged) != 0 {
		t.Fatalf("expected no secret changed, got %#v", result)
	}
}

func TestDetectDriftSecretTargetTypeMismatch(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}

	// Create live app with secret target as symlink
	liveDir := filepath.Join(destination, "apps", "web")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir live app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "compose.yaml"), []byte("services: {}"), 0o600); err != nil {
		t.Fatalf("write live compose: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(liveDir, "secrets", ".env")), 0o755); err != nil {
		t.Fatalf("mkdir live secret: %v", err)
	}
	if err := os.Symlink("target", filepath.Join(liveDir, "secrets", ".env")); err != nil {
		t.Fatalf("symlink live secret: %v", err)
	}

	// Create staging with regular secret target
	stagingRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingRoot, "compose.yaml"), []byte("services: {}"), 0o600); err != nil {
		t.Fatalf("write staged compose: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(stagingRoot, "secrets", ".env")), 0o755); err != nil {
		t.Fatalf("mkdir staging secret: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingRoot, "secrets", ".env"), []byte("TOKEN=secret"), 0o600); err != nil {
		t.Fatalf("write staged secret: %v", err)
	}

	fileset := application.FileSet{
		ManagedFiles:          []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: "/unused"}},
		DeferredSecretTargets: []string{"secrets/.env"},
	}

	result, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if !result.HasDrift {
		t.Fatalf("expected drift for type mismatch on secret target, got %#v", result)
	}
	if len(result.SecretChanged) != 1 || result.SecretChanged[0] != "secrets/.env" {
		t.Fatalf("expected secret changed for type mismatch, got %#v", result)
	}
}

func TestDetectDriftLiveMissingSecretTargetsSetDrift(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}
	stagingRoot := t.TempDir()

	// No live directory exists. Staging has both managed and secret files.
	if err := os.WriteFile(filepath.Join(stagingRoot, "compose.yaml"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(stagingRoot, "secrets", ".env")), 0o755); err != nil {
		t.Fatalf("mkdir staging secret: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingRoot, "secrets", ".env"), []byte("s"), 0o600); err != nil {
		t.Fatalf("write staged secret: %v", err)
	}

	fileset := application.FileSet{
		ManagedFiles:          []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: "/unused"}},
		DeferredSecretTargets: []string{"secrets/.env"},
	}

	result, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if !result.HasDrift {
		t.Fatalf("expected drift when live missing, got %#v", result)
	}
	if len(result.Missing) != 1 || result.Missing[0] != "compose.yaml" {
		t.Fatalf("expected compose.yaml in Missing, got %#v", result.Missing)
	}
	if len(result.SecretChanged) != 1 || result.SecretChanged[0] != "secrets/.env" {
		t.Fatalf("expected secrets/.env in SecretChanged, got %#v", result.SecretChanged)
	}
}
