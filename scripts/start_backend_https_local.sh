#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CERT="/tmp/sfluv-localhost.crt"
KEY="/tmp/sfluv-localhost.key"

if [[ ! -f "$CERT" || ! -f "$KEY" ]]; then
  echo "Generating self-signed localhost cert..."
  openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "$KEY" \
    -out "$CERT" \
    -days 365 \
    -subj "/CN=localhost" >/dev/null 2>&1
fi

PID=$(lsof -ti :8080 -sTCP:LISTEN || true)
if [[ -n "$PID" ]]; then
  kill "$PID" || true
  sleep 1
fi

echo "Starting backend with HTTPS enabled..."
echo "  HTTP:  http://localhost:8080"
echo "  HTTPS: https://localhost:8443"
echo ""
echo "Note: You must trust the self-signed cert in your browser for https://localhost:8443 to work."
echo "Open https://localhost:8443 once and proceed through the warning."

(
  cd "$ROOT_DIR/backend"
  TLS_CERT_FILE="$CERT" TLS_KEY_FILE="$KEY" TLS_PORT=8443 nohup go run main.go > /tmp/sfluv_backend.log 2>&1 &
)
