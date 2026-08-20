#!/usr/bin/env bash
# Sets the faucet's SFLUV balance directly, by writing the token's storage.
#
# The suite cannot otherwise run twice. Every scenario that creates an event
# commits reward x participants against UNALLOCATED faucet balance, and nothing
# gives it back — so each run leaves less than the last, and eventually event
# creation refuses with a message about unallocated balance that reads like a
# bug in the code under test.
#
# Draining real holders (fund-faucet.sh ALL_DONORS=1) buys a run or two and then
# runs out, because a fork only has what the chain had. Writing the balance slot
# is unbounded and repeatable, and it is only ever legitimate because this is a
# disposable local fork.
#
#   ./mint-to-faucet.sh [whole-sfluv]     default 100000
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

TOKEN="${SFLUV_TOKEN_ADDRESS:-}"
FAUCET="${SFLUV_FAUCET_ADDRESS:-}"
AMOUNT_SFLUV="${1:-100000}"
[[ "$TOKEN"  =~ ^0x[a-fA-F0-9]{40}$ ]] || die "could not discover the token address"
[[ "$FAUCET" =~ ^0x[a-fA-F0-9]{40}$ ]] || die "could not discover the faucet address"

decimals="$(cast call "$TOKEN" 'decimals()(uint8)' --rpc-url "$RPC" 2>/dev/null | awk '{print $1}')"
[[ "$decimals" =~ ^[0-9]+$ ]] || decimals=6
target="$(python3 -c "print(int($AMOUNT_SFLUV * 10**$decimals))")"

step "Minting to the faucet by storage write"
info "token $TOKEN · faucet $FAUCET · target ${AMOUNT_SFLUV} SFLUV"

# The balances mapping's slot number is not knowable from outside — it depends
# on the contract's layout, and a proxy moves it. So probe: write a marker into
# the slot each candidate implies, and see which one balanceOf actually reads.
before="$(token_balance "$FAUCET")"
marker="0x0000000000000000000000000000000000000000000000000000000000bada55"
found=""

# Try the namespaced slot first. This token is an upgradeable OpenZeppelin v5
# ERC20, which does NOT keep its balances at a small numbered slot — it puts the
# whole struct at keccak-derived base
# 0x52c6...ce00 ("openzeppelin.storage.ERC20"), so probing 0..20 finds nothing
# and the failure looks like the probe is broken rather than the layout being
# different.
OZ_ERC20_NS=0x52c63247e1f47db19d5ce0460030c497f067ca4cebf71ba98eeadabe20bace00
for slot in "$OZ_ERC20_NS" $(seq 0 20); do
  key="$(cast index address "$FAUCET" "$slot" 2>/dev/null)" || continue
  original="$(cast storage "$TOKEN" "$key" --rpc-url "$RPC" 2>/dev/null)"
  cast rpc anvil_setStorageAt "$TOKEN" "$key" "$marker" --rpc-url "$RPC" >/dev/null 2>&1 || continue
  if [[ "$(token_balance "$FAUCET")" == "12245589" ]]; then
    found="$slot"
    # Put it back before writing the real value, so a wrong guess leaves no trace.
    [[ -n "$original" ]] && cast rpc anvil_setStorageAt "$TOKEN" "$key" "$original" --rpc-url "$RPC" >/dev/null 2>&1
    break
  fi
  [[ -n "$original" ]] && cast rpc anvil_setStorageAt "$TOKEN" "$key" "$original" --rpc-url "$RPC" >/dev/null 2>&1
done

if [[ -z "$found" ]]; then
  fail "could not find the balances slot in the first 21 — fall back to ALL_DONORS=1 ./fund-faucet.sh"
  summary "Mint to faucet"; exit 1
fi
pass "balances mapping is at slot $found"

key="$(cast index address "$FAUCET" "$found")"
cast rpc anvil_setStorageAt "$TOKEN" "$key" "$(cast to-uint256 "$target")" --rpc-url "$RPC" >/dev/null
after="$(token_balance "$FAUCET")"

if [[ "$after" == "$target" ]]; then
  pass "faucet set to $after (was $before)"
else
  fail "wrote the slot but balanceOf reads $after, wanted $target"
fi

summary "Mint to faucet"
