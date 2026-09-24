package reconcile

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/lyssar/witness-cli/internal/application"
)

func TestNormalizeRelativeBindMount(t *testing.T) {
	tests := []struct {
		source      string
		want        string
		wantErr     bool
		errContains string
	}{
		{source: "./data", want: "data"},
		{source: "./config/app", want: "config/app"},
		{source: "./data/../config", want: "config"}, // cleaned
		{source: "/absolute/path", want: ""},
		{source: "named-volume", want: ""},
		{source: "data", want: ""}, // no ./ prefix
		{source: "./../escape", wantErr: true, errContains: "must be inside"},
		{source: "./", wantErr: true, errContains: "must be inside"},
		{source: "../escape", want: ""}, // no ./ prefix, treated as named
		{source: ".", want: ""},         // bare dot, no ./ prefix
		{source: "..", want: ""},        // bare dot-dot, no ./ prefix
		{source: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got, err := normalizeRelativeBindMount(tt.source)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestComposeVolumeSourcesShortSyntax(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n      - ./config:/config\n      - /abs:/abs\n      - named:/target\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	sources, err := composeVolumeSources(dest, "compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0] != "data" || sources[1] != "config" {
		t.Fatalf("expected [data config], got %#v", sources)
	}
}

func TestComposeVolumeSourcesLongSyntax(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	compose := "services:\n  app:\n    volumes:\n      - type: bind\n        source: ./data\n        target: /data\n      - type: volume\n        source: named\n        target: /target\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	sources, err := composeVolumeSources(dest, "compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0] != "data" {
		t.Fatalf("expected [data], got %#v", sources)
	}
}

func TestComposeVolumeSourcesDeduplicates(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n      - ./data:/data-again\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	sources, err := composeVolumeSources(dest, "compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0] != "data" {
		t.Fatalf("expected [data], got %#v", sources)
	}
}

func TestEnsureVolumeDirsSkipsFileBindMounts(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	// Stage tree containing a committed Caddyfile and a compose file that
	// bind-mounts both the single file and a directory.
	stage := filepath.Join(dir, "stage")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	compose := "services:\n  caddy:\n    image: caddy:latest\n    volumes:\n      - ./Caddyfile:/etc/caddy/Caddyfile:ro\n      - ./data:/data\n"
	if err := os.WriteFile(filepath.Join(stage, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	const caddyfile = ":80 {\n  respond \"hello\"\n}\n"
	if err := os.WriteFile(filepath.Join(stage, "Caddyfile"), []byte(caddyfile), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureVolumeDirs(dest, "stage", []string{"compose.yaml"}, nil); err != nil {
		t.Fatalf("ensureVolumeDirs: %v", err)
	}

	// The file bind-mount must remain a regular file with its content intact,
	// not be turned into a directory.
	info, err := os.Lstat(filepath.Join(stage, "Caddyfile"))
	if err != nil {
		t.Fatalf("stat Caddyfile: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("Caddyfile is no longer a regular file: %v", info.Mode())
	}
	b, err := os.ReadFile(filepath.Join(stage, "Caddyfile"))
	if err != nil {
		t.Fatalf("read Caddyfile: %v", err)
	}
	if string(b) != caddyfile {
		t.Fatalf("Caddyfile content changed: %q", string(b))
	}

	// The directory bind-mount must still be pre-created.
	info, err = os.Lstat(filepath.Join(stage, "data"))
	if err != nil {
		t.Fatalf("stat data dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("data is not a directory: %v", info.Mode())
	}
}

func TestPreserveVolumeDirsMovesWithOwnershipAndMode(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	oldLive := filepath.Join(dir, "old-live")
	stage := filepath.Join(dir, "stage")
	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n"
	writeFile(t, filepath.Join(oldLive, "compose.yaml"), compose)
	writeFile(t, filepath.Join(stage, "compose.yaml"), compose)

	// Container-owned 0600 file inside the volume dir (e.g. Caddy's ACME
	// private key). chown to a fixed container uid; skip ownership assertions
	// when the environment cannot chown.
	dataDir := filepath.Join(oldLive, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(dataDir, "acme-key.pem")
	if err := os.WriteFile(keyFile, []byte("private-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(keyFile, 1000, 1000); err != nil {
		t.Skipf("chown to container uid not permitted in this environment: %v", err)
	}

	moved, err := preserveVolumeDirs(dest, "old-live", "stage", []string{"compose.yaml"}, nil)
	if err != nil {
		t.Fatalf("preserveVolumeDirs: %v", err)
	}
	if len(moved) != 1 || moved[0] != "data" {
		t.Fatalf("expected moved=[data], got %#v", moved)
	}

	// The moved file must retain its mode and ownership.
	info, err := os.Lstat(filepath.Join(stage, "data", "acme-key.pem"))
	if err != nil {
		t.Fatalf("stat moved key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %v", info.Mode().Perm())
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if stat.Uid != 1000 || stat.Gid != 1000 {
			t.Fatalf("expected uid/gid 1000/1000, got %d/%d", stat.Uid, stat.Gid)
		}
	}
	// A move, not a copy: the old location must be gone.
	if _, err := os.Lstat(filepath.Join(oldLive, "data")); !os.IsNotExist(err) {
		t.Fatalf("expected old volume dir removed by move, stat err=%v", err)
	}
}

func TestPreserveVolumeDirsPreservesInteriorSymlink(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	oldLive := filepath.Join(dir, "old-live")
	stage := filepath.Join(dir, "stage")
	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n"
	writeFile(t, filepath.Join(oldLive, "compose.yaml"), compose)
	writeFile(t, filepath.Join(stage, "compose.yaml"), compose)

	dataDir := filepath.Join(oldLive, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dataDir, "real.txt"), "real")
	if err := os.Symlink("real.txt", filepath.Join(dataDir, "link")); err != nil {
		t.Fatal(err)
	}

	moved, err := preserveVolumeDirs(dest, "old-live", "stage", []string{"compose.yaml"}, nil)
	if err != nil {
		t.Fatalf("preserveVolumeDirs: %v", err)
	}
	if len(moved) != 1 || moved[0] != "data" {
		t.Fatalf("expected moved=[data], got %#v", moved)
	}
	// Interior symlinks are moved as-is, not rejected.
	info, err := os.Lstat(filepath.Join(stage, "data", "link"))
	if err != nil {
		t.Fatalf("stat moved symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink preserved, got %v", info.Mode())
	}
	target, err := os.Readlink(filepath.Join(stage, "data", "link"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "real.txt" {
		t.Fatalf("expected symlink target %q, got %q", "real.txt", target)
	}
}

func TestPreserveVolumeDirsRejectsOldLiveSymlinkAncestry(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	oldLive := filepath.Join(dir, "old-live")
	stage := filepath.Join(dir, "stage")
	compose := "services:\n  app:\n    volumes:\n      - ./link/data:/data\n"
	writeFile(t, filepath.Join(stage, "compose.yaml"), compose)

	// The old live volume dir sits behind a symlink ancestor.
	if err := os.MkdirAll(filepath.Join(oldLive, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(oldLive, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(oldLive, "real", "data"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = preserveVolumeDirs(dest, "old-live", "stage", []string{"compose.yaml"}, nil)
	if err == nil {
		t.Fatal("expected symlink ancestry rejection, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestPreserveVolumeDirsRejectsStageSymlinkAncestry(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	oldLive := filepath.Join(dir, "old-live")
	stage := filepath.Join(dir, "stage")
	compose := "services:\n  app:\n    volumes:\n      - ./link/data:/data\n"
	writeFile(t, filepath.Join(oldLive, "compose.yaml"), compose)
	writeFile(t, filepath.Join(stage, "compose.yaml"), compose)

	// Old live volume dir exists as a real directory.
	if err := os.MkdirAll(filepath.Join(oldLive, "link", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The stage tree has a symlink ancestor for the volume dir.
	if err := os.MkdirAll(filepath.Join(stage, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(stage, "link")); err != nil {
		t.Fatal(err)
	}

	_, err = preserveVolumeDirs(dest, "old-live", "stage", []string{"compose.yaml"}, nil)
	if err == nil {
		t.Fatal("expected symlink ancestry rejection, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestPreserveVolumeDirsSeedWinsOnCollision(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	oldLive := filepath.Join(dir, "old-live")
	stage := filepath.Join(dir, "stage")
	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n"
	writeFile(t, filepath.Join(oldLive, "compose.yaml"), compose)
	writeFile(t, filepath.Join(stage, "compose.yaml"), compose)

	// Old live volume dir with data.
	if err := os.MkdirAll(filepath.Join(oldLive, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(oldLive, "data", "old.txt"), "old")

	// Committed non-empty seed in the stage tree.
	if err := os.MkdirAll(filepath.Join(stage, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stage, "data", "seed.txt"), "seed")

	moved, err := preserveVolumeDirs(dest, "old-live", "stage", []string{"compose.yaml"}, nil)
	if err != nil {
		t.Fatalf("preserveVolumeDirs: %v", err)
	}
	if len(moved) != 0 {
		t.Fatalf("expected no moves on seed collision, got %#v", moved)
	}
	// The committed seed wins; old data is not moved into the stage.
	assertFileContent(t, filepath.Join(stage, "data", "seed.txt"), "seed")
	if _, err := os.Lstat(filepath.Join(stage, "data", "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old data not moved into stage, stat err=%v", err)
	}
	// The old live dir remains in place for the backup.
	if _, err := os.Lstat(filepath.Join(oldLive, "data")); err != nil {
		t.Fatalf("expected old live volume dir retained, stat err=%v", err)
	}
}

func TestPreserveVolumeDirsReplacesEmptyStageDir(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	oldLive := filepath.Join(dir, "old-live")
	stage := filepath.Join(dir, "stage")
	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n"
	writeFile(t, filepath.Join(oldLive, "compose.yaml"), compose)
	writeFile(t, filepath.Join(stage, "compose.yaml"), compose)

	// Old live volume dir with data.
	if err := os.MkdirAll(filepath.Join(oldLive, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(oldLive, "data", "persistent.db"), "data")

	// ensureVolumeDirs pre-creates an empty stage dir that the move replaces.
	if err := os.MkdirAll(filepath.Join(stage, "data"), 0o755); err != nil {
		t.Fatal(err)
	}

	moved, err := preserveVolumeDirs(dest, "old-live", "stage", []string{"compose.yaml"}, nil)
	if err != nil {
		t.Fatalf("preserveVolumeDirs: %v", err)
	}
	if len(moved) != 1 || moved[0] != "data" {
		t.Fatalf("expected moved=[data], got %#v", moved)
	}
	assertFileContent(t, filepath.Join(stage, "data", "persistent.db"), "data")
	if _, err := os.Lstat(filepath.Join(oldLive, "data")); !os.IsNotExist(err) {
		t.Fatalf("expected old volume dir removed by move, stat err=%v", err)
	}
}

func TestPreserveVolumeDirsSkipsSingleFileBindMounts(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	oldLive := filepath.Join(dir, "old-live")
	stage := filepath.Join(dir, "stage")
	compose := "services:\n  caddy:\n    image: caddy:latest\n    volumes:\n      - ./Caddyfile:/etc/caddy/Caddyfile:ro\n      - ./data:/data\n"
	writeFile(t, filepath.Join(oldLive, "compose.yaml"), compose)
	writeFile(t, filepath.Join(stage, "compose.yaml"), compose)

	// Old live has a committed Caddyfile (single-file bind mount) and a data dir.
	writeFile(t, filepath.Join(oldLive, "Caddyfile"), ":80 {\n  respond \"hello\"\n}\n")
	if err := os.MkdirAll(filepath.Join(oldLive, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(oldLive, "data", "persistent.db"), "data")

	moved, err := preserveVolumeDirs(dest, "old-live", "stage", []string{"compose.yaml"}, nil)
	if err != nil {
		t.Fatalf("preserveVolumeDirs: %v", err)
	}
	// Only the directory bind-mount is moved; the single file is skipped.
	if len(moved) != 1 || moved[0] != "data" {
		t.Fatalf("expected moved=[data], got %#v", moved)
	}
	// The Caddyfile stays in old-live (untouched) and is not moved into stage.
	if _, err := os.Lstat(filepath.Join(oldLive, "Caddyfile")); err != nil {
		t.Fatalf("expected Caddyfile retained in old live, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(stage, "Caddyfile")); !os.IsNotExist(err) {
		t.Fatalf("expected Caddyfile not moved into stage, stat err=%v", err)
	}
}

func TestReverseVolumeMovesRestoresOldLive(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	oldLive := filepath.Join(dir, "old-live")
	stage := filepath.Join(dir, "stage")
	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n      - ./config:/config\n"
	writeFile(t, filepath.Join(oldLive, "compose.yaml"), compose)
	writeFile(t, filepath.Join(stage, "compose.yaml"), compose)
	if err := os.MkdirAll(filepath.Join(oldLive, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(oldLive, "data", "persistent.db"), "data")
	if err := os.MkdirAll(filepath.Join(oldLive, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(oldLive, "config", "app.conf"), "conf")

	moved, err := preserveVolumeDirs(dest, "old-live", "stage", []string{"compose.yaml"}, nil)
	if err != nil {
		t.Fatalf("preserveVolumeDirs: %v", err)
	}
	if len(moved) != 2 {
		t.Fatalf("expected two moves, got %#v", moved)
	}
	if err := reverseVolumeMoves(dest, "stage", "old-live", moved); err != nil {
		t.Fatalf("reverseVolumeMoves: %v", err)
	}
	// Data is back in the old live tree.
	assertFileContent(t, filepath.Join(oldLive, "data", "persistent.db"), "data")
	assertFileContent(t, filepath.Join(oldLive, "config", "app.conf"), "conf")
	// The stage tree no longer holds the moved dirs.
	if _, err := os.Lstat(filepath.Join(stage, "data")); !os.IsNotExist(err) {
		t.Fatalf("expected stage data dir removed by reversal, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(stage, "config")); !os.IsNotExist(err) {
		t.Fatalf("expected stage config dir removed by reversal, stat err=%v", err)
	}
}

func TestPromoteRestoresLiveOnPreserveFailure(t *testing.T) {
	dir := t.TempDir()
	dest, err := openDestinationFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.Close() }()

	// Live app with two volume sources: ./data (real) and ./link/data
	// (behind a symlink ancestor in the old live tree).
	live := filepath.Join(dir, "hello")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	compose := "services:\n  app:\n    volumes:\n      - ./data:/data\n      - ./link/data:/data2\n"
	writeFile(t, filepath.Join(live, "compose.yaml"), compose)
	if err := os.MkdirAll(filepath.Join(live, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(live, "data", "persistent.db"), "data")
	// Container-owned 0600 file (e.g. Caddy's ACME private key) that must
	// survive the failed promotion in the restored live tree.
	keyFile := filepath.Join(live, "data", "acme-key.pem")
	if err := os.WriteFile(keyFile, []byte("private-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	canChown := os.Chown(keyFile, 1000, 1000) == nil
	if err := os.MkdirAll(filepath.Join(live, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(live, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, "real", "data"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Staging tree with the same compose file.
	stagingRoot := filepath.Join(dir, "staging-root")
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stagingRoot, "compose.yaml"), compose)

	app := application.DiscoveredApplication{
		OperationalID: "hello",
		Application: application.Application{
			Spec: application.Spec{
				ComposeFiles: []string{"compose.yaml"},
			},
		},
	}
	_, err = promoteRootedStagingToLive(dest, app, AppStaging{Root: stagingRoot})
	if err == nil {
		t.Fatal("expected preserve failure, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}

	// The old live tree must be restored: the reversed move and the
	// container-owned 0600 file survive in live, and backup is gone.
	assertFileContent(t, filepath.Join(live, "data", "persistent.db"), "data")
	info, err := os.Lstat(filepath.Join(live, "data", "acme-key.pem"))
	if err != nil {
		t.Fatalf("stat restored key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %v", info.Mode().Perm())
	}
	if canChown {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			if stat.Uid != 1000 || stat.Gid != 1000 {
				t.Fatalf("expected uid/gid 1000/1000, got %d/%d", stat.Uid, stat.Gid)
			}
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "hello.previous")); !os.IsNotExist(err) {
		t.Fatalf("expected backup removed after restore, stat err=%v", err)
	}
	// The stage tree must not retain the reversed move.
	stageDirs, err := filepath.Glob(filepath.Join(dir, ".witness-stage-hello-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stageDirs) != 1 {
		t.Fatalf("expected one stage dir, got %#v", stageDirs)
	}
	if _, err := os.Lstat(filepath.Join(stageDirs[0], "data")); !os.IsNotExist(err) {
		t.Fatalf("expected stage data dir reversed out, stat err=%v", err)
	}
}
