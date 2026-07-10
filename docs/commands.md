---
layout: default
title: Commands
---

# Commands

> *"Command, control, reconcile."*

## `witness init`

Create an Observer manifest.

```bash
witness init
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `-k, --age-key` | `""` | Path to existing age private key |

The interactive wizard prompts for:

- **Age key path** — existing age key for secret encryption
- **Observer name** — identifier for this observer
- **Execution user** — system user for reconcile on target host
- **Destination path** — root for repo sync on target host
- **Repository URL** — Git repo to watch
- **Target revision** — branch, tag, or commit
- **Git user** — Git auth user
- **Access token** — Git auth token (encrypted with age)

Output: a single YAML manifest file in the current directory.

<div class="highlight-box">
<strong>Usage:</strong><br>
Run <code>witness init</code> on your local machine to create the manifest.<br>
Then deploy it to the server with <code>witness deploy</code>.
</div>

---

## `witness new-app`

Create an Application manifest interactively.

```bash
witness new-app -a ~/.config/witness/my-server/age.key
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `-a, --age-key` | `""` | Path to age private key for secret encryption |

Prompts for:

- **App name** — identifier for the application
- **Provisioner type** — `docker-compose` (currently the only option)
- **Compose files** — path per prompt, empty to finish
- **Secrets** — optional: source file, target file, decryptor (`age`)

Output: `witness-app.yaml`

---

## `witness reconcile`

Run one reconciliation cycle.

```bash
witness reconcile ~/.config/witness/my-server/
```

The config root must contain:

| File | Purpose |
|---|---|
| `manifest.yaml` | Observer definition |
| `age.key` | Age identity for secret decryption |

**Reconciliation phases:**

```
Input ──► Git Sync ──► Discovery ──► Staging ──► Diff ──► Apply
```

1. **Git Sync** — clone or fetch the repository
2. **Discovery** — walk the repo for `witness.yaml` manifests
3. **Staging** — build runtime filesets with decrypted secrets
4. **Diff** — compare against stored state
5. **Apply** — create, update, or delete applications

---

## `witness deploy`

Deploy the observer to a remote host with systemd.

```bash
witness deploy my-observer.yaml \
  --host myserver.example.com \
  --ssh-user shens \
  --ssh-key ~/.ssh/id_rsa \
  --age-key ~/.age/infra.key
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `-a, --age-key` | `""` | Local path to age key (uploaded to server) |
| `-u, --ssh-user` | `""` | SSH user with sudo access |
| `-k, --ssh-key` | `""` | SSH private key path (optional, uses SSH config if omitted) |
| `--host` | `""` | Remote host to deploy to |
| `--binary-path` | auto-detect | Path to witness binary to upload |

<div class="highlight-box">
<strong>Two different users:</strong><br>
• <code>--ssh-user</code> — your SSH login (e.g. <code>shens</code>) with sudo access<br>
• <code>metadata.user</code> in manifest — the execution user for reconcile (e.g. <code>witness</code>)
</div>

**Deploy phases:**

1. Validate prerequisites (user exists, witness present, age installed)
2. Create config directories on remote host
3. Upload manifest, age key, and binary
4. Install systemd service and timer units
5. Enable and start the timer

**Systemd hardening** applied to service unit:

```
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
NoNewPrivileges=yes
MemoryDenyWriteExecute=yes
SystemCallFilter=@system-service
```

---

## `witness version`

Print the installed version:

```bash
witness version
```

Output includes the build version from `git describe`.
