#!/usr/bin/env bash
# Puts an account at a W-9 warning tier, so the tier modal can be exercised.
#
# TWO MODES, and the difference matters.
#
#   --real (default)  Drives an actual redemption: creates an event whose reward
#                     alone reaches the tier, mints a code, and redeems it. The
#                     money moves on the fork, decidePayout runs, and the ledger
#                     row and tier notice are written by the system.
#
#   --fast            Writes the ledger row and tier notice directly. Instant,
#                     and useful when you only want to look at the modal.
#
# Why --real is the default: a fabricated row can express a state the system
# cannot produce, and then the test is checking a fiction. An earlier version of
# this script seeded 'blocked' at 701 SFLUV, which cannot happen — the payment
# that crosses 600 is escrowed and every payment after it is refused, so a
# blocked account sits at 600 and stops. A real redemption cannot get that
# wrong, because the system decides the numbers.
#
# Note what --real does NOT undo: --revert deletes the ledger rows and notices,
# but the on-chain transfer already happened. Balances accumulate across runs.
# That is fine on a fork and worth knowing before you read a balance.
#
#   ./scripts/seed-w9-tier/seed-w9-tier.sh <address|did> notice_400
#   ./scripts/seed-w9-tier/seed-w9-tier.sh --fast <address|did> blocked
#   ./scripts/seed-w9-tier/seed-w9-tier.sh --revert <address|did>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
discover_stack   # picks up the admin key and chain addresses from the running stack

MODE=real
REVERT=0
ARGS=()
for a in "$@"; do
  case "$a" in
    --fast)   MODE=fast ;;
    --real)   MODE=real ;;
    --revert) REVERT=1 ;;
    *)        ARGS+=("$a") ;;
  esac
done
TARGET="${ARGS[0]:-}"; TIER="${ARGS[1]:-notice_400}"

if [[ ! "$TARGET" =~ ^0x[0-9a-fA-F]{40}$ && ! "$TARGET" =~ ^did:privy:[0-9a-z]+$ ]]; then
  echo "usage: $0 [--fast|--real|--revert] <wallet-address|did> [notice_400|warning_500|escrow_600|blocked]" >&2
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

# Resolve to a did and an address; the redeem endpoint wants the address, the
# ledger wants the did.
#
# Matched on ONE field at a time, and ambiguity is an error rather than a
# LIMIT 1 coin flip. Wallet addresses are not unique across accounts here: while
# a prank is active the app writes the PRANKER's wallet into the PRANKEE's row
# via PUT /users/primary-wallet, so two accounts genuinely share an address and
# "the first row" silently seeded the wrong person.
if [[ "$TARGET" == did:privy:* ]]; then
  UID_="$(psql_app "SELECT id FROM users WHERE id='$TARGET';")"
else
  matches="$(psql_app "SELECT id FROM users WHERE primary_wallet_address ILIKE '$TARGET' ORDER BY id;")"
  count="$(printf '%s\n' "$matches" | grep -c . || true)"
  if [[ "$count" -gt 1 ]]; then
    echo "  $TARGET is the primary wallet of more than one account:" >&2
    printf '%s\n' "$matches" | sed 's/^/    /' >&2
    echo "  pass the did instead so this is unambiguous." >&2
    exit 1
  fi
  UID_="$matches"
fi
[[ -n "$UID_" ]] || { echo "no account matches $TARGET" >&2; exit 1; }
ADDR="$(psql_app "SELECT COALESCE(primary_wallet_address,'') FROM users WHERE id='$UID_';")"

clear_seeded(){
  psql_app "DELETE FROM payout_ledger   WHERE user_id='$UID_' AND idempotency_key LIKE 'w9-seed-%';" >/dev/null
  psql_app "DELETE FROM w9_tier_notices WHERE user_id='$UID_' AND tax_year=$YEAR;" >/dev/null
}

if [[ "$REVERT" == "1" ]]; then
  clear_seeded
  echo "  cleared seeded W-9 state for $UID_ (on-chain balance is unchanged)"
  exit 0
fi

# Exactly on each line, not one over. decidePayout compares with >=, so 400 is
# already the notice and 600 is already the crossing; overshooting only made the
# modal read "401 of 600" and invited the reader to doubt the boundary.
case "$TIER" in
  notice_400)  REACH=400 ;;
  warning_500) REACH=500 ;;
  escrow_600)  REACH=600 ;;
  blocked)     REACH=600 ;;
  *) echo "unknown tier: $TIER" >&2; exit 1 ;;
