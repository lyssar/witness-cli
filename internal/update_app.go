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

	// === Fix required fields ===
	changed := false

	if app.Spec.Provisioner == "" {
		err = huh.NewSelect[string]().
			Title("Provisioner is missing — select one").
			Options(
				huh.NewOption("Docker Compose", "docker-compose"),
			).
			Value(&app.Spec.Provisioner).
			Run()
		utils.CheckErr(err)
		changed = true
	}

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
		changed = true
	}

	// Write required fixes immediately
	if changed {
		if err := validateAndWrite(manifestPath, &app); err != nil {
			return err
		}
		utils.LogSuccess(fmt.Sprintf("Fixed required fields in %s", manifestPath))
	}

	utils.LogInfo("Current manifest",
		"name", app.Metadata.Name,
		"provisioner", app.Spec.Provisioner,
		"composeFiles", len(app.Spec.ComposeFiles),
		"volumeClaims", len(app.Spec.VolumeClaims),
		"secrets", len(app.Spec.Secrets),
		"registry", app.Spec.RegistryCredentials != nil,
	)

	// === Optional changes loop ===
	for {
		var action string
		err = huh.NewSelect[string]().
			Title("Anything else to update?").
			Options(
				huh.NewOption("Update registry credentials", "registry"),
				huh.NewOption("Add secret", "secret"),
				huh.NewOption("Add volume claim", "volume-claim"),
				huh.NewOption("Add compose file", "compose"),
				huh.NewOption("No, done", "done"),
			).
			Value(&action).
			Run()
		utils.CheckErr(err)

		if action == "done" {
			break
		}

		switch action {
		case "registry":
			utils.CheckErr(updateRegistryCredentials(&app, ageKey))
		case "secret":
			utils.CheckErr(addSecret(&app))
		case "volume-claim":
			utils.CheckErr(addVolumeClaim(&app))
		case "compose":
			utils.CheckErr(addComposeFile(&app))
		}

		utils.CheckErr(validateAndWrite(manifestPath, &app))
		utils.LogSuccess(fmt.Sprintf("Updated %s", manifestPath))
	}

	return nil
}

func validateAndWrite(manifestPath string, app *App) error {
	if err := validateAppManifest(app); err != nil {
		return fmt.Errorf("manifest invalid, not writing: %w", err)
	}

	renderer, err := templates.NewRenderer()
	if err != nil {
		return err
	}

	f, err := os.Create(manifestPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	if err := renderer.Render("application", app, f); err != nil {
		return fmt.Errorf("rendering manifest: %w", err)
	}
	return f.Sync()
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
	for i, claim := range app.Spec.VolumeClaims {
		if err := application.ValidateVolumeClaim(claim); err != nil {
			return fmt.Errorf("volumeClaims[%d]: %w", i, err)
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

func addVolumeClaim(app *App) error {
	var claim application.VolumeClaim

	err := huh.NewInput().
		Title("Volume directory").
		Description("Bind-mount directory name relative to the compose file (e.g., 'data')").
		Validate(huh.ValidateNotEmpty()).
		Value(&claim.Dir).
		Run()
	utils.CheckErr(err)

	var uidStr, gidStr string

	err = huh.NewInput().
		Title("Container UID").
		Description("Numeric UID the container process runs as").
		Validate(func(s string) error {
			if s == "" {
				return fmt.Errorf("uid is required")
			}
			return nil
		}).
		Value(&uidStr).
		Run()
	utils.CheckErr(err)

	err = huh.NewInput().
		Title("Container GID").
		Description("Numeric GID the container process runs as").
		Validate(func(s string) error {
			if s == "" {
				return fmt.Errorf("gid is required")
			}
			return nil
		}).
		Value(&gidStr).
		Run()
	utils.CheckErr(err)

	uid, err := parseUint(uidStr)
	utils.CheckErr(err)
	gid, err := parseUint(gidStr)
	utils.CheckErr(err)

	claim.UID = uid
	claim.GID = gid

	app.Spec.VolumeClaims = append(app.Spec.VolumeClaims, claim)
	return nil
}
