#!/usr/bin/env bash
set -euo pipefail

# VibeCockpit build script
#
# Default action ("all") builds inside Docker so the only host requirement
# is Docker itself; the resulting host-platform binary is copied to
# ./vibecockpit. Use "local" if you want to build directly with the Go
# and Node toolchains on your machine instead.

BINARY="vibecockpit"
IMAGE="${BINARY}-builder"

info() { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
ok()   { printf "\033[1;32m==>\033[0m %s\n" "$*"; }
err()  { printf "\033[1;31m==>\033[0m %s\n" "$*" >&2; exit 1; }

detect_host() {
  local os arch
  case "$(uname -s)" in
    Linux*)  os="linux" ;;
    Darwin*) os="darwin" ;;
    *) err "Unsupported host OS: $(uname -s)" ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) err "Unsupported host arch: $(uname -m)" ;;
  esac
  printf "%s-%s" "$os" "$arch"
}

require_docker() {
  command -v docker >/dev/null 2>&1 || err "docker not found. Install Docker or run './build.sh local'."
  docker info >/dev/null 2>&1 || err "docker daemon is not running."
}

# Build the multi-platform builder image and extract the requested artifact(s).
# Args: 1=output_dir 2..N=asset names to copy out of the image.
docker_extract() {
  local out_dir="$1"; shift
  require_docker
  info "Building Docker image (${IMAGE})…"
  docker build -t "$IMAGE" .
  mkdir -p "$out_dir"
  local cid
  cid=$(docker create "$IMAGE")
  trap "docker rm -f \"$cid\" >/dev/null 2>&1 || true" EXIT
  for asset in "$@"; do
    info "Extracting $asset → $out_dir/"
    docker cp "$cid:/$asset" "$out_dir/"
  done
  docker rm "$cid" >/dev/null
  trap - EXIT
}

build_host_via_docker() {
  local host
  host=$(detect_host)
  local asset="${BINARY}-${host}"
  docker_extract "." "$asset"
  mv "./$asset" "./$BINARY"
  chmod +x "./$BINARY"
  ok "Binary: ./$BINARY (host=$host, $(du -h "./$BINARY" | cut -f1))"
}

build_dist_via_docker() {
  docker_extract "dist" \
    "${BINARY}-linux-amd64" \
    "${BINARY}-linux-arm64" \
    "${BINARY}-darwin-amd64" \
    "${BINARY}-darwin-arm64"
  chmod +x dist/${BINARY}-*
  ok "Binaries: dist/"
  ls -1 dist/${BINARY}-*
}

# --- local (non-docker) path ---------------------------------------------

setup_node() {
  if command -v node &>/dev/null; then
    NODE_VER=$(node --version | sed 's/v//' | cut -d. -f1)
    [ "$NODE_VER" -ge 22 ] 2>/dev/null && return 0
  fi
  if [ -s "${NVM_DIR:-$HOME/.nvm}/nvm.sh" ]; then
    . "${NVM_DIR:-$HOME/.nvm}/nvm.sh"
    nvm use 22 &>/dev/null && return 0
  fi
  err "Node.js 22+ required. Install via nvm: nvm install 22"
}

build_frontend_local() {
  info "Building frontend…"
  setup_node
  # vite's emptyDir doesn't reliably wipe nested assets/ from prior runs,
  # so old hashed bundles linger and bloat the embed. Clean it explicitly.
  rm -rf internal/web/static/assets
  (cd frontend && npm ci --silent && npm run build)
  ok "Frontend built"
}

build_go_local() {
  command -v go >/dev/null 2>&1 || err "go not found. Install Go or run './build.sh' (uses Docker)."
  info "Building Go binary…"
  local version="${VERSION:-dev}"
  if command -v git &>/dev/null && git rev-parse HEAD &>/dev/null; then
    version="$(git describe --tags --always --dirty 2>/dev/null || echo "$version")"
  fi
  go build -ldflags="-s -w -X main.version=$version" -o "$BINARY" .
  ok "Binary: ./$BINARY ($(du -h "$BINARY" | cut -f1))"
}

