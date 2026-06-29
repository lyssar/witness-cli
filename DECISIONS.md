# DECISIONS

## 2026-06-18

- Keep agent instructions sourced from `AGENTS.md` via `opencode.json`
- Inherit the global/default agent and model instead of pinning project-specific values
- Preserve the existing project-local `skuld-cli` skill as the canonical local skill
- Remove outdated top-level `mode` config rather than replacing it with unnecessary overrides
- Add `STATUS.md`, `TASKS.md`, and `DECISIONS.md` for project state tracking
- Treat `STATUS.md`, `TASKS.md`, and `DECISIONS.md` as canonical state; `next.md` is handoff-only
- Keep local validation honest: the current harness validates the runtime reconcile command path, not SSH/systemd target-host bootstrap behavior
- Use Docker Compose as the only local harness orchestrator
- Use a dedicated harness image definition at `docker/local-harness/Dockerfile`
- Build `skuld-cli` on the host first, then mount the built binary read-only into the harness container
- Prepare harness inputs under `.local/`, mount only the runtime state read-write, mount manifest and age key read-only, and mount the seeded source repo read-only
- Run `skuld-cli reconcile <config-root>` directly in-container
- Do not use systemd, SSH, sudo, privileged mode, or cgroup mounts in the reduced-scope local harness
- Keep harness automation in `Taskfile.yml` and Docker Compose; avoid standalone harness shell-script files
