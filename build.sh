#!/usr/bin/env bash
set -euo pipefail

# VibeCockpit build script
# Builds the Svelte frontend and Go binary

BINARY="vibecockpit"

info() { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
ok()   { printf "\033[1;32m==>\033[0m %s\n" "$*"; }
err()  { printf "\033[1;31m==>\033[0m %s\n" "$*" >&2; exit 1; }

# Ensure Node 22+ is available (try nvm, then system node)
setup_node() {
  if command -v node &>/dev/null; then
    NODE_VER=$(node --version | sed 's/v//' | cut -d. -f1)
    if [ "$NODE_VER" -ge 22 ] 2>/dev/null; then
      return 0
    fi
  fi

  if [ -s "${NVM_DIR:-$HOME/.nvm}/nvm.sh" ]; then
    . "${NVM_DIR:-$HOME/.nvm}/nvm.sh"
    if nvm use 22 &>/dev/null; then
      return 0
    fi
  fi

  err "Node.js 22+ required. Install via nvm: nvm install 22"
}

build_frontend() {
  info "Building frontend..."
  setup_node
  cd frontend
  npm ci --silent
  npm run build
  cd ..
  ok "Frontend built"
}

build_go() {
  info "Building Go binary..."
  VERSION="${VERSION:-dev}"
  if command -v git &>/dev/null && git rev-parse HEAD &>/dev/null; then
    VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo "$VERSION")"
  fi
  go build -ldflags="-s -w -X main.version=$VERSION" -o "$BINARY" .
  ok "Binary: ./$BINARY ($(du -h "$BINARY" | cut -f1))"
}

case "${1:-all}" in
  frontend)
    build_frontend
    ;;
  go)
    build_go
    ;;
  all)
    build_frontend
    build_go
    echo ""
    ok "Done! Run './$BINARY --web' or './$BINARY' to start"
    ;;
  clean)
    rm -rf "$BINARY" dist/ frontend/node_modules internal/web/static/assets
    ok "Cleaned"
    ;;
  *)
    echo "Usage: ./build.sh [frontend|go|all|clean]"
    exit 1
    ;;
esac
