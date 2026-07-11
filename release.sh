#!/bin/bash
set -euo pipefail

VERSION="${1:?Usage: ./release.sh VERSION}"

git add -A
git commit -m "fix: install GitHub host key during deploy

Pre-install github.com to known_hosts so witness reconcile can
clone repos without interactive SSH host verification prompt."

git push --force origin main

git tag -d "v${VERSION}" 2>/dev/null; git push origin --delete "v${VERSION}" 2>/dev/null; true
git tag "v${VERSION}"
git push origin "v${VERSION}"

rm -rf .local/release && mkdir -p .local/release
for P in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  O=${P%/*}; A=${P#*/}
  CGO_ENABLED=0 GOOS=$O GOARCH=$A go build -trimpath -ldflags="-s -w -X github.com/lyssar/witness-cli/version.Version=${VERSION}" -o .local/release/witness .
  tar -czf ".local/release/witness_${VERSION}_${O}_${A}.tar.gz" -C .local/release witness
  rm .local/release/witness
done

gh release delete "v${VERSION}" --yes 2>/dev/null; true
gh release create "v${VERSION}" \
  --title "v${VERSION}" \
  --notes "## Bug Fixes
- Install GitHub host key during deploy (known_hosts)
- Fix ObserverConfigPath: include project name in reconcile path
- Fix WorkingDirectory: point to config root, not home
- Fix permissions: recursive chown after file uploads
- Fix ReloadSystemD: ignore SSH EOF errors
- Fix systemd-analyze verify: use service names instead of glob
- Fix deploy: create temp dir without sudo (SFTP runs as SSH user)
- Fix SSH EOF errors in all deploy operations
- Fix deploy: check age as root, not execution user
- Fix deploy: remove witness binary pre-check
- Fix deploy: treat 'not-found' service state as expected on first deploy" \
  .local/release/witness_${VERSION}_*.tar.gz
