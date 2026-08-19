#!/usr/bin/env bash
# The flagship test: earn past the annual limit and watch the gate work.
#
#   pay below the limit        → paid
#   the payment that crosses   → HELD, money reserved, code consumed (202)
#   any payment after that     → REFUSED, code handed back (409)
#   file the form              → the held money releases on chain
#
# This is the whole point of the W-9 system, and none of it can be proven with
# an already-cleared account — the gate short-circuits. Set SFLUV_SUBJECT_DID
# and SFLUV_SUBJECT_ADDRESS to somebody with filing_status not_started.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

SUBJECT_DID="${SFLUV_SUBJECT_DID:-}"
SUBJECT_ADDR="${SFLUV_SUBJECT_ADDRESS:-}"
[[ -n "$SUBJECT_DID" && -n "$SUBJECT_ADDR" ]] || die "set SFLUV_SUBJECT_DID and SFLUV_SUBJECT_ADDRESS"

step "W-9 threshold crossing, as ${SUBJECT_DID#did:privy:}"
prank_as "$SUBJECT_DID"

before="$(api GET /w9/status)"
if [[ "$(printf '%s' "$before" | jq -r '.cleared')" == "true" ]]; then
  fail "this account is already cleared — the gate will short-circuit and prove nothing"
  prank_clear; summary "W-9 crossing"; exit 1
fi
threshold="$(printf '%s' "$before" | jq -r '.threshold_sfluv')"
info "starting at $(printf '%s' "$before" | jq -r '.earned_sfluv') of $threshold"

snap="$(chain_snapshot)"
bal_before="$(token_balance "$SUBJECT_ADDR")"
info "chain snapshot $snap · balance $bal_before"

# One event whose reward alone clears the limit, so a single redemption crosses
# rather than a hundred small ones.
TZ_NAME="America/Los_Angeles"
s_at="$(TZ=$TZ_NAME date -v+1d '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day' '+%Y-%m-%dT%H:%M:%S')"
e_at="$(TZ=$TZ_NAME date -v+1d -v+3H '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day +3 hours' '+%Y-%m-%dT%H:%M:%S')"
reward=$(( threshold + 50 ))

# max_participants is 1 on purpose. An event reserves reward x participants
# against unallocated faucet balance, so a 650 reward for 5 people demands 3250
# and is refused — correctly. One seat is all a crossing test needs.
mk_event(){
  admin_api POST /admin/volunteer-events "$(jq -nc \
    --arg t "$1" --arg s "$s_at" --arg e "$e_at" --arg tz "$TZ_NAME" --argjson r "$2" \
    '{title:$t, description:"W-9 crossing test", start_at_local:$s, end_at_local:$e,
      timezone:$tz, max_participants:1, reward_amount_sfluv:$r, signup_mode:"internal"}')"
}
code_for(){
  admin_api GET "/admin/volunteer-events/$1/codes" \
    | jq -r '[.. | objects | select(has("number")) | .id] | first // empty'
}
redeem(){
  curl -s -X POST "$API/redeem" -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg c "$1" --arg a "$SUBJECT_ADDR" '{code:$c, address:$a}')" \
    -w '\n%{http_code}' --max-time 90
}

step "The crossing payment — expect it to be HELD"
ev1_raw="$(mk_event "w9-cross-$RUN_ID" "$reward")"
if [[ ! "$(status)" =~ ^20 ]]; then
  fail "could not create the crossing event ($(status)): $(printf '%s' "$ev1_raw" | head -c 160)"
  prank_clear; summary "W-9 crossing"; exit 1
fi
ev1="$(printf '%s' "$ev1_raw" | jq -r '.id')"
code1="$(code_for "$ev1")"
expect_nonempty "$code1" "a code was issued for $reward SFLUV"

r1="$(redeem "$code1")"; st1="${r1##*$'\n'}"; body1="${r1%$'\n'*}"
printf '%s' "$body1" > "$RUN_DIR/crossing-redeem.json"
case "$st1" in
  202) pass "held pending a W-9 (202) — the gate caught the crossing"
       expect_json "$body1" '.reason' 'w9_required' "the hold names the reason" ;;
  200) fail "PAID a payment that crosses the limit — the gate did not fire (is W9_ENFORCEMENT=enforce?)" ;;
  *)   fail "crossing redemption returned $st1" ;;
esac

mid="$(token_balance "$SUBJECT_ADDR")"
if [[ "$mid" == "$bal_before" ]]; then
  pass "nothing reached the wallet while held"
else
  fail "money moved despite a hold ($bal_before → $mid)"
fi

step "A second payment — expect it REFUSED, with the code handed back"
ev2="$(mk_event "w9-block-$RUN_ID" 5 | jq -r '.id')"
code2="$(code_for "$ev2")"
r2="$(redeem "$code2")"; st2="${r2##*$'\n'}"
case "$st2" in
  409) pass "refused (409) — escrow cannot accumulate"
       r2b="$(redeem "$code2")"
       case "${r2b##*$'\n'}" in
         409) pass "the code survives — the volunteer can present it again after filing" ;;
         400) fail "the refusal CONSUMED the code — a volunteer would lose this reward" ;;
         *)   info "second attempt returned ${r2b##*$'\n'}" ;;
       esac ;;
  202) fail "held a SECOND payment — escrow is accumulating, which the tier design forbids" ;;
  *)   fail "second redemption returned $st2" ;;
esac

step "Filing the form"
start="$(api POST /w9/start)"
url="$(printf '%s' "$start" | jq -r '.form_url // empty')"
expect_nonempty "$url" "a hosted form URL was issued"
info "form: $url"

case "$url" in
  *localhost*|*127.0.0.1*)
    curl -s -o /dev/null -X POST "$url" --max-time 30
    pass "stub form submitted"

    # Completion is discovered by the maintenance sweep, never pushed.
    step "Waiting for the sweep to notice and release"
    released=0
    for i in $(seq 1 30); do
      st="$(api GET /w9/status)"
      if [[ "$(printf '%s' "$st" | jq -r '.cleared')" == "true" ]]; then
        pass "filing cleared after ~$((i*10))s"
        released=1; break
      fi
      sleep 10
    done
    [[ $released -eq 1 ]] || fail "the filing never cleared — is the maintenance scheduler running?"

    after="$(token_balance "$SUBJECT_ADDR")"
    if [[ "$after" != "$bal_before" ]]; then
      pass "held money released on chain ($bal_before → $after)"
    else
      fail "cleared, but the escrowed payout never landed"
    fi
    ;;
  *) skip "real vendor — open the URL and sign it, then re-run to see the release" ;;
esac

[[ -n "$snap" ]] && chain_revert "$snap" && info "chain reverted"
prank_clear
summary "W-9 crossing"
