---
layout: default
---

# Architecture

## Overview

Skuld follows a **two-phase controller** pattern: observe/plan then execute. Every reconciliation loop is read-only during the observation phase; mutations happen only in the apply phase.

```
Git Repo ──► Sync ──► Discover ──► Stage ──► Diff ──► Apply
                 │           │           │        │
                 ▼           ▼           ▼        ▼
              git pull    find apps   decrypt    create/update/delete
                           skuld.yaml   secrets     compose
```

## Core Concepts

### Observer

The **Observer** is the top-level configuration — it defines the Git source, the sync destination, and the reconciliation interval. Each observer manages exactly one Git repository.

```yaml
apiVersion: skuld/v1alpha1
kind: Observer
metadata:
  name: my-server
  user: skuld-daemon
spec:
  project: my-server
  destination: /var/lib/skuld
  source:
    repoURL: https://github.com/my-org/infra.git
    targetRevision: main
    path: /apps
  timeout:
    reconciliation: 180s
```

### Application

An **Application** declares a workload to deploy. Applications are discovered from `skuld.yaml` files in the synced repository.

```yaml
apiVersion: skuld.dev/v1alpha1
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
```

### Reconciliation Cycle

1. **Git Sync** — clones or fetches the repository to a local working directory
2. **Discovery** — walks the repo tree for `skuld.yaml` files and validates them against the v1 schema
3. **Staging** — assembles each application's runtime directory with decrypted secrets and compose files
4. **Drift Detection** — compares the staged state against the local `state.json`
5. **Apply** — creates, updates, or deletes applications to match the desired state

### Secrets

Secrets are encrypted with [age](https://age-encryption.org/) at rest in the Git repository. At reconcile time, they are decrypted with the observer's `age.key` before the compose file is uploaded to the destination.

The identity file is a standard age private key:

```
# created: 2026-07-08T12:00:00Z
# public key: age1…
AGE-SECRET-KEY-1…
```

### Deletion and Cleanup

When an application is removed from the repository:

1. The runtime directory is archived (timestamped backup)
2. The provisioner (`docker-compose down`) tears down the application
3. Any `.age` files in the archive are **scrubbed** — overwritten with zeros before the final cleanup
4. Scrubbing uses `filepath.EvalSymlinks` to prevent symlink-based path traversal

## Security Model

- **No secret material in the Git repo** — only age-encrypted files
- **No secrets in logs** — git URLs are sanitized before logging
- **Symlink-safe cleanup** — path traversal prevented via `EvalSymlinks` + prefix check
- **Hardened systemd units** — `NoNewPrivileges`, `ProtectHome`, `PrivateTmp`, system call filtering
- **SSH-based operation** — no open ports beyond SSH and the deployed services

## Data Flow

```
┌──────────────────┐
│   Git Repository │  ◄── skuld-cli reconcile
│   apps/hello/    │
│   ├─ skuld.yaml  │
│   ├─ compose.yaml│
│   └─ secret.age  │
└────────┬─────────┘
         │ git clone/fetch
         ▼
┌──────────────────┐
│  Sync Directory   │  /var/lib/skuld/repo/
│  (.git + apps/)   │
└────────┬─────────┘
         │ discover skuld.yaml
         ▼
┌──────────────────┐
│  State Store      │  state.json (per-application SHA tracking)
└────────┬─────────┘
         │ diff & apply
         ▼
┌──────────────────┐
│  Destination      │  /var/lib/skuld/destination/apps/hello/
│  ├─ compose.yaml  │
│  └─ .env          │  (decrypted from secret.age)
└────────┬─────────┘
         │ docker compose up
         ▼
    Running Containers
```