esac

if [[ "$MODE" == "fast" ]]; then
  # paid for the two warning tiers, escrowed past the crossing — the state each
  # tier is actually in, not merely an amount.
  case "$TIER" in
    notice_400|warning_500) STATE=paid ;;
    *)                      STATE=escrowed ;;
  esac
  clear_seeded
  psql_app "
    INSERT INTO payout_ledger
      (idempotency_key, user_id, recipient_address, chain_id, tax_year, source,
       source_ref, amount_base, state, paid_at, escrowed_at, counts_toward_threshold)
    VALUES ('w9-seed-$UID_-$TIER', '$UID_', '$ADDR', 42220, $YEAR, 'redemption_code',
       'w9-seed-' || '$UID_', ${REACH}000000, '$STATE',
       CASE WHEN '$STATE'='paid' THEN NOW() ELSE NULL END,
       CASE WHEN '$STATE'='escrowed' THEN NOW() ELSE NULL END, TRUE);
    INSERT INTO w9_tier_notices (user_id, tax_year, tier, notified_at, acknowledged_at)
    VALUES ('$UID_', $YEAR, '$TIER', NOW(), NULL);" >/dev/null
  echo "  [fast] $UID_ seeded at $TIER (${REACH} SFLUV, $STATE) — no money moved"
  exit 0
fi

# ---- real mode ----
[[ -n "$ADDR" ]] || { echo "account has no primary wallet address; --fast is the only option" >&2; exit 1; }
clear_seeded

TZ_NAME="America/Los_Angeles"
s_at="$(TZ=$TZ_NAME date -v+1d '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day' '+%Y-%m-%dT%H:%M:%S')"
e_at="$(TZ=$TZ_NAME date -v+1d -v+3H '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day +3 hours' '+%Y-%m-%dT%H:%M:%S')"

# max_participants is 1 deliberately: an event reserves reward x participants
# against unallocated faucet balance, so 601 for five seats demands 3005 and is
# correctly refused. One seat is all a crossing needs.
mk_event(){
  admin_api POST /admin/volunteer-events "$(jq -nc \
    --arg t "$1" --arg s "$s_at" --arg e "$e_at" --arg tz "$TZ_NAME" --argjson r "$2" \
    '{title:$t, description:"W-9 tier fixture", start_at_local:$s, end_at_local:$e,
      timezone:$tz, max_participants:1, reward_amount_sfluv:$r, signup_mode:"internal"}')"
}
code_for(){ admin_api GET "/admin/volunteer-events/$1/codes" | jq -r '[.. | objects | select(has("number")) | .id] | first // empty'; }
redeem(){ curl -s -X POST "$API/redeem" -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg c "$1" --arg a "$ADDR" '{code:$c, address:$a}')" -w '\n%{http_code}' --max-time 120; }

RUN="w9tier-$(date +%H%M%S)"
ev="$(mk_event "$RUN-reach" "$REACH" | jq -r '.id')"
[[ -n "$ev" && "$ev" != "null" ]] || { echo "could not create the event — is the faucet funded? (./scripts/fund-faucet/fund-faucet.sh)" >&2; exit 1; }
code="$(code_for "$ev")"
[[ -n "$code" ]] || { echo "no code was minted for $ev" >&2; exit 1; }

r="$(redeem "$code")"; st="${r##*$'\n'}"
echo "  redeem -> HTTP $st"

if [[ "$TIER" == "blocked" ]]; then
  # The crossing is held; the NEXT payment is what gets refused, and refusal is
  # what arms the blocked tier. A second, deliberately small event does it.
  ev2="$(mk_event "$RUN-refused" 5 | jq -r '.id')"
  code2="$(code_for "$ev2")"
  r2="$(redeem "$code2")"; st2="${r2##*$'\n'}"
  echo "  second redeem -> HTTP $st2 (409 means refused, which is the point)"
fi

psql_app "SELECT '  now at tier: ' || COALESCE((SELECT tier FROM w9_tier_notices WHERE user_id='$UID_' AND tax_year=$YEAR ORDER BY notified_at DESC LIMIT 1),'(none)');"
