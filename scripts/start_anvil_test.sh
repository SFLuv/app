#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ANVIL_ENV="$ROOT_DIR/scripts/anvil.env"
BACKEND_ENV_TEST="$ROOT_DIR/backend/.env.test"
PONDER_ENV_TEST="$ROOT_DIR/ponder/.env.test"

if [[ ! -f "$ANVIL_ENV" ]]; then
  echo "Missing $ANVIL_ENV"
  exit 1
fi
if [[ ! -f "$BACKEND_ENV_TEST" ]]; then
  echo "Missing $BACKEND_ENV_TEST"
  exit 1
fi
if [[ ! -f "$PONDER_ENV_TEST" ]]; then
  echo "Missing $PONDER_ENV_TEST"
  exit 1
fi

echo "Stopping existing services on ports 8545, 8080, 42069, 3000..."
for port in 8545 8080 42069 3000; do
  PID="$(lsof -ti :${port} -sTCP:LISTEN || true)"
  if [[ -n "$PID" ]]; then
    kill "$PID" || true
  fi
done
sleep 1

set -a
source "$ANVIL_ENV"
set +a

echo "Starting anvil fork..."
nohup anvil \
  --fork-url "$ANVIL_FORK_URL" \
  --fork-block-number "$ANVIL_FORK_BLOCK" \
  --chain-id "$ANVIL_CHAIN_ID" \
  > /tmp/anvil.log 2>&1 &

sleep 1

if command -v cast >/dev/null 2>&1; then
  echo "Impersonating faucet address on anvil..."
  cast rpc --rpc-url http://127.0.0.1:8545 anvil_impersonateAccount "$ANVIL_UNLOCK" >/dev/null || true
fi

echo "Starting backend with test env..."
(
  cd "$ROOT_DIR/backend"
  ENV_FILE="$BACKEND_ENV_TEST" nohup go run main.go > /tmp/sfluv_backend.log 2>&1 &
)

echo "Starting ponder with test env..."
(
  set -a
  source "$PONDER_ENV_TEST"
  set +a
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
echo "  /tmp/anvil.log"
echo "  /tmp/sfluv_backend.log"
echo "  /tmp/sfluv_ponder.log"
echo "  /tmp/sfluv_frontend.log"
