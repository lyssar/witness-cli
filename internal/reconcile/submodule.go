package reconcile

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func extractSubmodulePaths(ctx context.Context, repoRoot string) ([]string, error) {
	gitmodulesPath := filepath.Join(repoRoot, ".gitmodules")
	if _, err := os.Stat(gitmodulesPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat .gitmodules: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "config", "--file", ".gitmodules", "--get-regexp", "path")
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && strings.TrimSpace(string(output)) == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("parsing .gitmodules: %s", strings.TrimSpace(string(output)))
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	paths := make([]string, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid .gitmodules path line: %q", line)
		}
		cleaned, err := normalizeSubmodulePath(parts[1])
		if err != nil {
			return nil, err
		}
		paths = append(paths, cleaned)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading .gitmodules output: %w", err)
	}

	return paths, nil
}

func normalizeSubmodulePath(pathValue string) (string, error) {
	trimmed := strings.TrimSpace(pathValue)
	if trimmed == "" {
		return "", fmt.Errorf("submodule path is empty")
	}
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("submodule path must be relative: %q", pathValue)
	}
	clean := filepath.Clean(filepath.FromSlash(trimmed))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("submodule path escapes repository root: %q", pathValue)
	}
	return filepath.ToSlash(clean), nil
}
