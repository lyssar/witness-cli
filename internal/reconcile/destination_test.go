package reconcile

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/internal/state"
)

func TestCanonicalArchiveRefRejectsForgedState(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"/tmp/archive/hello/20260712T010203.000000004Z",
		"archive/other/20260712T010203.000000004Z",
		"archive/hello/not-a-timestamp",
		"archive/hello/../other/20260712T010203.000000004Z",
	} {
		if _, ok := canonicalArchiveRef("hello", value); ok {
			t.Fatalf("forged archive reference %q accepted", value)
		}
	}
}

func TestForgedArchiveStateDoesNotReachProvisioner(t *testing.T) {
	destinationRoot := t.TempDir()
	destination, err := openDestinationFS(destinationRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := destination.Close(); closeErr != nil {
			t.Errorf("close destination: %v", closeErr)
		}
	}()

	p := &fakeProvisioner{}
	entry := deletionStateEntry()
	entry.ArchivePath = "/tmp/forged"
	_, removed, err := reconcileDeletedApp(context.Background(), destination, "hello", entry, p, time.Now().UTC())
	if err == nil {
		t.Fatal("expected forged archive state rejection")
	}
	if removed || p.deleteCalls != 0 || p.cleanupCalls != 0 {
		t.Fatalf("forged archive state reached provisioner or removed state: removed=%t delete=%d cleanup=%d", removed, p.deleteCalls, p.cleanupCalls)
	}
}

func TestPersistedRetrySymlinkedArchiveDoesNotReachDelete(t *testing.T) {
	destinationRoot := t.TempDir()
	destination, err := openDestinationFS(destinationRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := destination.Close(); closeErr != nil {
			t.Errorf("close destination: %v", closeErr)
		}
	}()

	archive, err := archiveRef("hello", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(destinationRoot, archive)
	if err := os.MkdirAll(archivePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(archivePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), archivePath); err != nil {
		t.Fatal(err)
	}

	p := &fakeProvisioner{}
	entry := deletionStateEntry()
	entry.ArchivePath = archive
	if _, _, err := reconcileDeletedApp(context.Background(), destination, "hello", entry, p, time.Now().UTC()); err == nil {
		t.Fatal("expected symlinked persisted archive rejection")
	}
	if p.deleteCalls != 0 {
		t.Fatalf("symlinked archive reached Delete: %d calls", p.deleteCalls)
	}
}

func TestUntrustedStateDoesNotMutateOrReachProvisioner(t *testing.T) {
	tests := []struct {
		name  string
		entry state.Entry
	}{
		{name: "forged compose path", entry: func() state.Entry {
			entry := deletionStateEntry()
			entry.ComposeFiles = []string{"../compose.yaml"}
			return entry
		}()},
		{name: "duplicate canonical compose path", entry: func() state.Entry {
			entry := deletionStateEntry()
			entry.ComposeFiles = []string{"compose.yaml", "dir/../compose.yaml"}
			return entry
		}()},
		{name: "forged runtime slug", entry: func() state.Entry { entry := deletionStateEntry(); entry.RuntimeSlug = "other"; return entry }()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			destinationRoot := t.TempDir()
			destination, err := openDestinationFS(destinationRoot)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if closeErr := destination.Close(); closeErr != nil {
					t.Errorf("close destination: %v", closeErr)
				}
			}()
			if err := os.Mkdir(filepath.Join(destinationRoot, "hello"), 0o700); err != nil {
				t.Fatal(err)
			}
			test.entry.SecretTargetsKnown = false

			p := &fakeProvisioner{}
			if _, _, err := reconcileDeletedApp(context.Background(), destination, "hello", test.entry, p, time.Now().UTC()); err == nil {
				t.Fatal("expected untrusted state rejection")
			}
			if p.deleteCalls != 0 || p.cleanupCalls != 0 {
				t.Fatalf("untrusted state reached provisioner: delete=%d cleanup=%d", p.deleteCalls, p.cleanupCalls)
			}
			if info, err := os.Stat(filepath.Join(destinationRoot, "hello")); err != nil || !info.IsDir() {
				t.Fatalf("live directory was mutated: info=%v err=%v", info, err)
			}
		})
	}
}

