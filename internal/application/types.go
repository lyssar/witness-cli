package application

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ManifestFileName is the exact supported application manifest filename.
const ManifestFileName = "witness.yaml"

// APIVersionV1Alpha1 is the only supported Application apiVersion in v1.
const APIVersionV1Alpha1 = "witness.dev/v1alpha1"

// KindApplication is the only supported Application kind in v1.
const KindApplication = "Application"

// ProvisionerDockerCompose is the only supported v1 provisioner.
const ProvisionerDockerCompose = "docker-compose"

// DecryptorAge is the only supported v1 secret decryptor.
const DecryptorAge = "age"

// Application is the canonical v1 workload manifest consumed by Observer reconcile.
type Application struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

// Metadata stores human-oriented application information.
type Metadata struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// Spec stores the application runtime contract.
type Spec struct {
	Provisioner         string               `yaml:"provisioner"`
	ComposeFiles        []string             `yaml:"composeFiles"`
	VolumeClaims        []VolumeClaim        `yaml:"volumeClaims,omitempty"`
	Secrets             []Secret             `yaml:"secrets,omitempty"`
	RegistryCredentials *RegistryCredentials `yaml:"registryCredentials,omitempty"`
}

// VolumeClaim instructs the reconciler to set ownership on a bind-mount
// directory so the Docker container can read and write regardless of the
// image's hardcoded UID/GID.
type VolumeClaim struct {
	Dir string `yaml:"dir"`
	UID int    `yaml:"uid"`
	GID int    `yaml:"gid"`
}

// RegistryCredentials stores encrypted docker registry authentication.
type RegistryCredentials struct {
	Registry string `yaml:"registry"`
	Username string `yaml:"username"`
	Password string `yaml:"password"` // age-encrypted
}

// Secret defines one encrypted file input and decrypted runtime output.
type Secret struct {
	Source    string     `yaml:"source"`
	Target    string     `yaml:"target"`
	Decryptor string     `yaml:"decryptor"`
	Mode      SecretMode `yaml:"mode,omitempty"`
}

// SecretMode is the quoted manifest representation of secret target permissions.
type SecretMode struct {
	value   string
	present bool
}

// NewSecretMode returns an explicitly supplied secret mode for programmatic manifests.
func NewSecretMode(value string) SecretMode {
	return SecretMode{value: value, present: true}
}

// UnmarshalYAML accepts only explicitly quoted YAML strings so permission modes
// cannot be interpreted as YAML numbers.
func (m *SecretMode) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || (value.Style != yaml.DoubleQuotedStyle && value.Style != yaml.SingleQuotedStyle) {
		return fmt.Errorf("mode must be a quoted string")
	}
	m.value = value.Value
	m.present = true
	return nil
}

// ResolvedMode returns the secret target permissions declared by the manifest.
// An omitted mode defaults to 0600.
func (s Secret) ResolvedMode() (os.FileMode, error) {
	if !s.Mode.present {
		return 0o600, nil
	}

	return resolveSecretMode(s.Mode.value)
}

// Identity captures deterministic operational identity and runtime slug.
type Identity struct {
	OperationalID string
	RuntimeSlug   string
}

// DiscoveredApplication describes one validated application found by discovery.
type DiscoveredApplication struct {
	ManifestPath  string
	SourceDir     string
	RelativePath  string
	OperationalID string
	RuntimeSlug   string
	Application   Application
}
