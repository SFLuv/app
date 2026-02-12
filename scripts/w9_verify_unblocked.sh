#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_ENV="$ROOT_DIR/backend/.env"

if [[ ! -f "$BACKEND_ENV" ]]; then
  echo "Missing $BACKEND_ENV"
  exit 1
fi

set -a
source "$BACKEND_ENV"
set +a

FAUCET_ADDR="$(echo "${PAID_ADMIN_ADDRESSES}" | cut -d, -f1 | tr -d '[:space:]')"
if [[ -z "$FAUCET_ADDR" ]]; then
  echo "Missing PAID_ADMIN_ADDRESSES in $BACKEND_ENV"
  exit 1
fi

TEST_WALLET="$(psql -d ponder -t -c "SELECT \"to\" FROM transfer_event WHERE \"from\" = LOWER('${FAUCET_ADDR}') ORDER BY timestamp DESC LIMIT 1;" | xargs || true)"
if [[ -z "$TEST_WALLET" ]]; then
  echo "No transfer_event found for faucet ${FAUCET_ADDR}"
  exit 1
fi

echo "Checking W9 compliance for ${TEST_WALLET}..."
RESP="$(curl -s -X POST http://localhost:8080/w9/check \
  -H "X-Admin-Key: ${ADMIN_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"from_address\":\"${FAUCET_ADDR}\",\"to_address\":\"${TEST_WALLET}\",\"amount\":\"1\"}")"

echo "$RESP"

if echo "$RESP" | grep -q "\"allowed\":true"; then
  echo "Unblocked: OK"
  exit 0
fi

echo "Still blocked or unexpected response."
exit 1
