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
  --ssh-user shens \
  --ssh-key ~/.ssh/id_rsa \
  --age-key ~/.age/infra.key
```

<div class="highlight-box">
<strong>Two different users:</strong><br>
• <code>--ssh-user</code> — your SSH login (e.g. <code>shens</code>) with sudo access<br>
• <code>metadata.user</code> in manifest — the execution user for reconcile (e.g. <code>witness</code>)
</div>

The deploy command:
1. Connects via SSH as your user
2. Creates config at <code>/home/witness/.config/witness/my-observer/</code>
3. Sets ownership to <code>witness:witness</code>
4. Uploads manifest and age key
5. Installs systemd service + timer running as <code>witness</code>

## Step 5: Verify

SSH into the server as the execution user and check:

```bash
ssh witness@myserver.example.com

# Check timer status
systemctl --user status witness.timer

# Check last reconcile
journalctl --user -u witness.service --since "5 minutes ago"

# Manual reconcile (if needed)
witness reconcile ~/.config/witness/my-observer/
```

---

## Runtime Layout on the Server

```
/home/witness/.config/witness/my-observer/
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
