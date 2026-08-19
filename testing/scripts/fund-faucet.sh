#!/usr/bin/env bash
# Tops the local faucet up from any holder on the fork.
#
# The faucet starting empty is the single most common reason a payout scenario
# fails for a reason unrelated to the code under test. dev-up.sh clones the
# production balance at boot (dev-up.sh:991); this does the same thing on
# demand, for when a run has drained it.
#
# anvil forks mainnet, so impersonating a real holder costs nothing and touches
# no live system — the transfer happens only on the local fork.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

TOKEN="${SFLUV_TOKEN_ADDRESS:-}"
FAUCET="${SFLUV_FAUCET_ADDRESS:-}"
DONOR="${SFLUV_DONOR_ADDRESS:-${PROD_FAUCET_ADDRESS:-}}"

if [[ -z "$DONOR" && -f "$SFLUV_ROOT/backend/.env" ]]; then
  DONOR="$(grep -E '^BOT_ADDRESS=' "$SFLUV_ROOT/backend/.env" | head -1 | cut -d= -f2- | tr -d "\"' [:space:]")"
fi

[[ "$TOKEN"  =~ ^0x[a-fA-F0-9]{40}$ ]] || die "set SFLUV_TOKEN_ADDRESS"
[[ "$FAUCET" =~ ^0x[a-fA-F0-9]{40}$ ]] || die "set SFLUV_FAUCET_ADDRESS (the local faucet, from tmp/faucet.key)"
[[ "$DONOR"  =~ ^0x[a-fA-F0-9]{40}$ ]] || die "set SFLUV_DONOR_ADDRESS, or BOT_ADDRESS in backend/.env"

before="$(token_balance "$FAUCET")"
donor_bal="$(cast call "$TOKEN" 'balanceOf(address)(uint256)' "$DONOR" --rpc-url "$RPC" 2>/dev/null | awk '{print $1}')"
info "faucet $before · donor $donor_bal"
[[ "${donor_bal:-0}" =~ ^[0-9]+$ && "$donor_bal" != "0" ]] || die "donor $DONOR holds no tokens on the fork"

AMOUNT="${1:-$donor_bal}"

cast rpc anvil_impersonateAccount "$DONOR" --rpc-url "$RPC" >/dev/null
cast rpc anvil_setBalance "$DONOR" 0xDE0B6B3A7640000 --rpc-url "$RPC" >/dev/null   # 1 ether for gas
if cast send "$TOKEN" 'transfer(address,uint256)' "$FAUCET" "$AMOUNT" \
     --from "$DONOR" --unlocked --rpc-url "$RPC" >/dev/null 2>&1; then
  pass "transferred $AMOUNT raw units to the faucet"
else
  fail "transfer failed"
fi
cast rpc anvil_stopImpersonatingAccount "$DONOR" --rpc-url "$RPC" >/dev/null 2>&1 || true

info "faucet now: $(token_balance "$FAUCET")"
summary "Fund faucet"
