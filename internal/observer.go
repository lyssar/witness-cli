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
)

type GitSource struct {
	RepoURL        string
	TargetRevision string
	Path           string
}

type Spec struct {
	Project     string
	Handler     string
	Source      GitSource
	Destination string
}

type Observer struct {
	ApiVersion string            `default:"skuld/v1alpha1" yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Spec       Spec              `yaml:"spec"`
	Secrets    map[string]string `yaml:"secrets"`
	AgeKeyFile string            `yaml:"-"`
}

func NewObserver() Observer {
	gitSource := GitSource{
		RepoURL:        "",
		TargetRevision: "HEAD",
		Path:           "/",
	}

	spec := Spec{
		Project:     "",
		Handler:     "",
		Source:      gitSource,
		Destination: "",
	}

	observer := Observer{
		ApiVersion: "skuld/v1alpha1",
		Kind:       "Observer",
		Spec:       spec,
		Secrets:    map[string]string{},
		AgeKeyFile: "",
	}
	return observer
}

func (observer *Observer) Configure() {
	huh.NewInput().
		Title("Path age key to use for secret encryption").
		Value(&observer.AgeKeyFile).
		Validate(func(ageKeyFile string) error {
			_, error := os.Stat(ageKeyFile)
			if os.IsNotExist(error) {
				return errors.New("you must enter an existing age key file with path")
			}
			return nil
		}).
		Run()

	huh.NewInput().
		Title("[Spec] Observer name").
		Value(&observer.Spec.Project).
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
		Title("[Spec] Root destination path").
		Value(&observer.Spec.Destination).
		Validate(func(destination string) error {
			if len(destination) <= 0 {
				return errors.New("you must enter an absolut path for destination. [default: '/']")
			}
			return nil
		}).
		Run()

	huh.NewInput().
		Title("[Source] Repository url").
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
		Title("[Source] Target revision").
		Value(&observer.Spec.Source.TargetRevision).
		Validate(func(revision string) error {
			if len(revision) <= 0 {
				return errors.New("you must enter a revision [default: 'HEAD']")
			}
			return nil
		}).
		Run()

	for {
		var secretKey string
		var secretValue string

		secretForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Secret Name").
					Value(&secretKey),
				huh.NewInput().
					Title("Secret Value").
					Value(&secretValue),
			),
		)
		if err := secretForm.Run(); err != nil {
			break
		}

		if secretKey == "" || secretValue == "" {
			break
		}
		observer.Secrets[secretKey] = secretValue
	}
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
