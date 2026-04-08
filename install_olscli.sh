#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-Code-Egg/ols-cli}"
BIN_NAME="ols"
INSTALL_DIR="/usr/local/bin"
TMP_FILE=""

log() {
  printf '\033[1;34m[ols-installer]\033[0m %s\n' "$1"
}

warn() {
  printf '\033[1;33m[ols-installer]\033[0m %s\n' "$1"
}

err() {
  printf '\033[1;31m[ols-installer]\033[0m %s\n' "$1" >&2
}

cleanup() {
  if [ -n "${TMP_FILE:-}" ]; then
    rm -f "$TMP_FILE"
  fi
}

run_as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi

  if command -v sudo >/dev/null 2>&1; then
    sudo "$@"
    return
  fi

  err "this step requires root privileges; re-run as root or install sudo"
  exit 1
}

get_os_id_like() {
  if [ ! -r /etc/os-release ]; then
    echo ""
    return
  fi

  # shellcheck disable=SC1091
  . /etc/os-release
  echo "${ID:-} ${ID_LIKE:-}"
}

is_debian_or_ubuntu() {
  case " $(get_os_id_like) " in
    *" debian "*|*" ubuntu "*)
      return 0
      ;;
  esac
  return 1
}

is_rhel_family() {
  case " $(get_os_id_like) " in
    *" rhel "*|*" centos "*|*" rocky "*|*" alma "*|*" fedora "*)
      return 0
      ;;
  esac
  return 1
}

install_packages() {
  if command -v apt-get >/dev/null 2>&1 && is_debian_or_ubuntu; then
    run_as_root apt-get update
    run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
    return
  fi

  if command -v dnf >/dev/null 2>&1 && is_rhel_family; then
    run_as_root dnf install -y "$@"
    return
  fi

  if command -v yum >/dev/null 2>&1 && is_rhel_family; then
    run_as_root yum install -y "$@"
    return
  fi

  err "unable to auto-install packages on this distro/package manager"
  exit 1
}

ensure_go_installed() {
  if command -v go >/dev/null 2>&1; then
    log "Go detected: $(go version)"
    return
  fi

  if is_debian_or_ubuntu; then
    log "Go not found. Installing golang-go via apt..."
    install_packages golang-go
    log "Go installed: $(go version)"
    return
  fi

  if is_rhel_family; then
    log "Go not found. Installing golang via dnf/yum..."
    install_packages golang
    log "Go installed: $(go version)"
    return
  fi

  err "Go is not installed. Automatic installation is supported for Debian/Ubuntu/CentOS-family systems."
  err "Please install Go manually, then re-run this installer."
  exit 1
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)
      echo "amd64"
      ;;
    aarch64|arm64)
      echo "arm64"
      ;;
    *)
      err "unsupported architecture: $(uname -m). Supported: amd64, arm64"
      exit 1
      ;;
  esac
}

ensure_downloader() {
  if command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1; then
    return
  fi

  if is_debian_or_ubuntu; then
    log "Neither curl nor wget found. Installing curl via apt..."
    install_packages curl
    return
  fi

  if is_rhel_family; then
    log "Neither curl nor wget found. Installing curl via dnf/yum..."
    install_packages curl
    return
  fi

  err "either curl or wget is required"
  exit 1
}

main() {
  if [ "$(uname -s)" != "Linux" ]; then
    err "this installer supports Linux only"
    exit 1
  fi

  ensure_go_installed
  ensure_downloader

  local arch url
  arch="$(detect_arch)"
  url="https://github.com/${REPO}/releases/latest/download/${BIN_NAME}-linux-${arch}"
  TMP_FILE="$(mktemp)"

  log "Downloading ${BIN_NAME} (${arch}) from latest release..."
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 3 --retry-delay 1 "$url" -o "$TMP_FILE"
  else
    wget -qO "$TMP_FILE" "$url"
  fi

  if [ ! -s "$TMP_FILE" ]; then
    err "download failed or produced an empty file"
    exit 1
  fi

  run_as_root install -d "$INSTALL_DIR"
  run_as_root install -m 0755 "$TMP_FILE" "${INSTALL_DIR}/${BIN_NAME}"
  run_as_root chmod u+w "${INSTALL_DIR}/${BIN_NAME}"

  log "Installed ${BIN_NAME} to ${INSTALL_DIR}/${BIN_NAME}"
  log "Run '${BIN_NAME} --help' to verify"
}

trap cleanup EXIT

main "$@"
