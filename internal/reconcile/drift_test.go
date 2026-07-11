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

func TestDetectDriftDeferredSecretsDoNotSetHasDrift(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	app := application.DiscoveredApplication{OperationalID: "apps/web"}
	stagingRoot := t.TempDir()
	fileset := application.FileSet{DeferredSecretTargets: []string{"secrets/.env"}}

	result, err := detectDrift(destination, app, fileset, AppStaging{Root: stagingRoot})
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if result.HasDrift {
		t.Fatalf("expected no drift for deferred secrets only, got %#v", result)
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
