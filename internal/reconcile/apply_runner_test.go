package reconcile

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/internal/decryptor"
	"github.com/lyssar/witness-cli/internal/provisioner"
	"github.com/lyssar/witness-cli/internal/state"
)

var errBoom = errors.New("boom")

func TestRunnerApplyUpdateAndDelete(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for reconcile runner tests")
	}

	fixture := newMutableGitFixture(t)
	configRoot := t.TempDir()
	destinationRoot := filepath.Join(t.TempDir(), "dest")
	writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest(fixture.remotePath, "main", "apps", destinationRoot))
	writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

	decrypt := fakeDecryptor{content: "TOKEN=initial\n"}
	provisioner := &fakeProvisioner{}
	runner := NewRunner(configRoot, WithDecryptor(decrypt), WithProvisioner(provisioner))

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}

	liveDir := filepath.Join(destinationRoot, "hello")
	assertFileContent(t, filepath.Join(liveDir, "compose.yaml"), "services:\n  hello:\n    image: hello:v1\n")
	assertFileContent(t, filepath.Join(liveDir, "secrets", ".env"), "TOKEN=initial\n")
	if provisioner.applyCalls != 1 || provisioner.validateCalls != 1 {
		t.Fatalf("expected one apply/validate call, got apply=%d validate=%d", provisioner.applyCalls, provisioner.validateCalls)
	}

	stateFile := loadStateFileForTest(t, filepath.Join(configRoot, "state.json"))
	entry := stateFile.Applications["hello"]
	if entry.Status != state.StatusHealthy {
		t.Fatalf("expected healthy state, got %#v", entry)
	}

	fixture.updateCompose(t, "services:\n  hello:\n    image: hello:v2\n")
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("update reconcile: %v", err)
	}
	assertFileContent(t, filepath.Join(liveDir, "compose.yaml"), "services:\n  hello:\n    image: hello:v2\n")
	if provisioner.applyCalls != 2 {
		t.Fatalf("expected second apply call, got %d", provisioner.applyCalls)
	}

	fixture.removeApp(t)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("delete reconcile: %v", err)
	}
	if provisioner.deleteCalls != 1 {
		t.Fatalf("expected delete call, got %d", provisioner.deleteCalls)
	}
	if _, err := os.Stat(liveDir); !os.IsNotExist(err) {
		t.Fatalf("expected live dir to be archived, stat err=%v", err)
	}
	archived, err := filepath.Glob(filepath.Join(destinationRoot, "..", "archive", "hello", "*", "compose.yaml"))
	if err != nil {
		t.Fatalf("glob archive: %v", err)
	}
	if len(archived) != 1 {
		t.Fatalf("expected archived compose file, got %#v", archived)
	}
	secretArchives, err := filepath.Glob(filepath.Join(destinationRoot, "..", "archive", "hello", "*", "secrets", ".env"))
	if err != nil {
		t.Fatalf("glob archived secret: %v", err)
	}
	if len(secretArchives) != 0 {
		t.Fatalf("expected archived decrypted secret to be scrubbed, got %#v", secretArchives)
	}
	stateFile = loadStateFileForTest(t, filepath.Join(configRoot, "state.json"))
	if _, ok := stateFile.Applications["hello"]; ok {
		t.Fatalf("expected state entry removed after delete, got %#v", stateFile.Applications)
	}
}

func TestRunnerDeleteFailureKeepsArchivedState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for reconcile runner tests")
	}

	fixture := newMutableGitFixture(t)
	configRoot := t.TempDir()
	destinationRoot := filepath.Join(t.TempDir(), "dest")
	writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest(fixture.remotePath, "main", "apps", destinationRoot))
	writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

	provisioner := &fakeProvisioner{deleteErr: errBoom}
	runner := NewRunner(configRoot, WithDecryptor(fakeDecryptor{content: "TOKEN=initial\n"}), WithProvisioner(provisioner))
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	fixture.removeApp(t)

	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("expected delete failure")
	}

	stateFile := loadStateFileForTest(t, filepath.Join(configRoot, "state.json"))
	entry := stateFile.Applications["hello"]
	if entry.Status != state.StatusDeleting || entry.ArchivePath == "" {
		t.Fatalf("expected deleting state with archive path, got %#v", entry)
	}
	if _, err := os.Stat(entry.ArchivePath); err != nil {
		t.Fatalf("expected archive path to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(entry.ArchivePath, "secrets", ".env")); !os.IsNotExist(err) {
		t.Fatalf("expected archived decrypted secret to be scrubbed after failed delete, stat err=%v", err)
	}
}

