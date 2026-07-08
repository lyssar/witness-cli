package application

// ManifestFileName is the exact supported application manifest filename.
const ManifestFileName = "skuld.yaml"

// APIVersionV1Alpha1 is the only supported Application apiVersion in v1.
const APIVersionV1Alpha1 = "skuld.dev/v1alpha1"

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
	Provisioner  string   `yaml:"provisioner"`
	ComposeFiles []string `yaml:"composeFiles"`
	Secrets      []Secret `yaml:"secrets,omitempty"`
}

// Secret defines one encrypted file input and decrypted runtime output.
type Secret struct {
	Source    string `yaml:"source"`
	Target    string `yaml:"target"`
	Decryptor string `yaml:"decryptor"`
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
