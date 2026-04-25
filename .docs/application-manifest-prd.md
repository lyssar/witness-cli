## Problem Statement

`skuld-cli` already has an Observer model that can be deployed to a bare server and run reconciliation on a systemd timer. What is missing is a clear, final contract for how that observer should discover, validate, reconcile, update, and delete observed applications.

The current application concept is unclear and partially conflicts with the actual product direction. The project needs a finalized v1 `Application` manifest contract for bare-server GitOps, centered on directory-based application discovery, Docker Compose deployments, file-based encrypted secrets, deterministic reconciliation, and a pluggable architecture for both provisioners and decryptors.

## Solution

Define a canonical v1 `Application` manifest (`skuld.yaml`) that lives inside each application directory in the observed Git repo. The Observer will recursively scan a configured repo subpath for these manifests, derive operational identity from each app’s relative path, and reconcile each app independently.

Each Application will describe only runtime/deployment behavior, not source control configuration. In v1, the only supported provisioner is `docker-compose`, but the architecture must introduce a provisioner abstraction so additional provisioners can be added later without changing reconcile orchestration. Likewise, file-based secrets will use a pluggable decryptor abstraction, with `age` as the first implementation and the observer’s age key as the shared decryption identity.

The resulting PRD should define:
- the exact `Application` schema
- application discovery and identity rules
- managed file semantics and `.skuldignore` behavior
- secret decryption/materialization rules
- pluggable provisioner/decryptor abstractions
- Docker Compose validation/apply/delete semantics
- archival and deletion behavior
- observer-local state expectations
- reconcile success/failure/retry rules

## User Stories

1. As an operator, I want to place one `skuld.yaml` file inside each app directory, so that app configuration lives next to the files it governs.
2. As an operator, I want the observer to recursively discover apps below a configured repo subpath, so that one repo can contain multiple managed applications.
3. As an operator, I want multiple apps with the same leaf directory name in different subpaths, so that I can organize repos cleanly without naming collisions.
4. As an operator, I want app deployment paths on the server to preserve repo-relative structure, so that the live layout remains easy to understand.
5. As an operator, I want `metadata.name` to remain descriptive only, so that human-readable metadata does not affect runtime identity.
6. As an operator, I want the app manifest to explicitly declare its provisioner, so that future provisioners can be added without changing schema shape.
7. As an operator, I want `docker-compose` to be the only supported v1 provisioner, so that the first release stays focused and reliable.
8. As an operator, I want compose file paths to be explicitly declared, so that the manifest fully documents what is used for deployment.
9. As an operator, I want compose file order to matter, so that Compose merge semantics remain correct and deterministic.
10. As an operator, I want all normal app files copied by default unless excluded, so that the whole app directory acts as the deployable unit.
11. As an operator, I want `.skuldignore` support, so that unnecessary local-only files can be excluded from deployment.
12. As an operator, I want files explicitly referenced by the manifest to override `.skuldignore`, so that the manifest remains authoritative.
13. As an operator, I want encrypted secret source files declared explicitly in the manifest, so that secrets remain documented without embedding plaintext in YAML.
14. As an operator, I want decrypted secret targets written into the live app directory, so that Docker Compose can consume them directly.
15. As an operator, I want env-var secrets handled through decrypted `.env` files, so that there is only one v1 secret model.
16. As an operator, I want decryptors to be pluggable from the start, so that future encryption backends can be added without refactoring reconcile orchestration.
17. As an operator, I want the observer’s age key reused for all app secret decryption, so that key management stays simple in v1.
18. As an operator, I want manifest validation to fail before touching the live app, so that existing running workloads remain untouched on validation errors.
19. As an operator, I want any managed-file drift to trigger reconciliation, so that Git and declared secret outputs remain authoritative.
20. As an operator, I want manual changes to managed files on the server overwritten on reconcile, so that local drift is corrected automatically.
21. As an operator, I want changes to secrets or config files to recreate containers, so that runtime state actually picks up updated configuration.
22. As an operator, I want removed services to be cleaned up automatically, so that orphaned containers do not linger after config changes.
23. As an operator, I want one broken application to fail independently without blocking other applications, so that unrelated workloads can still converge.
24. As an operator, I want apps removed from Git to be undeployed automatically, so that Git remains the source of truth for desired app existence.
25. As an operator, I want deleted apps archived before removal, so that runtime data and unmanaged files can be recovered if needed.
26. As an operator, I want deletion failures to preserve local state and retry later, so that transient shutdown/archive problems can self-heal.
27. As an operator, I want observer-local status persisted per app, so that I can understand what last succeeded or failed even after the process exits.
28. As an operator, I want strict schema validation and strict collision checks, so that app behavior stays explicit and predictable.
29. As an operator, I want nested apps forbidden, so that one app cannot accidentally manage another app’s subtree.
30. As an operator, I want unsupported constructs like symlinks and in-app submodules to fail the affected app, so that runtime behavior remains safe and deterministic.

