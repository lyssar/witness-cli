# Phase 3 Managed Files, Staging, and Drift Decisions

Date: 2026-05-08

## 1. Context

This document persists the completed grill-me decisions for Phase 3 so Product Manager, Platform Lead, and Go implementation can proceed from one durable source of truth.

Phase 3 scope is limited to managed file set definition, staging behavior, and drift detection semantics. It is documentation-first and must precede code changes. In this critical infrastructure project, core decisions here are binding for downstream implementation/review and may only be changed with explicit redirect/user approval.

## 2. Core Decisions

- **Scope boundaries:** Phase 3 covers managed files, staging, and content drift only. No Docker execution, no real age decryption, no live mutation/apply.
- **Secrets in Phase 3:** Secret targets are modeled only. Secret sources are excluded from managed files. Secret targets are path/collision-validated, but target content is not materialized and target-content drift is deferred.
- **Apps with secrets:** Apps declaring secrets may still pass Phase 3 staging/drift.
- **Drift output for secrets:** Drift includes `DeferredSecretTargets []string`; deferred secret targets do **not** set `HasDrift=true`.
- **Removed managed files:** Detection of removed managed files is deferred until state/managed inventory exists.
- **Inventory prep:** Phase 3 still builds an in-memory managed inventory with deterministic managed paths.
- **Ignore engine preference:** `.skuldignore` should use `github.com/codeglyph/go-dotignore` unless Platform Lead finds a concrete blocker.
- **Ignore parse behavior:** Invalid/unusable `.skuldignore` lines follow Git-like lenient behavior (silent ignore where library behaves that way).
- **Manifest-required files + ignore:** Explicit `composeFiles` matched by `.skuldignore` remain managed/staged and emit structured warning `ignored_manifest_required_file`.
- **Secret sources + ignore warnings:** Secret sources are always excluded without `.skuldignore` warnings.
- **Symlink policy:**
  - Ignored symlinks are skipped.
  - Non-ignored symlinks fail the app.
  - Manifest-required symlinks fail the app.
  - Symlink directories are never traversed.
- **Git submodule policy:** Unsupported anywhere inside app tree.
  - Detection is Git-aware outside fileset module.
  - Preferred source: `git config --file .gitmodules --get-regexp path`.
  - Invalid `.gitmodules` fails whole run.
  - Submodule inside app fails only that app.
- **Staging behavior:**
  - Per-app staging root mirrors app-root layout.
  - Builder creates temp dir and returns cleanup function.
  - Cleanup errors are warnings, not retroactive failures.
  - Source file modes are best-effort preserved.
  - Drift remains content-only.
  - Empty directories are ignored.
- **Drift behavior:**
  - Streaming byte comparison; no hash inventory.
  - Structured result contains: `HasDrift`, `Changed`, `Missing`, `TypeMismatch`, `DeferredSecretTargets`, warnings.
  - Live type mismatches for managed file paths are drift (not errors).
  - Live symlinks are detected via `Lstat`, not followed, and reported as replace-required/type-mismatch drift.
  - Missing live root => all desired files are `Missing`.
  - Live root exists as file/non-dir => app failure.
  - Inaccessible live dir or live-file read permission error => app failure.
  - Missing expected staging file => internal error.
- **“Live wins” interpretation:** Live is the measured reality for drift only; Git/staging remains desired state for future apply.
- **Runner integration:** `Runner.Run` integrates Phase 3 per app: build fileset, build staging, run drift, log result, no apply.
- **Runner error model:** App errors are collected while processing continues across all apps; final error summarizes count + app IDs. Drift itself is not a Runner error.
- **Discovery behavior:** Discovery remains fail-fast in Phase 3.
- **Destination requirement:** `observer.spec.destination` becomes required and absolute for live-path drift resolution.
  - Destination path itself need not already exist.
  - Live app dir resolves to `<spec.destination>/apps/<operationalID>`.
  - Add defense-in-depth check ensuring live dir remains under `<destination>/apps`.
- **Package split:**
  - `internal/application/fileset.go`
  - `internal/application/ignore.go`
  - `internal/reconcile/staging.go`
  - `internal/reconcile/drift.go`
- **Fileset contract:** Fileset returns structured deterministic entries:
  - `ManagedFile{RelativePath, SourcePath}`
  - Canonical slash-relative paths
  - Deterministic ordering
  - Structured warnings
- **Managed-file inclusion rules:**
  - Hidden files included by default.
  - Plaintext `.env` without secret declaration is a normal managed file.
  - Non-ignored special files fail the app.
  - Permission/IO errors during fileset walk or staging copy fail the app.
