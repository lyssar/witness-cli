package internal

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/templates"
	"github.com/lyssar/witness-cli/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func UpdateAppCmd(cmd *cobra.Command, args []string) error {
	var manifestPath string

	err := huh.NewInput().
		Title("Path to witness.yaml").
		Description("Path to the application manifest to update").
		Value(&manifestPath).
		Validate(func(path string) error {
			if !utils.FileExists(path) {
				return errors.New("manifest file does not exist")
			}
			return nil
		}).
		Run()
	utils.CheckErr(err)

	ageKey, err := cmd.Flags().GetString("age-key")
	utils.CheckErr(err)
	if ageKey == "" {
		err = huh.NewInput().
			Title("Path to age key file").
			Description("Used to encrypt registry password").
			Value(&ageKey).
			Validate(func(path string) error {
				if !utils.FileExists(path) {
					return errors.New("age key file does not exist")
				}
				return nil
			}).
			Run()
		utils.CheckErr(err)
	}

	ageKey, err = utils.NormalizeHomePath(ageKey)
	utils.CheckErr(err)

	// Read existing manifest
	content, err := os.ReadFile(manifestPath)
	utils.CheckErr(err)

	var app App
	if err := yaml.Unmarshal(content, &app); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	// === Validate and fix required fields ===

	// Provisioner — required
	if app.Spec.Provisioner == "" {
		err = huh.NewSelect[string]().
			Title("Provisioner is missing — select one").
			Options(
				huh.NewOption("Docker Compose", "docker-compose"),
			).
			Value(&app.Spec.Provisioner).
			Run()
		utils.CheckErr(err)
	}

	// ComposeFiles — required, must have at least one
	if len(app.Spec.ComposeFiles) == 0 {
		var composeFile string
		err = huh.NewInput().
			Title("At least one compose file is required").
			Description("Path to docker-compose file (relative to app directory)").
			Validate(huh.ValidateNotEmpty()).
			Value(&composeFile).
			Run()
		utils.CheckErr(err)
		app.Spec.ComposeFiles = append(app.Spec.ComposeFiles, composeFile)
	}

	utils.LogInfo("Current manifest",
		"name", app.Metadata.Name,
		"provisioner", app.Spec.Provisioner,
		"composeFiles", len(app.Spec.ComposeFiles),
		"secrets", len(app.Spec.Secrets),
		"registry", app.Spec.RegistryCredentials != nil,
	)

	// === Ask what to update ===
	var actions []string
	actionOptions := []huh.Option[string]{
		huh.NewOption("Update registry credentials", "registry"),
		huh.NewOption("Add secret", "secret"),
		huh.NewOption("Add compose file", "compose"),
		huh.NewOption("Done", "done"),
	}

	for {
		var action string
		err = huh.NewSelect[string]().
			Title("What do you want to do?").
			Options(actionOptions...).
			Value(&action).
			Run()
		utils.CheckErr(err)

		if action == "done" {
			break
		}

		switch action {
		case "registry":
			if err := updateRegistryCredentials(&app, ageKey); err != nil {
				return err
			}
		case "secret":
			if err := addSecret(&app); err != nil {
				return err
			}
		case "compose":
			if err := addComposeFile(&app); err != nil {
				return err
			}
		}

		actions = append(actions, action)
	}

	if len(actions) == 0 {
		utils.LogInfo("No changes made")
		return nil
	}

	// === Validate before writing ===
	if err := validateAppManifest(&app); err != nil {
		return fmt.Errorf("manifest invalid, not writing: %w", err)
	}

	// === Write ===
	renderer, err := templates.NewRenderer()
	utils.CheckErr(err)

	f, err := os.Create(manifestPath)
	utils.CheckErr(err)
	defer func() {
		_ = f.Close()
	}()

	if err := renderer.Render("application", &app, f); err != nil {
		return fmt.Errorf("rendering manifest: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing manifest: %w", err)
	}

	utils.LogSuccess(fmt.Sprintf("Updated %s (%d changes)", manifestPath, len(actions)))
	return nil
}

func validateAppManifest(app *App) error {
	if app.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if app.Spec.Provisioner == "" {
		return fmt.Errorf("spec.provisioner is required")
	}
	if len(app.Spec.ComposeFiles) == 0 {
		return fmt.Errorf("spec.composeFiles must have at least one entry")
	}
	if app.Spec.RegistryCredentials != nil {
		creds := app.Spec.RegistryCredentials
		if creds.Registry == "" {
			return fmt.Errorf("registryCredentials.registry is required")
		}
		if creds.Username == "" {
			return fmt.Errorf("registryCredentials.username is required")
		}
		if creds.Password == "" {
			return fmt.Errorf("registryCredentials.password is required")
		}
	}
	for i, secret := range app.Spec.Secrets {
		if secret.Source == "" {
			return fmt.Errorf("secrets[%d].source is required", i)
		}
		if secret.Target == "" {
			return fmt.Errorf("secrets[%d].target is required", i)
		}
		if secret.Decryptor == "" {
			return fmt.Errorf("secrets[%d].decryptor is required", i)
		}
	}
	return nil
}

func updateRegistryCredentials(app *App, ageKey string) error {
	creds := &application.RegistryCredentials{}

	if app.Spec.RegistryCredentials != nil {
		creds = app.Spec.RegistryCredentials
		utils.LogInfo("Current registry", "registry", creds.Registry, "username", creds.Username)
	}

	err := huh.NewInput().
		Title("Registry URL").
		Description("Docker registry URL (e.g., docker.io, ghcr.io)").
		Value(&creds.Registry).
		Validate(huh.ValidateNotEmpty()).
		Run()
	utils.CheckErr(err)

	err = huh.NewInput().
		Title("Registry username").
		Value(&creds.Username).
		Validate(huh.ValidateNotEmpty()).
		Run()
	utils.CheckErr(err)

	var password string
	err = huh.NewInput().
		Title("Registry password").
		EchoMode(huh.EchoModePassword).
		Value(&password).
		Validate(huh.ValidateNotEmpty()).
		Run()
	utils.CheckErr(err)

	encryptedPassword, err := utils.EncryptSecret(password, ageKey)
	utils.CheckErr(err)
	creds.Password = encryptedPassword

	app.Spec.RegistryCredentials = creds
	return nil
}

func addSecret(app *App) error {
	secret := application.Secret{Decryptor: "age"}

	err := huh.NewInput().
		Title("Secret source file").
		Description("Path to the encrypted secret file (relative to app directory)").
		Validate(huh.ValidateNotEmpty()).
		Value(&secret.Source).
		Run()
	utils.CheckErr(err)

	err = huh.NewInput().
		Title("Secret target file").
		Description("Path where the decrypted secret will be written (relative to app directory)").
		Validate(huh.ValidateNotEmpty()).
		Value(&secret.Target).
		Run()
	utils.CheckErr(err)

	app.Spec.Secrets = append(app.Spec.Secrets, secret)
	return nil
}

func addComposeFile(app *App) error {
	var composeFile string

	err := huh.NewInput().
		Title("Compose file path").
		Description("Path to docker-compose file (relative to app directory)").
		Value(&composeFile).
		Validate(huh.ValidateNotEmpty()).
		Run()
	utils.CheckErr(err)

	app.Spec.ComposeFiles = append(app.Spec.ComposeFiles, composeFile)
	return nil
}
