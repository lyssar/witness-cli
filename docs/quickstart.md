---
layout: default
title: Quickstart
---

# Quickstart

> *"From one Git repo, many applications arise."*

This guide walks through setting up Witness as a daemon on a remote server.

## Step 1: Install Witness

```bash
curl -sfL https://raw.githubusercontent.com/lyssar/witness-cli/main/install.sh | sh
```

Or see the [installation guide](installation) for alternatives.

## Step 2: Create an Observer Manifest

The **Observer** defines the Git repository to watch and where to sync. Run on your local machine:

```bash
witness init
```

The interactive wizard will ask for:

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

Output: `my-observer.yaml` in the current directory.

## Step 3: Create an Application

Applications declare what runs on your server. Generate one with:

```bash
witness new-app -a /home/deploy/.age/infra.key
```

The wizard prompts for:

| Prompt | Example | Description |
|---|---|---|
| App name | `hello` | Application identifier |
| Provisioner | `docker-compose` | Deploy method |
| Compose files | `compose.yaml` | Paths relative to app directory |
| Secrets | (optional) | Encrypted files + decrypt target |

Output is a `witness-app.yaml` file. Place it in your Git repo:

```
apps/
└── hello/
    ├── witness.yaml       # ← generated manifest
    ├── compose.yaml       # your Docker Compose file
    └── secret.env.age     # optional encrypted secret
```

Push the app manifest to your Git repository.

## Step 4: Deploy to the Server

Deploy the Observer to your remote server via SSH:

```bash
witness deploy my-observer.yaml \
  --host myserver.example.com \
  --ssh-user deploy \
  --ssh-key ~/.ssh/id_rsa \
  --age-key /home/deploy/.age/infra.key
```

This deploys:
- **witness binary** to `/usr/local/bin/`
- **Observer config** to `/home/deploy/.config/witness/my-observer/`
- **systemd service** + **timer** for periodic reconciliation
- **Hardened security** — `NoNewPrivileges`, `ProtectHome`, `PrivateTmp`, system call filtering

<div class="highlight-box">
<strong>What happens on the server:</strong><br>
<strong>1.</strong> Config directory created at <code>/home/deploy/.config/witness/my-observer/</code><br>
<strong>2.</strong> Manifest and age key uploaded<br>
<strong>3.</strong> systemd service and timer installed<br>
<strong>4.</strong> Timer enabled — reconcile runs every few minutes
</div>

## Step 5: Verify

SSH into the server and check the service:

```bash
ssh deploy@myserver.example.com

# Check timer status
systemctl status witness.timer

# Check last reconcile
journalctl -u witness.service --since "5 minutes ago"

# Manual reconcile (if needed)
witness reconcile /home/deploy/.config/witness/my-observer/
```

---

## Runtime Layout on the Server

```
/home/deploy/.config/witness/my-observer/
├── manifest.yaml    # Observer definition
├── age.key          # Age identity for secret decryption
├── state.json       # Reconcile state
└── repo/            # Cloned Git repository

/opt/witness/
├── apps/
│   └── hello/
│       ├── docker-compose.yaml
│       ├── .env
│       └── data/
└── archives/
    └── hello-2026-04-25T12-30-00Z.tar.gz
```

---

## Next Steps

- [Commands →](commands) — full reference for all commands
- [Architecture →](architecture) — deep dive into the reconcile cycle
