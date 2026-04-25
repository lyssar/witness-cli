# Application Reconciliation Implementation Plan

This document is the concrete implementation plan for bringing the finalized `Application` concept into the current Go codebase.

It is intentionally based on the repository **as it exists today** and is meant to be the starting point for development work.

---

## 1. Current Codebase Snapshot

### Existing command surface

- `cmd/init.go` — creates an Observer manifest
- `cmd/deploy.go` — deploys the Observer to a host
- `cmd/reconcile.go` — entrypoint for the observer reconcile loop
- `cmd/new-app.go` — legacy app scaffolding command tied to the old app concept
- `cmd/root.go` — root CLI and logging setup

### Existing internal modules

- `internal/observer.go` — current Observer manifest model and interactive creation flow
- `internal/deployment.go` — observer deployment over SSH/systemd
- `internal/reconcile.go` — current reconcile shell, mostly unimplemented
- `internal/app.go` — **legacy app concept**, not aligned with finalized Application contract
- `internal/driftManager/manager.go` — early drift/state types, likely reusable only in spirit
- `internal/sync.go` — empty placeholder
- `internal/cmd.go` — command handlers binding Cobra to internal logic
- `internal/prereq.go` — binary prerequisite checks

### Existing utilities

- `utils/age.go` — current age encryption helper, can inform the decryptor implementation
- `utils/ssh.go` — SSH utilities for observer deployment
- `utils/validation.go` — filesystem/path/binary helpers
- `utils/misc.go`, `utils/log.go` — shared misc/logging helpers

### Existing templates

- `templates/manifests/observer.yaml.gotmpl` — current observer manifest template
- `templates/manifests/application.yaml.gotmpl` — **legacy application template**, not aligned with finalized schema
- `templates/service.gotmpl`, `templates/timer.gotmpl` — systemd deployment templates

### Important current realities

- Observer deployment already exists and should remain the outer control-plane workflow.
- Reconcile exists only as a shell and is the main greenfield implementation area.
- There is currently **no test suite**.
- The current `App` concept and `new-app` flow are not aligned with the finalized Application contract and should not drive new architecture.

---

## 2. Recommended Implementation Strategy

Build the new application-management system **beside** the current legacy app concept first.

Do **not** start by refactoring `internal/app.go` into the new model.

Instead:

1. keep Observer bootstrap/deploy behavior intact
2. introduce the new Application runtime modules cleanly
3. implement the reconcile pipeline against the new modules
4. only later decide whether to remove or replace the legacy app scaffolding flow

This keeps product risk low and avoids mixing old assumptions into the new architecture.

---

## 3. Target Module Breakdown

The implementation should be split into deep, testable modules.

## 3.1 Application schema and validation

### Responsibility

- represent the canonical `Application` manifest
- parse `skuld.yaml`
- reject unknown fields
- validate `apiVersion`, `kind`, metadata, provisioner fields, secret definitions, path constraints, duplicate entries, and slug collisions

### Recommendation

Create a dedicated package for the new Application model rather than evolving `internal/app.go`.

Suggested package shape:

- `internal/application/manifest.go`
- `internal/application/validate.go`
- `internal/application/types.go`

### Why separate it

`internal/app.go` is tied to the old schema and interactive generation path. Reusing it would introduce conceptual confusion immediately.

---

## 3.2 Application discovery and identity

### Responsibility

- recursively scan the checked-out repo under `observer.spec.source.path`
- find exact `skuld.yaml` files
- reject nested apps
- compute operational identity from relative app path
- compute runtime slug
- sort discovered apps lexicographically by operational identity

### Suggested package

- `internal/application/discovery.go`
- `internal/application/identity.go`

### Key outputs

Per discovered app, produce a normalized descriptor like:

- manifest path
- app source dir
- relative app path
- runtime slug
- computed live deploy dir
- archive prefix

This descriptor should become the canonical input to the rest of reconcile.

