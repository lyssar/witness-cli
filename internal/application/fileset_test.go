package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFileSet(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")
	mustWrite(t, filepath.Join(appDir, "README.md"), "readme")
	mustWrite(t, filepath.Join(appDir, "secrets.enc"), "cipher")
	mustWrite(t, filepath.Join(appDir, ".witnessignore"), "README.md\n")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "apps/web",
		Application: Application{Spec: Spec{
			ComposeFiles: []string{"compose.yaml"},
			Secrets:      []Secret{{Source: "secrets.enc", Target: "secrets/.env", Decryptor: DecryptorAge}},
		}},
	}

	fileset, err := BuildFileSet(app, nil)
	if err != nil {
		t.Fatalf("build fileset: %v", err)
	}

	if len(fileset.ManagedFiles) != 2 {
		t.Fatalf("expected 2 managed files (.witnessignore + compose), got %d", len(fileset.ManagedFiles))
	}
	if len(fileset.DeferredSecretTargets) != 1 || fileset.DeferredSecretTargets[0] != "secrets/.env" {
		t.Fatalf("unexpected deferred secret targets: %#v", fileset.DeferredSecretTargets)
	}
}

func TestBuildFileSetSubmoduleFailsApp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "apps/web",
		Application:   Application{Spec: Spec{ComposeFiles: []string{"compose.yaml"}}},
	}

	_, err := BuildFileSet(app, []string{"apps/web/submodule"})
	if err == nil {
		t.Fatalf("expected submodule error")
	}
}

func TestBuildFileSetSubmoduleFailsAppWhenDiscoveryRootIsNested(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "web",
		Application:   Application{Spec: Spec{ComposeFiles: []string{"compose.yaml"}}},
	}

	_, err := BuildFileSet(app, []string{"web/vendor/dep"})
	if err == nil {
		t.Fatalf("expected submodule error")
	}
}

func TestBuildFileSetIgnoredManifestRequiredWarns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")
	mustWrite(t, filepath.Join(appDir, ".witnessignore"), "compose.yaml\n")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "apps/web",
		Application:   Application{Spec: Spec{ComposeFiles: []string{"compose.yaml"}}},
	}

	fileset, err := BuildFileSet(app, nil)
	if err != nil {
		t.Fatalf("build fileset: %v", err)
	}
	if len(fileset.Warnings) != 1 || fileset.Warnings[0].Code != WarningCodeIgnoredManifestRequiredFile {
		t.Fatalf("expected ignored manifest warning, got %#v", fileset.Warnings)
	}
}

func TestBuildFileSetSecretTargetCollisionComposeFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")
	mustWrite(t, filepath.Join(appDir, "secrets.enc"), "cipher")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "apps/web",
		Application: Application{Spec: Spec{
			ComposeFiles: []string{"compose.yaml"},
			Secrets:      []Secret{{Source: "secrets.enc", Target: "compose.yaml", Decryptor: DecryptorAge}},
		}},
	}

	if _, err := BuildFileSet(app, nil); err == nil {
		t.Fatalf("expected secret target/compose collision error")
	}
}

func TestBuildFileSetSecretTargetCollisionManagedFileFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")
	mustWrite(t, filepath.Join(appDir, "config.env"), "FOO=1")
	mustWrite(t, filepath.Join(appDir, "secrets.enc"), "cipher")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "apps/web",
		Application: Application{Spec: Spec{
			ComposeFiles: []string{"compose.yaml"},
			Secrets:      []Secret{{Source: "secrets.enc", Target: "config.env", Decryptor: DecryptorAge}},
		}},
	}

	if _, err := BuildFileSet(app, nil); err == nil {
		t.Fatalf("expected secret target/managed file collision error")
	}
}

func TestBuildFileSetSecretTargetCollisionSecretSourceFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")
	mustWrite(t, filepath.Join(appDir, "secret.age"), "cipher")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "apps/web",
		Application: Application{Spec: Spec{
			ComposeFiles: []string{"compose.yaml"},
			Secrets: []Secret{
				{Source: "secret.age", Target: ".env", Decryptor: DecryptorAge},
				{Source: "other.age", Target: "secret.age", Decryptor: DecryptorAge},
			},
		}},
	}

	if _, err := BuildFileSet(app, nil); err == nil {
		t.Fatalf("expected secret target/secret source collision error")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestBuildFileSetRejectsUndeclaredAgeFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")
	mustWrite(t, filepath.Join(appDir, "secret.age"), "cipher")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "apps/web",
		Application: Application{Spec: Spec{
			ComposeFiles: []string{"compose.yaml"},
		}},
	}

	_, err := BuildFileSet(app, nil)
	if err == nil {
		t.Fatalf("expected undeclared .age file error")
	}
	if !strings.Contains(err.Error(), "undeclared encrypted file") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "secret.age") {
		t.Fatalf("expected error to mention the file, got: %v", err)
	}
}

func TestBuildFileSetAllowsDeclaredAgeSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")
	mustWrite(t, filepath.Join(appDir, "secret.age"), "cipher")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "apps/web",
		Application: Application{Spec: Spec{
			ComposeFiles: []string{"compose.yaml"},
			Secrets:      []Secret{{Source: "secret.age", Target: "secrets/.env", Decryptor: DecryptorAge}},
		}},
	}

	fileset, err := BuildFileSet(app, nil)
	if err != nil {
		t.Fatalf("build fileset: %v", err)
	}
	// secret.age should NOT be in ManagedFiles (it is a declared secret source)
	for _, mf := range fileset.ManagedFiles {
		if mf.RelativePath == "secret.age" {
			t.Fatalf("declared secret source should not be in ManagedFiles")
		}
	}
}

func TestBuildFileSetRejectsUndeclaredAgeInSubdir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "web")
	mustWrite(t, filepath.Join(appDir, "compose.yaml"), "services: {}")
	mustWrite(t, filepath.Join(appDir, "keys", "backup.age"), "cipher")

	app := DiscoveredApplication{
		SourceDir:     appDir,
		OperationalID: "apps/web",
		Application: Application{Spec: Spec{
			ComposeFiles: []string{"compose.yaml"},
		}},
	}

	_, err := BuildFileSet(app, nil)
	if err == nil {
		t.Fatalf("expected undeclared .age file error in subdir")
	}
	if !strings.Contains(err.Error(), "keys/backup.age") {
		t.Fatalf("expected error to mention subdir file, got: %v", err)
	}
}
