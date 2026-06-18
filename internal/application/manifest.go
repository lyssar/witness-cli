package application

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadManifest decodes the manifest strictly and rejects unknown YAML fields.
func LoadManifest(manifestPath string) (Application, error) {
	var app Application

	if filepath.Base(manifestPath) != ManifestFileName {
		return app, fmt.Errorf("loading manifest %q: filename must be %s", manifestPath, ManifestFileName)
	}

	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return app, fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	if err := decoder.Decode(&app); err != nil {
		return app, fmt.Errorf("decoding manifest %q: %w", manifestPath, err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return app, fmt.Errorf("decoding manifest %q: multiple YAML documents are not allowed", manifestPath)
	} else if err != io.EOF {
		return app, fmt.Errorf("decoding manifest %q trailing document: %w", manifestPath, err)
	}

	return app, nil
}

// ParseManifest loads and validates one Application manifest.
func ParseManifest(manifestPath string) (Application, error) {
	app, err := LoadManifest(manifestPath)
	if err != nil {
		return Application{}, err
	}

	if err := ValidateManifest(app); err != nil {
		return Application{}, fmt.Errorf("validating manifest %q: %w", manifestPath, err)
	}

	if err := validateComposeFilesExist(manifestPath, app); err != nil {
		return Application{}, err
	}

	return app, nil
}

func validateComposeFilesExist(manifestPath string, app Application) error {
	manifestDir := filepath.Dir(manifestPath)

	for i, composeFile := range app.Spec.ComposeFiles {
		normalized, err := normalizeRelativePath(composeFile)
		if err != nil {
			return fmt.Errorf("validating manifest %q composeFiles[%d]: %w", manifestPath, i, err)
		}

		composePath := filepath.Join(manifestDir, filepath.FromSlash(normalized))
		stat, err := os.Stat(composePath)
		if err != nil {
			return fmt.Errorf("validating manifest %q composeFiles[%d]: file %q does not exist: %w", manifestPath, i, composeFile, err)
		}

		if !stat.Mode().IsRegular() {
			return fmt.Errorf("validating manifest %q composeFiles[%d]: path %q must be a regular file", manifestPath, i, composeFile)
		}
	}

	return nil
}
