package internal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/creasty/defaults"
	"github.com/lyssar/skuld-cli/templates"
	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type v1Secret struct {
	Source    string `yaml:"source"`
	Target    string `yaml:"target"`
	Decryptor string `yaml:"decryptor"`
}

type AppSpec struct {
	Provisioner  string     `yaml:"-"`
	ComposeFiles []string   `yaml:"composeFiles"`
	Secrets      []v1Secret `yaml:"secrets,omitempty"`
}

type App struct {
	ApiVersion string   `default:"skuld.dev/v1alpha1" yaml:"apiVersion"`
	Kind       string   `default:"Application" yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       AppSpec  `yaml:"spec"`
	AgeKeyFile string   `yaml:"-"`
}

func NewApp(cmd *cobra.Command) App {
	spec := &AppSpec{}
	err := defaults.Set(spec)
	utils.CheckErr(err)

	metadata := &Metadata{}
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
			var secret v1Secret
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
}

func (app *App) WriteConfig() {
	renderer, err := templates.NewRenderer()
	utils.CheckErr(err)

	pwd, err := os.Getwd()
	utils.CheckErr(err)

	pathOut := filepath.Join(pwd, "skuld-app.yaml")
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
