---
layout: default
title: Architecture
---

# Architecture

> *"Know thy state, reconcile thy fleet."*

## Overview

Witness follows a **two-phase controller** pattern: observe/plan then execute. Every reconciliation loop is read-only during observation; mutations happen only in the apply phase.

<div class="highlight-box">
<strong>Click to expand:</strong> Full-resolution architecture diagram below.
</div>

![Witness Reconciliation Pipeline]({{ site.baseurl }}/assets/img/architecture.svg)

## Core Concepts

### Observer

The **Observer** is the watcher — it defines the Git source, sync destination, and timing.

```yaml
apiVersion: witness/v1alpha1
kind: Observer
metadata:
  name: my-server
  user: witness-daemon
spec:
  project: my-server
  destination: /var/lib/witness
  source:
    repoURL: https://github.com/my-org/infra.git
    targetRevision: main
    path: /apps
  timeout:
    reconciliation: 180s
```

### Application

An **Application** declares a workload. Discovered from `witness.yaml` files in the synced repo.

```yaml
apiVersion: witness.dev/v1alpha1
kind: Application
metadata:
  name: hello
spec:
  provisioner: docker-compose
  composeFiles:
    - compose.yaml
  secrets:
    - source: secret.env.age
      target: .env
      decryptor: age
      mode: "0444"
```

### Reconciliation Cycle

| Phase | Description |
|---|---|
| **Git Sync** | Clone or fetch the repository to a local working directory |
| **Discovery** | Walk the repo tree for `witness.yaml` files, validate against v1 schema |
| **Staging** | Assemble runtime directory with decrypted secrets and compose files |
| **Drift Detection** | Compare staged state against `state.json` |
| **Apply** | Create, update, or delete applications to match desired state |

## Secrets

