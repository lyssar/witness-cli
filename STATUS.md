# STATUS

## Current State

- OpenCode project setup aligned to current schema expectations
- Project instructions loaded from `AGENTS.md`
- Project-local skill present at `.opencode/skills/skuld-cli/SKILL.md`
- Canonical state tracking now lives in `STATUS.md`, `TASKS.md`, and `DECISIONS.md`
- Phase 3 reconcile work is implemented in the main codebase, including fileset discovery, ignore handling, staging, drift analysis, warning propagation, and stricter manifest validation
- Local validation now uses a reduced-scope Docker Compose harness that runs `skuld-cli reconcile` directly inside a container against mounted local inputs

## Recent Changes

- Consolidated prior implementation reality from `next.md` into the canonical tracking docs
- Replaced the broken systemd/SSH host-simulation harness with a reduced-scope runtime-command harness
- Simplified `docker/local-harness/Dockerfile` to a small runtime image with only reconcile dependencies
- Simplified `docker/local-harness/compose.yml` to mount the local binary, config root, and seeded source repo without privileged mode, SSH, sudo, or cgroup handling
- Reworked `Taskfile.yml` around `task local:prepare` and `task local:run`
- Narrowed writable bind mounts so the container can write only runtime reconcile state while manifest/key remain mounted read-only, and local prepare now cleans stale abandoned SSH harness artifacts

## Next Checkpoints

- Keep canonical state docs updated; `next.md` is now handoff-only and not the source of truth
- Expand local validation coverage deliberately if future runtime behavior needs stronger end-to-end checks
- Add CI automation for `task validate` and a local harness smoke path when a CI environment becomes available
