---
layout: default
---

# Quickstart

This guide walks through setting up Skuld on a local machine from scratch.

## 1. Install Skuld

```bash
curl -sfL https://raw.githubusercontent.com/lyssar/skuld-cli/main/install.sh | sh
```

Or follow the [installation guide](installation) for other methods.

## 2. Create an Observer

An **Observer** is the top-level config — it defines which Git repo to watch, where to sync, and how often.

```bash
skuld-cli init my-server --local
```

This interactive wizard will ask for:
- Observer name
- Execution user (e.g., `skuld-daemon`)
- Destination path (where repos are synced)
- Git repo URL and credentials
- Target revision and source path

The `--local` flag creates a complete config root at `~/.config/skuld-cli/my-server/` with:
- `manifest.yaml` — the observer definition
- `age.key` — a freshly generated age identity

## 3. Create an Application

Applications declare what runs on the server.

```bash
cd ~/.config/skuld-cli/my-server/
skuld-cli new-app
```

The wizard will prompt for:
- App name (e.g., `hello`)
- Provisioner type (`docker-compose`)
- Compose file paths
- Optional age-encrypted secrets

Output is a `skuld-app.yaml` file. Place it in your Git repo:

```
apps/
  hello/
    skuld.yaml       # ← generated here
    compose.yaml     # your Docker Compose file
    secret.env.age   # optional encrypted secret
```

## 4. Reconcile

Run a reconciliation cycle:

```bash
skuld-cli reconcile ~/.config/skuld-cli/my-server/
```

Skuld will:
1. Clone/fetch the Git repo
2. Discover applications from the repo
3. Build a fileset per application
4. Stage compose files and decrypt secrets
5. Compare against the running state
6. Apply changes (create/update/delete)

## 5. Deploy to a Remote Host

For production, deploy the observer to a remote host:

```bash
skuld-cli deploy manifest.yaml \
  --host myserver.example.com \
  --ssh-user deploy \
  --age-key ~/.config/skuld-cli/my-server/age.key
```

This installs:
- `skuld-cli` binary to `/usr/local/bin/`
- Systemd service + timer for periodic reconciliation
- Hardened security directives (NoNewPrivileges, ProtectHome, PrivateTmp, …)