## Implementation Decisions

- Introduce a canonical `Application` manifest with `apiVersion: skuld.dev/v1alpha1` and `kind: Application`.
- The manifest filename is exactly `skuld.yaml`.
- The application source is implicit: the directory containing `skuld.yaml` is the app source directory.
- Applications are discovered recursively under the observer’s configured repo subpath.
- The observer’s existing repo source configuration remains the only source-control definition; `Application` has no `source` block in v1.
- Operational identity is derived from the app’s relative path under the observer discovery root.
- Runtime slug generation is derived from that relative path by lowercasing and replacing path separators with `-`, while preserving safe characters.
- Deployment layout preserves the relative path under the observer runtime root.
- `metadata.name` is required and descriptive only.
- `metadata.description`, `metadata.labels`, and `metadata.annotations` are optional.
- `metadata.labels` and `metadata.annotations` are string-to-string maps.
- No metadata keys are reserved in v1.
- `spec.provisioner` is required and remains explicit even though only `docker-compose` is implemented in v1.
- Provisioner-specific validation is strict one-of validation.
- Add a Provisioner Abstraction module that defines the stable interface between reconcile orchestration and concrete provisioner implementations.
- Reconcile orchestration must not hardcode Docker Compose logic; instead it should invoke the selected provisioner through that abstraction.
- The first provisioner implementation is Docker Compose.
- For Docker Compose apps, `composeFiles` is required, non-empty, ordered, relative to the app directory, and duplicate-free.
- Declared compose files must exist and are part of desired state, including their order.
- Add a Decryptor Abstraction module that mirrors the provisioner pluggability approach.
- Each secret entry requires `source`, `target`, and `decryptor`.
- Secret sources are relative to the app directory and must stay within it.
- Secret targets are relative to the deployed app root and must stay within it.
- All configured secret source files are excluded from normal materialization.
- Decrypted secret target files are materialized into the deployed app directory and are part of the managed file set.
- Secret target collisions with normal managed files are validation errors.
- Duplicate secret sources and duplicate secret targets are validation errors.
- Unknown decryptors are validation errors.
- The only implemented decryptor in v1 is `age`, using the observer-level age key.
- The managed file set is defined as all app files minus `skuld.yaml`, `.skuldignore`, ignored files, and secret source files, plus decrypted secret targets.
- `.skuldignore` uses `.gitignore`-style patterns relative to the app directory.
- If `.skuldignore` matches a manifest-required file, Skuld warns and still includes that file.
- Drift detection compares staged desired managed files against live managed files on disk.
- Drift is content-only in v1.
- Symlinks in the managed file set are forbidden and fail the app.
- Git submodules inside an app tree are unsupported and fail only that app.
- Nested applications are forbidden.
- App path segments are restricted to lowercase letters, numbers, and hyphens.
- Runtime slug collisions are validation errors for the affected apps.
- Validation happens in a temporary staging directory that mirrors the final live app layout.
- Staging is always cleaned up afterward; cleanup failure is a warning, not a failed successful deploy.
- First-time deploys must validate successfully before creating the live app directory.
- Normal reconciles mutate only the managed file set in the live app directory.
- Unmanaged/runtime files in the app directory are untouched during normal reconcile.
- Compose validation contract is `docker compose ... config` using declared compose files and the explicit runtime project name.
- Compose apply contract is `docker compose ... up -d --force-recreate --remove-orphans`.
- Compose project name is explicitly set from the runtime slug.
- Healthy apps are change-driven; failed apps retry even without new drift.
- Deleted apps that previously failed deletion retry on every reconcile until success.
- Success for an app means validation passed, managed files were reconciled, and the compose command completed successfully.
- Success for an observer run means repo update succeeded and all discovered apps reconciled successfully.
- Zero discovered apps is a successful no-op run.
- The observer keeps a persistent local Git checkout/worktree in its control-plane work directory.
- Git update failures abort the whole reconcile run before app processing.
- If the observer’s target revision changes, checkout behavior must hard-switch/reset rather than merge/rebase.
- Runtime prerequisites for v1 include modern `docker compose`; missing Docker/Compose aborts the whole run early.
- App deletion is triggered when an app previously known to the observer is no longer discovered from Git.
- Deletion runs Compose shutdown, archives the entire live app directory, and then removes it.
- Archive filenames are timestamped and stored under the observer root’s fixed `archives` directory.
- If the app directory is already missing, Skuld performs best-effort cleanup using Docker project labels/metadata.
- Compose-managed networks are removed during deletion; Docker volumes are not removed in v1.
- If deletion or archival fails, the app state remains and the deletion is marked failed for retry/manual intervention.
- Observer-local state is a single aggregate versioned JSON file keyed by app operational identity.
- Per-app persisted state includes status, last error, last attempted reconcile timestamp, last successful reconcile timestamp, and last successful resolved Git commit.
- Supported app status values in v1 are `healthy`, `failed`, and `deleting`.
- Last error is cleared on the next successful reconcile.
- Successful deletion removes the app’s state entry immediately with no tombstone.

