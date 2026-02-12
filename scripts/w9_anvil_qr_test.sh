#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_ENV="$ROOT_DIR/backend/.env"
ANVIL_ENV="$ROOT_DIR/scripts/anvil.env"

if [[ ! -f "$BACKEND_ENV" ]]; then
  echo "Missing $BACKEND_ENV"
  exit 1
fi
if [[ ! -f "$ANVIL_ENV" ]]; then
  echo "Missing $ANVIL_ENV"
  exit 1
fi

set -a
source "$BACKEND_ENV"
source "$ANVIL_ENV"
set +a

if ! command -v cast >/dev/null 2>&1; then
  echo "cast not found. Please install Foundry (cast/anvil)."
  exit 1
fi

if [[ -z "${TOKEN_ID:-}" || -z "${ADMIN_KEY:-}" ]]; then
  echo "Missing TOKEN_ID or ADMIN_KEY in $BACKEND_ENV"
  exit 1
fi

ANVIL_RPC="http://127.0.0.1:8545"
PAID_ADDR="${ANVIL_UNLOCK}"
TEST_WALLET="0x$(openssl rand -hex 20)"
AMOUNT_WEI="200000000000000000000"

echo "Test wallet: ${TEST_WALLET}"
echo "Sending 200 SFLUV from faucet ${PAID_ADDR} via anvil..."

cast rpc --rpc-url "$ANVIL_RPC" anvil_impersonateAccount "$PAID_ADDR" >/dev/null || true

cast send "$TOKEN_ID" "transfer(address,uint256)" "$TEST_WALLET" "$AMOUNT_WEI" \
  --rpc-url "$ANVIL_RPC" \
  --from "$PAID_ADDR" \
  --unlocked >/dev/null

echo "Waiting for ponder to index transfer_event..."
FOUND=""
for i in {1..30}; do
  FOUND="$(psql -d ponder -t -c "SELECT 1 FROM transfer_event WHERE \"from\" = LOWER('${PAID_ADDR}') AND \"to\" = LOWER('${TEST_WALLET}') AND amount = ${AMOUNT_WEI} LIMIT 1;" | xargs || true)"
  if [[ -n "$FOUND" ]]; then
    break
  fi
  sleep 1
done

if [[ -z "$FOUND" ]]; then
  echo "Timed out waiting for ponder to index the transfer."
  exit 1
fi

echo "Waiting for w9_wallet_earnings to update..."
EARNINGS=""
for i in {1..30}; do
  EARNINGS="$(psql -d app -t -c "SELECT amount_received FROM w9_wallet_earnings WHERE wallet_address = LOWER('${TEST_WALLET}') ORDER BY year DESC LIMIT 1;" | xargs || true)"
  if [[ -n "$EARNINGS" ]]; then
    break
  fi
  sleep 1
done

if [[ -z "$EARNINGS" ]]; then
  echo "Timed out waiting for w9_wallet_earnings."
  exit 1
fi

echo "Creating event + code..."
EVENT_ID="$(curl -fsS -X POST http://localhost:8080/events \
  -H "X-Admin-Key: ${ADMIN_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"title":"W9 Anvil Test","description":"test","codes":1,"amount":1,"expiration":0}' | tr -d '\r\n' | tr -d '"')"

if [[ -z "$EVENT_ID" ]]; then
  echo "Failed to create event (empty id). Check backend logs and ADMIN_KEY."
  exit 1
fi

CODE_JSON="$(curl -fsS "http://localhost:8080/events/${EVENT_ID}?count=1&page=0" \
  -H "X-Admin-Key: ${ADMIN_KEY}")"
CODE="$(CODE_JSON="$CODE_JSON" python3 - <<'PY'
import json,os
try:
    data=json.loads(os.environ["CODE_JSON"])
    print(data[0]["id"])
except Exception as e:
    print("ERR:"+str(e))
PY
)"

if [[ "$CODE" == ERR:* || -z "$CODE" ]]; then
  echo "Failed to parse event codes JSON: $CODE"
  exit 1
fi

FAUCET_URL="http://localhost:3000/faucet/redeem?sigAuthAccount=${TEST_WALLET}&sigAuthSignature=0xdummy&sigAuthRedirect=http://localhost:3000&sigAuthExpiry=9999999999&code=${CODE}"

echo ""
echo "Open this URL to verify W9 Required + submit button:"
echo "${FAUCET_URL}"
