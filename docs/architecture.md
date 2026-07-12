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

## Deletion & Cleanup

When an application is removed from the repository:

1. The runtime directory is **archived** (timestamped backup)
2. The provisioner runs `docker compose down` to tear down the application
3. Any `.age` files in the archive are **scrubbed** — overwritten with zeros
4. Scrubbing uses `filepath.EvalSymlinks` to prevent symlink-based path traversal

## Security Model

| Layer | Protection |
|---|---|
| **Repository** | No plaintext secrets — only age-encrypted files |
| **Logging** | Git URLs sanitized before logging (credentials stripped) |
| **Cleanup** | Symlink-safe via `EvalSymlinks` + prefix path check |
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
│  ~/.local/harness/runtime/repo/                          │
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
