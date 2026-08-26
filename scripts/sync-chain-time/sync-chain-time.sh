#!/usr/bin/env bash
# Drags the anvil fork's clock back to wall time.
#
# Why this matters far more than it looks: the paymaster signs every
# UserOperation with a validity window taken from REAL time, and the chain
# judges it against block.timestamp. Let those drift apart and every
# account-abstraction operation fails with:
#
#     AA32 expired or not due
#
# which surfaces as the web app hanging on its loading spinner forever — login,
# sends, everything — with a backend that looks perfectly healthy because it is.
#
# Two things cause the drift:
#   - anvil only advances time when blocks are mined, so an idle fork falls
#     behind by however long it sat there;
#   - evm_revert restores an OLD snapshot, timestamp included, so any test that
#     snapshots and reverts drags the clock backwards.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"

step "Chain clock"

chain_ts="$(cast block latest --rpc-url "$RPC" 2>/dev/null | awk '/^timestamp/{print $2}')"
[[ -n "$chain_ts" ]] || die "no chain on $RPC"
now="$(date +%s)"
drift=$(( now - chain_ts ))

info "chain $(date -r "$chain_ts" '+%Y-%m-%d %H:%M:%S') · wall $(date -r "$now" '+%Y-%m-%d %H:%M:%S') · drift ${drift}s"

if [[ ${drift#-} -lt 120 ]]; then
  pass "within two minutes of wall clock — nothing to do"
  summary "Chain clock"; exit 0
fi

cast rpc anvil_setTime "$now" --rpc-url "$RPC" >/dev/null 2>&1 || die "anvil_setTime failed"
cast rpc evm_mine --rpc-url "$RPC" >/dev/null 2>&1 || die "evm_mine failed"

after="$(cast block latest --rpc-url "$RPC" 2>/dev/null | awk '/^timestamp/{print $2}')"
new_drift=$(( $(date +%s) - after ))
if [[ ${new_drift#-} -lt 120 ]]; then
  pass "resynchronised (was ${drift}s out, now ${new_drift}s)"
else
  fail "still ${new_drift}s out after setting the time"
fi

summary "Chain clock"
