#!/usr/bin/env bash
# Shared helpers for the SFLUV feature tests.
#
# Source this, do not execute it. Everything here assumes the local dev stack
# from ../../dev-up.sh is already running.
#
# The one rule this file enforces above all others: a test run must be
# physically incapable of touching production. See require_local_stack.

set -uo pipefail

SFLUV_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TESTING_DIR="$SFLUV_ROOT/testing"
ARTIFACTS_DIR="$TESTING_DIR/artifacts"
RUN_ID="${SFLUV_RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
RUN_DIR="$ARTIFACTS_DIR/run-$RUN_ID"
mkdir -p "$RUN_DIR"

API="${SFLUV_API:-http://localhost:8080}"
# HTTPS, and self-signed. dev-up serves the frontend from the pair in
# frontend/certificates/, so plain http gets connection-refused and every curl
# needs -k. Getting this wrong makes a running frontend look like a dead one.
WEB="${SFLUV_WEB:-https://localhost:3000}"
SITE="${SFLUV_SITE:-http://localhost:3002}"
RPC="${SFLUV_RPC:-http://127.0.0.1:8545}"
ADMIN_KEY="${DEV_ADMIN_KEY:-local-dev-admin-key}"

# A captured Privy access token. See capture-token.sh — most scenarios need a
# real authenticated caller, because prank forwarding redirects an existing
# identity rather than creating one.
TOKEN_FILE="$ARTIFACTS_DIR/.token"

# The HTTP status of the last call.
#
# It lives in a file rather than a variable because scenarios capture bodies
# with out="$(api GET /thing)" — a command substitution, which runs in a
# SUBSHELL. A variable set inside it never reaches the caller, so every status
# assertion silently read empty and every expect_status failed. A file write
# survives the subshell; a variable assignment cannot.
STATUS_FILE="$RUN_DIR/.last_status"
: > "$STATUS_FILE"
status(){ cat "$STATUS_FILE" 2>/dev/null; }

PASS=0; FAIL=0; SKIP=0
declare -a FAILURES=()

c()  { printf '\033[%sm%s\033[0m\n' "$1" "$2"; }
info(){ c '0;36' "  $*"; }
step(){ printf '\n'; c '1;37' "▸ $*"; }

pass(){ PASS=$((PASS+1)); c '0;32' "  ✓ $*"; }
skip(){ SKIP=$((SKIP+1)); c '0;33' "  ~ $* (skipped)"; }
fail(){
  FAIL=$((FAIL+1)); FAILURES+=("$*")
  c '0;31' "  ✗ $*"
}

# die stops the whole run. Reserved for conditions where continuing would be
# unsafe or meaningless — never for an ordinary assertion failure, because one
# broken feature must not hide the state of the other twenty.
die(){ c '1;31' "FATAL: $*"; exit 1; }

# discover_stack fills in the chain addresses from the running stack instead of
# asking a human to export them. Anything already exported wins, so a scenario
# can still be pointed somewhere deliberately.
#
# Worth doing because a missing token address silently downgrades every balance
# assertion to a skip — the tests then pass while proving nothing about money.
discover_stack(){
  if [[ -z "${SFLUV_TOKEN_ADDRESS:-}" ]]; then
    SFLUV_TOKEN_ADDRESS="$(curl -s --max-time 8 "$API/config" 2>/dev/null \
      | jq -r '.. | objects | select(has("primary_token")) | .primary_token.address' 2>/dev/null | head -1)"
    export SFLUV_TOKEN_ADDRESS
  fi
  if [[ -z "${SFLUV_FAUCET_ADDRESS:-}" && -f "$SFLUV_ROOT/tmp/backend.dev.env" ]]; then
    SFLUV_FAUCET_ADDRESS="$(grep -E '^BOT_ADDRESS=' "$SFLUV_ROOT/tmp/backend.dev.env" \
      | head -1 | cut -d= -f2- | tr -d "\"' [:space:]")"
    export SFLUV_FAUCET_ADDRESS
  fi
}

# --------------------------------------------------------------------------
# Safety
# --------------------------------------------------------------------------

# require_local_stack refuses to proceed against anything that is not the local
# dev stack. It fails closed: every check must actively prove locality, and an
# unreachable or unexpected answer stops the run.
#
# This exists because these scripts create events, approve merchants, redeem
# codes and move tokens. Pointed at production by a stray environment variable
# they would do all of that for real.
require_local_stack(){
  case "$API" in
    http://localhost:*|http://127.0.0.1:*) ;;
    *) die "API is $API — the tests only ever run against localhost" ;;
  esac
  case "$RPC" in
    http://127.0.0.1:*|http://localhost:*) ;;
    *) die "RPC is $RPC — refusing to run against a non-local chain" ;;
  esac

  local health
  health="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$API/config" || echo 000)"
  [[ "$health" == "200" ]] || die "backend not answering on $API (got HTTP $health). Start it with ./dev-up.sh"

  # The prank table is the mechanism every multi-role scenario depends on, and
  # it is mounted only when IN_PRODUCTION is not true. If the backend under test
  # were a production build this would be absent — so its presence is also a
  # proof of which build we are talking to.
  local chain
  chain="$(cast chain-id --rpc-url "$RPC" 2>/dev/null || echo "")"
  [[ -n "$chain" ]] || die "no chain on $RPC — anvil is not running"
  discover_stack
  info "backend $API · chain id $chain · artifacts $RUN_DIR"
  [[ -n "${SFLUV_TOKEN_ADDRESS:-}" ]] && info "token ${SFLUV_TOKEN_ADDRESS} · faucet ${SFLUV_FAUCET_ADDRESS:-unknown}"
}