func TestRunnerDeleteRecoversOrphanedArchiveSecrets(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for reconcile runner tests")
	}

	fixture := newMutableGitFixture(t)
	configRoot := t.TempDir()
	destinationRoot := filepath.Join(t.TempDir(), "dest")
	writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest(fixture.remotePath, "main", "apps", destinationRoot))
	writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

	provisioner := &fakeProvisioner{}
	runner := NewRunner(configRoot, WithDecryptor(fakeDecryptor{content: "TOKEN=initial\n"}), WithProvisioner(provisioner))
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	fixture.removeApp(t)

	liveDir := filepath.Join(destinationRoot, "hello")
	orphanArchiveDir, err := archiveAppDir(destinationRoot, "hello", time.Now().UTC())
	if err != nil {
		t.Fatalf("archive app dir path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(orphanArchiveDir), 0o755); err != nil {
		t.Fatalf("mkdir orphan archive parent: %v", err)
	}
	if err := os.Rename(liveDir, orphanArchiveDir); err != nil {
		t.Fatalf("rename live dir to orphan archive: %v", err)
	}

	statePath := filepath.Join(configRoot, "state.json")
	store := state.NewStore(statePath)
	stateFile, err := store.Load()
	if err != nil {
		t.Fatalf("load state file: %v", err)
	}
	entry := stateFile.Applications["hello"]
	entry.ArchivePath = ""
	stateFile.Applications["hello"] = entry
	if err := store.Save(stateFile); err != nil {
		t.Fatalf("save state file: %v", err)
	}

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("reconcile orphaned archive: %v", err)
	}
	if provisioner.deleteCalls != 1 {
		t.Fatalf("expected delete call during orphaned archive recovery, got %d", provisioner.deleteCalls)
	}
	if _, err := os.Stat(filepath.Join(orphanArchiveDir, "secrets", ".env")); !os.IsNotExist(err) {
		t.Fatalf("expected orphaned archived secret to be scrubbed, stat err=%v", err)
	}
}

type fakeDecryptor struct{ content string }

func (f fakeDecryptor) Name() string { return application.DecryptorAge }

func (f fakeDecryptor) DecryptFile(_ context.Context, request decryptor.Request) error {
	if err := os.MkdirAll(filepath.Dir(request.TargetPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(request.TargetPath, []byte(f.content), request.Mode)
}

type fakeProvisioner struct {
	validateCalls int
	applyCalls    int
	deleteCalls   int
	deleteErr     error
	lastRuntime   provisioner.RuntimeContext
}

func (f *fakeProvisioner) Name() string { return application.ProvisionerDockerCompose }

func (f *fakeProvisioner) Validate(_ context.Context, _ provisioner.RuntimeContext, _ application.Application) error {
	f.validateCalls++
	return nil
}

func (f *fakeProvisioner) Apply(_ context.Context, rt provisioner.RuntimeContext, _ application.Application) error {
	f.applyCalls++
	f.lastRuntime = rt
	return nil
}

func (f *fakeProvisioner) Delete(_ context.Context, _ provisioner.RuntimeContext, _ application.Application) error {
	f.deleteCalls++
	return f.deleteErr
}

func (f *fakeProvisioner) CleanupMissing(_ context.Context, _ provisioner.RuntimeContext) error {
	return nil
}

type mutableGitFixture struct {
	root       string
	seedPath   string
	remotePath string
}

func newMutableGitFixture(t *testing.T) mutableGitFixture {
	t.Helper()

	root := t.TempDir()
	seedPath := filepath.Join(root, "seed")
	remotePath := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(filepath.Join(seedPath, "apps", "hello"), 0o755); err != nil {
		t.Fatalf("mkdir seed app: %v", err)
	}
	runGit(t, seedPath, "init", "-b", "main")
	runGit(t, seedPath, "config", "user.email", "test@example.com")
	runGit(t, seedPath, "config", "user.name", "test")
	writeFile(t, filepath.Join(seedPath, "apps", "hello", "witness.yaml"), "apiVersion: witness.dev/v1alpha1\nkind: Application\nmetadata:\n  name: hello\nspec:\n  provisioner: docker-compose\n  composeFiles:\n    - compose.yaml\n  secrets:\n    - source: secret.age\n      target: secrets/.env\n      decryptor: age\n")
	writeFile(t, filepath.Join(seedPath, "apps", "hello", "compose.yaml"), "services:\n  hello:\n    image: hello:v1\n")
	writeFile(t, filepath.Join(seedPath, "apps", "hello", "secret.age"), "unused")
	runGit(t, seedPath, "add", ".")
	runGit(t, seedPath, "commit", "-m", "seed")
	runGit(t, root, "init", "--bare", remotePath)
	runGit(t, seedPath, "remote", "add", "origin", remotePath)
	runGit(t, seedPath, "push", "--set-upstream", "origin", "main")
	return mutableGitFixture{root: root, seedPath: seedPath, remotePath: remotePath}
}

func (f mutableGitFixture) updateCompose(t *testing.T, content string) {
	t.Helper()
	writeFile(t, filepath.Join(f.seedPath, "apps", "hello", "compose.yaml"), content)
	runGit(t, f.seedPath, "add", "apps/hello/compose.yaml")
	runGit(t, f.seedPath, "commit", "-m", "update compose")
	runGit(t, f.seedPath, "push", "origin", "main")
}

func (f mutableGitFixture) removeApp(t *testing.T) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(f.seedPath, "apps", "hello")); err != nil {
		t.Fatalf("remove app dir: %v", err)
	}
	runGit(t, f.seedPath, "add", "-A")
	runGit(t, f.seedPath, "commit", "-m", "remove app")
	runGit(t, f.seedPath, "push", "origin", "main")
}

