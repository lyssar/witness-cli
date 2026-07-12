package reconcile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lyssar/witness-cli/internal/application"
)

// DriftResult captures drift findings for managed files and secret targets.
type DriftResult struct {
	HasDrift              bool
	Changed               []string
	Missing               []string
	TypeMismatch          []string
	SecretChanged         []string
	DeferredSecretTargets []string
	Warnings              []application.FilesetWarning
}

func detectDrift(destinationRoot string, app application.DiscoveredApplication, fileset application.FileSet, staging AppStaging) (DriftResult, error) {
	destination, err := openDestinationFS(destinationRoot)
	if err != nil {
		return DriftResult{}, err
	}
	defer func() {
		if closeErr := destination.Close(); closeErr != nil {
			return
		}
	}()
	return detectRootedDrift(destination, app, fileset, staging)
}

func detectRootedDrift(destination *destinationFS, app application.DiscoveredApplication, fileset application.FileSet, staging AppStaging) (DriftResult, error) {
	liveAppDir, err := liveRef(app.OperationalID)
	if err != nil {
		return DriftResult{}, err
	}
	if err := destination.ensureNoSymlinkAncestry(liveAppDir); err != nil {
		return DriftResult{}, err
	}

	result := DriftResult{
		DeferredSecretTargets: append([]string(nil), fileset.DeferredSecretTargets...),
		Warnings:              append([]application.FilesetWarning(nil), fileset.Warnings...),
	}

	liveInfo, err := destination.root.Stat(liveAppDir)
	if err != nil {
		if os.IsNotExist(err) {
			for _, f := range fileset.ManagedFiles {
				result.Missing = append(result.Missing, f.RelativePath)
			}
			result.SecretChanged = append(result.SecretChanged, fileset.DeferredSecretTargets...)
			result.HasDrift = len(result.Missing) > 0 || len(result.SecretChanged) > 0
			return result, nil
		}
		return DriftResult{}, fmt.Errorf("stat rooted live app dir %q: %w", liveAppDir, err)
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
		liveEntry, err := destination.root.Lstat(livePath)
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

		same, err := filesEqualRooted(stagedPath, destination, livePath)
		if err != nil {
			return DriftResult{}, fmt.Errorf("comparing %q: %w", managed.RelativePath, err)
		}
		if !same {
			result.Changed = append(result.Changed, managed.RelativePath)
		}
	}

	// Compare secret targets — explicit drift detection for decrypted secrets.
	for _, secretTarget := range fileset.DeferredSecretTargets {
		stagedPath := filepath.Join(staging.Root, filepath.FromSlash(secretTarget))
		if _, err := os.Stat(stagedPath); err != nil {
			if os.IsNotExist(err) {
				// Staged secret target missing — skip comparison.
				continue
			}
			return DriftResult{}, fmt.Errorf("stat staging secret target %q: %w", secretTarget, err)
		}

		livePath := filepath.Join(liveAppDir, filepath.FromSlash(secretTarget))
		liveEntry, err := destination.root.Lstat(livePath)
		if err != nil {
			if os.IsNotExist(err) {
				result.SecretChanged = append(result.SecretChanged, secretTarget)
				continue
			}
			return DriftResult{}, fmt.Errorf("lstat live secret target %q: %w", livePath, err)
		}

		if !liveEntry.Mode().IsRegular() {
			result.SecretChanged = append(result.SecretChanged, secretTarget)
			continue
		}

		same, err := filesEqualRooted(stagedPath, destination, livePath)
		if err != nil {
			return DriftResult{}, fmt.Errorf("comparing secret %q: %w", secretTarget, err)
		}
		if !same {
			result.SecretChanged = append(result.SecretChanged, secretTarget)
		}
	}

	result.HasDrift = len(result.Changed) > 0 || len(result.Missing) > 0 || len(result.TypeMismatch) > 0 || len(result.SecretChanged) > 0
	return result, nil
}

func filesEqualRooted(pathA string, destination *destinationFS, pathB string) (bool, error) {
	fileA, err := os.Open(pathA)
	if err != nil {
		return false, fmt.Errorf("open %q: %w", pathA, err)
	}
	defer func() {
		_ = fileA.Close()
	}()

	fileB, err := destination.root.Open(pathB)
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
