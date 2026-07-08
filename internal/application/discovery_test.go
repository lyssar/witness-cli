package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverApplicationsRecursiveAndSorted(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeApp(t, filepath.Join(root, "zeta", "api"))
	writeApp(t, filepath.Join(root, "alpha", "web"))

	apps, err := DiscoverApplications(root)
	if err != nil {
		t.Fatalf("discover applications: %v", err)
	}

	if len(apps) != 2 {
		t.Fatalf("expected 2 applications, got %d", len(apps))
	}

	if apps[0].OperationalID != "alpha/web" {
		t.Fatalf("expected first app alpha/web, got %q", apps[0].OperationalID)
	}
	if apps[1].OperationalID != "zeta/api" {
		t.Fatalf("expected second app zeta/api, got %q", apps[1].OperationalID)
	}
}

func TestDiscoverApplicationsExactManifestNameOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "alpha", "web")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir app dir: %v", err)
	}

	manifest := validManifestYAML("web")
	if err := os.WriteFile(filepath.Join(appDir, "skuld.yml"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write wrong manifest: %v", err)
	}

	apps, err := DiscoverApplications(root)
	if err != nil {
		t.Fatalf("discover applications: %v", err)
	}

	if len(apps) != 0 {
		t.Fatalf("expected zero discovered apps, got %d", len(apps))
	}
}

func TestDiscoverApplicationsRejectsNonDirectoryRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	discoveryRoot := filepath.Join(root, "skuld.yaml")
	if err := os.WriteFile(discoveryRoot, []byte(validManifestYAML("app")), 0o600); err != nil {
		t.Fatalf("write discovery root file: %v", err)
	}

	_, err := DiscoverApplications(discoveryRoot)
	if err == nil {
		t.Fatal("expected non-directory root error")
	}

	if !strings.Contains(err.Error(), "must be a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoverApplicationsRejectsNestedApps(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeApp(t, filepath.Join(root, "alpha"))
	writeApp(t, filepath.Join(root, "alpha", "web"))

	_, err := DiscoverApplications(root)
	if err == nil {
		t.Fatal("expected nested app error")
	}

	if !strings.Contains(err.Error(), "nested applications") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoverApplicationsRejectsSlugCollisions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeApp(t, filepath.Join(root, "a", "b-c"))
	writeApp(t, filepath.Join(root, "a-b", "c"))

	_, err := DiscoverApplications(root)
	if err == nil {
		t.Fatal("expected runtime slug collision error")
	}

	if !strings.Contains(err.Error(), "runtime slug collision") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeApp(t *testing.T, appDir string) {
	t.Helper()

	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir app dir %q: %v", appDir, err)
	}

	if err := os.WriteFile(filepath.Join(appDir, ManifestFileName), []byte(validManifestYAML(filepath.Base(appDir))), 0o600); err != nil {
		t.Fatalf("write manifest in %q: %v", appDir, err)
	}

	if err := os.WriteFile(filepath.Join(appDir, "docker-compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose file in %q: %v", appDir, err)
	}
}

func validManifestYAML(name string) string {
	return "apiVersion: skuld.dev/v1alpha1\n" +
		"kind: Application\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"spec:\n" +
		"  provisioner: docker-compose\n" +
		"  composeFiles:\n" +
		"    - docker-compose.yaml\n"
}
