---
layout: default
---

# Installation

## Prerequisites

- **Git** — for repository sync
- **age** — for secret encryption/decryption ([install](https://github.com/FiloSottile/age#installation))
- **Docker Compose** — for application deployment (if using the `docker-compose` provisioner)

## Option 1: Install Script (recommended)

```bash
curl -sfL https://raw.githubusercontent.com/lyssar/skuld-cli/main/install.sh | sh
```

This will:
1. Detect your OS and architecture
2. Download the latest release from GitHub
3. Install to `~/.local/bin/skuld-cli`
4. Add `~/.local/bin` to your `PATH` if needed

## Option 2: Go Install

```bash
go install github.com/lyssar/skuld-cli@latest
```

Requires Go 1.25+.

## Option 3: Build from Source

```bash
git clone https://github.com/lyssar/skuld-cli.git
cd skuld-cli
task build
# Binary at .local/bin/skuld-cli
```

## Verify

```bash
skuld-cli version
```