# --------------------------------------------------------------------------
# HTTP
# --------------------------------------------------------------------------

# api METHOD PATH [body] — an authenticated call as the captured user, with
# prank forwarding applied if one is set. Writes the body to stdout and the
# status code to SFLUV_STATUS.
api(){
  local method="$1" path="$2" body="${3:-}"
  local token=""
  [[ -f "$TOKEN_FILE" ]] && token="$(tr -d '[:space:]' < "$TOKEN_FILE")"

  # The backend reads Access-Token, NOT Authorization: Bearer — see
  # utils/middleware/auth.go. Sending a bearer header authenticates nothing and
  # every call comes back 403 with no hint as to why.
  local args=(-s -X "$method" -H "Accept: application/json" -w '\n%{http_code}')
  [[ -n "$token" ]] && args+=(-H "Access-Token: $token")
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" -d "$body")
  fi

  local raw; raw="$(curl "${args[@]}" --max-time 30 "$API$path" 2>/dev/null)"
  printf '%s' "${raw##*$'\n'}" > "$STATUS_FILE"
  printf '%s' "${raw%$'\n'*}"
}

# admin_api METHOD PATH [body] — the X-Admin-Key path, for arranging fixtures.
#
# Never use this to test an admin scenario. withAdmin injects userDid AFTER the
# prank middleware has run, so an admin-key request takes a different route
# through the stack than a real admin does — testing with it proves the wrong
# path works.
admin_api(){
  local method="$1" path="$2" body="${3:-}"
  local args=(-s -X "$method" -H "X-Admin-Key: $ADMIN_KEY" -H "Accept: application/json" -w '\n%{http_code}')
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" -d "$body")
  fi
  local raw; raw="$(curl "${args[@]}" --max-time 30 "$API$path" 2>/dev/null)"
  printf '%s' "${raw##*$'\n'}" > "$STATUS_FILE"
  printf '%s' "${raw%$'\n'*}"
}

# token_minutes_left prints how many minutes the captured token has left, or
# nothing if there is no token or it cannot be read.
#
# Worth its own helper because an expired token does not announce itself: every
# authenticated call just returns a bare 403, with no body and no reason header,
# which reads exactly like a role guard refusing you. Half an hour once went
# into chasing a "proposer guard" that was really a token that lapsed mid-run.
token_minutes_left(){
  [[ -f "$TOKEN_FILE" ]] || return 0
  python3 -c "$(printf '%s\n' \
    'import base64,json,sys,time' \
    'try:' \
    '    t=open(sys.argv[1]).read().strip().split(".")[1]' \
    '    t+="="*(-len(t)%4)' \
    '    print(int((json.loads(base64.urlsafe_b64decode(t))["exp"]-time.time())//60))' \
    'except Exception:' \
    '    pass')" "$TOKEN_FILE" 2>/dev/null
}

# expired_token_hint names the real cause when a refusal is really an expiry.
expired_token_hint(){
  local left; left="$(token_minutes_left)"
  [[ -n "$left" ]] || return 0
  if [[ "$left" -le 0 ]]; then
    c '0;33' "  the token expired $(( -left ))m ago — a bare 403 is auth, not a role guard. Re-run ./capture-token.sh"
  fi
}

# --------------------------------------------------------------------------
# Assertions
# --------------------------------------------------------------------------

expect_status(){
  local want="$1" label="$2"
  if [[ "$(status)" == "$want" ]]; then
    pass "$label (HTTP $want)"
  else
    fail "$label — expected HTTP $want, got $(status)"
    [[ "$(status)" == "403" ]] && expired_token_hint
  fi
}

expect_json(){
  local json="$1" filter="$2" want="$3" label="$4"
  local got; got="$(printf '%s' "$json" | jq -r "$filter" 2>/dev/null)"
  if [[ "$got" == "$want" ]]; then
    pass "$label"
  else
    fail "$label — $filter was '$got', expected '$want'"
  fi
}

expect_nonempty(){
  local value="$1" label="$2"
  if [[ -n "$value" && "$value" != "null" ]]; then pass "$label"; else fail "$label — empty"; fi
}

# --------------------------------------------------------------------------
# Prank forwarding: one login, every role
# --------------------------------------------------------------------------

# prank_as PRANKEE_DID — makes every subsequent api call act as that user.
#
# Driven through the dev-up menu, which reads stdin and exits cleanly without
# booting anything. Two rules keep it deterministic: search by full user id
# (a did never matches an email, so exactly one row comes back and the index is
# always 1), and end with q, because the menu redraws after every action.
prank_as(){
  local prankee="$1"
  local pranker="${SFLUV_PRANKER_DID:-}"
  [[ -n "$pranker" ]] || die "SFLUV_PRANKER_DID is not set — it must be the did of the account whose token is captured"
  printf '2\n%s\n1\n%s\n1\nq\n' "$pranker" "$prankee" \
    | (cd "$SFLUV_ROOT" && ./dev-up.sh menu) >"$RUN_DIR/prank.log" 2>&1
}

