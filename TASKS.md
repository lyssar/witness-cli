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

- [x] Extend validation beyond reconcile runtime execution in a bounded way around reconcile/discovery behavior
- [x] Add CI automation for `task validate` and a harness smoke path
- [x] Keep `AGENTS.md`, `opencode.json`, local skills, and canonical state docs aligned with project changes
- [x] Add concrete decryptor/provisioner seams for real reconcile execution
- [x] Add atomic observer-local reconcile state storage
- [x] Implement create/update reconcile apply flow for discovered apps
- [x] Implement archive-first deletion flow for apps removed from Git
- [x] Expand reconcile tests and keep the reduced-scope harness honest for no-drift runtime smoke
- [x] Fix age-key validation: generate valid identities in tests and harness seed
- [x] Make secret scrubbing symlink-safe against path traversal
- [x] Add injectable structured logger and timing to Runner

## Next

- [x] Add `--local` flag to `skuldcli init` for creating a complete config root directory with generated age key

## Later

- [ ] Expand runtime validation further only if reconcile begins applying destination changes or needs broader end-to-end assertions
- [ ] Add a dedicated honest apply-path integration environment for docker-compose reconcile once the project is ready to exercise real runtime mutation outside the reduced harness
- [ ] Consider stronger CI supply-chain pinning where practical (for example a non-floating CircleCI machine image and the local harness base image)
- [ ] Address symlink-scrubbing TOCTOU gap with O_NOFOLLOW fd-based deletion (currently documented in code)
