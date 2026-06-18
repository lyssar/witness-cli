package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lyssar/skuld-cli/internal/application"
)

func TestBuildAppStaging(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(source, []byte("services: {}"), 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}

	staging, cleanup, err := buildAppStaging(application.DiscoveredApplication{}, application.FileSet{ManagedFiles: []application.ManagedFile{{RelativePath: "compose.yaml", SourcePath: source}}})
	if err != nil {
		t.Fatalf("build staging: %v", err)
	}
	defer func() {
		_ = cleanup()
	}()

	staged := filepath.Join(staging.Root, "compose.yaml")
	content, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("read staged: %v", err)
	}
	if string(content) != "services: {}" {
		t.Fatalf("unexpected staged content: %q", string(content))
	}

	stagedInfo, err := os.Stat(staged)
	if err != nil {
		t.Fatalf("stat staged: %v", err)
	}
	if stagedInfo.Mode().Perm() != 0o640 {
		t.Fatalf("expected staged mode 0640, got %o", stagedInfo.Mode().Perm())
	}
}
