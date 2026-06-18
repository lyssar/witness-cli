package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	dotignore "github.com/codeglyph/go-dotignore"
)

const ignoreFileName = ".skuldignore"

type ignoreMatcher interface {
	Match(path string, isDir bool) bool
}

type defaultIgnoreMatcher struct{}

func (defaultIgnoreMatcher) Match(_ string, _ bool) bool { return false }

type dotIgnoreMatcher struct {
	matcher *dotignore.PatternMatcher
}

func (m dotIgnoreMatcher) Match(path string, isDir bool) bool {
	matched, err := m.matcher.Matches(path)
	if err != nil {
		return false
	}
	if isDir {
		matchedWithSlash, slashErr := m.matcher.Matches(path + "/")
		if slashErr == nil {
			return matched || matchedWithSlash
		}
	}
	return matched
}

func loadIgnoreMatcher(appRoot string) (ignoreMatcher, error) {
	ignorePath := filepath.Join(appRoot, ignoreFileName)
	if _, err := os.Stat(ignorePath); err != nil {
		if os.IsNotExist(err) {
			return defaultIgnoreMatcher{}, nil
		}
		return nil, fmt.Errorf("stat ignore file %q: %w", ignorePath, err)
	}

	content, err := os.ReadFile(ignorePath)
	if err != nil {
		return nil, fmt.Errorf("reading ignore file %q: %w", ignorePath, err)
	}

	validPatterns := make([]string, 0)
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := dotignore.NewPatternMatcher([]string{line}); err != nil {
			continue
		}
		validPatterns = append(validPatterns, line)
	}

	matcher, err := dotignore.NewPatternMatcher(validPatterns)
	if err != nil {
		return nil, fmt.Errorf("building ignore matcher for %q: %w", ignorePath, err)
	}

	return dotIgnoreMatcher{matcher: matcher}, nil
}
