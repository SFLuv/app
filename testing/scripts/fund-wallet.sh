#!/usr/bin/env bash
# Puts SFLUV into any wallet on the local fork.
#
# Distinct from fund-faucet.sh, which tops up the payout faucet. This one funds
# a PERSON's wallet, so a browser spec can actually send money — a send spec
# against an empty wallet fails for a reason that has nothing to do with the
# code under test.
#
# anvil forks mainnet, so impersonating a real holder costs nothing and touches
# no live system: the transfer exists only on the local fork.
#
#   ./fund-wallet.sh 0xRecipient [whole-sfluv]
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

TO="${1:-}"
AMOUNT_SFLUV="${2:-500}"
[[ "$TO" =~ ^0x[a-fA-F0-9]{40}$ ]] || die "usage: ./fund-wallet.sh 0xRecipient [whole-sfluv]"

TOKEN="${SFLUV_TOKEN_ADDRESS:-}"
DONOR="${SFLUV_FAUCET_ADDRESS:-}"
[[ "$TOKEN" =~ ^0x ]] || die "could not discover the token address"
[[ "$DONOR" =~ ^0x ]] || die "could not discover a donor wallet"

# 6 decimals — the token's own, not 18. Getting this wrong sends a millionth of
# what was asked for, or a million times too much.
decimals="$(cast call "$TOKEN" 'decimals()(uint8)' --rpc-url "$RPC" 2>/dev/null | awk '{print $1}')"
[[ "$decimals" =~ ^[0-9]+$ ]] || decimals=6
raw="$(cast to-unit "${AMOUNT_SFLUV}" wei 2>/dev/null)"
raw="$(python3 -c "print(int($AMOUNT_SFLUV * 10**$decimals))")"

before="$(token_balance "$TO")"
info "token $TOKEN (${decimals} decimals) · donor $DONOR"
info "recipient balance before: ${before:-0}"

cast rpc anvil_impersonateAccount "$DONOR" --rpc-url "$RPC" >/dev/null
cast rpc anvil_setBalance "$DONOR" 0xDE0B6B3A7640000 --rpc-url "$RPC" >/dev/null   # gas
if cast send "$TOKEN" 'transfer(address,uint256)' "$TO" "$raw" \
     --from "$DONOR" --unlocked --rpc-url "$RPC" >/dev/null 2>&1; then
  pass "sent ${AMOUNT_SFLUV} SFLUV (${raw} base units)"
else
  fail "transfer failed"
fi
cast rpc anvil_stopImpersonatingAccount "$DONOR" --rpc-url "$RPC" >/dev/null 2>&1 || true

after="$(token_balance "$TO")"
info "recipient balance after: ${after:-0}"
[[ "${after:-0}" != "${before:-0}" ]] && pass "balance moved" || fail "balance did not move"

summary "Fund wallet"
