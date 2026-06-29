# Next Session Handoff

`next.md` is no longer the canonical project tracker.

Use these files first:

- `STATUS.md` for implementation state
- `TASKS.md` for active and upcoming work
- `DECISIONS.md` for architectural/project decisions

## Current Handoff Notes

- Phase 3 reconcile/file-set/drift work has been merged into canonical state docs.
- The local validation path is now the reduced-scope Docker Compose reconcile harness in `Taskfile.yml` plus `docker/local-harness/`.
- Recommended next review flow remains `@code-analyst` → `@security-analyst` → human review/commit.
