#!/usr/bin/env bash
set -euo pipefail

# ──────────────────────────────────────────────
#  Skuld CLI — install.sh
#  Download and install the latest skuld-cli binary
#  USAGE: curl -sfL https://raw.githubusercontent.com/lyssar/skuld-cli/main/install.sh | sh
# ──────────────────────────────────────────────

REPO="lyssar/skuld-cli"
BIN_NAME="skuld-cli"
DEFAULT_INSTALL_DIR="${HOME}/.local/bin"

# --- Colors ---
BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

info()  { printf "${GREEN}✓${NC} %s\n" "$*"; }
warn()  { printf "${YELLOW}⚠${NC} %s\n" "$*"; }
err()   { printf "${RED}✗${NC} %s\n" "$*"; exit 1; }
header(){ printf "\n${BOLD}%s${NC}\n" "$*"; }

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

  if ! command -v git &>/dev/null; then
    warn "git is not installed. Install it first: https://git-scm.com/"
    missing=1
  fi

  if ! command -v age-keygen &>/dev/null; then
    warn "age is not installed. Install it first: https://github.com/FiloSottile/age#installation"
    missing=1
  fi

  if ! command -v docker &>/dev/null; then
    warn "docker is not installed. Needed for the docker-compose provisioner."
    warn "Install it first: https://docs.docker.com/engine/install/"
  fi

  if [ "$missing" -eq 1 ]; then
    err "missing required dependencies. Install them and re-run this script."
  fi
}

# --- Get Latest Release ---
get_latest_version() {
  if command -v curl &>/dev/null; then
    curl -sfL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name":' \
      | sed -E 's/.*"([^"]+)".*/\1/' \
      || echo ""
  elif command -v wget &>/dev/null; then
    wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" \
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

  if command -v curl &>/dev/null; then
    curl -sfL "${download_url}" -o "${archive_path}"
  elif command -v wget &>/dev/null; then
    wget -qO "${archive_path}" "${download_url}"
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
  mv "${binary_src}" "${dest_dir}/${BIN_NAME}"
  rm -rf "${tmpdir}"

  info "binary installed to ${dest_dir}/${BIN_NAME}"
}

# --- Ensure Install Directory is in PATH ---
ensure_path() {
  local install_dir="$1"

  # Already in PATH?
  if command -v "${BIN_NAME}" &>/dev/null; then
    return 0
  fi

  # Detect shell rc
  local rc_file=""
  case "${SHELL}" in
    */zsh) rc_file="${HOME}/.zshrc" ;;
    */bash) rc_file="${HOME}/.bashrc" ;;
  esac

  # Add to PATH if rc file exists
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

header "Skuld CLI — Installer"
echo ""

# Parse arguments
INSTALL_DIR="${DEFAULT_INSTALL_DIR}"
while [ $# -gt 0 ]; do
  case "$1" in
    --dir) INSTALL_DIR="$2"; shift 2 ;;
    --help|-h)
      echo "Usage: install.sh [--dir <path>]"
      echo ""
      echo "  --dir <path>   Install directory (default: ~/.local/bin)"
      exit 0
      ;;
    *) err "unknown argument: $1 (use --help for usage)" ;;
  esac
done

PLATFORM="$(detect_platform)"
info "detected platform: ${PLATFORM}"

header "Checking prerequisites..."
check_prereqs

header "Fetching latest release..."
VERSION="$(get_latest_version)"
if [ -z "${VERSION}" ]; then
  err "could not determine latest release. Check your internet connection."
fi
info "latest version: ${VERSION}"

header "Installing ${BIN_NAME} ${VERSION} ..."
mkdir -p "${INSTALL_DIR}"
download_binary "${PLATFORM}" "${VERSION}" "${INSTALL_DIR}"

ensure_path "${INSTALL_DIR}"

header "Verifying installation..."
if command -v "${BIN_NAME}" &>/dev/null; then
  "${BIN_NAME}" version
  echo ""
  info "${BIN_NAME} ${VERSION} installed successfully!"
else
  warn "${BIN_NAME} installed at ${INSTALL_DIR}/${BIN_NAME} but not in PATH."
  info "run: export PATH=\"\${PATH}:${INSTALL_DIR}\""
fi

header "Next steps"
echo "  1. Create an observer:     ${BIN_NAME} init my-server --local"
echo "  2. Create an application:  ${BIN_NAME} new-app"
echo "  3. Run reconciliation:     ${BIN_NAME} reconcile ~/.config/skuld-cli/my-server/"
echo ""
echo "  📖 Full documentation: https://lyssar.github.io/skuld-cli/"
echo ""
