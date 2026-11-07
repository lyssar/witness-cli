package reconcile

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"filippo.io/age"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "witness-app-group-gid")
	if err != nil {
		panic(err)
	}
	appGroupGIDPath = filepath.Join(dir, "app-group-gid")
	if err := os.WriteFile(appGroupGIDPath, []byte(strconv.Itoa(os.Getegid())+"\n"), 0o644); err != nil {
		panic(err)
	}
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func writeValidAgeKey(t *testing.T, path string) string {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate age identity: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir age key parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(identity.String()+"\n"), 0o600); err != nil {
		t.Fatalf("write valid age key: %v", err)
	}
	return identity.String()
}
