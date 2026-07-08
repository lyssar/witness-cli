package reconcile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var commitPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

const maxGitCommandErrorOutputLen = 512

type gitRepository struct {
	rootPath  string
	remoteURL string
}

func newGitRepository(rootPath, remoteURL string) (*gitRepository, error) {
	trimmedRootPath := strings.TrimSpace(rootPath)
	if trimmedRootPath == "" {
		return nil, fmt.Errorf("repository root path is required")
	}

	trimmedRemoteURL := strings.TrimSpace(remoteURL)
	if trimmedRemoteURL == "" {
		return nil, fmt.Errorf("repository remote url is required")
	}
	if strings.HasPrefix(trimmedRemoteURL, "-") {
		return nil, fmt.Errorf("repository remote url must not start with '-' (got %q)", sanitizeGitSourceForLog(trimmedRemoteURL))
	}

	return &gitRepository{rootPath: trimmedRootPath, remoteURL: trimmedRemoteURL}, nil
}

func (r *gitRepository) Sync(ctx context.Context, targetRevision string) (string, error) {
	trimmedRevision := strings.TrimSpace(targetRevision)
	if trimmedRevision == "" {
		return "", fmt.Errorf("target revision is required")
	}

	operation, err := r.ensureLocalRepository(ctx)
	if err != nil {
		return "", err
	}

	if _, err := r.run(ctx, "fetch", "--prune", "--tags", "origin"); err != nil {
		return "", fmt.Errorf("fetching repository updates: %w", err)
	}

	if err := r.checkoutTargetRevision(ctx, trimmedRevision); err != nil {
		return "", err
	}

	return operation, nil
}

func (r *gitRepository) CurrentCommit(ctx context.Context) (string, error) {
	resolved, err := r.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolving HEAD commit: %w", err)
	}
	return strings.TrimSpace(resolved), nil
}

func (r *gitRepository) ensureLocalRepository(ctx context.Context) (string, error) {
	info, err := os.Stat(r.rootPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspecting repository path %q: %w", r.rootPath, err)
		}

		parentDir := filepath.Dir(r.rootPath)
		if err := os.MkdirAll(parentDir, 0o755); err != nil {
			return "", fmt.Errorf("creating repository parent directory %q: %w", parentDir, err)
		}

		if err := r.clone(ctx); err != nil {
			return "", err
		}

		return "clone", nil
	}

	if !info.IsDir() {
		return "", fmt.Errorf("repository path %q exists but is not a directory", r.rootPath)
	}

	insideWorktree, err := r.run(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return "", fmt.Errorf("repository path %q is not a valid git working tree: %w", r.rootPath, err)
	}

	if strings.TrimSpace(insideWorktree) != "true" {
		return "", fmt.Errorf("repository path %q is not inside a git working tree", r.rootPath)
	}

	remoteURL, err := r.run(ctx, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("reading git remote for %q: %w", r.rootPath, err)
	}

	trimmedRemote := strings.TrimSpace(remoteURL)
	if trimmedRemote != r.remoteURL {
		return "", fmt.Errorf(
			"repository %q has unexpected origin remote (got %q, expected %q)",
			r.rootPath,
			sanitizeGitSourceForLog(trimmedRemote),
			sanitizeGitSourceForLog(r.remoteURL),
		)
	}

	return "update", nil
}

func (r *gitRepository) clone(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--", r.remoteURL, r.rootPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := sanitizeAndTruncateGitCommandOutput(string(output))
		if message == "" {
			return fmt.Errorf("cloning repository from %q failed: %w", sanitizeGitSourceForLog(r.remoteURL), err)
		}

		return fmt.Errorf("cloning repository from %q failed: %s: %w", sanitizeGitSourceForLog(r.remoteURL), message, err)
	}

	return nil
}

func sanitizeAndTruncateGitCommandOutput(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}

	sanitized := sanitizeGitSourceForLog(trimmed)
	runes := []rune(sanitized)
	if len(runes) <= maxGitCommandErrorOutputLen {
		return sanitized
	}

	return string(runes[:maxGitCommandErrorOutputLen]) + "…"
}

func (r *gitRepository) checkoutTargetRevision(ctx context.Context, targetRevision string) error {
	if targetRevision == "HEAD" {
		return r.checkoutRemoteDefaultBranch(ctx)
	}

	branchExists, err := r.refExists(ctx, "refs/remotes/origin/"+targetRevision)
	if err != nil {
		return err
	}

	tagExists, err := r.refExists(ctx, "refs/tags/"+targetRevision)
	if err != nil {
		return err
	}

	isCommit := commitPattern.MatchString(targetRevision)

	if branchExists && tagExists {
		return fmt.Errorf("target revision %q is ambiguous: both branch and tag exist", targetRevision)
	}

	if isCommit && (branchExists || tagExists) {
		return fmt.Errorf("target revision %q is ambiguous: commit format conflicts with existing branch or tag", targetRevision)
	}

	if branchExists {
		if _, err := r.run(ctx, "checkout", "--force", "-B", targetRevision, "refs/remotes/origin/"+targetRevision); err != nil {
			return fmt.Errorf("checking out branch %q: %w", targetRevision, err)
		}

		return nil
	}

	if tagExists {
		if _, err := r.run(ctx, "checkout", "--force", "--detach", "refs/tags/"+targetRevision); err != nil {
			return fmt.Errorf("checking out tag %q: %w", targetRevision, err)
		}

		return nil
	}

	if isCommit {
		resolved, err := r.run(ctx, "rev-parse", "--verify", targetRevision+"^{commit}")
		if err != nil {
			return fmt.Errorf("resolving commit %q: %w", targetRevision, err)
		}

		if _, err := r.run(ctx, "checkout", "--force", "--detach", strings.TrimSpace(resolved)); err != nil {
			return fmt.Errorf("checking out commit %q: %w", targetRevision, err)
		}

		return nil
	}

	return fmt.Errorf("invalid target revision %q: expected HEAD, branch, tag, or commit SHA", targetRevision)
}

func (r *gitRepository) checkoutRemoteDefaultBranch(ctx context.Context) error {
	defaultRef, err := r.run(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return fmt.Errorf("resolving remote default branch: %w", err)
	}

	defaultBranch := strings.TrimPrefix(strings.TrimSpace(defaultRef), "origin/")
	if defaultBranch == "" || defaultBranch == strings.TrimSpace(defaultRef) {
		return fmt.Errorf("remote default branch reference %q is invalid", strings.TrimSpace(defaultRef))
	}

	if _, err := r.run(ctx, "checkout", "--force", "-B", defaultBranch, "refs/remotes/origin/"+defaultBranch); err != nil {
		return fmt.Errorf("checking out remote default branch %q: %w", defaultBranch, err)
	}

	return nil
}

func (r *gitRepository) refExists(ctx context.Context, ref string) (bool, error) {
	_, err := r.run(ctx, "show-ref", "--verify", "--quiet", ref)
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("checking reference %q: %w", ref, err)
}

func (r *gitRepository) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.rootPath}, args...)...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		stderrMessage := sanitizeAndTruncateGitCommandOutput(stderr.String())
		if stderrMessage == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}

		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), stderrMessage, err)
	}

	return stdout.String(), nil
}
