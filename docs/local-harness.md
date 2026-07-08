# Local Harness

The local harness validates the reconcile runtime command against a disposable Docker Compose container.

## Scope

This harness validates:

1. local `skuld-cli` build output
2. prepared observer config root under `.local/`
3. seeded source repository mounted into the container
4. `skuld-cli reconcile <config-root>` execution inside the container
5. reconcile-time repository sync, application discovery, drift analysis, and state persistence against mounted local inputs

It does **not** validate SSH, sudo, systemd, timers, or full target-host bootstrap behavior.

## Design

- Docker Compose is the only harness orchestrator.
- No standalone project shell-script files are used in the harness execution path.
- The host builds `skuld-cli` first via `Taskfile.yml`.
- The container mounts only:
  - the built binary read-only
  - a writable runtime root for reconcile output (`repo/`, destination files)
  - the prepared manifest and age key read-only
  - the seeded source repo read-only
- The harness runs `skuld-cli reconcile <config-root>` directly in-container.
- The host-prepared `manifest.yaml` and `age.key` are mounted into the container runtime root, so the in-container command path is `skuld-cli reconcile /workspace/harness/runtime`.
- The host also pre-seeds the destination tree with the seeded application's managed files so the reduced smoke path remains a no-drift reconcile run and does not pretend to validate real docker-compose apply behavior.
- No systemd, SSH, sudo, privileged mode, or cgroup mounts are used.
- The harness image marks the mounted source repo path as a Git `safe.directory` so local bind-mounted repositories work reliably inside the container.

## Files

- Harness Dockerfile: `docker/local-harness/Dockerfile`
- Compose file: `docker/local-harness/compose.yml`
- Prepared local inputs: `.local/harness/`
- Built CLI binary: `.local/bin/skuld-cli`

## Workflow

```bash
task local:prepare
task local:run
```

For the dedicated smoke path used by CircleCI:

```bash
task local:smoke
```

## Prepared Local Inputs

`task local:prepare` creates or refreshes:

- `.local/harness/config/manifest.yaml`
- `.local/harness/config/age.key`
- `.local/harness/runtime/destination/`
- `.local/harness/runtime/`
- `.local/harness/source-repo/`

The seeded source repo contains a minimal `Application` manifest and compose file so reconcile has realistic input data.

## Requirements

- Docker
- Docker Compose v2 (`docker compose`)
- `task`
- local Go toolchain
- local `git`

## Notes

- This reduced-scope harness intentionally favors simplicity and repeatability over target-host parity.
- It is honest about scope: it exercises the reconcile runtime command path, not the former SSH/systemd deploy path.
- `task local:prepare` removes stale abandoned SSH harness directories under `.local/harness/ssh` and `.local/harness/home` so old key material does not linger after the former SSH-based harness design.
- `task local:smoke` additionally asserts that reconcile synced the seeded repository into `.local/harness/runtime/repo/`, preserved the seeded no-drift destination files, and wrote observer-local `state.json` output.
