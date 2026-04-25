# First Coding Session Checklist

This checklist is the practical execution order for the first implementation session.

It is optimized to create a strong foundation before touching runtime Docker behavior.

---

## Session Goal

By the end of the first session, Skuld should be able to:

- represent the new `Application` manifest in code
- strictly parse and validate `skuld.yaml`
- discover applications recursively from an observer checkout path
- compute operational identity and runtime slug deterministically

Do **not** try to finish reconcile in this session.

---

## 0. Ground Rules for the Session

- Do not modify observer deployment behavior unless absolutely necessary.
- Do not evolve the legacy `internal/app.go` into the new model.
- Do not shell out to `docker compose` from reconcile yet.
- Do not start with `.skuldignore`, secrets, or deletion logic first.
- Build the core contracts first, then test them.

---

## 1. Create the New Package Skeleton

### Objective

Create the package boundaries the rest of the implementation will grow into.

### Tasks

- Create `internal/application/`
- Create `internal/provisioner/`
- Create `internal/decryptor/`
- Create `internal/state/`
- Create `internal/reconcile/`

### Minimum files to create

- `internal/application/types.go`
- `internal/application/manifest.go`
- `internal/application/validate.go`
- `internal/application/discovery.go`
- `internal/application/identity.go`
- `internal/provisioner/provisioner.go`
- `internal/decryptor/decryptor.go`
- `internal/state/types.go`
- `internal/reconcile/run.go`

### Definition of done

- packages compile as empty or stubbed modules
- names clearly reflect the new architecture

---

## 2. Define Core Types First

### Objective

Lock the internal contracts before writing behavior.

### Tasks

Define the new `Application` manifest structs:

- top-level Application
- metadata struct
- spec struct
- secret struct

Define supporting runtime types:

- discovered app descriptor
- runtime identity / slug fields
- state entry struct skeleton

Define interface skeletons:

- provisioner interface
- decryptor interface

### Important rules

- `Application` types must reflect the finalized docs, not the old legacy schema.
- Keep `spec.provisioner` explicit.
- Keep `composeFiles` and `secrets` in the new shape.
- Keep metadata optional fields (`description`, `labels`, `annotations`).

### Definition of done

- core types compile
- field names and structure align with the finalized concept docs

---

## 3. Implement Strict Manifest Parsing

### Objective

Make `skuld.yaml` parsing trustworthy.

### Tasks

- Add loader for `skuld.yaml`
- Enforce strict decoding with unknown-field rejection
- Validate:
  - `apiVersion`
  - `kind`
  - required `metadata.name`
  - required `spec.provisioner`
  - `composeFiles` rules for `docker-compose`
  - secret field presence rules

### Explicit checks to include now

- unknown fields fail
- missing `composeFiles` fail for `docker-compose`
- empty `composeFiles` fail
- duplicate `composeFiles` fail
- duplicate secret sources fail
- duplicate secret targets fail
- unsupported provisioner fails
- unsupported decryptor fails at validation layer if declared

### Definition of done

- given a path to `skuld.yaml`, code returns either:
  - a fully parsed application
  - a deterministic validation error

---

## 4. Implement Identity Helpers

### Objective

Make application identity deterministic before discovery is wired in.

### Tasks

- Add helper to compute relative app path from discovery root
- Add helper to validate path segments
- Add helper to compute runtime slug from relative app path

### Rules to enforce

- path segments may only contain lowercase letters, numbers, and hyphens
- slug is lowercase relative path with `/` replaced by `-`

### Definition of done

- one small pure module exists for identity and slug logic
- easy to test in isolation

---

## 5. Implement Application Discovery

### Objective

Turn a checked-out repo path into a sorted list of valid discovered applications.

### Tasks

- Scan recursively below a given root path
- Find exact `skuld.yaml` files only
- Build discovered app descriptors
- Detect nested apps
- Sort apps lexicographically by operational identity

### Important constraints

- discovery should not depend on Docker, secrets, or runtime state
- keep it purely filesystem + manifest based

### Definition of done

- given a discovery root, code returns discovered applications in deterministic order
- nested app layouts fail correctly

---

## 6. Add First Tests Before Going Further

### Objective

Establish testing style early and protect the new contracts.

### Tests to write in session one

#### Application manifest parsing

- valid minimal manifest parses successfully
- unknown field fails
- wrong apiVersion fails
- wrong kind fails
- missing `metadata.name` fails
- missing `composeFiles` fails for compose provisioner
- duplicate compose files fail

#### Identity helpers

- valid relative path produces correct slug
- invalid path segment fails
- repeated leaf names in different paths produce distinct identities/slugs when valid

#### Discovery

- recursive discovery finds multiple apps
- exact `skuld.yaml` naming is enforced
- nested apps fail
- lexicographic ordering is stable

### Definition of done

- tests compile and pass
- the new architecture has its first safety net

---

## 7. Add Minimal Reconcile Wiring Only If Time Remains

### Objective

Optionally connect the new modules to the existing reconcile entrypoint without implementing full runtime behavior.

### Tasks

- introduce a new reconcile runner skeleton in `internal/reconcile`
- load observer config
- resolve repo discovery root
- call discovery
- log discovered apps

### Do not do yet

- no staging
- no drift calculation
- no secret materialization
- no provisioner invocation
- no state persistence unless trivially scaffolded

### Definition of done

- `reconcile` can at least enumerate apps conceptually through the new path
- or the new reconcile runner exists as a stub ready for session two

---

## 8. Explicitly Defer to Session Two

These should be next, but not first-session priorities:

- `.skuldignore`
- managed file set resolution
- staging directory assembly
- decryptor registry and age implementation
- provisioner registry and docker compose implementation
- state store persistence
- deletion/archive logic
- failed-app retry semantics

---

## 9. Best Possible End State for Day One

If the first session goes very well, the ideal end state is:

- new package structure exists
- new Application schema exists
- strict manifest validation works
- discovery works recursively
- identity and slug logic work
- tests cover those modules
- reconcile entrypoint is ready to consume the new path next session

That is enough to start session two on staging, filesets, decryptors, and provisioners without redesign.

---

## 10. Suggested Commit Boundaries

If you decide to commit in small steps later, the clean boundaries are:

1. package skeleton + core types
2. strict Application manifest parsing/validation
3. discovery + identity helpers
4. first tests
5. reconcile skeleton wiring

---

## 11. Session Kickoff Command Mindset

Tomorrow, start by asking:

> Can the codebase correctly understand what an Application is before it tries to deploy one?

If the answer becomes yes by the end of the session, the day was successful.
