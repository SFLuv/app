#!/usr/bin/env bash
# Proves the stack is up, local, and in a state where testing means something.
#
# Run this first when something feels off. Every failure here is a reason a
# testing session would produce misleading results rather than useful ones.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"

step "Preflight"
require_local_stack

# --- services -------------------------------------------------------------
check_port(){
  local name="$1" url="$2" required="$3"
  local code; code="$(curl -sk -o /dev/null -w '%{http_code}' --max-time 6 "$url" 2>/dev/null || echo 000)"
  if [[ "$code" =~ ^(200|301|302|307|308|401|403|404)$ ]]; then
    pass "$name reachable ($url → $code)"
  elif [[ "$required" == "required" ]]; then
    fail "$name unreachable at $url"
  else
    skip "$name not running at $url — features needing it cannot be tested"
  fi
}

check_port "backend"  "$API/config"        required
check_port "frontend" "$WEB"               optional
check_port "website"  "$SITE"              optional
# Ponder answers 500 at its root and 200 at /health — probing the root reports a
# healthy indexer as dead.
check_port "ponder"   "http://localhost:42069/health" optional

# --- tooling --------------------------------------------------------------
for tool in jq cast curl; do
  if command -v "$tool" >/dev/null 2>&1; then pass "$tool present"; else fail "$tool missing — required"; fi
done

# --- is the running backend built from the current source? ----------------
#
# The single most expensive way to waste a morning: debugging "missing" fields
# and 404 routes that exist perfectly well in the source, because the process
# answering has been up since before they were written.
#
# Compares the backend process start time against the newest commit touching
# backend/. Cheap, and it turns hours of confusion into one line.
backend_pid="$(lsof -ti:"${API##*:}" -sTCP:LISTEN 2>/dev/null | head -1)"
if [[ -n "$backend_pid" ]]; then
  started_epoch="$(ps -p "$backend_pid" -o lstart= 2>/dev/null | xargs -0 date -jf '%a %b %e %T %Y' '+%s' 2>/dev/null)"
  commit_epoch="$(cd "$SFLUV_ROOT" && git log -1 --format=%ct -- backend/ 2>/dev/null)"
  if [[ -n "$started_epoch" && -n "$commit_epoch" ]]; then
    if [[ "$started_epoch" -lt "$commit_epoch" ]]; then
      age_min=$(( (commit_epoch - started_epoch) / 60 ))
      if [[ $age_min -lt 60 ]]; then
        # Committing code that was already running trips this harmlessly, which
        # is common right after a fix. Worth saying, not worth failing over.
        skip "the last backend commit is ${age_min}m newer than the process — fine if you just committed code that was already running, otherwise run ./scripts/restart-backend/restart-backend.sh"
      else
        fail "the running backend predates the newest backend commit by ~$(( age_min / 60 ))h — run ./scripts/restart-backend/restart-backend.sh, or you will debug code that is not running"
      fi
    else
      pass "the running backend is newer than the last backend commit"
    fi
  else
    skip "could not compare backend start time to the last commit"
  fi

  # Uncommitted backend work is invisible to the check above.
  if [[ -n "$(cd "$SFLUV_ROOT" && git status --porcelain backend/ 2>/dev/null)" ]]; then
    info "backend/ has uncommitted changes — the running process may not include them either"
  fi
fi

# --- chain state ----------------------------------------------------------
# A faucet with no balance is the single most common reason a payout fails for
# a reason that has nothing to do with the code being tested.
if [[ -n "${SFLUV_FAUCET_ADDRESS:-}" && -n "${SFLUV_TOKEN_ADDRESS:-}" ]]; then
  bal="$(token_balance "$SFLUV_FAUCET_ADDRESS")"
  if [[ "${bal:-0}" =~ ^[0-9]+$ ]] && [[ "$bal" -gt 0 ]]; then
    pass "faucet holds tokens (raw $bal)"
  else
    fail "faucet balance is zero — payouts will fail for the wrong reason. Run ./scripts/fund-faucet/fund-faucet.sh"
  fi
else
  skip "SFLUV_FAUCET_ADDRESS / SFLUV_TOKEN_ADDRESS unset — cannot check the faucet"
fi

# --- chain clock ----------------------------------------------------------
#
# A fork whose timestamp has drifted from wall time fails EVERY
# account-abstraction operation with "AA32 expired or not due", because the
# paymaster signs a real-time validity window that block.timestamp then
# disagrees with. It presents as the web app hanging on its spinner forever,
# with a backend that looks entirely healthy. Cheap to check, expensive to
# diagnose from the symptom.
chain_ts="$(cast block latest --rpc-url "$RPC" 2>/dev/null | awk '/^timestamp/{print $2}')"
if [[ -n "$chain_ts" ]]; then
  drift=$(( $(date +%s) - chain_ts ))
  if [[ ${drift#-} -lt 300 ]]; then
    pass "chain clock is within 5 minutes of wall time (${drift}s)"
  else
    fail "chain clock is ${drift}s out — account abstraction will fail with AA32. Run ./scripts/sync-chain-time/sync-chain-time.sh"
  fi
fi

# --- W-9 enforcement ------------------------------------------------------
# Shadow mode computes the decision and pays anyway, so the W-9 gate looks
# broken while working exactly as configured.
info "W9_ENFORCEMENT must be 'enforce' locally or the W-9 tiers will not gate anything"

summary "Preflight"
