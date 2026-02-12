#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_ENV="$ROOT_DIR/backend/.env"

ADMIN_KEY="$(grep -m1 '^ADMIN_KEY=' "$BACKEND_ENV" | cut -d= -f2-)"
PAID_ADDR="$(grep -m1 '^PAID_ADMIN_ADDRESSES=' "$BACKEND_ENV" | cut -d= -f2- | cut -d, -f1 | tr -d '[:space:]')"

if [[ -z "$ADMIN_KEY" || -z "$PAID_ADDR" ]]; then
  echo "Missing ADMIN_KEY or PAID_ADMIN_ADDRESSES in $BACKEND_ENV"
  exit 1
fi

TEST_TO="0x1111111111111111111111111111111111111111"
NOW="$(date +%s)"

echo "Using paid address: $PAID_ADDR"
echo "Target wallet: $TEST_TO"

echo "Inserting test transfer into ponder DB..."
psql -d ponder -c "INSERT INTO transfer_event (id, hash, amount, timestamp, \"from\", \"to\") VALUES ('test-w9', '0xdeadbeef', 600000000000000000000, ${NOW}, LOWER('${PAID_ADDR}'), LOWER('${TEST_TO}')) ON CONFLICT DO NOTHING;"

echo "Calling /w9/transaction..."
curl -s -X POST http://localhost:8080/w9/transaction \
  -H "X-Admin-Key: ${ADMIN_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"from_address\":\"${PAID_ADDR}\",\"to_address\":\"${TEST_TO}\",\"amount\":\"600000000000000000000\",\"hash\":\"0xdeadbeef\",\"timestamp\":${NOW}}"
echo ""

echo "w9_wallet_earnings:"
psql -d app -c "SELECT wallet_address, year, amount_received, w9_required FROM w9_wallet_earnings WHERE wallet_address = LOWER('${TEST_TO}');"

echo "w9/check (should be blocked):"
curl -s -X POST http://localhost:8080/w9/check \
  -H "X-Admin-Key: ${ADMIN_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"from_address\":\"${PAID_ADDR}\",\"to_address\":\"${TEST_TO}\",\"amount\":\"1\"}"
echo ""

echo "Submitting W9..."
curl -s -X POST http://localhost:8080/w9/submit \
  -H "Content-Type: application/json" \
  -d "{\"wallet_address\":\"${TEST_TO}\",\"email\":\"user@example.com\"}"
echo ""

echo "Pending submissions:"
curl -s http://localhost:8080/admin/w9/pending -H "X-Admin-Key: ${ADMIN_KEY}"
echo ""

APPROVAL_ID="$(psql -d app -t -c "SELECT id FROM w9_submissions WHERE wallet_address = LOWER('${TEST_TO}') ORDER BY id DESC LIMIT 1;" | xargs)"
if [[ -n "$APPROVAL_ID" ]]; then
  echo "Approving submission id ${APPROVAL_ID}..."
  curl -s -X PUT http://localhost:8080/admin/w9/approve \
    -H "X-Admin-Key: ${ADMIN_KEY}" \
    -H "Content-Type: application/json" \
    -d "{\"id\":${APPROVAL_ID}}"
  echo ""
fi

echo "w9/check after approval (should be allowed):"
curl -s -X POST http://localhost:8080/w9/check \
  -H "X-Admin-Key: ${ADMIN_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"from_address\":\"${PAID_ADDR}\",\"to_address\":\"${TEST_TO}\",\"amount\":\"1\"}"
echo ""
