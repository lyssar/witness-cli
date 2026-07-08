package application

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateManifest enforces v1 Application schema and path-safety rules.
func ValidateManifest(app Application) error {
	if app.APIVersion != APIVersionV1Alpha1 {
		return fmt.Errorf("apiVersion must be %q", APIVersionV1Alpha1)
	}

	if app.Kind != KindApplication {
		return fmt.Errorf("kind must be %q", KindApplication)
	}

	if strings.TrimSpace(app.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if app.Spec.Provisioner != ProvisionerDockerCompose {
		return fmt.Errorf("spec.provisioner must be %q", ProvisionerDockerCompose)
	}

	if len(app.Spec.ComposeFiles) == 0 {
		return fmt.Errorf("spec.composeFiles must contain at least one entry")
	}

	composeSeen := make(map[string]struct{}, len(app.Spec.ComposeFiles))
	for i, composeFile := range app.Spec.ComposeFiles {
		normalized, err := normalizeRelativePath(composeFile)
		if err != nil {
			return fmt.Errorf("spec.composeFiles[%d]: %w", i, err)
		}

		if _, ok := composeSeen[normalized]; ok {
			return fmt.Errorf("spec.composeFiles[%d]: duplicate path %q", i, composeFile)
		}
		composeSeen[normalized] = struct{}{}
	}

	secretSources := make(map[string]struct{}, len(app.Spec.Secrets))
	secretTargets := make(map[string]struct{}, len(app.Spec.Secrets))
	normalizedTargets := make([]string, len(app.Spec.Secrets))

	for i, secret := range app.Spec.Secrets {
		source, target, err := validateSecret(secret)
		if err != nil {
			return fmt.Errorf("spec.secrets[%d]: %w", i, err)
		}

		normalizedTargets[i] = target

		if _, ok := secretSources[source]; ok {
			return fmt.Errorf("spec.secrets[%d]: duplicate secret source %q", i, secret.Source)
		}
		secretSources[source] = struct{}{}

		if _, ok := secretTargets[target]; ok {
			return fmt.Errorf("spec.secrets[%d]: duplicate secret target %q", i, secret.Target)
		}
		secretTargets[target] = struct{}{}
	}

	for i, target := range normalizedTargets {
		if _, ok := composeSeen[target]; ok {
			return fmt.Errorf("spec.secrets[%d]: secret target collides with compose file %q", i, app.Spec.Secrets[i].Target)
		}

		if _, ok := secretSources[target]; ok {
			return fmt.Errorf("spec.secrets[%d]: secret target collides with secret source %q", i, app.Spec.Secrets[i].Target)
		}

	}

	return nil
}

func validateSecret(secret Secret) (string, string, error) {
	source, err := normalizeRelativePath(secret.Source)
	if err != nil {
		return "", "", fmt.Errorf("source: %w", err)
	}

	target, err := normalizeRelativePath(secret.Target)
	if err != nil {
		return "", "", fmt.Errorf("target: %w", err)
	}

	if strings.TrimSpace(secret.Decryptor) == "" {
		return "", "", fmt.Errorf("decryptor is required")
	}

	if secret.Decryptor != DecryptorAge {
		return "", "", fmt.Errorf("decryptor must be %q", DecryptorAge)
	}

	return source, target, nil
}

func normalizeRelativePath(pathValue string) (string, error) {
	trimmed := strings.TrimSpace(pathValue)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}

	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("path must be relative")
	}

	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if cleaned == "." || cleaned == ".." {
		return "", fmt.Errorf("path %q is not allowed", pathValue)
	}

	if strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes root", pathValue)
	}

	root := string(filepath.Separator)
	joined := filepath.Join(root, cleaned)
	relative, err := filepath.Rel(root, joined)
	if err != nil {
		return "", fmt.Errorf("normalizing path %q: %w", pathValue, err)
	}

	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes root", pathValue)
	}

	return filepath.ToSlash(cleaned), nil
}
