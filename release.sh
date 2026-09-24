#!/bin/bash
VERSION="${1:?Usage: ./release.sh VERSION}"

if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "ERROR: VERSION must be strict semver (MAJOR.MINOR.PATCH, numeric only), got '$VERSION'" >&2
  exit 1
fi

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
  --notes "## Improvements
- Volume claim apply now triggers \`docker compose up --force-recreate\` so containers restart and pick up the corrected bind-mount directory ownership." \
  .local/release/witness_${VERSION}_*.tar.gz
