package internal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/lyssar/skuld-cli/templates"
	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
)

type GitSource struct {
	RepoURL        string
	TargetRevision string
	Path           string
	User           string
	AccessToken    string
}

type Spec struct {
	Project     string
	User        string
	Handler     string
	Source      GitSource
	Destination string
}

type Observer struct {
	ApiVersion string `default:"skuld/v1alpha1" yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Spec       Spec   `yaml:"spec"`
	AgeKeyFile string `yaml:"-"`
}

func NewObserver(cmd *cobra.Command) Observer {
	gitSource := GitSource{
		RepoURL:        "",
		TargetRevision: "HEAD",
		Path:           "/",
		User:           "",
		AccessToken:    "",
	}

	spec := Spec{
		Project:     "",
		User:        "",
		Handler:     "",
		Source:      gitSource,
		Destination: "",
	}

	observer := Observer{
		ApiVersion: "skuld/v1alpha1",
		Kind:       "Observer",
		Spec:       spec,
	}

	ageKey, err := cmd.Flags().GetString("age-key")
	utils.CheckErr(err)
	if ageKey != "" && utils.FileExists(ageKey) {
		normalizedPath, err := utils.NormalizeHomePath(ageKey)
		utils.CheckErr(err)
		observer.AgeKeyFile = normalizedPath
	}

	return observer
}

func (observer *Observer) Configure() {
	if observer.AgeKeyFile == "" {
		huh.NewInput().
			Title("Path age key to use for secret encryption").
			Value(&observer.AgeKeyFile).
			Validate(func(ageKeyFile string) error {
				if !utils.FileExists(ageKeyFile) {
					return errors.New("you must enter an existing age key file with path")
				}
				return nil
			}).
			Run()
		var err error
		observer.AgeKeyFile, err = utils.NormalizeHomePath(observer.AgeKeyFile)
		utils.CheckErr(err)
	}

	huh.NewInput().
		Title("Observer name").
		Description("Name of the observer service").
		Value(&observer.Spec.Project).
		Validate(func(projectName string) error {
			if len(projectName) <= 0 {
				return errors.New("you must enter a project name")
			}
			return nil
		}).Run()

	huh.NewInput().
		Title("Execution user").
		Description("User which is used to run the reconcile with. Must have read/write access to .spec.destination").
		Value(&observer.Spec.User).
		Validate(func(projectName string) error {
			if len(projectName) <= 0 {
				return errors.New("you must enter a project name")
			}
			return nil
		}).
		Run()

	// TODO use later for Application
	// huh.NewSelect[string]().
	// 	Title("Handler type").
	//     Options(
	//     	huh.NewOption("Docker", "docker"),
	//         huh.NewOption("Docker Compose", "docker-compose"),
	//         huh.NewOption("Podman Compose", "podman-compose"),
	//         huh.NewOption("Shell", "shell"),
	//     ).
	//     Value(&observer.Spec.Handler)

	huh.NewInput().
		Title("Root destination path").
		Description("The root path to sync the repository into").
		Value(&observer.Spec.Destination).
		Validate(func(destination string) error {
			if len(destination) <= 0 {
				return errors.New("you must enter an absolut path for destination.")
			}
			return nil
		}).
		Run()

	huh.NewInput().
		Title("Repository url").
		Description("The repository to reconcile from").
		Value(&observer.Spec.Source.RepoURL).
		Validate(func(repoUrl string) error {
			if len(repoUrl) <= 0 {
				return errors.New("you must enter a existing git repository")
			}
			return nil
		}).
		Run()

		// TODO use later for application
		// huh.NewInput().
		//   		Title("[Source] Path").
		//    	Value(&observer.Spec.Source.Path).
		//   		Validate(func(path string) error {
		//   			if len(path) <= 0 {
	//   				return errors.New("you must enter a existing git repository")
	//   			}
	//   			return nil
	//   		}).
	//   		Run()

	huh.NewInput().
		Title("Target revision").
		Description("Target git revision to reconcile from").
		Value(&observer.Spec.Source.TargetRevision).
		Validate(func(revision string) error {
			if len(revision) <= 0 {
				return errors.New("you must enter a revision")
			}
			return nil
		}).
		Run()

	huh.NewInput().
		Title("Git User").
		Description("The use to use fetch changes on reconcilation").
		Value(&observer.Spec.Source.User).
		Validate(func(gitUser string) error {
			if len(gitUser) <= 0 {
				return errors.New("you must enter a username")
			}
			return nil
		}).
		Run()

	huh.NewInput().
		Title("Git access token").
		Description("The access token to use fetch changes on reconcilation").
		EchoMode(huh.EchoModePassword).
		Value(&observer.Spec.Source.AccessToken).
		Validate(func(gitAccessToken string) error {
			if len(gitAccessToken) <= 0 {
				return errors.New("you must enter a git access token")
			}
			return nil
		}).
		Run()
	// for {
	// 	var secretKey string
	// 	var secretValue string

	// 	secretForm := huh.NewForm(
	// 		huh.NewGroup(
	// 			huh.NewInput().
	// 				Title("Secret Name").
	// 				Value(&secretKey),
	// 			huh.NewInput().
	// 				Title("Secret Value").
	// 				Value(&secretValue),
	// 		),
	// 	)
	// 	if err := secretForm.Run(); err != nil {
	// 		break
	// 	}

	// 	if secretKey == "" || secretValue == "" {
	// 		break
	// 	}
	// 	observer.Secrets[secretKey] = secretValue
	// } FIXME: Reuse later if needed
}

func (observer *Observer) WriteConfig() {
	renderer, err := templates.NewRenderer()
	utils.CheckErr(err)

	pwd, err := os.Getwd()
	utils.CheckErr(err)

	pathOut := filepath.Join(pwd, fmt.Sprintf("%s.yaml", strings.ToLower(observer.Spec.Project)))
	f, err := os.Create(pathOut)
	utils.CheckErr(err)

	defer f.Close()

	err = renderer.Render("observer", observer, f)
	utils.CheckErr(err)
	f.Sync()
}
