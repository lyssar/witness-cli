package application

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverApplications recursively finds, parses, validates, and describes apps.
func DiscoverApplications(discoveryRoot string) ([]DiscoveredApplication, error) {
	stat, err := os.Stat(discoveryRoot)
	if err != nil {
		return nil, fmt.Errorf("stat discovery root %q: %w", discoveryRoot, err)
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("discovery root %q must be a directory", discoveryRoot)
	}

	var manifestPaths []string
	err = filepath.WalkDir(discoveryRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walking %q: %w", path, walkErr)
		}

		if d.IsDir() {
			return nil
		}

		if d.Name() != ManifestFileName {
			return nil
		}

		manifestPaths = append(manifestPaths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	discovered := make([]DiscoveredApplication, 0, len(manifestPaths))
	for _, manifestPath := range manifestPaths {
		app, err := ParseManifest(manifestPath)
		if err != nil {
			return nil, err
		}

		sourceDir := filepath.Dir(manifestPath)
		identity, err := BuildIdentity(discoveryRoot, sourceDir)
		if err != nil {
			return nil, fmt.Errorf("building identity for %q: %w", manifestPath, err)
		}

		discovered = append(discovered, DiscoveredApplication{
			ManifestPath:  manifestPath,
			SourceDir:     sourceDir,
			RelativePath:  identity.OperationalID,
			OperationalID: identity.OperationalID,
			RuntimeSlug:   identity.RuntimeSlug,
			Application:   app,
		})
	}

	sort.Slice(discovered, func(i, j int) bool {
		return discovered[i].OperationalID < discovered[j].OperationalID
	})

	if err := validateNoNestedApplications(discovered); err != nil {
		return nil, err
	}

	if err := validateNoSlugCollisions(discovered); err != nil {
		return nil, err
	}

	return discovered, nil
}

func validateNoNestedApplications(discovered []DiscoveredApplication) error {
	for i := 1; i < len(discovered); i++ {
		parent := discovered[i-1]
		child := discovered[i]

		if strings.HasPrefix(child.OperationalID, parent.OperationalID+"/") {
			return fmt.Errorf("nested applications are not allowed: %q contains %q", parent.ManifestPath, child.ManifestPath)
		}
	}

	return nil
}

func validateNoSlugCollisions(discovered []DiscoveredApplication) error {
	seen := make(map[string]DiscoveredApplication, len(discovered))

	for _, app := range discovered {
		existing, ok := seen[app.RuntimeSlug]
		if !ok {
			seen[app.RuntimeSlug] = app
			continue
		}

		if existing.OperationalID != app.OperationalID {
			return fmt.Errorf(
				"runtime slug collision %q between %q (%s) and %q (%s)",
				app.RuntimeSlug,
				existing.OperationalID,
				existing.ManifestPath,
				app.OperationalID,
				app.ManifestPath,
			)
		}
	}

	return nil
}