---

## 3.3 Managed file resolution

### Responsibility

- load `.skuldignore` if present
- evaluate ignore rules relative to app dir
- compute the managed file set
- exclude `skuld.yaml`, `.skuldignore`, and secret source files
- include decrypted secret targets as managed outputs
- enforce symlink restrictions
- detect secret collisions with normal files

### Suggested package

- `internal/application/fileset.go`
- `internal/application/ignore.go`

### Key note

This should be a pure module that maps:

- application descriptor
- parsed manifest
- app source tree

to:

- managed input files
- managed output files
- warnings
- validation errors

This module should not know anything about Docker or runtime apply.

---

## 3.4 Decryptor abstraction

### Responsibility

- define the stable interface for secret decryption
- route each secret entry by `decryptor`
- implement `age` first
- write decrypted secret outputs into staging

### Suggested package

- `internal/decryptor/decryptor.go`
- `internal/decryptor/age.go`
- `internal/decryptor/registry.go`

### Suggested interface

At minimum, something equivalent to:

- `Decrypt(source []byte, context ...) ([]byte, error)`
- or `DecryptFile(srcPath, dstPath, context ...) error`

Prefer a small interface that keeps file IO orchestration outside the decryptor if possible.

### Reuse from current code

`utils/age.go` should be treated as implementation inspiration, not the final abstraction boundary.

It likely needs to be split into:

- reusable age identity loading/decryption helpers
- decryptor implementation code

---

## 3.5 Provisioner abstraction

### Responsibility

- define the stable interface between reconcile orchestration and runtime execution backends
- implement Docker Compose as the first provisioner
- isolate validation/apply/delete/runtime-cleanup behavior from the main reconcile loop

### Suggested package

- `internal/provisioner/provisioner.go`
- `internal/provisioner/registry.go`
- `internal/provisioner/dockercompose.go`

### Suggested interface

The interface should cover the lifecycle decisions already made:

- validate staged app
- apply staged/live app
- delete live app
- best-effort cleanup by runtime identity when live dir is missing

Likely methods:

- `Validate(...) error`
- `Apply(...) error`
- `Delete(...) error`
- `CleanupMissing(...) error`

The exact signatures should be designed around an app runtime context object rather than many primitive arguments.

### Important guardrail

`internal/reconcile.go` must never shell out to `docker compose` directly.

All provisioner-specific behavior must stay behind this interface.

---

## 3.6 Staging and drift engine

### Responsibility

- create temporary staging dirs
- assemble desired app state into staging
- materialize decrypted secret targets there
- compare staged desired managed files against live managed files
- decide whether apply is required
- clean up staging dirs after use

### Suggested package

- `internal/reconcile/staging.go`
- `internal/reconcile/drift.go`

### Current repo note

`internal/driftManager/manager.go` contains early drift-related types, but not the finalized model.

Recommendation:

- either replace it with the new drift engine
- or move only truly reusable concepts from it into the new package

Do not force the new design into the old type names if they no longer fit.

---

## 3.7 Observer-local state store

### Responsibility

- read/write one aggregate JSON state file
- maintain top-level version field
- maintain per-app entries keyed by operational identity
- update `healthy`, `failed`, `deleting`
- store timestamps, last error, last successful commit
- remove entries on successful deletion

### Suggested package

- `internal/state/store.go`
- `internal/state/types.go`

### Required design property

State operations should be simple and explicit:

- load whole file
- mutate in memory
- write atomically back

There is no need for a complicated persistence layer in v1.

---

## 3.8 Reconcile orchestration

### Responsibility

- load observer config from config root
- resolve observer-local paths
- verify host prerequisites
- update the persistent repo checkout
- discover apps
- reconcile each app independently in deterministic order
- process deleted apps from state
- aggregate run result

### Suggested package

- `internal/reconcile/run.go`
- `internal/reconcile/repository.go`
- `internal/reconcile/apply.go`
- `internal/reconcile/delete.go`

