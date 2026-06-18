package internal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/creasty/defaults"
	"github.com/lyssar/skuld-cli/templates"
	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type AppSpec struct {
	Handler     string    `yaml:"-"`
	Inventory   []string  `yaml:"inventory"`
	SecretFiles []string  `yaml:"secretFiles"`
	Source      GitSource `yaml:"source"`
}

type App struct {
	ApiVersion string   `default:"skuld/v1alpha1" yaml:"apiVersion"`
	Kind       string   `default:"App" yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       AppSpec  `yaml:"spec"`
	AgeKeyFile string   `yaml:"-"`
}

func NewApp(cmd *cobra.Command) App {
	gitSource := &GitSource{}
	err := defaults.Set(gitSource)
	utils.CheckErr(err)

	spec := &AppSpec{}
	err = defaults.Set(spec)
	utils.CheckErr(err)
	spec.Source = *gitSource

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
		Title("Handler type").
		Options(
			huh.NewOption("Docker", "docker"),
			huh.NewOption("Docker Compose", "docker-compose"),
			huh.NewOption("Podman Compose", "podman-compose"),
			huh.NewOption("Shell", "shell"),
		).
		Value(&app.Spec.Handler).
		Run()
	utils.CheckErr(err)

	err = huh.NewInput().
		Title("Repository url").
		Description("The repository to reconcile from").
		Value(&app.Spec.Source.RepoURL).
		Validate(func(repoUrl string) error {
			if len(repoUrl) <= 0 {
				return errors.New("you must enter a existing git repository")
			}
			return nil
		}).
		Run()
	utils.CheckErr(err)

	app.Spec.Source.Path = strings.ReplaceAll(strings.ToLower(app.Metadata.Name), " ", "-")

	err = huh.NewInput().
		Title("App destination").
		Description("App target destination in repo").
		Value(&app.Spec.Source.Path).
		Validate(func(revision string) error {
			if len(revision) <= 0 {
				return errors.New("you must enter a revision")
			}
			return nil
		}).
		Run()
	utils.CheckErr(err)

	err = huh.NewInput().
		Title("Target revision").
		Description("Target git revision to reconcile from").
		Value(&app.Spec.Source.TargetRevision).
		Validate(func(revision string) error {
			if len(revision) <= 0 {
				return errors.New("you must enter a revision")
			}
			return nil
		}).
		Run()
	utils.CheckErr(err)

	err = huh.NewInput().
		Title("Git User").
		Description("The use to use fetch changes on reconcilation").
		Value(&app.Spec.Source.User).
		Validate(func(gitUser string) error {
			if len(gitUser) <= 0 {
				return errors.New("you must enter a username")
			}
			return nil
		}).
		Run()
	utils.CheckErr(err)

	err = huh.NewInput().
		Title("Git access token").
		Description("The access token to use fetch changes on reconcilation").
		EchoMode(huh.EchoModePassword).
		Value(&app.Spec.Source.AccessToken).
		Validate(func(gitAccessToken string) error {
			if len(gitAccessToken) <= 0 {
				return errors.New("you must enter a git access token")
			}
			return nil
		}).
		Run()
	utils.CheckErr(err)

	for {
		var inventoryEntry string

		err = huh.NewInput().
			Title("Enter an inventory item to pull file or folder is possible").
			Description("Leave empty if nothing to add").
			Value(&inventoryEntry).
			Run()
		utils.CheckErr(err)

		if inventoryEntry == "" {
			break
		}

		app.Spec.Inventory = append(app.Spec.Inventory, inventoryEntry)
	}

	var addSecretFile bool
	err = huh.NewConfirm().
		Title("Add add secret files?").
		Description("Secret files are handled like the inventory but it is asumed to be encrypted with the age key").
		Value(&addSecretFile).
		Run()
	utils.CheckErr(err)

	if addSecretFile {
		for {
			var file string

			err = huh.NewInput().
				Title("File").
				Validate(huh.ValidateNotEmpty()).
				Value(&file).
				Run()
			utils.CheckErr(err)

			app.Spec.SecretFiles = append(app.Spec.SecretFiles, file)

			var addAnother bool
			err = huh.NewConfirm().
				Title("Add another secret file?").
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
