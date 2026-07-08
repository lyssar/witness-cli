---
layout: default
---

# Skuld CLI

**Skuld** is a lightweight GitOps CLI for bare servers. It continuously reconciles applications from a Git repository — similar to ArgoCD, but designed for single-server deployments without Kubernetes.

## Key Features

- **Observer-driven reconciliation** — daemon or timer-based sync from a Git repo
- **Application model** — declare apps with provisioner, compose files, and age-encrypted secrets
- **Docker Compose provisioner** — deploy and update Compose-based workloads
- **Age encryption** — secrets encrypted at rest with `age`, decrypted at runtime
- **SSH deploy** — push config, binary, and systemd units to remote hosts
- **Secret scrubbing** — sensitive data removed from archives on deletion
- **Symlink-safe paths** — hardened against path traversal during cleanup

## Quick Start

```bash
# Create an observer config root
skuld-cli init my-observer --local

# Reconcile against your Git repo
skuld-cli reconcile ~/.config/skuld-cli/my-observer/
```

## Next Steps

- [Installation](installation) — install via binary, Go toolchain, or build from source
- [Quickstart](quickstart) — full walkthrough from zero to deployed app
- [Commands](commands) — reference for `init`, `new-app`, `reconcile`, `deploy`, `version`
- [Architecture](architecture) — how the observer, discovery, and apply cycle work
