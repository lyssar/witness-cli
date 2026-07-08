# STATUS

## Current State

- OpenCode project setup aligned to current schema expectations
- Project instructions loaded from `AGENTS.md`
- Project-local skill present at `.opencode/skills/skuld-cli/SKILL.md`
- Canonical state tracking now lives in `STATUS.md`, `TASKS.md`, and `DECISIONS.md`
- Phase 3 reconcile work is implemented in the main codebase, including fileset discovery, ignore handling, staging, drift analysis, warning propagation, and stricter manifest validation
- Local validation now uses a reduced-scope Docker Compose harness that runs `skuld-cli reconcile` directly inside a container against mounted local inputs
- CircleCI now runs `task validate` and a reduced-scope harness smoke path
- Reconcile validation coverage now includes targeted negative-path checks around discovery-root handling in addition to existing manifest/fileset tests
- Reconcile now has concrete age decryptor and docker-compose provisioner seams, an atomic observer-local state store, create/update apply behavior, and archive-first deletion tracking for Git-removed apps
- Secret scrubbing is now symlink-safe: `scrubArchivedSecrets` resolves paths with `filepath.EvalSymlinks` and refuses to delete outside the archive root
- Runner supports injectable structured logger via `WithLogger` option; all internal log calls use `r.logger` instead of package-level `slog`
- Timing/duration added for git sync and total reconcile run

## Recent Changes

- Added `--local` flag to `skuldcli init` that creates a complete config root directory (`~/.config/skuld-cli/<name>/`) with manifest.yaml and a generated age.key
- `WriteConfigRoot()` generates a new X25519 age identity using `filippo.io/age` library directly (no `age-keygen` exec dependency)
- Consolidated prior implementation reality from `next.md` into the canonical tracking docs
- Replaced the broken systemd/SSH host-simulation harness with a reduced-scope runtime-command harness
- Simplified `docker/local-harness/Dockerfile` to a small runtime image with only reconcile dependencies
- Simplified `docker/local-harness/compose.yml` to mount the local binary, config root, and seeded source repo without privileged mode, SSH, sudo, or cgroup handling
- Reworked `Taskfile.yml` around `task local:prepare` and `task local:run`
- Narrowed writable bind mounts so the container can write only runtime reconcile state while manifest/key remain mounted read-only, and local prepare now cleans stale abandoned SSH harness artifacts
- Added `task local:smoke` to assert reduced-scope repo-sync outputs after the harness run
- Added CircleCI automation for `task validate` and `task local:smoke`
- Split CI execution between a Go validation job and a machine-executor harness smoke job so Docker bind mounts remain honest
- Verified the machine-executor Go toolchain download by SHA-256 before installation in CircleCI
- Expanded reconcile/application tests with focused discovery-root validation coverage
- Tightened README secret-inspection guidance to avoid printing decrypted plaintext to stdout or leaving a default plaintext file behind
- Added atomic `state.json` persistence for per-application reconcile status, retry metadata, and deletion bookkeeping
- Implemented runtime apply flow that stages managed files, decrypts secrets outside the repo checkout, promotes desired app trees into the destination, and delegates runtime actions through provisioner seams
- Implemented archive-first deletion flow for apps removed from Git, with persisted deletion retry state and archived app trees under the destination root
- Updated the reduced-scope local harness to preseed a no-drift destination tree so smoke validation stays honest without simulating docker runtime apply behavior inside the container
- Replaced placeholder "dummy" age keys in tests and harness with real generated age identities (`writeValidAgeKey`, `age-keygen`)
- Made `scrubArchivedSecrets` symlink-safe: resolves and rejects paths that escape the archive root
- Added `WithLogger` option to Runner and migrated all log calls to injectable structured logger
- Added timing info for git sync and total run duration

## Next Checkpoints

- Keep canonical state docs updated; `next.md` is now handoff-only and not the source of truth
- Expand local validation coverage deliberately if future runtime behavior needs stronger end-to-end checks beyond reconcile sync/discovery smoke
- Decide when and how to run honest end-to-end apply validation for docker-compose workloads outside the reduced local harness scope
- Consider CI supply-chain pinning (CircleCI machine image, harness base image SHA)
