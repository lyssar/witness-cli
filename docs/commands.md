---
layout: default
title: Commands
---

# Commands

> *"Command, control, reconcile."*

## `witness init`

Create an Observer manifest.

```bash
witness init my-server
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `-k, --age-key` | `""` | Path to existing age private key |
| `--local` | `false` | Create complete config root with generated `age.key` |

<div class="highlight-box">
<strong>Without <code>--local</code>:</strong> writes a single YAML manifest to the current directory.<br>
<strong>With <code>--local</code>:</strong> creates <code>~/.config/witness/&lt;project&gt;/</code> with <code>manifest.yaml</code> and a freshly generated <code>age.key</code>.
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
witness deploy manifest.yaml \
  --host myserver.example.com \
  --ssh-user deploy \
  --age-key ~/.config/witness/my-server/age.key
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `-a, --age-key` | `""` | Path to age key for remote deployment |
| `-u, --ssh-user` | `""` | SSH user for remote host |
| `-k, --ssh-key` | `""` | SSH private key path (optional, uses SSH config if omitted) |
| `--host` | `""` | Remote host to deploy to |
| `--binary-path` | auto-detect | Path to witness binary to upload |

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
