#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_ENV_DEFAULT="$ROOT_DIR/backend/.env"
BACKEND_ENV="${ENV_FILE:-$BACKEND_ENV_DEFAULT}"

ADMIN_KEY="$(grep -m1 '^ADMIN_KEY=' "$BACKEND_ENV" | cut -d= -f2-)"
PAID_ADDR="$(grep -m1 '^PAID_ADMIN_ADDRESSES=' "$BACKEND_ENV" | cut -d= -f2- | cut -d, -f1 | tr -d '[:space:]')"

if [[ -z "$ADMIN_KEY" || -z "$PAID_ADDR" ]]; then
  echo "Missing ADMIN_KEY or PAID_ADMIN_ADDRESSES in $BACKEND_ENV"
  exit 1
fi

TEST_WALLET="0x$(openssl rand -hex 20)"
NOW="$(date +%s)"
USER_ID="$(python3 - <<'PY'
import uuid
print(str(uuid.uuid4()))
PY
)"
TEST_EMAIL="user+${NOW}@example.com"
TRANSFER_ID="test-w9-ui-${NOW}"

echo "Creating event + code..."
EVENT_ID="$(curl -fsS -X POST http://localhost:8080/events \
  -H "X-Admin-Key: ${ADMIN_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"title":"W9 UI Test","description":"test","codes":1,"amount":1,"expiration":0}' | tr -d '\r\n' | tr -d '"')"

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

echo "Test wallet: ${TEST_WALLET}"
echo "Seeding user email: ${TEST_EMAIL}"
psql -d app -c "INSERT INTO users (id, contact_email) VALUES ('${USER_ID}', '${TEST_EMAIL}') ON CONFLICT (id) DO UPDATE SET contact_email = EXCLUDED.contact_email;"
psql -d app -c "INSERT INTO wallets (owner, name, is_eoa, eoa_address) VALUES ('${USER_ID}', 'Test Wallet', TRUE, LOWER('${TEST_WALLET}')) ON CONFLICT DO NOTHING;"
echo "Seeding paid transfer into ponder..."
psql -d ponder -c "CREATE TABLE IF NOT EXISTS live_query_tables (table_name text primary key);"
psql -d ponder -c "INSERT INTO transfer_event (id, hash, amount, timestamp, \"from\", \"to\") VALUES ('${TRANSFER_ID}','0xdeadbeef${NOW}',600000000000000000000, ${NOW}, LOWER('${PAID_ADDR}'), LOWER('${TEST_WALLET}')) ON CONFLICT DO NOTHING;"

curl -s -X POST http://localhost:8080/w9/transaction \
  -H "X-Admin-Key: ${ADMIN_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"from_address\":\"${PAID_ADDR}\",\"to_address\":\"${TEST_WALLET}\",\"amount\":\"600000000000000000000\",\"hash\":\"0xdeadbeef${NOW}\",\"timestamp\":${NOW}}" >/dev/null

FAUCET_URL="http://localhost:3000/faucet/redeem?sigAuthAccount=${TEST_WALLET}&sigAuthSignature=0xdummy&sigAuthRedirect=http://localhost:3000&sigAuthExpiry=9999999999&code=${CODE}"

echo ""
echo "Open this URL in your browser to validate the UI:"
echo "${FAUCET_URL}"
