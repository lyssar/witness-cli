package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildIdentity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appA := filepath.Join(root, "team-a", "caddy")
	appB := filepath.Join(root, "team-b", "caddy")

	if err := os.MkdirAll(appA, 0o755); err != nil {
		t.Fatalf("create appA: %v", err)
	}
	if err := os.MkdirAll(appB, 0o755); err != nil {
		t.Fatalf("create appB: %v", err)
	}

	identityA, err := BuildIdentity(root, appA)
	if err != nil {
		t.Fatalf("build identity A: %v", err)
	}
	if identityA.OperationalID != "team-a/caddy" {
		t.Fatalf("unexpected operational id: %q", identityA.OperationalID)
	}
	if identityA.RuntimeSlug != "team-a-caddy" {
		t.Fatalf("unexpected runtime slug: %q", identityA.RuntimeSlug)
	}

	identityB, err := BuildIdentity(root, appB)
	if err != nil {
		t.Fatalf("build identity B: %v", err)
	}
	if identityA.RuntimeSlug == identityB.RuntimeSlug {
		t.Fatalf("expected distinct slugs, got %q", identityA.RuntimeSlug)
	}
}

func TestBuildIdentityRejectsInvalidSegments(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	invalid := filepath.Join(root, "TeamA", "caddy")
	if err := os.MkdirAll(invalid, 0o755); err != nil {
		t.Fatalf("create invalid dir: %v", err)
	}

	_, err := BuildIdentity(root, invalid)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("unexpected error: %v", err)
	}
}
