---
layout: default
title: Commands
---

# Commands

> *"Command, control, reconcile."*

## `skuld-cli init`

Create an Observer manifest.

```bash
skuld-cli init my-server
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `-k, --age-key` | `""` | Path to existing age private key |
| `--local` | `false` | Create complete config root with generated `age.key` |

<div class="highlight-box">
<strong>Without <code>--local</code>:</strong> writes a single YAML manifest to the current directory.<br>
<strong>With <code>--local</code>:</strong> creates <code>~/.config/skuld-cli/&lt;project&gt;/</code> with <code>manifest.yaml</code> and a freshly generated <code>age.key</code>.
</div>

---

## `skuld-cli new-app`

Create an Application manifest interactively.

```bash
skuld-cli new-app -a ~/.config/skuld-cli/my-server/age.key
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

Output: `skuld-app.yaml`

---

## `skuld-cli reconcile`

Run one reconciliation cycle.

```bash
skuld-cli reconcile ~/.config/skuld-cli/my-server/
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
2. **Discovery** — walk the repo for `skuld.yaml` manifests
3. **Staging** — build runtime filesets with decrypted secrets
4. **Diff** — compare against stored state
5. **Apply** — create, update, or delete applications

---

## `skuld-cli deploy`

Deploy the observer to a remote host with systemd.

```bash
skuld-cli deploy manifest.yaml \
  --host myserver.example.com \
  --ssh-user deploy \
  --age-key ~/.config/skuld-cli/my-server/age.key
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `-a, --age-key` | `""` | Path to age key for remote deployment |
| `-u, --ssh-user` | `""` | SSH user for remote host |
| `-k, --ssh-key` | `""` | SSH private key path (optional, uses SSH config if omitted) |
| `--host` | `""` | Remote host to deploy to |
| `--binary-path` | auto-detect | Path to skuld-cli binary to upload |

**Deploy phases:**

1. Validate prerequisites (user exists, skuld-cli present, age installed)
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

## `skuld-cli version`

Print the installed version:

```bash
skuld-cli version
```

Output includes the build version from `git describe`.
