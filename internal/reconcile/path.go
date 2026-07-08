package reconcile

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var urlWithSchemePattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s'"<>]+`)

func resolveDiscoveryRoot(repoRoot, sourcePath string) (string, error) {
	trimmed := strings.TrimSpace(sourcePath)
	if trimmed == "" || trimmed == "." || trimmed == "/" {
		return repoRoot, nil
	}

	trimmed = strings.TrimPrefix(trimmed, "/")

	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if cleaned == "" || cleaned == "." {
		return repoRoot, nil
	}

	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("manifest spec.source.path %q resolves to a host absolute path", sourcePath)
	}

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("manifest spec.source.path %q escapes repository root", sourcePath)
	}

	discoveryRoot := filepath.Join(repoRoot, cleaned)
	relativeToRepo, err := filepath.Rel(repoRoot, discoveryRoot)
	if err != nil {
		return "", fmt.Errorf("resolving discovery path %q: %w", sourcePath, err)
	}

	if relativeToRepo == ".." || strings.HasPrefix(relativeToRepo, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("manifest spec.source.path %q escapes repository root", sourcePath)
	}

	return discoveryRoot, nil
}

func sanitizeGitSourceForLog(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return ""
	}

	return sanitizeCredentialedURLs(trimmed)
}

func sanitizeCredentialedURLs(input string) string {
	return urlWithSchemePattern.ReplaceAllStringFunc(input, func(rawURL string) string {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.User == nil {
			return rawURL
		}

		parsed.User = nil
		return parsed.String()
	})
}
