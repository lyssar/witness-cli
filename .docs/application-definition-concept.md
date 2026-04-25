# Application Definition Concept

This document describes the finalized v1 `Application` contract for `skuld-cli`.

`skuld-cli` is a GitOps tool for bare servers. The `Observer` is the host-level control loop. It watches a Git repository, discovers applications, and reconciles them locally on the target machine.

The `Application` manifest is the workload-level contract consumed by the Observer.

---

## Role of the Application Manifest

An `Application` does **not** define:

- Git repository access
- target revision
- observer runtime settings
- remote deployment/bootstrap behavior

Those concerns stay at the `Observer` level.

An `Application` **does** define:

- descriptive metadata
- which provisioner should manage the app
- which compose files are used
- which encrypted secret files should be decrypted into runtime files

---

## Discovery Model

- The Observer checks out a Git repository locally.
- The Observer scans recursively below `observer.spec.source.path`.
- Every directory containing exactly `skuld.yaml` is treated as one application.
- Nested applications are forbidden.
- The directory containing `skuld.yaml` is the application source.

Example repository layout:

```text
apps/
  bla/
    caddy/
      docker-compose.yaml
      .env.age
      skuld.yaml
  blub/
    caddy/
      docker-compose.yaml
      skuld.yaml
```

Both applications are valid. Their operational identities are derived from their relative paths, not only from the leaf directory name.

---

## Identity Model

- `metadata.name` is required.
- `metadata.name` is descriptive only.
- Operational identity is derived from the app's relative path below the Observer discovery root.
- Runtime slug is derived from that relative path by:
  - lowercasing
  - replacing `/` with `-`
  - keeping other safe characters

Example:

- relative path: `bla/caddy`
- deploy path: `<observer-root>/apps/bla/caddy`
- runtime slug: `bla-caddy`

Path segments are restricted to lowercase letters, numbers, and hyphens.

---

## Deployment Layout

`observer.spec.destination` is the Observer root.

Fixed derived paths:

- applications: `<observer-root>/apps`
- archives: `<observer-root>/archives`

Example:

```text
/opt/skuld/
├── apps/
│   └── bla/
│       └── caddy/
│           ├── docker-compose.yaml
│           ├── .env
│           └── data/
└── archives/
    └── bla-caddy-2026-04-25T12-30-00Z.tar.gz
```

The Observer keeps control-plane artifacts separately in its local config/work directory.

---

## Managed File Model

By default, the entire application directory is considered deployable.

The managed file set is:

- all files in the app directory
- minus `skuld.yaml`
- minus `.skuldignore`
- minus files ignored by `.skuldignore`
- minus configured encrypted secret source files
- plus decrypted secret target files

Normal reconcile only mutates the managed file set.

Unmanaged/runtime files inside the live app directory are left untouched during normal reconcile.

### `.skuldignore`

- `.skuldignore` uses `.gitignore`-style patterns
- patterns are evaluated relative to the app directory
- if an ignored file is explicitly referenced by `skuld.yaml`, Skuld warns and still includes it

`skuld.yaml` and `.skuldignore` are never copied to the live app directory.

---

## Secrets Model

V1 supports **file-materialized secrets only**.

Environment variables are handled through decrypted files such as `.env`.

Each secret entry declares:

- `source`: encrypted source file inside the app directory
- `target`: relative runtime path inside the deployed app directory
- `decryptor`: decryptor type

Rules:

- `source` is required
- `target` is required
- `decryptor` is required
- `source` must be relative and stay inside the app directory
- `target` must be relative and stay inside the deployed app directory
- secret source files are not copied to the target
- only decrypted targets are materialized
- duplicate secret sources are invalid
- duplicate secret targets are invalid
- collisions between secret targets and normal managed files are invalid

### Decryptors

Decryptors must be pluggable from the start.

V1 supports only:

- `age`

The decryption identity is shared at the Observer level:

- one Observer-level age key is used for all applications handled by that Observer

---

## Provisioner Model

Provisioners must be pluggable from the start.

