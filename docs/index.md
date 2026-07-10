---
layout: default
title: Home
---

# Witness

> *"The future is not written in stone — it is reconciled from Git."*

**Witness** — watches your bare servers and reconciles them against a Git repository. Like ArgoCD, but for the metal fleet.

## For the Brotherhood

<div class="highlight-box">
<strong>☥ One Git repo to rule them all.</strong><br>
Define your applications, push to the repo, and Witness enforces the state on every reconciliation cycle. Secrets encrypted with <strong>age</strong>, deployed with <strong>Docker Compose</strong>, hardened with <strong>systemd</strong>.
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
curl -sfL https://raw.githubusercontent.com/lyssar/witness-cli/main/install.sh | sh

# Create an observer manifest
witness init

# Deploy to your server
witness deploy my-observer.yaml \
  --host myserver.example.com \
  --ssh-user deploy \
  --age-key /home/deploy/.age/infra.key
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
