#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Stopping existing services on ports 8080, 8443, 42069, 3000..."
for port in 8080 8443 42069 3000; do
  PID="$(lsof -ti :${port} -sTCP:LISTEN || true)"
  if [[ -n "$PID" ]]; then
    kill "$PID" || true
  fi
done
sleep 1

echo "Starting backend (HTTP + HTTPS)..."
"$ROOT_DIR/scripts/start_backend_https_local.sh"

echo "Starting ponder..."
(
  cd "$ROOT_DIR/ponder"
  nohup pnpm dev > /tmp/sfluv_ponder.log 2>&1 &
)

echo "Starting frontend..."
(
  cd "$ROOT_DIR/frontend"
  nohup pnpm dev > /tmp/sfluv_frontend.log 2>&1 &
)

echo "Services started."
echo "Logs:"
echo "  /tmp/sfluv_backend.log"
echo "  /tmp/sfluv_ponder.log"
echo "  /tmp/sfluv_frontend.log"