- **Key requirement:** `age.key` remains required even though Phase 3 does not use it.
- **Out-of-scope legacy hardening:** Legacy `internal/git_repository.go` hardening remains out of scope.
- **Documentation discipline:**
  - Keep docs current.
  - Create this decision record at `.docs/decisions/phase-3-managed-files-staging-drift.md`.
  - Create/maintain `.docs/todo.md` as project-wide checklist with phase sections.
  - Continuously check off `.docs/todo.md` after each completed task.
  - Update `next.md`.
- **Docs-first gate:** Before code, Product Manager defines task/todo structure; Platform Lead validates technical decision record/guardrails; Go implementer may update docs/todo but may not change core decisions without redirect.
- **Pipeline continuity:** After this summary, continue with Product Manager; no need to rerun Product Owner.

## 3. Resolved Trade-offs

- Secret modeling vs fake decryptor: chose **modeling only** to avoid sneaking in partial secret implementation.
- Deferred secret targets as drift vs separate caveat: chose **separate `DeferredSecretTargets`** so `HasDrift` means observed non-secret content drift.
- Extra live files as drift vs ignore: chose **ignore for now** until state/managed inventory exists.
- Gitignore library vs self-built subset: chose **`github.com/codeglyph/go-dotignore` preference** due to risk of custom semantics.
- Invalid ignore file fail vs lenient: chose **Git-like lenient/silent** behavior.
- All symlinks fail vs only managed/non-ignored fail: chose **only managed/non-ignored symlinks fail**.
- Filesystem-only submodule detection vs Git-aware: chose **Git-aware** detection from reconcile/repository boundary.
- App-root staging vs destination-root staging: chose **app-root staging** because Phase 3 is per-app.
- Cleanup failure error vs warning: chose **warning** to avoid invalidating successful analysis.
- Hash inventory vs streaming compare: chose **streaming compare** (no persisted inventory yet).
- Type mismatch error vs drift: chose **drift for managed file paths**; root non-dir remains app error.
- Fail-fast app processing vs aggregate: chose **aggregate app errors** so unrelated apps still process.
- Runner drift as error vs report only: chose **report only** because apply does not exist in Phase 3.
- Docs after code vs docs-first: chose **docs-first** to preserve decisions before implementation.

## 4. Open Risks / Unknowns

- `github.com/codeglyph/go-dotignore` still requires Platform Lead validation for import path, license, API fit, maintenance, and security.
- Lenient invalid-pattern behavior details depend on actual `go-dotignore` API behavior.
- `.gitmodules` handling via `git config --file .gitmodules --get-regexp path` needs precise behavior for missing file, no matches, parse errors, and path normalization.
- Future state/inventory phase must resolve removed-managed-file detection.
- Future apply phase must safely handle symlink/type-mismatch replacement without following symlinks.
- Future UI/status model likely needs richer structured results than current `Run(ctx) error`.

## 5. Out of Scope / Deferred

- Docker Compose validation/apply/delete.
- Real age decryptor implementation.
- Secret target materialization.
- Secret target content drift.
- Live file mutation/sync.
- Removed managed file cleanup.
- State store persistence.
- Deletion/archive workflow.
- Legacy `internal/git_repository.go` hardening.
- Legacy new-app/template cleanup.
- Full UI/status API.

## 6. Next Pipeline Steps

1. **Product Manager:** Convert decisions into ordered implementation tasks, define `.docs/todo.md` checklist structure, and sequence dependencies.
2. **Platform Lead:** Validate architecture guardrails, validate `go-dotignore` (or provide blocker-backed alternative), and define package/API guardrails.
3. **Go Implementer:** Docs first; then implement fileset/ignore, staging, drift, runner integration; continuously update `.docs/todo.md`; update `next.md`.
4. **Code Analyst:** Review changes.
5. **Security Analyst:** Security review.
6. **Human:** Final review and commit.

## 7. Implementation Dependencies

- Existing Application discovery/validation remains the upstream input.
- Observer manifest must include valid absolute `spec.destination`.
- Git repository checkout must exist before submodule path extraction.
- Platform Lead must explicitly approve or reject `github.com/codeglyph/go-dotignore`.
- No implementation should begin until docs/todo/decision-record plan is accepted by Product Manager and Platform Lead.

---

**Change-control note:** Any change to core decisions in this document requires explicit redirect/user approval before implementation or review criteria are altered.