### Important split

Keep orchestration separate from:

- Application parsing/validation
- file-set calculation
- decryptor implementations
- provisioner implementations
- state storage

The orchestration layer should compose these modules, not absorb their logic.

---

## 3.9 Repository update module

### Responsibility

- manage the observer-local persistent checkout
- clone if missing
- fetch on each run
- hard switch/reset when required
- surface repo-update failure before app processing begins

### Suggested package

- `internal/reconcile/repository.go`

### Design note

This should be implemented as a small repository manager abstraction, because it is a major root-of-truth boundary in reconcile.

---

## 4. Recommended Directory/Package Plan

One clean way to evolve the current repo without breaking existing observer deployment code:

```text
internal/
  application/
    types.go
    manifest.go
    validate.go
    discovery.go
    identity.go
    ignore.go
    fileset.go
  decryptor/
    decryptor.go
    registry.go
    age.go
  provisioner/
    provisioner.go
    registry.go
    dockercompose.go
  state/
    types.go
    store.go
  reconcile/
    run.go
    repository.go
    staging.go
    drift.go
    app.go
    delete.go
```

And then gradually retire or isolate these older files:

- `internal/app.go`
- `templates/manifests/application.yaml.gotmpl`
- `cmd/new-app.go`

Not necessarily on day one, but they should no longer guide the implementation architecture.

---

## 5. Concrete Phase Plan

## Phase 1 — Internal types and interfaces

### Goal

Create the stable internal contracts before implementing runtime behavior.

### Deliverables

- new Application types
- new state types
- provisioner interface
- decryptor interface
- app descriptor / runtime context types

### Exit criteria

- team agrees the module boundaries are correct
- no compose logic leaks into orchestration design
- no decryption logic leaks into manifest parsing

---

## Phase 2 — Manifest parsing, validation, and discovery

### Goal

Make the repo checkout produce validated discovered applications.

### Deliverables

- strict `skuld.yaml` parser
- unknown-field rejection
- discovery under observer `source.path`
- nested app detection
- slug generation and collision detection
- deterministic sorted app list

### Exit criteria

- given a checked-out repo, the code can list valid apps and fail invalid ones correctly

---

## Phase 3 — Managed file set and staging

### Goal

Build desired app state without touching the live deployment.

### Deliverables

- `.skuldignore` support
- managed file set calculation
- symlink rejection
- secret source exclusion
- staging dir builder
- content-based drift detection against live files

### Exit criteria

- code can assemble a full desired app layout in staging
- code can decide whether drift exists without invoking Docker

---

## Phase 4 — Decryptor abstraction and age implementation

### Goal

Support secret materialization through a pluggable decryptor path.

### Deliverables

- decryptor registry
- age decryptor implementation
- secret target materialization into staging
- secret permission handling policy hooks

### Exit criteria

- staging can contain decrypted runtime targets for all declared secrets

---

## Phase 5 — Provisioner abstraction and Docker Compose implementation

### Goal

Make runtime execution pluggable, with Docker Compose as the first provisioner.

### Deliverables

- provisioner registry
- docker compose validation implementation
- docker compose apply implementation
- docker compose delete implementation
- missing-live-dir cleanup by project metadata

### Exit criteria

- no `docker compose` shelling from reconcile orchestration
- all compose behavior is behind the provisioner interface

---

## Phase 6 — Observer-local state store

### Goal

Persist app results and deletion progress.

### Deliverables

- aggregate JSON state file read/write
- atomic write strategy
- per-app status update helpers
- deletion cleanup of state entries

### Exit criteria

- state reflects success, failure, retry, and deletion flows correctly

---

## Phase 7 — Reconcile orchestration

### Goal

Wire everything into the actual observer runtime loop.

### Deliverables

