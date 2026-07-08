package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/charmbracelet/huh"
	"github.com/creasty/defaults"
	"github.com/lyssar/skuld-cli/templates"
	"github.com/lyssar/skuld-cli/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type GitSource struct {
	RepoURL        string `yaml:"repoURL"`
	TargetRevision string `default:"HEAD" yaml:"targetRevision"`
	Path           string `default:"/" yaml:"path"`
	User           string `yaml:"user"`
	AccessToken    string `yaml:"accessToken"`
}

type Metadata struct {
	Name string `yaml:"name"`
	User string `yaml:"user"`
}

type Timeout struct {
	Reconciliation time.Duration `default:"180s" yaml:"reconciliation"`
}

type Spec struct {
	Project     string    `yaml:"project"`
	Destination string    `yaml:"destination"`
	Handler     string    `yaml:"-"`
	Source      GitSource `yaml:"source"`
	Timeout     Timeout   `yaml:"timeout"`
}

type Observer struct {
	ApiVersion string   `default:"skuld/v1alpha1" yaml:"apiVersion"`
	Kind       string   `default:"Observer" yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
	AgeKeyFile string   `yaml:"-"`
}

func (observer Observer) FullServicePath() string {
	return fmt.Sprintf("/etc/systemd/system/%s.service", strings.ToLower(observer.Spec.Project))
}

func (observer Observer) FullServiceTimerPath() string {
	return fmt.Sprintf("/etc/systemd/system/%s.timer", strings.ToLower(observer.Spec.Project))
}

func NewObserver(cmd *cobra.Command) Observer {
	gitSource := &GitSource{}
	err := defaults.Set(gitSource)
	utils.CheckErr(err)

	spec := &Spec{}
	err = defaults.Set(spec)
	utils.CheckErr(err)
	spec.Source = *gitSource

	metadata := &Metadata{}
	err = defaults.Set(metadata)
	utils.CheckErr(err)

	timeout := &Timeout{}
	err = defaults.Set(timeout)
	utils.CheckErr(err)

	observer := &Observer{}
	err = defaults.Set(observer)
	utils.CheckErr(err)
	observer.Metadata = *metadata
	observer.Spec.Timeout = *timeout
	observer.Spec = *spec

	ageKey, err := cmd.Flags().GetString("age-key")
	utils.CheckErr(err)
	if ageKey != "" && utils.FileExists(ageKey) {
		normalizedPath, err := utils.NormalizeHomePath(ageKey)
		utils.CheckErr(err)
		observer.AgeKeyFile = normalizedPath
	}

	return *observer
}

func NewObserverFromManifest(manifestFile, ageKeyFile string) (Observer, error) {
	observer := &Observer{}
	if !utils.FileExists(manifestFile) {
		return *observer, fmt.Errorf("manifest file %s does not exist", manifestFile)
	}

	manifestContent, err := os.ReadFile(manifestFile)

	if err != nil {
		return *observer, err
	}

	if err := yaml.Unmarshal(manifestContent, observer); err != nil {
		return *observer, err
	}

	return *observer, nil
}

func (observer *Observer) Configure() {
	var err error

	if observer.AgeKeyFile == "" {
		err = huh.NewInput().
			Title("Path age key to use for secret encryption").
			Value(&observer.AgeKeyFile).
			Validate(func(ageKeyFile string) error {
				if !utils.FileExists(ageKeyFile) {
					return errors.New("you must enter an existing age key file with path")
				}
				return nil
			}).
			Run()
		utils.CheckErr(err)

		observer.AgeKeyFile, err = utils.NormalizeHomePath(observer.AgeKeyFile)
		utils.CheckErr(err)
	}

	err = huh.NewInput().
		Title("Observer name").
		Description("Name of the observer service").
		Value(&observer.Spec.Project).
		Validate(func(projectName string) error {
			if len(projectName) <= 0 {
				return errors.New("you must enter a project name")
			}
			return nil
		}).Run()
	utils.CheckErr(err)
	observer.Metadata.Name = observer.Spec.Project

	err = huh.NewInput().
		Title("Execution user").
		Description("User which is used to run the reconcile with. Must have read/write access to .spec.destination").
		Value(&observer.Metadata.User).
		Validate(func(projectName string) error {
			if len(projectName) <= 0 {
				return errors.New("you must enter a project name")
			}
			return nil
		}).
		Run()
	utils.CheckErr(err)

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

	err = huh.NewInput().
		Title("Root destination path").
		Description("The root path to sync the repository into").
		Value(&observer.Spec.Destination).
		Validate(func(destination string) error {
			if len(destination) <= 0 {
				return errors.New("you must enter an absolut path for destination")
			}
			return nil
		}).
		Run()
	utils.CheckErr(err)

	err = huh.NewInput().
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
	utils.CheckErr(err)

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

	err = huh.NewInput().
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
	utils.CheckErr(err)

	err = huh.NewInput().
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
	utils.CheckErr(err)

	err = huh.NewInput().
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
	utils.CheckErr(err)
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

	defer func() {
		err := f.Close()
		utils.CheckErr(err)
	}()

	err = renderer.Render("observer", observer, f)
	utils.CheckErr(err)
	err = f.Sync()
	utils.CheckErr(err)
}

func (observer *Observer) WriteConfigRoot() error {
	projectName := strings.ToLower(observer.Spec.Project)
	if projectName == "" {
		return fmt.Errorf("observer project name must not be empty")
	}

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("getting user config dir: %w", err)
	}

	configRoot := filepath.Join(userConfigDir, utils.APP_NAME, projectName)

	if err := os.MkdirAll(configRoot, 0700); err != nil {
		return fmt.Errorf("creating config root directory %s: %w", configRoot, err)
	}

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return fmt.Errorf("generating age identity: %w", err)
	}

	ageKeyPath := filepath.Join(configRoot, "age.key")
	ageKeyContent := fmt.Sprintf("# created: %s\n# public key: %s\n%s\n",
		time.Now().Format(time.RFC3339),
		identity.Recipient().String(),
		identity.String())

	if err := os.WriteFile(ageKeyPath, []byte(ageKeyContent), 0600); err != nil {
		return fmt.Errorf("writing age key %s: %w", ageKeyPath, err)
	}

	observer.AgeKeyFile = ageKeyPath

	renderer, err := templates.NewRenderer()
	if err != nil {
		return fmt.Errorf("creating template renderer: %w", err)
	}

	manifestPath := filepath.Join(configRoot, "manifest.yaml")
	f, err := os.Create(manifestPath)
	if err != nil {
		return fmt.Errorf("creating manifest file %s: %w", manifestPath, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing manifest file: %w", cerr)
		}
	}()

	if err := renderer.Render("observer", observer, f); err != nil {
		return fmt.Errorf("rendering manifest: %w", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing manifest: %w", err)
	}

	utils.LogSuccess("Observer config root created", "path", configRoot)
	slog.Info(fmt.Sprintf("Run: skuldcli reconcile %s", configRoot))

	return nil
}
