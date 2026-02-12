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

NOW="$(date +%s)"
TEST_EMAIL="user+${NOW}@example.com"

echo "Submitting W9 for ${TEST_WALLET} (${TEST_EMAIL})..."
curl -s -X POST http://localhost:8080/w9/submit \
  -H "Content-Type: application/json" \
  -d "{\"wallet_address\":\"${TEST_WALLET}\",\"email\":\"${TEST_EMAIL}\"}"
echo ""

echo "Done."