func TestUntrustedDeletionRemovesLiveTreeWithoutArchiveOrSecretInventory(t *testing.T) {
	destinationRoot := t.TempDir()
	destination, err := openDestinationFS(destinationRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := destination.Close(); closeErr != nil {
			t.Errorf("close destination: %v", closeErr)
		}
	}()

	writeFile(t, filepath.Join(destinationRoot, "hello", "compose.yaml"), "services: {}\n")
	writeFile(t, filepath.Join(destinationRoot, "hello", "secrets", ".env"), "plaintext-secret")
	writeFile(t, filepath.Join(destinationRoot, "hello", ".registry-password"), "plaintext-password")
	entry := deletionStateEntry()
	entry.SecretTargetsKnown = false
	entry.SecretTargets = []string{"../untrusted-secret"}
	p := &fakeProvisioner{}

	updated, removed, err := reconcileDeletedApp(context.Background(), destination, "hello", entry, p, time.Now().UTC())
	if err != nil || !removed {
		t.Fatalf("untrusted deletion = (%#v, %t, %v), want removed", updated, removed, err)
	}
	if p.deleteCalls != 1 || p.cleanupCalls != 1 {
		t.Fatalf("provisioner calls = delete=%d cleanup=%d, want 1 each", p.deleteCalls, p.cleanupCalls)
	}
	if len(p.lastApp.Spec.Secrets) != 0 {
		t.Fatalf("untrusted deletion passed secrets to provisioner: %#v", p.lastApp.Spec.Secrets)
	}
	if _, err := os.Stat(filepath.Join(destinationRoot, "hello")); !os.IsNotExist(err) {
		t.Fatalf("live tree remained after deletion: %v", err)
	}
	if archives, err := filepath.Glob(filepath.Join(destinationRoot, "archive", "hello", "*")); err != nil || len(archives) != 0 {
		t.Fatalf("untrusted deletion created archives: paths=%#v err=%v", archives, err)
	}
}

func TestUntrustedDeletionFailuresKeepLiveTreeForRetry(t *testing.T) {
	tests := []struct {
		name string
		p    *fakeProvisioner
	}{
		{name: "delete", p: &fakeProvisioner{deleteErr: errBoom}},
		{name: "cleanup", p: &fakeProvisioner{cleanupErr: errBoom}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			destinationRoot := t.TempDir()
			destination, err := openDestinationFS(destinationRoot)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if closeErr := destination.Close(); closeErr != nil {
					t.Errorf("close destination: %v", closeErr)
				}
			}()
			writeFile(t, filepath.Join(destinationRoot, "hello", "compose.yaml"), "services: {}\n")
			entry := deletionStateEntry()
			entry.SecretTargetsKnown = false

			updated, removed, err := reconcileDeletedApp(context.Background(), destination, "hello", entry, test.p, time.Now().UTC())
			if err == nil || removed {
				t.Fatalf("untrusted %s failure = removed=%t err=%v", test.name, removed, err)
			}
			if updated.Status != state.StatusDeleting || updated.ArchivePath != "" || updated.LastError == "" {
				t.Fatalf("retry state = %#v, want deleting state without archive and diagnostic", updated)
			}
			if _, err := os.Stat(filepath.Join(destinationRoot, "hello")); err != nil {
				t.Fatalf("live tree removed after failed %s: %v", test.name, err)
			}
		})
	}
}

func TestUntrustedDeletionMissingLiveCleansUpAndClearsState(t *testing.T) {
	destination, err := openDestinationFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := destination.Close(); closeErr != nil {
			t.Errorf("close destination: %v", closeErr)
		}
	}()
	entry := deletionStateEntry()
	entry.SecretTargetsKnown = false
	p := &fakeProvisioner{}

	_, removed, err := reconcileDeletedApp(context.Background(), destination, "hello", entry, p, time.Now().UTC())
	if err != nil || !removed || p.deleteCalls != 0 || p.cleanupCalls != 1 {
		t.Fatalf("missing live recovery = removed=%t delete=%d cleanup=%d err=%v", removed, p.deleteCalls, p.cleanupCalls, err)
	}
}

func TestUntrustedDeletionArchiveBlocksWithoutMutation(t *testing.T) {
	for _, archivePath := range []string{"archive/hello/20260712T010203.000000004Z", ""} {
		t.Run("archive path "+archivePath, func(t *testing.T) {
			destinationRoot := t.TempDir()
			destination, err := openDestinationFS(destinationRoot)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if closeErr := destination.Close(); closeErr != nil {
					t.Errorf("close destination: %v", closeErr)
				}
			}()
			writeFile(t, filepath.Join(destinationRoot, "hello", "compose.yaml"), "services: {}\n")
			entry := deletionStateEntry()
			entry.SecretTargetsKnown = false
			entry.ArchivePath = archivePath
			if archivePath == "" {
				writeFile(t, filepath.Join(destinationRoot, "archive", "hello", "20260712T010203.000000004Z", "compose.yaml"), "services: {}\n")
			}
			p := &fakeProvisioner{}

			if _, removed, err := reconcileDeletedApp(context.Background(), destination, "hello", entry, p, time.Now().UTC()); err == nil || removed {
				t.Fatalf("archive-blocked untrusted deletion = removed=%t err=%v", removed, err)
			}
			if p.deleteCalls != 0 || p.cleanupCalls != 0 {
				t.Fatalf("archive-blocked deletion reached provisioner: %#v", p)
			}
			if _, err := os.Stat(filepath.Join(destinationRoot, "hello")); err != nil {
				t.Fatalf("archive-blocked deletion mutated live tree: %v", err)
			}
		})
	}
}

