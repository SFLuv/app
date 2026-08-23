#!/usr/bin/env bash
# Pays an account real money the way a volunteer actually earns it: creates a
# one-seat event, mints its QR redemption code, and redeems that code.
#
# Run it repeatedly to walk somebody up the W-9 ladder. Against the default
# thresholds (notice 400, warning 500, limit 600):
#
#   ./testing/scripts/reset-w9.sh   <address|did>        # start from zero
#   ./testing/scripts/w9-earn.sh    <address|did> 400    # -> notice modal
#   ./testing/scripts/w9-earn.sh    <address|did> 100    # -> warning modal at 500
#   ./testing/scripts/w9-earn.sh    <address|did> 100    # -> crossing at 600, held
#   ./testing/scripts/w9-earn.sh    <address|did> 100    # -> refused, 409
#
# The amounts land exactly on each line because decidePayout compares with >=.
# 400 is already the notice, not 401.
#
# Nothing here fabricates a row. The event, the code, the transfer, the ledger
# entry and the tier notice are all produced by the system deciding what to do
# with a redemption — which is the point, because a hand-written ledger row can
# express a state the system cannot actually reach.
#
# max_participants is 1 deliberately: an event reserves reward x seats against
# unallocated faucet balance, so 600 for five seats demands 3000 and is
# correctly refused. One seat is all a crossing needs.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
discover_stack   # picks up the admin key and API base from the running stack

TARGET="${1:-}"; AMOUNT="${2:-}"
if [[ ! "$TARGET" =~ ^0x[0-9a-fA-F]{40}$ && ! "$TARGET" =~ ^did:privy:[0-9a-z]+$ ]] \
   || [[ ! "$AMOUNT" =~ ^[0-9]+$ ]] || [[ "$AMOUNT" -le 0 ]]; then
  echo "usage: $0 <wallet-address|did> <amount-sfluv>" >&2
  exit 1
fi

eval "$(python3 - "$SFLUV_ROOT/tmp/backend.dev.env" <<'PY'
import re,sys,shlex
t=open(sys.argv[1]).read()
for k in ("DB_USER","DB_PASSWORD","DB_URL","APP_DB_NAME"):
    m=re.search(r'^%s=(.*)$'%k,t,re.M)
    if m: print("%s=%s"%(k,shlex.quote(m.group(1).strip())))
PY
)"
host="${DB_URL%%:*}"; port="${DB_URL##*:}"
YEAR="$(date -u +%Y)"
psql_app(){ PGPASSWORD="$DB_PASSWORD" psql -h "$host" -p "$port" -U "$DB_USER" -d "${APP_DB_NAME:-app}" -tAc "$1"; }

# One field at a time, and ambiguity is an error: a prank makes two accounts
# share a primary wallet address, and "the first row" pays the wrong person.
if [[ "$TARGET" == did:privy:* ]]; then
  UID_="$(psql_app "SELECT id FROM users WHERE id='$TARGET';")"
else
  matches="$(psql_app "SELECT id FROM users WHERE primary_wallet_address ILIKE '$TARGET' ORDER BY id;")"
  if [[ "$(printf '%s\n' "$matches" | grep -c . || true)" -gt 1 ]]; then
    echo "  $TARGET is the primary wallet of more than one account:" >&2
    printf '%s\n' "$matches" | sed 's/^/    /' >&2
    echo "  pass the did instead so this is unambiguous." >&2
    exit 1
  fi
  UID_="$matches"
fi
[[ -n "$UID_" ]] || { echo "no account matches $TARGET" >&2; exit 1; }
ADDR="$(psql_app "SELECT COALESCE(primary_wallet_address,'') FROM users WHERE id='$UID_';")"
[[ -n "$ADDR" ]] || { echo "account $UID_ has no primary wallet address to pay" >&2; exit 1; }

before="$(psql_app "SELECT ROUND(COALESCE(SUM(amount_base),0)/1000000.0, 2) FROM payout_ledger WHERE user_id='$UID_' AND tax_year=$YEAR AND counts_toward_threshold AND state IN ('escrowed','releasing','paid');")"

TZ_NAME="America/Los_Angeles"
s_at="$(TZ=$TZ_NAME date -v+1d '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day' '+%Y-%m-%dT%H:%M:%S')"
e_at="$(TZ=$TZ_NAME date -v+1d -v+3H '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day +3 hours' '+%Y-%m-%dT%H:%M:%S')"

ev="$(admin_api POST /admin/volunteer-events "$(jq -nc \
  --arg t "w9-earn $AMOUNT" --arg s "$s_at" --arg e "$e_at" --arg tz "$TZ_NAME" --argjson r "$AMOUNT" \
  '{title:$t, description:"W-9 ladder step", start_at_local:$s, end_at_local:$e,
    timezone:$tz, max_participants:1, reward_amount_sfluv:$r, signup_mode:"internal"}')" \
  | jq -r '.id // empty')"
[[ -n "$ev" ]] || { echo "could not create the event — is the faucet funded? (./testing/scripts/fund-faucet.sh)" >&2; exit 1; }

code="$(admin_api GET "/admin/volunteer-events/$ev/codes" \
  | jq -r '[.. | objects | select(has("number")) | .id] | first // empty')"
[[ -n "$code" ]] || { echo "no code was minted for event $ev" >&2; exit 1; }
echo "  event $ev -> code $code (${AMOUNT} SFLUV)"

raw="$(curl -s -X POST "$API/redeem" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg c "$code" --arg a "$ADDR" '{code:$c, address:$a}')" \
  -w '\n%{http_code}' --max-time 120)"
status="${raw##*$'\n'}"

# Three of these are successful outcomes, and only the last is a problem.
case "$status" in
  200|201) echo "  redeem -> HTTP $status — paid" ;;
  202)     echo "  redeem -> HTTP 202 — held, which is what the crossing looks like" ;;
  # Not a failure of the script: past the crossing every further payout is
  # refused, and the code stays redeemable so it can be scanned again later.
  409)     echo "  redeem -> HTTP 409 — refused, which is what being blocked looks like" ;;
  *)       echo "  redeem -> HTTP $status"; echo "${raw%$'\n'*}" | head -3 ;;
esac

after="$(psql_app "SELECT ROUND(COALESCE(SUM(amount_base),0)/1000000.0, 2) FROM payout_ledger WHERE user_id='$UID_' AND tax_year=$YEAR AND counts_toward_threshold AND state IN ('escrowed','releasing','paid');")"
tier="$(psql_app "SELECT COALESCE((SELECT tier FROM w9_tier_notices WHERE user_id='$UID_' AND tax_year=$YEAR ORDER BY notified_at DESC LIMIT 1),'(none)');")"
held="$(psql_app "SELECT ROUND(COALESCE(SUM(amount_base),0)/1000000.0, 2) FROM payout_ledger WHERE user_id='$UID_' AND tax_year=$YEAR AND state IN ('escrowed','releasing');")"

echo "  annual total: ${before} -> ${after} SFLUV   held: ${held}   tier: ${tier}"
