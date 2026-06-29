# TASKS

## Current

- [x] Inspect existing OpenCode project setup
- [x] Modernize project config to current schema usage
- [x] Add project state tracking documents
- [x] Consolidate Phase 3 implementation reality from `next.md` into canonical project state docs
- [x] Redirect the local harness to a reduced-scope Docker Compose reconcile runner
- [x] Remove active systemd/SSH/sudo host-simulation behavior from the local harness path
- [x] Keep local harness inputs isolated under `.local/` and mounted explicitly into the container
- [x] Narrow local harness writable mounts to runtime state only and clean stale abandoned SSH harness artifacts

## Next

- [ ] Extend validation beyond reconcile runtime execution if stronger end-to-end coverage is required later
- [ ] Add CI automation for `task validate` and a harness smoke path
- [ ] Keep `AGENTS.md`, `opencode.json`, local skills, and canonical state docs aligned with project changes