func TestUntrustedDeletionCanonicalArchiveArtifactsBlockWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		create func(t *testing.T, path string)
	}{
		{name: "directory", create: func(t *testing.T, path string) {
			t.Helper()
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "regular file", create: func(t *testing.T, path string) {
			writeFile(t, path, "not an archive directory")
		}},
		{name: "symlink", create: func(t *testing.T, path string) {
			t.Helper()
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(t.TempDir(), path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "broken symlink", create: func(t *testing.T, path string) {
			t.Helper()
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), path); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			destinationRoot := t.TempDir()
			destination, err := openDestinationFS(destinationRoot)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if closeErr := destination.Close(); closeErr != nil {
					t.Errorf("close destination: %v", closeErr)
				}
			}()

			writeFile(t, filepath.Join(destinationRoot, "hello", "compose.yaml"), "services: {}\n")
			archivePath := filepath.Join(destinationRoot, "archive", "hello", "20260712T010203.000000004Z")
			test.create(t, archivePath)
			entry := deletionStateEntry()
			entry.SecretTargetsKnown = false
			p := &fakeProvisioner{}

			updated, removed, err := reconcileDeletedApp(context.Background(), destination, "hello", entry, p, time.Now().UTC())
			if err == nil || removed {
				t.Fatalf("artifact-blocked untrusted deletion = removed=%t err=%v", removed, err)
			}
			if !reflect.DeepEqual(updated, entry) {
				t.Fatalf("artifact-blocked deletion mutated state: got %#v, want %#v", updated, entry)
			}
			if p.deleteCalls != 0 || p.cleanupCalls != 0 {
				t.Fatalf("artifact-blocked deletion reached provisioner: %#v", p)
			}
			if _, err := os.Stat(filepath.Join(destinationRoot, "hello")); err != nil {
				t.Fatalf("artifact-blocked deletion mutated live tree: %v", err)
			}
		})
	}
}

func deletionStateEntry() state.Entry {
	return state.Entry{
		Provisioner:        application.ProvisionerDockerCompose,
		RuntimeSlug:        "hello",
		ComposeFiles:       []string{"compose.yaml"},
		SecretTargetsKnown: true,
	}
}

func TestRootedSecretScrubRejectsSymlinkedAncestry(t *testing.T) {
	destinationRoot := t.TempDir()
	destination, err := openDestinationFS(destinationRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := destination.Close(); closeErr != nil {
			t.Errorf("close destination: %v", closeErr)
		}
	}()

	archive, err := archiveRef("hello", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(destinationRoot, filepath.Dir(archive)), 0o755); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(destinationRoot, archive)); err != nil {
		t.Fatal(err)
	}
	if err := scrubArchivedSecrets(destination, archive, []string{"secret"}); err == nil {
		t.Fatal("expected symlinked archive ancestry rejection")
	}
}

func TestValidateOperationalIDNamespaceRejectsStateCollision(t *testing.T) {
	t.Parallel()

	discovered := []application.DiscoveredApplication{{OperationalID: "team/app"}}
	stateFile := state.File{Applications: map[string]state.Entry{"team": {}}}
	if err := validateOperationalIDNamespace(discovered, stateFile); err == nil {
		t.Fatal("expected parent/child collision")
	}
}

func TestValidateOperationalIDNamespaceRejectsRuntimeSlugCollision(t *testing.T) {
	t.Parallel()

	discovered := []application.DiscoveredApplication{{OperationalID: "a/b-c"}}
	stateFile := state.File{Applications: map[string]state.Entry{"a-b/c": {}}}
	err := validateOperationalIDNamespace(discovered, stateFile)
	if err == nil {
		t.Fatal("expected runtime slug collision")
	}
	const want = `runtime slug collision between "a-b/c" and "a/b-c": "a-b-c"`
	if err.Error() != want {
		t.Fatalf("collision error = %q, want %q", err, want)
	}
}