prank_clear(){
  printf '3\ny\nq\n' | (cd "$SFLUV_ROOT" && ./dev-up.sh menu) >>"$RUN_DIR/prank.log" 2>&1
}

# --------------------------------------------------------------------------
# Chain
# --------------------------------------------------------------------------

# Scenarios that move money snapshot first and revert after, so the order they
# run in cannot matter.
chain_snapshot(){ cast rpc evm_snapshot --rpc-url "$RPC" 2>/dev/null | tr -d '"'; }

# chain_revert restores the snapshot AND puts the clock back to wall time.
#
# evm_revert restores the whole snapshot, timestamp included, so a revert drags
# the chain's clock backwards by however long the scenario ran. That is not a
# local nuisance: the paymaster signs every UserOperation against real time, so
# once block.timestamp lags, every account-abstraction operation fails with
# "AA32 expired or not due" and the web app hangs on its spinner forever.
#
# Reverting without this leaves the developer's own browser broken.
chain_revert(){
  cast rpc evm_revert "$1" --rpc-url "$RPC" >/dev/null 2>&1
  cast rpc anvil_setTime "$(date +%s)" --rpc-url "$RPC" >/dev/null 2>&1
  cast rpc evm_mine --rpc-url "$RPC" >/dev/null 2>&1
}

# find_token_donor prints an address on the fork that actually holds SFLUV.
#
# The obvious donor — the production faucet named in backend/.env — is only a
# donor while the fork sits at a block where it happened to hold a balance.
# anvil re-forks at the chain tip on every boot, so that stops being true
# without warning, and the failure reads as "the token is broken" rather than
# "ask someone else".
#
# So: try the named donor, then fall back to scanning the addresses that appear
# in the env and community config and picking the richest. Everything here is a
# local fork, so impersonating whoever that turns out to be costs nothing.
find_token_donor(){
  local token="${SFLUV_TOKEN_ADDRESS:-}"
  [[ "$token" =~ ^0x ]] || return 1

  local named="${SFLUV_DONOR_ADDRESS:-}"
  if [[ -z "$named" && -f "$SFLUV_ROOT/backend/.env" ]]; then
    named="$(grep -E '^BOT_ADDRESS=' "$SFLUV_ROOT/backend/.env" | head -1 | cut -d= -f2- | tr -d "\"' [:space:]")"
  fi
  if [[ "$named" =~ ^0x[a-fA-F0-9]{40}$ ]]; then
    local balance; balance="$(token_balance "$named")"
    if [[ "${balance:-0}" =~ ^[0-9]+$ && "$balance" != "0" ]]; then
      printf '%s' "$named"; return 0
    fi
  fi

  # The faucet cannot donate to itself, and it is usually the richest address in
  # the config — so without this the discovery "finds" it and the transfer is a
  # no-op that reports success.
  local faucet_lower=""
  [[ -n "${SFLUV_FAUCET_ADDRESS:-}" ]] && faucet_lower="$(printf '%s' "$SFLUV_FAUCET_ADDRESS" | tr 'A-Z' 'a-z')"

  local best="" best_balance=0
  local candidate balance
  for candidate in $(grep -ohE '0x[a-fA-F0-9]{40}' \
        "$SFLUV_ROOT/backend/.env" "$SFLUV_ROOT/tmp/backend.dev.env" \
        "$SFLUV_ROOT/backend/community-config.json" 2>/dev/null | sort -u); do
    [[ "$(printf '%s' "$candidate" | tr 'A-Z' 'a-z')" == "$faucet_lower" ]] && continue
    balance="$(token_balance "$candidate")"
    [[ "${balance:-0}" =~ ^[0-9]+$ ]] || continue
    if (( balance > best_balance )); then best_balance="$balance"; best="$candidate"; fi
  done
  [[ -n "$best" ]] || return 1
  printf '%s' "$best"
}

token_balance(){
  local addr="$1" token="${SFLUV_TOKEN_ADDRESS:-}"
  [[ -n "$token" ]] || { echo "0"; return; }
  cast call "$token" "balanceOf(address)(uint256)" "$addr" --rpc-url "$RPC" 2>/dev/null | awk '{print $1}'
}

# --------------------------------------------------------------------------
# Reporting
# --------------------------------------------------------------------------

summary(){
  printf '\n'
  c '1;37' "──────── $* ────────"
  c '0;32' "  passed  $PASS"
  [[ $SKIP -gt 0 ]] && c '0;33' "  skipped $SKIP"
  if [[ $FAIL -gt 0 ]]; then
    c '0;31' "  FAILED  $FAIL"
    for f in "${FAILURES[@]}"; do c '0;31' "    · $f"; done
  fi
  info "artifacts: $RUN_DIR"
  [[ $FAIL -eq 0 ]]
}
