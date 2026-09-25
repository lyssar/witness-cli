<p align="center">
  <img src="assets/logo.png" alt="Witness Logo" width="320" />
</p>

<h1 align="center">Witness</h1>
<h3 align="center">the future’s watcher for your fleet</h3>

<p align="center">
  <a href="https://lyssar.github.io/witness-cli/">📖 Documentation</a>
  ·
  <a href="https://github.com/lyssar/witness-cli/releases">📦 Releases</a>
</p>

<p align="center">
  <em>☥ Peace through power ☥</em>
</p>

---

## Install

```bash
curl -sfL https://raw.githubusercontent.com/lyssar/witness-cli/main/install.sh | sh
```

Or build from source: `task build`

## Quick Start

- Build: `task build`
- Validate: `task validate`
- Local reconcile harness: `task local:run`
- Local harness smoke: `task local:smoke`
- [Full documentation →](https://lyssar.github.io/witness-cli/)

## Prepare AGE Key

```bash
AGE_KEY_FILE="/path/to/key.age"
age-keygen -o "${AGE_KEY_FILE}"

# if you use 1password push the entry to save it
op item create --category="API_Credential" --title "API Credentials ($(date +%d/%m/%Y))" "[file]=${AGE_KEY_FILE}"
```

## Local Harness

The local harness is Docker Compose-based and scoped to the runtime reconcile command.

It uses:

- `docker compose` as the only orchestrator
- a dedicated harness image from `docker/local-harness/Dockerfile`
- a host-built `witness` binary mounted read-only into the container
- a writable runtime root for reconcile output and cloned repo state
- a preseeded no-drift destination tree so the smoke path can stay honest without simulating real docker runtime apply behavior in-container
- the prepared manifest and age key mounted read-only
- a seeded source repo mounted read-only

Run it with:

```bash
task local:prepare
task local:run
```

Or run the reduced-scope smoke check end-to-end with:

```bash
task local:smoke
```

This reduced harness does **not** attempt to simulate SSH, sudo, systemd, full target-host bootstrap, or real docker-compose apply execution.

CircleCI runs the release pipeline only (semantic-release on `main` + GoReleaser on `v*` tags); `task validate` and `task local:smoke` remain local-only.

See [Local Harness documentation](https://lyssar.github.io/witness-cli/local-harness) for exact scope and mounted paths.

## Development

> `witness doctor` checks target-host prerequisites and is invoked by `witness deploy` at the start of every deploy. The commands below exercise it locally and against remote hosts.

Build and validate:

```bash
task build
task validate
```

Local non-interactive prerequisite report (installs nothing, non-zero exit if tools are missing):

```bash
./.local/bin/witness doctor --local --check-only
```

Local interactive prerequisite check (prompts before installing):

```bash
./.local/bin/witness doctor --local
```

Remote prerequisite check:

```bash
./.local/bin/witness doctor --host <host> --ssh-user <user> --check-only
```

Deploy skipping the prerequisite check (for already-provisioned hosts):

```bash
./.local/bin/witness deploy --host <host> --ssh-user <user> --skip-prereq-check --age-key <path>
```

Targeted unit tests for the doctor feature:

```bash
go test ./internal/ -run 'TestParseOSRelease|TestDoctorToolInstallCommands|TestResolvePkgMgr' -v
```

## Manual Secret Inspection

```bash
export manifest="my-manifest.yaml"
export ageKeyFile="my-key.age"
secret_file="$(mktemp)"
trap 'rm -f "${secret_file}"' EXIT
chmod 600 "${secret_file}"

secret_source="$(yq -r '.spec.secrets[0].source' "${manifest}")"
secret_source_path="$(dirname "${manifest}")/${secret_source}"
base64 -d "${secret_source_path}" | age -d -i "${ageKeyFile}" > "${secret_file}"

nano "${secret_file}"
```
