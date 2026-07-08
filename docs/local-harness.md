---
layout: default
title: Local Harness
---

# Local Harness

> *"Test in the temple, deploy to the field."*

The local harness validates the reconcile runtime command against a disposable Docker Compose container — no SSH, no systemd, no remote hosts needed.

## Scope

<div class="highlight-box">
<strong>Validates:</strong><br>
1. Local <code>witness</code> build output<br>
2. Prepared observer config root under <code>.local/</code><br>
3. Seeded source repository mounted into the container<br>
4. <code>witness reconcile</code> execution inside the container<br>
5. Repository sync, application discovery, drift analysis, and state persistence
</div>

**Does NOT validate:** SSH, sudo, systemd, timers, or full target-host bootstrap.

## Design

- Docker Compose is the only orchestrator
- The host builds `witness` first via `Taskfile.yml`
- The container mounts only what it needs — read-only binary, writable runtime, read-only config and source repo
- Runs `witness reconcile` directly in-container
- No privileged mode, no cgroup mounts, no systemd
- The harness image marks the source repo path as Git `safe.directory` for reliable bind mounts

## Files

| Path | Purpose |
|---|---|
| `docker/local-harness/Dockerfile` | Container image with age + git |
| `docker/local-harness/compose.yml` | Compose service definition |
| `.local/harness/` | Prepared inputs (config, repo, runtime) |
| `.local/bin/witness` | Built CLI binary |

## Workflow

```bash
# Prepare inputs
task local:prepare

# Run reconcile inside container
task local:run

# Full smoke test (CI path)
task local:smoke
```

## Prepared Inputs

`task local:prepare` creates or refreshes:

```
.local/harness/
├── config/
│   ├── manifest.yaml    # Observer with 30s interval
│   └── age.key          # Generated age identity
├── runtime/
│   └── destination/     # Pre-seeded no-drift state
└── source-repo/
    ├── apps/hello/
    │   ├── witness.yaml   # Application manifest
    │   └── compose.yaml # Docker Compose file
    └── .git/
```

## Requirements

- Docker & Docker Compose v2
- `task` (Go Task runner)
- Go toolchain
- Git
- age (`age-keygen`)

## Notes

- This reduced-scope harness favors simplicity and repeatability over target-host parity
- `task local:prepare` removes stale artifacts from earlier harness designs
- `task local:smoke` asserts repo was synced, destination files preserved, and `state.json` written
- CircleCI runs this as the `local_harness_smoke` job on a machine executor
