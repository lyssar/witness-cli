# Phase 3 Project-Wide Checklist

## Product Manager Checklist
- [x] Confirm Phase 3 scope is limited to managed files, staging, and drift only.
- [x] Confirm docs-first gate and bind implementation to `.docs/decisions/phase-3-managed-files-staging-drift.md`.
- [x] Define ordered implementation tasks and dependency sequence.
- [x] Track implementation progress and continuously check off completed tasks.

## Go Implementation Tasks
- [x] Require absolute `observer.spec.destination` in reconcile validation.
- [x] Implement `internal/application/fileset.go` and `internal/application/ignore.go`.
- [x] Add fileset/ignore tests.
- [x] Implement git-aware submodule extraction outside fileset.
- [x] Add submodule extraction tests.
- [x] Implement `internal/reconcile/staging.go` and tests.
- [x] Implement `internal/reconcile/drift.go` and tests.
- [x] Integrate Runner per-app fileset/staging/drift flow.
- [x] Aggregate per-app errors while continuing other apps.
- [x] Ensure drift is report-only (not run failure).
- [x] Ensure no apply/docker/decrypt behavior is introduced.
- [x] Update `next.md` with current Phase 3 handoff state.
- [x] Fix submodule detection for nested `spec.source.path` discovery roots.
- [x] Enforce secret-target collision checks (compose/managed/secret-source paths).
- [x] Propagate fileset structured warnings through drift result and runner logging.
- [x] Add focused regression tests for rejected code-analyst findings.
- [x] Fix order-dependent manifest validation for secret target vs later secret source collisions.

## Verification
- [x] `go vet ./...`
- [x] `golangci-lint run`
- [x] `go test ./... -race -count=1`
- [x] `go build ./...`
- [ ] `govulncheck ./...` (advisory, if available) — not installed in environment (`command not found`)