build_icon() {
  [ "$(uname -s)" = "Darwin" ] || err "build_icon requires macOS (qlmanage / sips / iconutil)."
  command -v qlmanage >/dev/null 2>&1 || err "qlmanage not found"
  command -v sips     >/dev/null 2>&1 || err "sips not found"
  command -v iconutil >/dev/null 2>&1 || err "iconutil not found"
  [ -f assets/AppIcon.svg ] || err "assets/AppIcon.svg missing"

  # We rasterize at 512px and skip the 512@2x (=1024) iconset tier — the
  # 1024 PNG alone is ~1MB and Launchpad / Spotlight / Dock never render
  # bigger than 256px in practice. Saves ~1MB off the ICNS.
  info "Rendering AppIcon.svg → 512px PNG via qlmanage…"
  local tmp
  tmp=$(mktemp -d)
  trap "rm -rf $tmp" RETURN
  qlmanage -t -s 512 -o "$tmp" assets/AppIcon.svg >/dev/null 2>&1
  local src="$tmp/AppIcon.svg.png"
  [ -f "$src" ] || err "qlmanage failed to produce a PNG"

  info "Building iconset…"
  local iconset="$tmp/AppIcon.iconset"
  mkdir -p "$iconset"
  for s in 16 32 64 128 256 512; do
    sips -z "$s" "$s" "$src" --out "$tmp/_$s.png" >/dev/null 2>&1
  done
  cp "$tmp/_16.png"   "$iconset/icon_16x16.png"
  cp "$tmp/_32.png"   "$iconset/icon_16x16@2x.png"
  cp "$tmp/_32.png"   "$iconset/icon_32x32.png"
  cp "$tmp/_64.png"   "$iconset/icon_32x32@2x.png"
  cp "$tmp/_128.png"  "$iconset/icon_128x128.png"
  cp "$tmp/_256.png"  "$iconset/icon_128x128@2x.png"
  cp "$tmp/_256.png"  "$iconset/icon_256x256.png"
  cp "$tmp/_512.png"  "$iconset/icon_256x256@2x.png"
  cp "$tmp/_512.png"  "$iconset/icon_512x512.png"

  info "Packing AppIcon.icns…"
  iconutil -c icns "$iconset" -o assets/AppIcon.icns
  cp assets/AppIcon.icns internal/install/assets/AppIcon.icns
  ok "Icon: assets/AppIcon.icns ($(du -h assets/AppIcon.icns | cut -f1))"
}

# --- entry point ---------------------------------------------------------

case "${1:-all}" in
  all|docker)
    build_host_via_docker
    ;;
  dist)
    build_dist_via_docker
    ;;
  local)
    build_frontend_local
    build_go_local
    ;;
  frontend)
    build_frontend_local
    ;;
  go)
    build_go_local
    ;;
  icon)
    build_icon
    ;;
  clean)
    rm -rf "$BINARY" dist/ frontend/node_modules internal/web/static/assets
    ok "Cleaned"
    ;;
  *)
    cat <<USAGE
Usage: ./build.sh [all|dist|local|frontend|go|clean]

  all       (default) build host-platform binary inside Docker → ./$BINARY
  dist      build all 4 platform binaries inside Docker → dist/
  local     build using the host's Go and Node toolchains
  frontend  build just the frontend locally (Node 22+ required)
  go        build just the Go binary locally (Go required)
  icon      regenerate AppIcon.icns from assets/AppIcon.svg (macOS only)
  clean     remove build artifacts

Examples:
  ./build.sh              # docker build for your platform
  ./build.sh dist         # docker build all 4 platforms for release
  ./build.sh local        # if you have Go + Node and want to skip Docker
USAGE
    exit 1
    ;;
esac
