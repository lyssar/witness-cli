---
layout: default
title: Quickstart
---

# Quickstart

> *"From one Git repo, many applications arise."*

This guide walks through setting up Skuld from scratch on a local machine.

## Step 1: Install Skuld

```bash
curl -sfL https://raw.githubusercontent.com/lyssar/skuld-cli/main/install.sh | sh
```

Or see the [installation guide](installation) for alternatives.

## Step 2: Create an Observer

The **Observer** is your watcher — it defines the Git repository to watch and where to sync:

```bash
skuld-cli init my-server --local
```

The interactive wizard will ask for:

| Prompt | Example | Description |
|---|---|---|
| Observer name | `my-server` | Identifier for this observer |
| Execution user | `skuld-daemon` | System user for reconcile |
| Destination path | `/var/lib/skuld` | Root for repo sync |
| Repository URL | `https://github.com/org/infra.git` | Git repo to watch |
| Target revision | `main` | Branch, tag, or commit |
| Git user | `harness` | Git auth user |
| Access token | `ghp_...` | Git auth token |

The `--local` flag creates a complete config root at `~/.config/skuld-cli/my-server/`:

```
~/.config/skuld-cli/my-server/
├── manifest.yaml    # Observer definition
└── age.key          # Generated age identity
```

## Step 3: Create an Application

Applications declare what runs on your server. Generate one with:

```bash
skuld-cli new-app -a ~/.config/skuld-cli/my-server/age.key
```

The wizard prompts for:

| Prompt | Example | Description |
|---|---|---|
| App name | `hello` | Application identifier |
| Provisioner | `docker-compose` | Deploy method |
| Compose files | `compose.yaml` | Paths relative to app directory |
| Secrets | (optional) | Encrypted files + decrypt target |

Output is a `skuld-app.yaml` file. Place it in your Git repo:

```
apps/
└── hello/
    ├── skuld.yaml       # ← generated manifest
    ├── compose.yaml     # your Docker Compose file
    └── secret.env.age   # optional encrypted secret
```

## Step 4: Reconcile

Run a reconciliation cycle to apply the desired state:

```bash
skuld-cli reconcile ~/.config/skuld-cli/my-server/
```

<div class="highlight-box">
<strong>What happens during reconcile:</strong><br>
<strong>1.</strong> Git repo sync (clone or fetch)<br>
<strong>2.</strong> Application discovery (finds all <code>skuld.yaml</code> files)<br>
<strong>3.</strong> Fileset staging with decrypted secrets<br>
<strong>4.</strong> Drift detection against current state<br>
<strong>5.</strong> Apply — create, update, or delete applications
</div>

## Step 5: Deploy to a Remote Host

For production, push the observer to a remote server:

```bash
skuld-cli deploy ~/.config/skuld-cli/my-server/manifest.yaml \
  --host myserver.example.com \
  --ssh-user deploy \
  --age-key ~/.config/skuld-cli/my-server/age.key
```

This deploys:
- **skuld-cli binary** to `/usr/local/bin/`
- **systemd service** + **timer** for periodic reconciliation
- **Hardened security** — `NoNewPrivileges`, `ProtectHome`, `PrivateTmp`, system call filtering

---

## Next Steps

- [Commands →](commands) — full reference for all commands
- [Architecture →](architecture) — deep dive into the reconcile cycle
