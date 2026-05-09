#!/usr/bin/env bash
set -euo pipefail

# VibeCockpit installer
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/njannasch/vibecockpit/main/install.sh | bash
#   wget -qO- https://raw.githubusercontent.com/njannasch/vibecockpit/main/install.sh | bash
#
# Options (env vars):
#   VERSION=v0.1.0    Install a specific version (default: latest)
#   PREFIX=~/.local   Install prefix (default: ~/.local for user, /usr/local for root)

REPO="njannasch/vibecockpit"
BINARY="vibecockpit"

info()  { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
ok()    { printf "\033[1;32m==>\033[0m %s\n" "$*"; }
err()   { printf "\033[1;31m==>\033[0m %s\n" "$*" >&2; exit 1; }

detect_os() {
  case "$(uname -s)" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "darwin" ;;
    *)       err "Unsupported OS: $(uname -s). VibeCockpit supports Linux and macOS." ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)   echo "amd64" ;;
    aarch64|arm64)   echo "arm64" ;;
    *)               err "Unsupported architecture: $(uname -m)" ;;
  esac
}

get_latest_version() {
  if command -v curl &>/dev/null; then
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
  elif command -v wget &>/dev/null; then
    wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
  else
    err "Neither curl nor wget found. Please install one of them."
  fi
}

download() {
  local url="$1" dest="$2"
  # Download to a sibling temp file then atomically `mv` over the
  # destination. Two reasons:
  #   1. Atomic — if curl fails midway, the existing binary at $dest is
  #      untouched. (curl -o would have already truncated it.)
  #   2. Fresh inode. macOS' syspolicyd tracks Gatekeeper trust per
  #      inode; when curl/wget overwrites a file in place the inode is
  #      preserved and any prior "blocked" verdict sticks even after
  #      the bytes are replaced. Symptom: Killed: 9 on launch with
  #      `load code signature error 2` in the system log, despite
  #      `codesign --verify` reporting the new bytes as valid. mv from
  #      a sibling temp file replaces the old inode entirely.
  local tmp
  tmp="$(mktemp "${dest}.XXXXXX")" || err "Could not create temp file in $(dirname "$dest")"
  if command -v curl &>/dev/null; then
    curl -fsSL -o "$tmp" "$url" || { rm -f "$tmp"; err "Download failed: $url"; }
  elif command -v wget &>/dev/null; then
    wget -qO "$tmp" "$url" || { rm -f "$tmp"; err "Download failed: $url"; }
  fi
  mv -f "$tmp" "$dest"
}

main() {
  local os arch version prefix bin_dir dest

  os=$(detect_os)
  arch=$(detect_arch)

  if [ -n "${VERSION:-}" ]; then
    version="$VERSION"
  else
    info "Fetching latest version..."
    version=$(get_latest_version)
    [ -z "$version" ] && err "Could not determine latest version. Set VERSION=v0.1.0 to install manually."
  fi

  if [ -n "${PREFIX:-}" ]; then
    prefix="$PREFIX"
  elif [ "$(id -u)" -eq 0 ]; then
    prefix="/usr/local"
  else
    prefix="${HOME}/.local"
  fi

  bin_dir="${prefix}/bin"
  dest="${bin_dir}/${BINARY}"

  # Check for existing install
  if [ -x "$dest" ]; then
    local current
    current=$("$dest" --version 2>/dev/null | awk '{print $2}' || echo "unknown")
    info "Updating VibeCockpit ${current} → ${version}"
  else
    info "Installing VibeCockpit ${version} for ${os}/${arch}"
  fi

  local asset="${BINARY}-${os}-${arch}"
  local url="https://github.com/${REPO}/releases/download/${version}/${asset}"

  info "Downloading ${url}"
  mkdir -p "$bin_dir"
  download "$url" "$dest"
  chmod +x "$dest"

  # macOS: clear every xattr the OS attached during the curl download.
  # com.apple.quarantine alone isn't enough — com.apple.provenance has
  # been observed to SIGKILL the binary on first launch even after
  # quarantine is stripped. The Go toolchain already ad-hoc signs the
  # binary at build time, so we don't need to re-sign here.
  if [ "$os" = "darwin" ]; then
    xattr -c "$dest" 2>/dev/null || true
  fi

  ok "Installed ${version} to ${dest}"

  # Delegate platform-specific install steps (PATH check, Linux .desktop
  # entry, macOS .app launcher with the embedded icon) to the binary
  # itself so there's a single source of truth and curl-piped installs
  # pick up the same launcher Spotlight shows for `--install` users.
  echo ""
  "$dest" --install --yes
}

main "$@"