The manifest keeps an explicit `spec.provisioner` field even though v1 supports only one concrete provisioner.

V1 supports only:

- `docker-compose`

Provisioner-specific validation is strict.

For `docker-compose`:

- `composeFiles` is required
- at least one compose file must be declared
- entries must be relative to the app directory
- entries must exist
- duplicate entries are invalid
- order is significant
- the declared list and order are part of desired state

### Compose Runtime Contract

Validation contract:

```text
docker compose ... config
```

Apply contract:

```text
docker compose ... up -d --force-recreate --remove-orphans
```

Compose project name is set explicitly from the runtime slug.

---

## Drift and Reconcile Rules

Drift detection compares:

- desired managed files assembled in staging
- against live managed files on disk

Drift is content-only in v1.

This means Skuld reacts to:

- missing files
- new files
- changed files
- removed files
- changed decrypted secret outputs
- changed compose file declaration/order

Healthy apps are change-driven.

Failed apps retry even if no new drift is detected.

---

## Validation Rules

The following fail the affected application before touching the live deployment:

- malformed `skuld.yaml`
- unknown fields
- unsupported `apiVersion` or `kind`
- invalid path escapes
- duplicate compose file entries
- missing compose files
- unknown decryptors
- secret collisions
- nested apps
- symlinks in the managed file set
- Git submodules inside the app tree
- runtime slug collisions

Validation happens in a temporary staging directory mirroring the final live layout.

---

## Deletion Rules

If an app disappears from Git, desired state becomes deletion.

Deletion flow:

1. stop the app via the provisioner
2. archive the **entire** live app directory
3. remove the live app directory
4. remove the app entry from observer-local state

Archive filenames are timestamped.

If the live app directory is already missing, Skuld performs best-effort cleanup using Docker labels/project metadata.

V1 deletion rules:

- remove Compose-managed networks
- do not remove Docker volumes
- if shutdown fails, deletion fails
- if archive fails, deletion fails
- failed deletions retry on later reconciles

---

## Observer-Local State

The Observer persists one aggregate JSON state file in its control-plane work directory.

State is keyed by app operational identity.

V1 per-app state includes:

- `status` (`healthy`, `failed`, `deleting`)
- `lastError`
- `lastAttemptedReconcileAt`
- `lastSuccessfulReconcileAt`
- `lastSuccessfulResolvedCommit`

The state file also includes a top-level schema `version`.

---

## Canonical Example

```yaml
apiVersion: skuld.dev/v1alpha1
kind: Application
metadata:
  name: Public Edge Caddy
  description: Reverse proxy for public endpoints
  labels:
    app.kubernetes.io/name: caddy
    skuld.dev/tier: edge
  annotations:
    docs.skuld.dev/owner: platform-team
spec:
  provisioner: docker-compose
  composeFiles:
    - docker-compose.yaml
    - docker-compose.prod.yaml
  secrets:
    - source: .env.age
      target: .env
      decryptor: age
    - source: certs/tls.crt.age
      target: certs/tls.crt
      decryptor: age
    - source: certs/tls.key.age
      target: certs/tls.key
      decryptor: age
```

---

## Proposed Go Shape

```go
package v1alpha1

type Application struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   AppMetadata       `yaml:"metadata"`
	Spec       ApplicationSpec   `yaml:"spec"`
}

type AppMetadata struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type ApplicationSpec struct {
	Provisioner string        `yaml:"provisioner"`
	ComposeFiles []string     `yaml:"composeFiles,omitempty"`
	Secrets     []AppSecret   `yaml:"secrets,omitempty"`
}

type AppSecret struct {
	Source    string `yaml:"source"`
	Target    string `yaml:"target"`
	Decryptor string `yaml:"decryptor"`
}
```

---

## Summary

The v1 `Application` contract is intentionally strict:

- strong conventions for discovery
- explicit runtime contract
- file-based encrypted secrets
- Compose-only execution
- pluggable provisioner/decryptor architecture
- deterministic reconcile and deletion semantics

This keeps the first implementation small, predictable, and extensible.
