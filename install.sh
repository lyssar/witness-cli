#!/usr/bin/env sh
set -eu

# ──────────────────────────────────────────────
#  Witness — install.sh
#  Download and install the latest witness binary
#
#  System-wide (default, uses sudo):
#    curl -sfL https://raw.githubusercontent.com/lyssar/witness-cli/main/install.sh | sh
#
#  User-local (no sudo):
#    curl -sfL https://raw.githubusercontent.com/lyssar/witness-cli/main/install.sh | sh -s -- --user
#
#  Private repo (set GITHUB_TOKEN):
#    export GITHUB_TOKEN=ghp_xxx
#    curl -sfL -H "Authorization: token $GITHUB_TOKEN" \
#      https://raw.githubusercontent.com/lyssar/witness-cli/main/install.sh \
#      | GITHUB_TOKEN=$GITHUB_TOKEN sh
# ──────────────────────────────────────────────

# --- Auth helpers for private repos ---
_AUTH_CURL="curl -sfL"
_AUTH_WGET="wget -qO-"
if [ -n "${GITHUB_TOKEN:-}" ]; then
  _AUTH_CURL="curl -sfL -H \"Authorization: token ${GITHUB_TOKEN}\""
  _AUTH_WGET="wget -qO- --header=\"Authorization: token ${GITHUB_TOKEN}\""
fi

REPO="lyssar/witness-cli"
BIN_NAME="witness"
WITNESS_USER="witness"

# --- Colors ---
BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { printf "${GREEN}✓${NC} %s\n" "$*"; }
warn()  { printf "${YELLOW}⚠${NC} %s\n" "$*"; }
err()   { printf "${RED}✗${NC} %s\n" "$*"; exit 1; }
header(){ printf "\n${BOLD}%s${NC}\n" "$*"; }

# --- Sudo helper ---
_SUDO=""
if [ "$(id -u)" -ne 0 ]; then
  _SUDO="sudo"
fi

# --- Detect OS and Architecture ---
detect_platform() {
  local os arch

  case "$(uname -s)" in
    Linux)  os="linux"   ;;
    Darwin) os="darwin"  ;;
    *)      err "unsupported OS: $(uname -s). Only Linux and macOS are supported." ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) err "unsupported architecture: $(uname -m). Only amd64 and arm64 are supported." ;;
  esac

  echo "${os}_${arch}"
}

# --- Check Prerequisites ---
check_prereqs() {
  local missing=0

  if ! command -v git >/dev/null 2>&1; then
    warn "git is not installed. Install it first: https://git-scm.com/"
    missing=1
  fi

  if ! command -v age-keygen >/dev/null 2>&1; then
    warn "age is not installed. Install it first: https://github.com/FiloSottile/age#installation"
    missing=1
  fi

  if ! command -v docker >/dev/null 2>&1; then
    warn "docker is not installed. Needed for the docker-compose provisioner."
    warn "Install it first: https://docs.docker.com/engine/install/"
  fi

  if [ "$missing" -eq 1 ]; then
    err "missing required dependencies. Install them and re-run this script."
  fi
}

# --- Get Latest Release ---
get_latest_version() {
  if command -v curl >/dev/null 2>&1; then
    eval "${_AUTH_CURL}" "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name":' \
      | sed -E 's/.*"([^"]+)".*/\1/' \
      || echo ""
  elif command -v wget >/dev/null 2>&1; then
    eval "${_AUTH_WGET}" "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name":' \
      | sed -E 's/.*"([^"]+)".*/\1/' \
      || echo ""
  else
    err "neither curl nor wget found. Install one of them and re-run."
  fi
}

# --- Download Binary ---
download_binary() {
  local platform="$1"
  local version="$2"
  local dest_dir="$3"

  local archive_name="${BIN_NAME}_${version#v}_${platform}.tar.gz"
  local download_url="https://github.com/${REPO}/releases/download/${version}/${archive_name}"

  info "downloading ${archive_name} ..."

  local tmpdir
  tmpdir="$(mktemp -d)"
  local archive_path="${tmpdir}/${archive_name}"

  if command -v curl >/dev/null 2>&1; then
    eval "${_AUTH_CURL}" -o "${archive_path}" "${download_url}"
  elif command -v wget >/dev/null 2>&1; then
    eval "${_AUTH_WGET}" -O "${archive_path}" "${download_url}"
  fi

  if [ ! -f "${archive_path}" ]; then
    rm -rf "${tmpdir}"
    err "download failed: ${download_url}"
  fi

  tar -xzf "${archive_path}" -C "${tmpdir}" "${BIN_NAME}" 2>/dev/null || {
    rm -rf "${tmpdir}"
    err "extraction failed — the archive format may have changed."
  }

  local binary_src="${tmpdir}/${BIN_NAME}"
  if [ ! -f "${binary_src}" ]; then
    rm -rf "${tmpdir}"
    err "binary not found in archive."
  fi

  chmod 755 "${binary_src}"

  # Install — system-wide via sudo or direct
  ${_SUDO} install -m 755 "${binary_src}" "${dest_dir}/${BIN_NAME}"
  rm -rf "${tmpdir}"

  info "binary installed to ${dest_dir}/${BIN_NAME}"
}

