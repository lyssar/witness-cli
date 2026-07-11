package application

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// WarningCodeIgnoredManifestRequiredFile reports composeFiles ignored by .witnessignore.
const WarningCodeIgnoredManifestRequiredFile = "ignored_manifest_required_file"

// FilesetWarning is a structured fileset warning.
type FilesetWarning struct {
	Code    string
	Path    string
	Message string
}

// ManagedFile is one managed source file with canonical slash-relative path.
type ManagedFile struct {
	RelativePath string
	SourcePath   string
}

// FileSet contains deterministic managed files and related warnings.
type FileSet struct {
	ManagedFiles          []ManagedFile
	DeferredSecretTargets []string
	Warnings              []FilesetWarning
}

// BuildFileSet resolves deterministic managed files for one discovered application.
func BuildFileSet(app DiscoveredApplication, submodulePaths []string) (FileSet, error) {
	ignoreMatcher, err := loadIgnoreMatcher(app.SourceDir)
	if err != nil {
		return FileSet{}, err
	}

	secretSources := make(map[string]struct{}, len(app.Application.Spec.Secrets))
	secretTargets := make([]string, 0, len(app.Application.Spec.Secrets))
	secretTargetSet := make(map[string]struct{}, len(app.Application.Spec.Secrets))
	for i, secret := range app.Application.Spec.Secrets {
		source, err := normalizeRelativePath(secret.Source)
		if err != nil {
			return FileSet{}, fmt.Errorf("normalizing secret source %d: %w", i, err)
		}
		secretSources[source] = struct{}{}

		target, err := normalizeRelativePath(secret.Target)
		if err != nil {
			return FileSet{}, fmt.Errorf("normalizing secret target %d: %w", i, err)
		}
		if _, exists := secretTargetSet[target]; exists {
			return FileSet{}, fmt.Errorf("duplicate secret target: %q", target)
		}
		secretTargetSet[target] = struct{}{}
		secretTargets = append(secretTargets, target)
	}

	composeFiles := make(map[string]struct{}, len(app.Application.Spec.ComposeFiles))
	for i, composePath := range app.Application.Spec.ComposeFiles {
		normalized, err := normalizeRelativePath(composePath)
		if err != nil {
			return FileSet{}, fmt.Errorf("normalizing compose file %d: %w", i, err)
		}
		composeFiles[normalized] = struct{}{}
	}

	for _, secretTarget := range secretTargets {
		if _, collides := secretSources[secretTarget]; collides {
			return FileSet{}, fmt.Errorf("secret target collides with secret source: %q", secretTarget)
		}
		if _, collides := composeFiles[secretTarget]; collides {
			return FileSet{}, fmt.Errorf("secret target collides with compose file: %q", secretTarget)
		}
	}

	managedByPath := make(map[string]ManagedFile)
	var warnings []FilesetWarning

	err = filepath.WalkDir(app.SourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walking %q: %w", path, walkErr)
		}

		rel, err := filepath.Rel(app.SourceDir, path)
		if err != nil {
			return fmt.Errorf("resolving relative path for %q: %w", path, err)
		}

		if rel == "." {
			return nil
		}

		relSlash := filepath.ToSlash(rel)
		ignored := ignoreMatcher.Match(relSlash, entry.IsDir())

		if entry.IsDir() {
			if ignored {
				return filepath.SkipDir
			}
			return nil
		}

		if ignored {
			if _, required := composeFiles[relSlash]; required {
				warnings = append(warnings, FilesetWarning{
					Code:    WarningCodeIgnoredManifestRequiredFile,
					Path:    relSlash,
					Message: "compose file is ignored by .witnessignore but remains managed",
				})
			} else {
				return nil
			}
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("reading file info for %q: %w", path, err)
		}

		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not supported: %q", relSlash)
		}

		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type at %q", relSlash)
		}

		if _, isSecretSource := secretSources[relSlash]; isSecretSource {
			return nil
		}

		managedByPath[relSlash] = ManagedFile{RelativePath: relSlash, SourcePath: path}
		return nil
	})
	if err != nil {
		return FileSet{}, err
	}

	for _, submodulePath := range submodulePaths {
		if submodulePath == app.OperationalID || strings.HasPrefix(submodulePath, app.OperationalID+"/") {
			return FileSet{}, fmt.Errorf("git submodule is not supported for app %q at %q", app.OperationalID, submodulePath)
		}
	}

	for rel := range composeFiles {
		if _, ok := managedByPath[rel]; ok {
			continue
		}
		return FileSet{}, fmt.Errorf("required compose file is missing: %q", rel)
	}

	for _, secretTarget := range secretTargets {
		if _, collides := managedByPath[secretTarget]; collides {
			return FileSet{}, fmt.Errorf("secret target collides with managed file: %q", secretTarget)
		}
	}

	sort.Strings(secretTargets)
	files := make([]ManagedFile, 0, len(managedByPath))
	paths := make([]string, 0, len(managedByPath))
	for rel := range managedByPath {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		files = append(files, managedByPath[rel])
	}

	return FileSet{ManagedFiles: files, DeferredSecretTargets: secretTargets, Warnings: warnings}, nil
}
