---
layout: default
---

# Command Reference

## `skuld-cli init [OBSERVER_NAME]`

Create an Observer manifest.

```bash
skuld-cli init my-server
```

Flags:
| Flag | Default | Description |
|---|---|---|
| `-k, --age-key` | `""` | Path to existing age private key |
| `--local` | `false` | Create a full config root with generated age.key |

Without `--local`: writes a single YAML file to the current directory.

With `--local`: creates `~/.config/skuld-cli/<project>/` with `manifest.yaml` + `age.key`.

---

## `skuld-cli new-app`

Create an Application manifest interactively.

```bash
skuld-cli new-app -a ~/.config/skuld-cli/my-server/age.key
```

Flags:
| Flag | Default | Description |
|---|---|---|
| `-a, --age-key` | `""` | Path to age private key for secret encryption |

Prompts for:
- App name
- Provisioner type (currently `docker-compose` only)
- Compose file paths (one per prompt, empty to finish)
- Secrets: source file, target file, decryptor (`age`)

Output: `skuld-app.yaml` in the current directory.

---

## `skuld-cli reconcile [CONFIG_ROOT]`

Run one reconciliation cycle.

```bash
skuld-cli reconcile ~/.config/skuld-cli/my-server/
```

The config root must contain:
- `manifest.yaml` — observer definition
- `age.key` — age identity for secret decryption

What happens:
1. Git repo sync (clone or fetch)
2. Application discovery (`skuld.yaml` files)
3. Build + stage each application
4. Drift detection against running state
5. Apply creates, updates, or deletions

---

## `skuld-cli deploy [MANIFEST]`

Deploy the observer to a remote host with systemd.

```bash
skuld-cli deploy manifest.yaml \
  --host myserver.example.com \
  --ssh-user deploy \
  --age-key ~/.config/skuld-cli/my-server/age.key
```

Flags:
| Flag | Default | Description |
|---|---|---|
| `-a, --age-key` | `""` | Path to age key for remote deployment |
| `-u, --ssh-user` | `""` | SSH user for the remote host |
| `-k, --ssh-key` | `""` | SSH private key path (optional, uses SSH config if omitted) |
| `--host` | `""` | Remote host to deploy to |
| `--binary-path` | auto-detect | Path to skuld-cli binary to upload |

What the deploy command does:
1. Validates prerequisites (user exists, binary present, age installed)
2. Creates config directories on the remote host
3. Uploads manifest, age key, and skuld-cli binary
4. Installs systemd service and timer units
5. Runs `systemd-analyze verify`, `daemon-reload`, and enables the timer

The systemd service has full hardening:
- `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`
- `NoNewPrivileges=yes`, `MemoryDenyWriteExecute=yes`
- System call filtering, namespace restrictions, and more

---

## `skuld-cli version`

Print the installed version.

```bash
skuld-cli version
```
