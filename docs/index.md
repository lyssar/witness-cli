---
layout: default
title: Home
---

# Skuld CLI

> *"The future is not written in stone — it is reconciled from Git."*

**Skuld** — named after the Norn of the future — watches your bare servers and reconciles them against a Git repository. Like ArgoCD, but for the metal fleet.

## For the Brotherhood

<div class="highlight-box">
<strong>☥ One Git repo to rule them all.</strong><br>
Define your applications, push to the repo, and Skuld enforces the state on every reconciliation cycle. Secrets encrypted with <strong>age</strong>, deployed with <strong>Docker Compose</strong>, hardened with <strong>systemd</strong>.
</div>

## Key Features

- **Observer-driven** — daemon or timer-based sync from a Git repo
- **Application model** — declare apps with provisioner, compose files, and age-encrypted secrets
- **Docker Compose** — deploy and update Compose-based workloads
- **Age encryption** — secrets encrypted at rest, decrypted at runtime
- **SSH deploy** — push config, binary, and systemd units to remote hosts
- **Secret scrubbing** — archives sanitized on application deletion
- **Symlink-safe** — hardened against path traversal during cleanup

## Get Started

```bash
# Install
curl -sfL https://raw.githubusercontent.com/lyssar/skuld-cli/main/install.sh | sh

# Create an observer
skuld-cli init my-server --local

# Reconcile
skuld-cli reconcile ~/.config/skuld-cli/my-server/
```

## Quick Links

| Command | Description |
|---|---|
| [Install →](installation) | Install via script, Go, or source |
| [Quickstart →](quickstart) | Full walkthrough from zero to deployed |
| [Commands →](commands) | Reference for all commands |
| [Architecture →](architecture) | How the observer cycle works |

---

*"The future is watching."*