- repo checkout/update flow
- prereq checks for reconcile host
- independent app reconcile loop
- failed-app retry logic
- deletion retry logic
- run-level success/failure result

### Exit criteria

- `reconcile` can execute the full app-management flow end-to-end

---

## Phase 8 — CLI/documentation cleanup

### Goal

Align command/docs/templates with the new implementation.

### Deliverables

- update or replace legacy application template
- decide future of `new-app`
- clean up stale concept remnants in code comments/help text

### Exit criteria

- no major user-facing docs or commands contradict the implemented runtime model

---

## 6. Recommended File-Level Changes

These are the most likely first edits when development starts.

### Keep and extend

- `internal/observer.go`
  - keep Observer manifest/runtime meaning
  - possibly reduce interactive assumptions later

- `internal/deployment.go`
  - keep observer deployment flow
  - update only if reconcile path/config-root layout changes

- `internal/prereq.go`
  - extend reconcile prerequisites to check `docker` / `docker compose`

- `internal/cmd.go`
  - rewire `ReconcileCmd` to the new reconcile package

### Replace or bypass

- `internal/reconcile.go`
  - likely replaced by new reconcile package orchestration

- `internal/driftManager/manager.go`
  - replace or heavily rewrite to fit finalized drift model

- `internal/app.go`
  - do not evolve into the new Application model; leave as legacy until cleanup phase

- `templates/manifests/application.yaml.gotmpl`
  - currently stale for the finalized schema

- `cmd/new-app.go`
  - keep only if explicitly needed; otherwise leave untouched until later cleanup

---

## 7. Testing Plan

There is no current prior art in the repo, so testing style will need to be established.

### First modules to test

1. Application schema + validation
2. Discovery + identity
3. Managed file set + ignore behavior
4. Decryptor registry + age decryptor behavior
5. Provisioner abstraction with mocked command runner
6. State store read/write/update behavior
7. Reconcile orchestration with fake provisioner/decryptor/state/repo modules

### Strong recommendation

Do **not** start with end-to-end shell-driven tests.

Start with pure module tests for:

- parsing
- validation
- file-set resolution
- state transitions
- interface-driven orchestration

Then add integration tests around Docker Compose behavior once the provisioner boundary exists.

---

## 8. Major Risks to Watch During Implementation

### Risk 1 — legacy app concept pollutes the new design

Mitigation:

- isolate new code into new packages
- avoid reusing `internal/app.go` as the canonical model

### Risk 2 — provisioner abstraction is skipped “temporarily”

Mitigation:

- define the interface before implementing compose
- refuse direct compose logic in reconcile orchestration

### Risk 3 — managed file logic becomes destructive

Mitigation:

- keep staging and drift modules pure and heavily tested
- never implement broad directory replacement for normal reconcile

### Risk 4 — deletion path becomes unsafe

Mitigation:

- keep archive-first semantics explicit
- never delete after archive failure
- never delete volumes in v1

### Risk 5 — state model grows too early

Mitigation:

- keep state minimal
- avoid file-by-file inventory in v1

---

## 9. Recommended First Day Task List

If development starts tomorrow, this is the best first slice:

1. Create new package skeletons:
   - `internal/application`
   - `internal/provisioner`
   - `internal/decryptor`
   - `internal/state`
   - `internal/reconcile`
2. Define core types only:
   - Application manifest structs
   - app descriptor/runtime context
   - state structs
   - provisioner interface
   - decryptor interface
3. Implement strict manifest parsing + validation
4. Implement discovery + identity + slug generation
5. Add tests for those first modules before touching Docker execution

That gives a solid foundation without yet committing to runtime execution details.

---

## 10. Recommended Decision for Tomorrow

Start with **architecture-first implementation**, not CLI UX and not Docker execution.

The most valuable first milestone is:

> “Given an observer checkout root, Skuld can discover, parse, validate, and describe all applications correctly.”

Once that exists, the rest of the reconcile pipeline becomes much safer to build.
