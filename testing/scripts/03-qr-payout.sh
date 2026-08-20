#!/usr/bin/env bash
# QR redemption: the three outcomes a volunteer can get.
#
#   200  paid
#   202  held  — past the limit, money reserved, code consumed
#   409  refused — past the limit with a hold already open; the code is HANDED
#        BACK, so the same QR works once the form is in
#
# The 409 is the one worth caring about. If it ever consumes the code, a
# volunteer loses a reward at a live event with nothing to re-present.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

step "QR redemption"

CODE="${SFLUV_TEST_CODE:-}"
if [[ -z "$CODE" && -f "$RUN_DIR/codes.txt" ]]; then CODE="$(head -1 "$RUN_DIR/codes.txt")"; fi
if [[ -z "$CODE" ]]; then
  latest="$(ls -t "$ARTIFACTS_DIR"/run-*/codes.txt 2>/dev/null | head -1)"
  [[ -n "$latest" ]] && CODE="$(head -1 "$latest")"
fi
# A recipient nobody has to remember to export.
#
# run-all sets no address, so this scenario used to fail for want of a variable
# rather than for anything about redemption. Any address works — the assertion
# is that the balance MOVES, not whose it is — so derive a stable one from the
# run id and let the chain create it on first receipt.
ADDR="${SFLUV_TEST_ADDRESS:-}"
if [[ -z "$ADDR" ]]; then
  ADDR="0x$(printf '%s' "qr-payout-$RUN_ID" | shasum -a 256 | cut -c1-40)"
  info "recipient (derived): $ADDR"
fi

if [[ -z "$CODE" || -z "$ADDR" ]]; then
  # Deliberately a failure, not a skip. This is the scenario that proves a
  # volunteer actually gets paid; letting it pass with nothing redeemed means
  # the suite reports green having tested the one thing that moves money not at
  # all. run-all.sh always runs 02 first, so reaching here means something is
  # genuinely wrong rather than merely unset.
  fail "no code or recipient — run 02-events.sh first and export SFLUV_TEST_ADDRESS"
  summary "QR payout"; exit 1
fi

snap="$(chain_snapshot)"
info "chain snapshot $snap — the chain is restored at the end"

before="$(token_balance "$ADDR")"
info "recipient balance before: ${before:-unknown}"

body="$(jq -nc --arg c "$CODE" --arg a "$ADDR" '{code:$c, address:$a}')"
resp="$(curl -s -X POST "$API/redeem" -H 'Content-Type: application/json' -d "$body" -w '\n%{http_code}' --max-time 60)"
status="${resp##*$'\n'}"; payload="${resp%$'\n'*}"
printf '%s' "$payload" > "$RUN_DIR/redeem-response.json"

case "$status" in
  200)
    pass "redeemed and paid (HTTP 200)"
    after="$(token_balance "$ADDR")"
    if [[ "${after:-0}" != "${before:-0}" ]]; then
      pass "on-chain balance moved ($before → $after)"
    else
      fail "HTTP 200 but the balance did not move — the payout did not land"
    fi
    ;;
  202)
    pass "held pending a W-9 (HTTP 202) — correct at the threshold crossing"
    expect_json "$payload" '.reason' 'w9_required' "the hold names the reason"
    after="$(token_balance "$ADDR")"
    if [[ "${after:-0}" == "${before:-0}" ]]; then
      pass "nothing was sent, as expected for a hold"
    else
      fail "money moved despite a 202 — escrow is not actually holding"
    fi
    ;;
  409)
    pass "refused (HTTP 409) — expected once a hold is already open"
    expect_json "$payload" '.status' 'blocked' "the refusal is labelled blocked"

    # The whole point of the 409 design: try the same code again and it must
    # still be claimable.
    retry="$(curl -s -X POST "$API/redeem" -H 'Content-Type: application/json' -d "$body" -w '\n%{http_code}' --max-time 60)"
    rstatus="${retry##*$'\n'}"
    if [[ "$rstatus" == "409" ]]; then
      pass "the code survives a refusal and can be presented again"
    elif [[ "$rstatus" == "400" ]]; then
      fail "the code was consumed by the refusal — a volunteer would lose this reward"
    else
      info "second attempt returned $rstatus"
    fi
    ;;
  400)
    body_text="$(printf '%s' "$payload" | tr -d '\n')"
    # Only the reasons the backend actually names are tolerable here. The old
    # version skipped on ANY 400 while asserting a cause it had not checked, so
    # a new refusal — a merchant bar, a changed guard — read as "probably
    # expired" and the suite stayed green having redeemed nothing.
    case "$body_text" in
      *"code expired"*|*"code redeemed"*|*"user redeemed"*|*"code not started"*)
        skip "code not usable: $body_text — try ./refresh-qr-windows.sh"
        ;;
      *)
        fail "redeem refused with an unrecognised 400: $body_text"
        ;;
    esac
    ;;
  *)
    fail "unexpected redeem status $status"
    ;;
esac

[[ -n "$snap" ]] && chain_revert "$snap" && info "chain reverted"
summary "QR payout"