func loadStateFileForTest(t *testing.T, path string) state.File {
	t.Helper()
	store := state.NewStore(path)
	file, err := store.Load()
	if err != nil {
		t.Fatalf("load state file: %v", err)
	}
	return file
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(content) != expected {
		t.Fatalf("unexpected content for %q: %q", path, string(content))
	}
}

func TestRunnerSecretOnlyDriftSetsSecretsChangedFlag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for reconcile runner tests")
	}

	fixture := newMutableGitFixture(t)
	configRoot := t.TempDir()
	destinationRoot := filepath.Join(t.TempDir(), "dest")
	writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest(fixture.remotePath, "main", "apps", destinationRoot))
	writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

	decrypt := fakeDecryptor{content: "TOKEN=initial\n"}
	prov := &fakeProvisioner{}
	runner := NewRunner(configRoot, WithDecryptor(decrypt), WithProvisioner(prov))

	// Initial reconcile: live doesn't exist → both managed files and secrets are missing
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if prov.applyCalls != 1 {
		t.Fatalf("expected one apply call, got %d", prov.applyCalls)
	}
	// First deploy: secrets are new → SecretsChanged must be true
	if !prov.lastRuntime.SecretsChanged {
		t.Fatalf("expected SecretsChanged=true on initial deploy, got false")
	}
	if !prov.lastRuntime.ComposeFilesChanged {
		t.Fatalf("expected ComposeFilesChanged=true on initial deploy (missing compose), got false")
	}

	// Change only the decrypted secret content — no compose source change
	decrypt.content = "TOKEN=rotated\n"
	runner = NewRunner(configRoot, WithDecryptor(decrypt), WithProvisioner(prov))
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("secret-only reconcile: %v", err)
	}
	if prov.applyCalls != 2 {
		t.Fatalf("expected second apply call, got %d", prov.applyCalls)
	}
	// Secret-only change: SecretsChanged=true, ComposeFilesChanged=false
	if !prov.lastRuntime.SecretsChanged {
		t.Fatalf("expected SecretsChanged=true after secret-only update, got false")
	}
	if prov.lastRuntime.ComposeFilesChanged {
		t.Fatalf("expected ComposeFilesChanged=false after secret-only update, got true")
	}
}

func TestRunnerNoDriftSkipsApply(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for reconcile runner tests")
	}

	fixture := newMutableGitFixture(t)
	configRoot := t.TempDir()
	destinationRoot := filepath.Join(t.TempDir(), "dest")
	writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest(fixture.remotePath, "main", "apps", destinationRoot))
	writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

	decrypt := fakeDecryptor{content: "TOKEN=initial\n"}
	prov := &fakeProvisioner{}
	runner := NewRunner(configRoot, WithDecryptor(decrypt), WithProvisioner(prov))

	// Initial reconcile
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if prov.applyCalls != 1 {
		t.Fatalf("expected one apply call, got %d", prov.applyCalls)
	}

	// Second reconcile with no changes — should skip apply
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if prov.applyCalls != 1 {
		t.Fatalf("expected no additional apply call on no-drift, got %d", prov.applyCalls)
	}
}

func TestRunnerComposeTypeMismatchSetsComposeFilesChanged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for reconcile runner tests")
	}

	fixture := newMutableGitFixture(t)
	configRoot := t.TempDir()
	destinationRoot := filepath.Join(t.TempDir(), "dest")
	writeFile(t, filepath.Join(configRoot, "manifest.yaml"), minimalManifest(fixture.remotePath, "main", "apps", destinationRoot))
	writeValidAgeKey(t, filepath.Join(configRoot, "age.key"))

	decrypt := fakeDecryptor{content: "TOKEN=initial\n"}
	prov := &fakeProvisioner{}
	runner := NewRunner(configRoot, WithDecryptor(decrypt), WithProvisioner(prov))

	// Initial reconcile — populates live dir with a regular compose.yaml
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if prov.applyCalls != 1 {
		t.Fatalf("expected one apply call, got %d", prov.applyCalls)
	}
	if !prov.lastRuntime.ComposeFilesChanged {
		t.Fatalf("expected ComposeFilesChanged=true on initial deploy, got false")
	}

	// Replace the live compose.yaml with a symlink to simulate TypeMismatch drift
	liveCompose := filepath.Join(destinationRoot, "hello", "compose.yaml")
	if err := os.Remove(liveCompose); err != nil {
		t.Fatalf("remove live compose: %v", err)
	}
	if err := os.Symlink("/somewhere/else", liveCompose); err != nil {
		t.Fatalf("symlink live compose: %v", err)
	}

	// Second reconcile — drift detector sees TypeMismatch, must set ComposeFilesChanged
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("type-mismatch reconcile: %v", err)
	}
	if prov.applyCalls != 2 {
		t.Fatalf("expected second apply call, got %d", prov.applyCalls)
	}
	if !prov.lastRuntime.ComposeFilesChanged {
		t.Fatalf("expected ComposeFilesChanged=true when compose file is a symlink, got false")
	}
}
