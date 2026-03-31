#!/usr/bin/env bash
set -euo pipefail

REPO="eric7/ols-cli"
BIN_NAME="ols"
INSTALL_DIR="/usr/local/bin"

log() {
  printf '\033[1;34m[ols-installer]\033[0m %s\n' "$1"
}

err() {
  printf '\033[1;31m[ols-installer]\033[0m %s\n' "$1" >&2
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    err "required command not found: $1"
    exit 1
  }
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      err "unsupported architecture: $arch"
      exit 1
      ;;
  esac
}

verify_checksum() {
  local tarball="$1"
  local checksums_file="$2"
  local expected_line expected_sum got_sum file_name

  file_name="$(basename "$tarball")"
  expected_line="$(grep "  ${file_name}$" "$checksums_file" || true)"
  if [[ -z "$expected_line" ]]; then
    err "checksum entry for ${file_name} was not found"
    exit 1
  fi

  expected_sum="$(printf '%s' "$expected_line" | awk '{print $1}')"
  got_sum="$(sha256sum "$tarball" | awk '{print $1}')"

  if [[ "$expected_sum" != "$got_sum" ]]; then
    err "checksum mismatch for ${file_name}"
    exit 1
  fi
}

install_binary() {
  require_cmd curl
  require_cmd tar
  require_cmd grep
  require_cmd awk
  require_cmd sha256sum

  local arch version base_url tmp_dir tarball checksums
  arch="$(detect_arch)"
  version="${1:-latest}"

  if [[ "$version" == "latest" ]]; then
    base_url="https://github.com/${REPO}/releases/latest/download"
  else
    base_url="https://github.com/${REPO}/releases/download/${version}"
  fi

  tmp_dir="$(mktemp -d)"
  tarball="${tmp_dir}/${BIN_NAME}-linux-${arch}.tar.gz"
  checksums="${tmp_dir}/checksums.txt"

  log "downloading release payload"
  curl -fsSL "${base_url}/${BIN_NAME}-linux-${arch}.tar.gz" -o "$tarball"
  curl -fsSL "${base_url}/checksums.txt" -o "$checksums"

  log "verifying checksum"
  verify_checksum "$tarball" "$checksums"

  log "extracting package"
  tar -xzf "$tarball" -C "$tmp_dir"

  log "installing to ${INSTALL_DIR}/${BIN_NAME}"
  install -m 0755 "${tmp_dir}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"

  rm -rf "$tmp_dir"
  log "installation complete"
}

install_binary "${1:-latest}"
