package application

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var identitySegmentPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// BuildIdentity derives operational identity and runtime slug from app source path.
func BuildIdentity(discoveryRoot, appSourceDir string) (Identity, error) {
	rootAbs, err := filepath.Abs(discoveryRoot)
	if err != nil {
		return Identity{}, fmt.Errorf("resolving discovery root %q: %w", discoveryRoot, err)
	}

	sourceAbs, err := filepath.Abs(appSourceDir)
	if err != nil {
		return Identity{}, fmt.Errorf("resolving app source directory %q: %w", appSourceDir, err)
	}

	relativePath, err := filepath.Rel(rootAbs, sourceAbs)
	if err != nil {
		return Identity{}, fmt.Errorf("computing app relative path: %w", err)
	}

	relativePath = filepath.ToSlash(relativePath)
	if err := ValidateIdentityPath(relativePath); err != nil {
		return Identity{}, err
	}

	slug, err := SlugFromIdentityPath(relativePath)
	if err != nil {
		return Identity{}, err
	}

	return Identity{OperationalID: relativePath, RuntimeSlug: slug}, nil
}

// ValidateIdentityPath validates operational identity path segment rules.
func ValidateIdentityPath(relativePath string) error {
	normalized, err := normalizeRelativePath(relativePath)
	if err != nil {
		return fmt.Errorf("identity path: %w", err)
	}

	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		if !identitySegmentPattern.MatchString(segment) {
			return fmt.Errorf("identity path segment %q is invalid", segment)
		}
	}

	return nil
}

// SlugFromIdentityPath computes runtime slug from operational identity path.
func SlugFromIdentityPath(relativePath string) (string, error) {
	if err := ValidateIdentityPath(relativePath); err != nil {
		return "", err
	}

	normalized, err := normalizeRelativePath(relativePath)
	if err != nil {
		return "", fmt.Errorf("normalizing identity path: %w", err)
	}

	return strings.ReplaceAll(normalized, "/", "-"), nil
}
