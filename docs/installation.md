---
layout: default
title: Installation
---

# Installation

## Prerequisites

### Local Machine (where you run `witness`)

- **Git** — repository operations
- **age** — secret encryption/decryption ([install age](https://github.com/FiloSottile/age#installation))
- **SSH** — deployment transport
- **Go 1.25+** — only if building from source

### Target Server (where apps run)

Install before the first `witness deploy`:

```bash
sudo apt install git age docker.io docker-compose-v2 libcap2-bin
```

| Package | Purpose |
|---|---|
| `git` | Repository sync |
| `age` | Secret decryption |
| `docker` + `docker compose` | Container runtime |
| `libcap2-bin` | Volume claim ownership (recommended; degrades gracefully without it) |

See the [Quickstart → Step 4](quickstart) for details on target server setup.

## Option 1: Install Script (recommended)

One command, ready to deploy:

```bash
curl -sfL https://raw.githubusercontent.com/lyssar/witness-cli/main/install.sh | sh
```

The script will:
1. Detect your OS (Linux/macOS) and architecture (amd64/arm64)
2. Download the latest release from GitHub
3. Install to `~/.local/bin/witness`
4. Add it to your `PATH` if needed

## Option 2: Go Install

```bash
go install github.com/lyssar/witness-cli@latest
```

Requires Go 1.25+.

## Option 3: Build from Source

```bash
git clone https://github.com/lyssar/witness-cli.git
cd witness-cli
task build
# Binary ready at .local/bin/witness
```

## Option 4: Taskfile

The project includes a `Taskfile.yml` with common tasks:

```bash
task build        # Build the binary
task validate     # Run full validation suite
task local:smoke  # Run the reduced-scope harness
```

## Verify

```bash
witness version
```

## Next Steps

<div class="highlight-box">
<strong>→ Proceed to the <a href="quickstart">Quickstart</a></strong> to set up your first observer and deploy an application.
</div>
