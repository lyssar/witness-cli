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
| `--local` | `false` | Run locally instead of against a remote host |

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
- **Volume claims** — optional: bind-mount directory, container UID, container GID

Output: `witness.yaml`

---

## `witness update-app`

Update an existing Application manifest interactively.

```bash
witness update-app -a ~/.config/witness/my-server/age.key
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `-a, --age-key` | `""` | Path to age private key for secret encryption |

Prompts for the manifest path, then allows adding or updating:

- **Registry credentials** — docker registry auth
- **Secrets** — encrypted file sources and targets
- **Volume claims** — bind-mount directory ownership
- **Compose files** — additional docker-compose file paths

---

## Volume Claims {: #volume-claims}

Volume claims let you declare the container UID/GID that should own a bind-mount directory. This is needed when a container image hardcodes a non-root user and Docker cannot auto-detect the correct ownership.

```yaml
spec:
  volumeClaims:
    - dir: data
      uid: 1000
      gid: 1000
```

| Field | Description |
|---|---|
| `dir` | Bind-mount directory relative to the compose file (e.g., `data`) |
| `uid` | Numeric container user ID (0–65535) |
| `gid` | Numeric container group ID (0–65535) |

**How it works:**
1. At deploy time, the witness binary receives `CAP_CHOWN` via `setcap`
2. The systemd unit configures `AmbientCapabilities=CAP_CHOWN`
3. On every reconcile, the runner checks if claimed directories exist with correct ownership
4. Missing or mis-owned directories are recreated and `chown`ed
5. Containers are restarted with `--force-recreate` to pick up the new ownership

**Volume data preservation:** On updates, Docker bind-mount directories are preserved by atomic same-filesystem rename (move), not copy. The old live tree is first moved aside to `.previous`, then each volume directory is renamed from the backup into the promoted staging tree. Moving preserves container-owned ownership and modes — including 0600 files such as Caddy's ACME private keys — without the non-root observer needing to read file contents, and without granting `CAP_DAC_READ_SEARCH` or `CAP_DAC_OVERRIDE`. A committed non-empty directory in the new tree wins over old live data; a failed promotion reverses the moves before restoring the old live tree.

**Requirements:** The target server needs `libcap2-bin` installed (`sudo apt install libcap2-bin`). Without it, the deploy warns and volume claims degrade to a no-op — the operator must `chown` directories manually.

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
| `--skip-prereq-check` | `false` | Skip the prerequisite check (for already-provisioned hosts) |

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
ProtectSystem=full
ProtectHome=no
PrivateTmp=yes
AmbientCapabilities=CAP_CHOWN
CapabilityBoundingSet=~CAP_SYS_ADMIN CAP_NET_ADMIN CAP_SYS_PTRACE
RestrictSUIDSGID=yes
MemoryDenyWriteExecute=yes
SystemCallFilter=@system-service
```

---

## `witness doctor`

Check that a target host has the prerequisites needed to run Witness.

```bash
witness doctor --host myserver.example.com --ssh-user shens
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--host` | `""` | Remote host to check |
| `-u, --ssh-user` | `""` | SSH user with sudo access |
| `-k, --ssh-key` | `""` | SSH private key path (optional, uses SSH config if omitted) |
| `--local` | `false` | Check the local machine instead of a remote host |
| `--check-only` | `false` | Report only; do not install anything |

`witness doctor` verifies the OS and the tools Witness needs on the target
host: `git`, `age`, `libcap`/`setcap`, `docker`, and `docker compose`. It runs
in report mode by default. With confirmation it can interactively install the
missing tools. `witness deploy` invokes `witness doctor` at the start of every
deploy.

---

## `witness version`

Print the installed version:

```bash
witness version
```

Output includes the build version and VCS revision, e.g. `witness <version> (commit <sha>)`.
