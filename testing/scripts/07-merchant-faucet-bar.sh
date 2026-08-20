#!/usr/bin/env bash
# The merchant faucet bar: a merchant scanning an event QR is refused, and the
# code they scanned is still there for a volunteer to claim.
#
# The refusal has to happen before the code is consumed. If it ever moves after
# db.Redeem it depends on UndoRedeem to hand the code back, and the two ways of
# getting that wrong are a code burnt for nothing and a code claimable twice.
# Scan 2 in this scenario is what actually proves it: the SAME code is presented
# by a regular account and must pay out.
#
# The merchant is identified by account type, not by address. resolveRedeem-
# PayoutAddress rewrites a till to the owner's personal wallet before anything
# else sees it, so the fixture below deliberately picks a till that is NOT the
# owner's personal wallet — an address-shaped check would sail straight past it.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

step "Merchant faucet bar"

# Reads the same database the backend is pointed at, for fixtures the API
# cannot hand out (account types) and for the one assertion that matters most
# (the code was never consumed).
DEV_ENV="$SFLUV_ROOT/tmp/backend.dev.env"
[[ -f "$DEV_ENV" ]] || { skip "no $DEV_ENV — cannot reach the dev databases"; summary "merchant faucet bar"; exit 0; }
DB_USER="$(grep -E '^DB_USER=' "$DEV_ENV" | head -1 | cut -d= -f2- | tr -d "\"' ")"
DB_HOST="$(grep -E '^DB_URL=' "$DEV_ENV" | head -1 | cut -d= -f2- | tr -d "\"' " | cut -d: -f1)"
app_sql(){ psql -U "$DB_USER" -h "$DB_HOST" -d app -tAc "$1" 2>/dev/null | head -1; }
bot_sql(){ psql -U "$DB_USER" -h "$DB_HOST" -d bot -tAc "$1" 2>/dev/null | head -1; }

# A till that is not also the owner's personal wallet, so the rewrite is real.
MERCHANT_TILL="$(app_sql "
  SELECT l.payment_wallet_address
  FROM locations l JOIN users u ON u.id = l.owner_id
  WHERE u.account_type = 'merchant' AND u.active
    AND COALESCE(l.payment_wallet_address,'') <> ''
    AND LOWER(l.payment_wallet_address) <> LOWER(COALESCE(u.primary_wallet_address,''))
  LIMIT 1;")"
# Under the reporting threshold, so an accepted scan pays rather than escrows.
REGULAR_ADDR="$(app_sql "
  SELECT u.primary_wallet_address
  FROM users u
  WHERE u.account_type = 'regular' AND u.active
    AND COALESCE(u.primary_wallet_address,'') <> ''
    AND NOT COALESCE((SELECT bool_or(e.w9_required) FROM w9_wallet_earnings e
                      WHERE LOWER(e.wallet_address) = LOWER(u.primary_wallet_address)), false)
  LIMIT 1;")"

if [[ -z "$MERCHANT_TILL" || -z "$REGULAR_ADDR" ]]; then
  skip "no merchant till and regular volunteer pair in this database"
  summary "merchant faucet bar"; exit 0
fi
info "merchant till     $MERCHANT_TILL"
info "regular volunteer $REGULAR_ADDR"

snap="$(chain_snapshot)"
info "chain snapshot $snap — restored at the end"

TZ_NAME="America/Los_Angeles"
start_at="$(TZ=$TZ_NAME date -v+1d '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day' '+%Y-%m-%dT%H:%M:%S')"
end_at="$(TZ=$TZ_NAME date -v+1d -v+3H '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day +3 hours' '+%Y-%m-%dT%H:%M:%S')"
created="$(admin_api POST /admin/volunteer-events "$(jq -nc \
  --arg t "merchant-bar-$RUN_ID" --arg s "$start_at" --arg e "$end_at" --arg tz "$TZ_NAME" \
  '{title:$t,description:"merchant faucet bar scenario",start_at_local:$s,end_at_local:$e,
    timezone:$tz,max_participants:5,reward_amount_sfluv:5,signup_mode:"internal"}')")"
expect_status 201 "event created"
event_id="$(printf '%s' "$created" | jq -r '.id // empty')"
CODE="$(admin_api GET "/admin/volunteer-events/$event_id/codes" \
        | jq -r '[.. | strings | select(test("^[0-9a-f]{8}-"))] | first')"
expect_nonempty "$CODE" "a redemption code was issued"
[[ -n "$CODE" && "$CODE" != "null" ]] || { chain_revert "$snap"; summary "merchant faucet bar"; exit 1; }

redeem(){ curl -s -X POST "$API/redeem" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg c "$1" --arg a "$2" '{code:$c,address:$a}')" -w '\n%{http_code}' --max-time 90; }

step "Scan 1 — the merchant"
r="$(redeem "$CODE" "$MERCHANT_TILL")"; st="${r##*$'\n'}"; body="${r%$'\n'*}"
printf '%s' "$body" > "$RUN_DIR/merchant-refusal.json"
if [[ "$st" == "409" ]]; then pass "the merchant is refused (HTTP 409)"; else fail "expected 409, got $st"; fi
expect_json "$body" '.reason' 'merchant_account' "the refusal names merchant_account"
expect_json "$body" '.status' 'blocked' "the refusal is labelled blocked"
if [[ -n "$(printf '%s' "$body" | jq -r '.message // empty')" ]]; then
  pass "the refusal carries a sentence a redeem screen can render"
else
  fail "no message — the app has nothing to show the person holding the phone"
fi

step "The code survived the refusal"
redeemed="$(bot_sql "SELECT redeemed FROM codes WHERE id = '$CODE';")"
if [[ "$redeemed" == "f" ]]; then
  pass "the code is still unredeemed"
else
  fail "the refusal consumed the code (redeemed=$redeemed) — a volunteer would lose this reward"
fi

step "Scan 2 — the same code, a regular volunteer"
before="$(token_balance "$REGULAR_ADDR")"
r="$(redeem "$CODE" "$REGULAR_ADDR")"; st="${r##*$'\n'}"; body="${r%$'\n'*}"
after="$(token_balance "$REGULAR_ADDR")"
case "$st" in
  200) pass "the regular volunteer is paid (HTTP 200)"
       if [[ "${after:-0}" != "${before:-0}" ]]; then
         pass "on-chain balance moved ($before → $after)"
       else
         fail "HTTP 200 but nothing moved"
       fi ;;
  202) pass "accepted and held pending a W-9 (HTTP 202) — accepted, which is the point here" ;;
  409) fail "a regular volunteer was refused: $(printf '%s' "$body" | jq -r '.reason // "?"')" ;;
  *)   fail "the regular volunteer was not accepted: HTTP $st $body" ;;
esac

chain_revert "$snap"
info "chain reverted"
summary "merchant faucet bar"