# --- Create witness system user (Linux only) ---
create_witness_user() {
  if id -u "${WITNESS_USER}" >/dev/null 2>&1; then
    info "witness system user already exists"
    return 0
  fi

  case "$(uname -s)" in
    Linux)
      ${_SUDO} useradd --system --create-home \
        --shell /usr/sbin/nologin \
        "${WITNESS_USER}"
      info "created witness system user (home: /home/witness)"
      ;;
    Darwin)
      warn "skipping witness system user creation (unsupported on macOS)"
      warn "create it manually: sudo sysadminctl -addUser witness"
      ;;
  esac
}

# --- Ensure Install Directory is in PATH (user-local only) ---
ensure_path() {
  local install_dir="$1"

  if command -v "${BIN_NAME}" >/dev/null 2>&1; then
    return 0
  fi

  local rc_file=""
  case "${SHELL}" in
    */zsh) rc_file="${HOME}/.zshrc" ;;
    */bash) rc_file="${HOME}/.bashrc" ;;
  esac

  if [ -n "${rc_file}" ] && [ -f "${rc_file}" ]; then
    local path_line="export PATH=\"\${PATH}:${install_dir}\""
    if ! grep -qF "${install_dir}" "${rc_file}" 2>/dev/null; then
      printf "\n%s\n" "${path_line}" >> "${rc_file}"
      info "added ${install_dir} to PATH in ${rc_file}"
      warn "restart your shell or run: source ${rc_file}"
    fi
  fi
}

# ──────────────────────────────────────────────
#  Main
# ──────────────────────────────────────────────

header "Witness — Installer"
echo ""

# Parse arguments
USER_INSTALL=false
INSTALL_DIR=""

while [ $# -gt 0 ]; do
  case "$1" in
    --user)
      USER_INSTALL=true
      shift
      ;;
    --dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --help|-h)
      echo "Usage: install.sh [options]"
      echo ""
      echo "Options:"
      echo "  --dir <path>    Install directory"
      echo "                  (default: /usr/local/bin, or ~/.local/bin with --user)"
      echo "  --user          User-local install (no sudo, no system user)"
      echo "  --help, -h      Show this help"
      echo ""
      echo "Examples:"
      echo "  curl -sfL https://raw.githubusercontent.com/lyssar/witness-cli/main/install.sh | sh"
      echo "  curl -sfL https://raw.githubusercontent.com/lyssar/witness-cli/main/install.sh | sh -s -- --user"
      exit 0
      ;;
    *) err "unknown argument: $1 (use --help for usage)" ;;
  esac
done

# Set defaults
if [ "${USER_INSTALL}" = "true" ]; then
  INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"
else
  INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
fi

PLATFORM="$(detect_platform)"
info "detected platform: ${PLATFORM}"

header "Checking prerequisites..."
check_prereqs

header "Fetching latest release..."
VERSION="$(get_latest_version)"

if [ -n "${VERSION}" ]; then
  info "latest version: ${VERSION}"
else
  warn "no GitHub release found for ${REPO}"
  VERSION="dev"
fi

header "Installing ${BIN_NAME} ${VERSION} ..."

case "${VERSION}" in
  dev)
    # Build from source via git clone + go build
    if ! command -v go >/dev/null 2>&1; then
      err "no release found and go is not installed.
  Create a GitHub release first, or install Go and re-run."
    fi
    info "building from source (this may take a moment)..."
    BUILD_DIR="$(mktemp -d)"
    git clone --depth 1 "https://github.com/${REPO}.git" "${BUILD_DIR}" 2>/dev/null || {
      rm -rf "${BUILD_DIR}"
      err "git clone failed. For private repos, configure git credentials first:
  - SSH key in ~/.ssh, or
  - git config --global credential.helper store, or
  - GIT_ASKPASS / GIT_TOKEN env var"
    }
    (cd "${BUILD_DIR}" && go build -o "${BIN_NAME}" .) || {
      rm -rf "${BUILD_DIR}"
      err "go build failed. Check the build log above."
    }
    ${_SUDO} install -m 755 "${BUILD_DIR}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
    rm -rf "${BUILD_DIR}"
    info "binary installed to ${INSTALL_DIR}/${BIN_NAME}"
    ;;
  *)
    # Download pre-built binary from GitHub Releases
    ${_SUDO} mkdir -p "${INSTALL_DIR}"
    download_binary "${PLATFORM}" "${VERSION}" "${INSTALL_DIR}"
    ;;
esac

# Create witness system user (system-wide install only)
if [ "${USER_INSTALL}" != "true" ]; then
  header "Setting up witness system user..."
  create_witness_user
fi

# PATH setup (user-local only)
if [ "${USER_INSTALL}" = "true" ]; then
  ensure_path "${INSTALL_DIR}"
fi

header "Verifying installation..."
if command -v "${BIN_NAME}" >/dev/null 2>&1; then
  "${BIN_NAME}" version
  echo ""
  info "${BIN_NAME} ${VERSION} installed successfully!"
else
  warn "${BIN_NAME} installed at ${INSTALL_DIR}/${BIN_NAME} but not in PATH."
  info "run: export PATH=\"\${PATH}:${INSTALL_DIR}\""
fi

header "Next steps"
echo "  1. Create an observer:          ${BIN_NAME} init my-server --local"
echo "  2. Deploy to target host:       ${BIN_NAME} deploy manifest.yaml --host <server>"
if [ "${USER_INSTALL}" != "true" ]; then
  echo "  3. System user '${WITNESS_USER}' created — use it in your observer manifest:"
  echo "       metadata:"
  echo "         name: my-server"
  echo "         user: ${WITNESS_USER}"
fi
echo ""
echo "  📖 Full documentation: https://lyssar.github.io/witness-cli/"
echo ""

# vim: ts=2 sw=2 et
