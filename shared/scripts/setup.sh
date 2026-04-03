#!/usr/bin/env bash
# setup.sh — First-run setup script for the Search Engine
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

log() { echo -e "\033[1;34m[setup]\033[0m $*"; }
err() { echo -e "\033[1;31m[error]\033[0m $*" >&2; }
ok()  { echo -e "\033[1;32m[ok]\033[0m $*"; }

# ── Check prerequisites ────────────────────────────────────────────────────
check_cmd() {
  if ! command -v "$1" &>/dev/null; then
    err "$1 is required but not installed."
    exit 1
  fi
}

log "Checking prerequisites..."
check_cmd docker
check_cmd docker-compose 2>/dev/null || check_cmd "docker compose"
ok "Prerequisites satisfied"

# ── Copy .env ─────────────────────────────────────────────────────────────
if [ ! -f "$ROOT_DIR/.env" ]; then
  log "Creating .env from template..."
  cp "$ROOT_DIR/configs/.env.example" "$ROOT_DIR/.env"
  ok ".env created — edit it to add your API keys before starting"
else
  ok ".env already exists"
fi

# ── Install Go dependencies ────────────────────────────────────────────────
log "Downloading Go module dependencies..."
(cd "$ROOT_DIR/services/go-api" && go mod tidy)
(cd "$ROOT_DIR/services/go-indexer" && go mod tidy)
ok "Go dependencies ready"

# ── Pull base images ───────────────────────────────────────────────────────
log "Pulling Docker base images..."
docker pull postgres:16-alpine
docker pull redis:7-alpine
docker pull opensearchproject/opensearch:2.14.0
ok "Base images pulled"

log "Setup complete!"
echo ""
echo "  Next steps:"
echo "    1. Edit .env and add your API keys"
echo "    2. cd docker && docker compose up --build"
echo "    3. Query: curl 'http://localhost:8080/api/v1/search?q=machine+learning'"
