# Witness CLI Concept

`witness` is a GitOps CLI for bare servers.

The system is split into two levels:

- **Observer** — host-level control loop, deployment bootstrap, Git checkout, periodic reconcile
- **Application** — workload-level manifest discovered inside the observed repository

The Observer is already the installed daemon-like component. The current concept focus is how it discovers and manages Applications.

---

## Core Model

### Observer

The Observer is deployed to a target host and runs from systemd.

Responsibilities:

- keep a local checkout of the configured Git repo
- fetch and update that checkout on each reconcile
- scan below `spec.source.path` for applications
- reconcile each discovered application independently
- keep local state for observed applications

The Observer owns:

- repo URL
- target revision
- repo credentials
- observer-level age key
- runtime destination root

The Observer does **not** define individual app runtime behavior.

### Application

Each application lives in its own directory inside the observed repo.

Example:

```text
apps/
  caddy/
    docker-compose.yaml
    witness.yaml
  ftb-stoneblock-2/
    docker-compose.yaml
    extra-mods/
    witness.yaml
  my-web-app/
    docker-compose.yaml
    witness.yaml
```

The directory containing `witness.yaml` is the deployable source for that app.

---

## Commands

### `init`

Creates an Observer manifest.

```bash
witness init
```

The interactive wizard prompts for:

| Prompt | Example | Description |
|---|---|---|
| Age key path | `/home/deploy/.age/infra.key` | Existing age key for secret encryption |
| Observer name | `my-observer` | Identifier for this observer |
| Execution user | `deploy` | System user for reconcile on target host |
| Destination path | `/opt/witness` | Root for repo sync on target host |
| Repository URL | `https://github.com/org/infra.git` | Git repo to watch |
| Target revision | `main` | Branch, tag, or commit |
| Git user | `deploy` | Git auth user |
| Access token | `ghp_...` | Git auth token (encrypted with age) |

Output: a single `my-observer.yaml` manifest file in the current directory.

### `deploy`

Deploys the Observer to a remote server.

```bash
witness deploy my-observer.yaml \
  --host myserver.example.com \
  --ssh-user shens \
  --ssh-key ~/.ssh/id_rsa \
  --age-key ~/.age/infra.key
```

<div class="highlight-box">
<strong>Two different users:</strong><br>
• <code>--ssh-user</code> — your SSH login with sudo access<br>
• <code>metadata.user</code> in manifest — the execution user for reconcile
</div>

Responsibilities:

- connect via SSH as your user
- create config directory at `/home/<metadata.user>/.config/witness/<project>/`
- set ownership to `metadata.user`
- copy observer manifest and age key
- install/update systemd service and timer
- prepare the observer runtime on the target host

Behavior:

- if `--host` is omitted, localhost is assumed
- if SSH user/key are omitted, normal SSH resolution is used
- existing deployed observer files may be replaced
- the target user (`metadata.user`) must exist on the remote host

### `reconcile`

Used by the systemd service on the target host.

```bash
witness reconcile /home/deploy/.config/witness/my-observer/
```

Responsibilities:

- use the observer-local config root
- use the observer-level age key
- update the repo checkout
- discover apps under `spec.source.path`
- validate and reconcile each app independently
- retry failed apps and failed deletions

---

## Observer Config Meaning

Canonical Observer shape:

```yaml
apiVersion: witness/v1alpha1
kind: Observer
metadata:
  name: my-observer
  user: deployuser
spec:
  destination: /opt/witness
  project: my-observer
  source:
    path: apps
    repoURL: https://github.com/my-org/my-infra-repo.git
    user: git-user
    sshKey: <encrypted>
    targetRevision: main
```

### Important field meanings

- `metadata.user`
  - execution user for reconciliation on the host
- `spec.destination`
  - Observer root on the target host
  - fixed derived paths:
    - `<destination>/apps`
    - `<destination>/archives`
- `spec.source.repoURL`
  - observed Git repository
- `spec.source.targetRevision`
  - branch/tag/revision to reconcile
- `spec.source.path`
  - repo subdirectory below which application discovery happens recursively

---

## Runtime Layout

The Observer keeps control-plane data separate from runtime payload.

Example:

```text
/home/deploy/.config/witness/my-observer/
├── manifest.yaml
├── age.key
├── state.json
└── repo/

/opt/witness/
├── apps/
│   └── my-app/
│       ├── docker-compose.yaml
│       ├── .env
│       └── data/
└── archives/
    └── my-app-2026-04-25T12-30-00Z.tar.gz
```

This separation is important so control-plane secrets like the observer age key are not exposed inside runtime app roots.

---

## Application Discovery Rules

- discovery is recursive below `spec.source.path`
- any directory containing exactly `witness.yaml` is an app
- nested apps are forbidden
- the app source is that directory itself
- operational identity is derived from the relative path below the discovery root

Examples:

- `apps/bla/caddy/witness.yaml` → app identity `bla/caddy`
- `apps/blub/caddy/witness.yaml` → app identity `blub/caddy`

Both are valid.

---

## Application Runtime Rules

V1 application behavior is intentionally strict:

- only `docker-compose` is supported as a provisioner
- provisioners must still be pluggable in architecture from the start
- secrets are file-based only
- decryptors must also be pluggable in architecture from the start
- v1 decryptor is `age`
- all decryption uses the observer-level age key
- drift is content-based
- any managed drift triggers apply
- failed apps retry automatically
- deleted apps are archived before removal

The detailed application contract lives in:

- `.docs/application-definition-concept.md`

---

## Summary

The Observer remains the installed bare-server control loop.

The new `Application` contract defines how app directories in Git are turned into managed runtime workloads on that host.

This gives `witness` an ArgoCD-like model for bare servers:

- Observer = host-level controller
- Application = workload definition
- Git = source of truth
- local reconcile = desired state convergence on the machine itself