Secrets are encrypted with [age](https://age-encryption.org/) at rest in the Git repository. At reconcile time, they are decrypted with the observer's `age.key`.

```
Git repo: secret.env.age  ──age-decrypt──►  Destination: .env
```

The identity file is a standard age private key:

```
# created: 2026-07-08T12:00:00Z
# public key: age1…
AGE-SECRET-KEY-1…
```

Secret `mode` is optional and defaults to `"0600"`. When set, it must be a
quoted canonical octal string matching `0[0-7]{3}`. Group and other may only
have read permission (for example, `"0444"` for a container-readable secret);
their write and execute permissions, and special bits, are rejected. Witness
applies the resolved mode exactly to the staged and live secret target. This
does not change Docker Compose secret semantics.

## Application Configuration Handling

Verified behavior for a typical application directory — one Compose file, one
encrypted secret, and additional configuration files.

```
apps/<app>/
├── witness.yaml            # required application manifest
├── docker-compose.yaml     # declared in spec.composeFiles, must exist
├── secret.env.age          # declared in spec.secrets[].source
└── <configuration files>   # further regular files, copied verbatim
```

### Recognized files and formats

- `witness.yaml` is the only accepted manifest filename. It is decoded strictly:
  unknown fields and multiple YAML documents are rejected. `apiVersion` must be
  `witness.dev/v1alpha1`, `kind` must be `Application`.
- `spec.composeFiles` is a required, ordered, non-empty list of paths relative to
  the app directory. Any filename or extension is accepted (`docker-compose.yaml`,
  `compose.yaml`, …), duplicates are rejected, and every entry must exist as a
  regular file. The order is significant and part of desired state.
- `spec.secrets[].source` is a relative path to an encrypted file inside the app
  directory. A file ending in `.age` that is not declared as a secret source fails
  the whole application.
- Every other regular file in the app directory is a **managed file**: it is copied
  verbatim, with its mode preserved, into the live app directory. `witness.yaml` and
  `.witnessignore` are not special-cased and are therefore managed as well.
- `.witnessignore` (optional) holds `.gitignore`-style patterns evaluated relative
  to the app directory. A file required by the manifest that is ignored still stays
  managed and produces a warning.
- Symlinks anywhere in the app tree, and Git submodules inside the app tree, fail
  the affected application.

### Secret handling

- The only supported `decryptor` is `age`; every declaration must name it.
- The observer-level `age.key` in the config root decrypts all secrets. It must be
  a regular file, not a symlink, carry no group/other permission bits, and contain
  at least one valid age identity.
- A secret source file must contain base64-encoded age ciphertext, not raw age
  binary output.
- The decrypted `target` is a relative path inside the live app directory. It must
  not collide with a compose file, another secret source or target, or a managed
  file.
- `mode` is optional and defaults to `0600`. When set it must be a quoted canonical
  octal string matching `0[0-7]{3}`; group/other write and execute and special bits
  are rejected (`"0444"` is allowed). The resolved mode is applied to the staged and
  live target.
- Only decrypted targets are materialized; encrypted source files are never copied
  to the live directory. A changed secret target triggers re-apply with
  `--force-recreate`.
- `spec.registryCredentials.password` is an inline base64 age-encrypted value. It is
  passed to `docker login --password-stdin` and removed immediately afterward.

### Limits and restrictions

- Application directory path segments must match `^[a-z0-9-]+$` (lowercase letters,
  digits, hyphen) and must not begin with `archive`. Segments are not lowercased
  automatically.
- One `witness.yaml` per directory; nested applications are forbidden. Two apps with
  the same leaf name in different subpaths are allowed.
- Secrets are file-based only. Inline plaintext secret values are not supported
  (the encrypted registry password is the sole inline credential).
- Witness performs no templating or environment-variable interpolation of its own.
  File contents and Compose files are used verbatim; any `${VAR}` interpolation is
  performed by `docker compose` itself.
- Drift detection is content-only, over managed files and decrypted secret targets.

## Deletion & Cleanup

When an application is removed from the repository:

1. The runtime directory is **archived** at
   `<destination>/archive/<operationalID>/<timestamp>`
2. The provisioner runs `docker compose down` to tear down the application
3. Each declared decrypted secret target and the transient decrypted registry
   password are **removed** from the archive
4. Before deletion or scrubbing, the archive reference must be canonical and
   resolve through the destination `os.Root` to a non-symlink directory. State
   without an explicit complete secret-target inventory fails closed.

The archive is revalidated immediately before each provisioner invocation.
This narrows, but cannot fully eliminate, the TOCTOU boundary for a provisioner
subprocess because it receives an absolute directory path and may open it after
the validation completes. `os.Root` continues to confine Witness's own file
operations.

For private registries, Witness passes the decrypted password to `docker login`
through stdin and removes the promoted plaintext file immediately afterward,
whether login succeeds or fails. Login command output is not propagated in
errors so it cannot disclose the password.

## Security Model

| Layer | Protection |
|---|---|
| **Repository** | No plaintext secrets — only age-encrypted files |
| **Logging** | Git URLs sanitized before logging (credentials stripped) |
| **Cleanup** | Rooted archive operations with symlink ancestry and final-directory checks |
| **Systemd** | `NoNewPrivileges`, `ProtectHome`, `PrivateTmp`, system call filtering |
| **Deploy** | SSH-only — no open ports beyond SSH and deployed services |

## Data Flow

```
┌─────────────────────────────────────────────────────────┐
│                    Git Repository                        │
│  apps/hello/{witness.yaml, compose.yaml, secret.age}       │
└────────────────────┬────────────────────────────────────┘
                     │ git clone/fetch
                     ▼
┌─────────────────────────────────────────────────────────┐
│                  Sync Directory                          │
│  ~/.config/witness/<project>/repo/                          │
│  ├── .git/                                               │
│  └── apps/hello/witness.yaml                              │
└────────────────────┬────────────────────────────────────┘
                     │ discover
                     ▼
┌─────────────────────────────────────────────────────────┐
│               State Store (state.json)                   │
│  Tracks per-application content SHA for drift detection  │
└────────────────────┬────────────────────────────────────┘
                     │ diff
                     ▼
┌─────────────────────────────────────────────────────────┐
│                  Destination                             │
│  /var/lib/witness/destination/apps/hello/                  │
│  ├── compose.yaml                                        │
│  └── .env  (decrypted from secret.age)                   │
└────────────────────┬────────────────────────────────────┘
                     │ docker compose up
                     ▼
┌─────────────────────────────────────────────────────────┐
│                Running Containers                        │
│  hello_web_1, hello_db_1, ...                            │
└─────────────────────────────────────────────────────────┘
```
