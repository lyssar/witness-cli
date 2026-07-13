package internal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/creasty/defaults"
	"github.com/lyssar/witness-cli/internal/application"
	"github.com/lyssar/witness-cli/templates"
	"github.com/lyssar/witness-cli/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type App struct {
	APIVersion string               `default:"witness.dev/v1alpha1" yaml:"apiVersion"`
	Kind       string               `default:"Application" yaml:"kind"`
	Metadata   application.Metadata `yaml:"metadata"`
	Spec       application.Spec     `yaml:"spec"`
	AgeKeyFile string               `yaml:"-"`
}

func NewApp(cmd *cobra.Command) App {
	spec := &application.Spec{}
	err := defaults.Set(spec)
	utils.CheckErr(err)

	metadata := &application.Metadata{}
	err = defaults.Set(metadata)
	utils.CheckErr(err)

	app := &App{}
	err = defaults.Set(app)
	utils.CheckErr(err)
	app.Metadata = *metadata
	app.Spec = *spec

	ageKey, err := cmd.Flags().GetString("age-key")
	utils.CheckErr(err)
	if ageKey != "" && utils.FileExists(ageKey) {
		normalizedPath, err := utils.NormalizeHomePath(ageKey)
		utils.CheckErr(err)
		app.AgeKeyFile = normalizedPath
	}

	return *app
}

func NewAppFromManifest(manifestFile, ageKeyFile string) (App, error) {
	app := &App{}
	if !utils.FileExists(manifestFile) {
		return *app, fmt.Errorf("manifest file %s does not exist", manifestFile)
	}

	manifestContent, err := os.ReadFile(manifestFile)

	if err != nil {
		return *app, err
	}

	if err := yaml.Unmarshal(manifestContent, app); err != nil {
		return *app, err
	}

	return *app, nil
}

func (app *App) Configure() {
	if app.AgeKeyFile == "" {
		err := huh.NewInput().
			Title("Path age key to use for secret encryption").
			Value(&app.AgeKeyFile).
			Validate(func(ageKeyFile string) error {
				if !utils.FileExists(ageKeyFile) {
					return errors.New("you must enter an existing age key file with path")
				}
				return nil
			}).
			Run()
		utils.CheckErr(err)

		app.AgeKeyFile, err = utils.NormalizeHomePath(app.AgeKeyFile)
		utils.CheckErr(err)
	}

	err := huh.NewInput().
		Title("App name").
		Description("Name of the app service").
		Value(&app.Metadata.Name).
		Validate(func(projectName string) error {
			if len(projectName) <= 0 {
				return errors.New("you must enter a project name")
			}
			return nil
		}).Run()
	utils.CheckErr(err)

	err = huh.NewSelect[string]().
		Title("Provisioner type").
		Options(
			huh.NewOption("Docker Compose", "docker-compose"),
		).
		Value(&app.Spec.Provisioner).
		Run()
	utils.CheckErr(err)

	for {
		var composeFile string

		err = huh.NewInput().
			Title("Enter a compose file").
			Description("Path to a docker-compose file (relative to app directory). Leave empty if done.").
			Value(&composeFile).
			Run()
		utils.CheckErr(err)

		if composeFile == "" {
			break
		}

		app.Spec.ComposeFiles = append(app.Spec.ComposeFiles, composeFile)
	}

	var addSecrets bool
	err = huh.NewConfirm().
		Title("Add secrets?").
		Description("Secrets are encrypted files that will be decrypted at runtime using the age key").
		Value(&addSecrets).
		Run()
	utils.CheckErr(err)

	if addSecrets {
		for {
			var secret application.Secret
			secret.Decryptor = "age"

			err = huh.NewInput().
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

			err = huh.NewSelect[string]().
				Title("Decryptor").
				Options(
					huh.NewOption("Age", "age"),
				).
				Value(&secret.Decryptor).
				Run()
			utils.CheckErr(err)

			app.Spec.Secrets = append(app.Spec.Secrets, secret)

			var addAnother bool
			err = huh.NewConfirm().
				Title("Add another secret?").
				Value(&addAnother).
				Run()
			utils.CheckErr(err)

			if !addAnother {
				break
			}
		}
	}

	var addVolumeClaims bool
	err = huh.NewConfirm().
		Title("Add volume claims?").
		Description("Volume claims set ownership on bind-mount directories for containers with hardcoded UIDs. Required when the container cannot write to its data directory.").
		Value(&addVolumeClaims).
		Run()
	utils.CheckErr(err)

	if addVolumeClaims {
		for {
			var claim application.VolumeClaim

			err = huh.NewInput().
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
						return errors.New("uid is required")
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
						return errors.New("gid is required")
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

			var addAnother bool
			err = huh.NewConfirm().
				Title("Add another volume claim?").
				Value(&addAnother).
				Run()
			utils.CheckErr(err)

			if !addAnother {
				break
			}
		}
	}

	var addRegistry bool
	err = huh.NewConfirm().
		Title("Add private registry credentials?").
		Description("Registry credentials allow pulling images from private Docker registries").
		Value(&addRegistry).
		Run()
	utils.CheckErr(err)

	if addRegistry {
		creds := &application.RegistryCredentials{}

		err = huh.NewInput().
			Title("Registry URL").
			Description("Docker registry URL (e.g., docker.io, ghcr.io)").
			Validate(huh.ValidateNotEmpty()).
			Value(&creds.Registry).
			Run()
		utils.CheckErr(err)

		err = huh.NewInput().
			Title("Registry username").
			Validate(huh.ValidateNotEmpty()).
			Value(&creds.Username).
			Run()
		utils.CheckErr(err)

		err = huh.NewInput().
			Title("Registry password").
			EchoMode(huh.EchoModePassword).
			Validate(huh.ValidateNotEmpty()).
			Value(&creds.Password).
			Run()
		utils.CheckErr(err)

		encryptedPassword, err := utils.EncryptSecret(creds.Password, app.AgeKeyFile)
		utils.CheckErr(err)
		creds.Password = encryptedPassword

		app.Spec.RegistryCredentials = creds
	}
}

func (app *App) WriteConfig() {
	renderer, err := templates.NewRenderer()
	utils.CheckErr(err)

	pwd, err := os.Getwd()
	utils.CheckErr(err)

	pathOut := filepath.Join(pwd, application.ManifestFileName)
	f, err := os.Create(pathOut)
	utils.CheckErr(err)

	defer func() {
		err := f.Close()
		utils.CheckErr(err)
	}()

	err = renderer.Render("application", app, f)
	utils.CheckErr(err)
	err = f.Sync()
	utils.CheckErr(err)
}

// parseUint converts a decimal string to an int. Negative values are rejected.
func parseUint(s string) (int, error) {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("must be a non-negative integer: %w", err)
	}
	return int(v), nil
}