## Testing Decisions

- Good tests for this feature should verify external behavior and contracts:
  - manifest validation outcomes
  - discovery/identity resolution
  - managed file set calculation
  - drift detection behavior
  - decryptor and provisioner interface contracts
  - deletion/archive decision logic
  - state transitions and retry behavior
- Tests should focus on deterministic inputs and outputs rather than internal implementation details.
- Modules that should have tests written:
  - Application Manifest Schema
  - Application Discovery & Identity
  - Managed File Resolution
  - Secret Decryption Pipeline
  - Provisioner Abstraction
  - Docker Compose Provisioner
  - Deletion & Archival Workflow
  - Observer Local State
  - Reconcile Execution Model
- The strongest early test seam is the Provisioner Abstraction, because it allows reconcile logic to be tested independently from Docker CLI execution.
- There is currently no prior test suite in the codebase to mirror; new tests will establish the project’s testing style.

## Out of Scope

- Implementing the new Application reconcile flow
- Refactoring or removing legacy app-generation code immediately
- Additional provisioners beyond `docker-compose`
- Additional decryptors beyond `age`
- Inline secret values in manifests
- Application-level source repo overrides
- Cross-app dependencies or ordered orchestration
- Automatic rename detection or rename-aware data migration
- Docker volume deletion during app deletion
- Runtime healthcheck-based success evaluation
- Git submodule support inside apps
- Symlink support
- CLI-first app scaffolding as the primary authoring path
- Observer schema redesign beyond assumptions needed by the Application contract
- Selective archival using a managed-state inventory
- Internal locking beyond relying on systemd/timer serialization
- Automatic image pulls without Git/file drift
- Reserved metadata keys

## Future Enhancements

- Additional provisioners such as `docker` and `shell`
- Additional decryptors and richer secret backends
- Managed-state snapshots for finer-grained deletion cleanup and archival
- Runtime health verification and degraded status reporting
- Optional per-app restart or apply policies
- Rename-safe app identity with stable IDs
- App scaffolding/generation UX
- Observer and Application status commands
- More advanced cleanup and recovery tooling
- Optional submodule support
- Optional symlink support with safe confinement rules

## References

- Existing concept notes in the repository describing Observer and Application directions
- Current observer-based reconcile/deploy command structure already present in the project
- Docker Compose CLI behavior for `config` and `up`
- age encryption tooling used by the current observer flow
