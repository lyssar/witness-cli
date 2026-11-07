package reconcile

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"

	"filippo.io/age"
)

func TestMain(m *testing.M) {
	lookupAppGroup = func() (*user.Group, error) {
		return &user.Group{Name: appGroupName, Gid: strconv.Itoa(os.Getegid())}, nil
	}
	os.Exit(m.Run())
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
