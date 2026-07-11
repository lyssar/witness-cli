package reconcile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	s "strings"

	"github.com/lyssar/witness-cli/internal/application"
)

// DriftResult captures non-secret drift findings.
type DriftResult struct {
	HasDrift              bool
	Changed               []string
	Missing               []string
	TypeMismatch          []string
	DeferredSecretTargets []string
	Warnings              []application.FilesetWarning
}

func detectDrift(destinationRoot string, app application.DiscoveredApplication, fileset application.FileSet, staging AppStaging) (DriftResult, error) {
	liveAppDir, err := safeJoinUnder(destinationRoot, filepath.FromSlash(app.OperationalID))
	if err != nil {
		return DriftResult{}, err
	}

	result := DriftResult{
		DeferredSecretTargets: append([]string(nil), fileset.DeferredSecretTargets...),
		Warnings:              append([]application.FilesetWarning(nil), fileset.Warnings...),
	}

	liveInfo, err := os.Stat(liveAppDir)
	if err != nil {
		if os.IsNotExist(err) {
			for _, f := range fileset.ManagedFiles {
				result.Missing = append(result.Missing, f.RelativePath)
			}
			result.HasDrift = len(result.Missing) > 0
			return result, nil
		}
		return DriftResult{}, fmt.Errorf("stat live app dir %q: %w", liveAppDir, err)
	}

	if !liveInfo.IsDir() {
		return DriftResult{}, fmt.Errorf("live app dir %q is not a directory", liveAppDir)
	}

	for _, managed := range fileset.ManagedFiles {
		stagedPath := filepath.Join(staging.Root, filepath.FromSlash(managed.RelativePath))
		if _, err := os.Stat(stagedPath); err != nil {
			if os.IsNotExist(err) {
				return DriftResult{}, fmt.Errorf("staging file missing for %q", managed.RelativePath)
			}
			return DriftResult{}, fmt.Errorf("stat staging file %q: %w", managed.RelativePath, err)
		}

		livePath := filepath.Join(liveAppDir, filepath.FromSlash(managed.RelativePath))
		liveEntry, err := os.Lstat(livePath)
		if err != nil {
			if os.IsNotExist(err) {
				result.Missing = append(result.Missing, managed.RelativePath)
				continue
			}
			return DriftResult{}, fmt.Errorf("lstat live file %q: %w", livePath, err)
		}

		if !liveEntry.Mode().IsRegular() {
			result.TypeMismatch = append(result.TypeMismatch, managed.RelativePath)
			continue
		}

		same, err := filesEqual(stagedPath, livePath)
		if err != nil {
			return DriftResult{}, fmt.Errorf("comparing %q: %w", managed.RelativePath, err)
		}
		if !same {
			result.Changed = append(result.Changed, managed.RelativePath)
		}
	}

	result.HasDrift = len(result.Changed) > 0 || len(result.Missing) > 0 || len(result.TypeMismatch) > 0
	return result, nil
}

func safeJoinUnder(base, relative string) (string, error) {
	joined := filepath.Join(base, relative)
	cleanBase := filepath.Clean(base)
	cleanJoined := filepath.Clean(joined)
	if cleanJoined != cleanBase && !s.HasPrefix(cleanJoined, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes base %q", relative, base)
	}
	return cleanJoined, nil
}

func filesEqual(pathA, pathB string) (bool, error) {
	fileA, err := os.Open(pathA)
	if err != nil {
		return false, fmt.Errorf("open %q: %w", pathA, err)
	}
	defer func() {
		_ = fileA.Close()
	}()

	fileB, err := os.Open(pathB)
	if err != nil {
		return false, fmt.Errorf("open %q: %w", pathB, err)
	}
	defer func() {
		_ = fileB.Close()
	}()

	bufA := make([]byte, 32*1024)
	bufB := make([]byte, 32*1024)
	for {
		nA, errA := fileA.Read(bufA)
		nB, errB := fileB.Read(bufB)

		if nA != nB || !bytes.Equal(bufA[:nA], bufB[:nB]) {
			return false, nil
		}

		if errA == io.EOF && errB == io.EOF {
			return true, nil
		}

		if errA != nil && errA != io.EOF {
			return false, errA
		}
		if errB != nil && errB != io.EOF {
			return false, errB
		}
	}
}
