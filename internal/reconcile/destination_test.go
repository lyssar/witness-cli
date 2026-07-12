package reconcile

import (
	"context"
	"os"
	"path/filepath"
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
		{name: "forged secret target", entry: func() state.Entry {
			entry := deletionStateEntry()
			entry.SecretTargets = []string{"../secret"}
			return entry
		}()},
		{name: "missing secret inventory", entry: func() state.Entry { entry := deletionStateEntry(); entry.SecretTargetsKnown = false; return entry }()},
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
