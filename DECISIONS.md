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

## 2026-07-08

- Keep `task validate` as a pure Go quality suite with no Docker dependency

## 2026-07-08

- Add `--local` flag to `init` command: generates a complete config root (`~/.config/skuld-cli/<name>/`) with manifest.yaml and age.key
- Generate age keys using `filippo.io/age` library (`age.GenerateX25519Identity`) instead of exec'ing `age-keygen`, keeping the experience self-contained
- For `--local` mode, skip the interactive age key prompt by pre-setting `observer.AgeKeyFile` to a placeholder before `Configure()`; `WriteConfigRoot()` generates and writes the actual key file before template rendering
- Add a dedicated `task local:smoke` target for reduced-scope Docker harness validation instead of folding Docker into `task validate`
- Use CircleCI to run both `task validate` and `task local:smoke`
- Run the Docker-based smoke path on a machine executor so the reduced-scope harness can use real local bind mounts
- Verify the downloaded Go toolchain checksum before installing it on the CircleCI machine executor
- Expand validation scope only in bounded reconcile/discovery areas already in scope, rather than reintroducing SSH/systemd/bootstrap simulation
- Keep reconcile orchestration dependency-driven: decryptor and provisioner command details live behind dedicated execution seams, not inside the runner
- Persist observer-local reconcile state atomically in `state.json` so retries and Git-driven deletions have first-class bookkeeping
- Keep drift detection read-only; runtime mutation happens only after drift analysis via explicit apply/delete steps
- For Git-removed applications, move the live app tree under `<destination>/archive/` before deletion and retain the archived files for inspection while state tracks retry progress
- De-emphasize the legacy top-level `internal/reconcile.go` path; the active reconcile architecture is `internal/reconcile.Runner`

## 2026-07-08

- Make archived secret scrubbing symlink-safe: resolve via `filepath.EvalSymlinks` and reject paths outside the archive root; TOCTOU gap remains documented for future O_NOFOLLOW hardening.
- Runner logging uses injectable `*slog.Logger` (default `slog.Default()`) instead of package-level calls, enabled via `WithLogger` option.
- Log timing for git sync and total run duration as structured attributes.
