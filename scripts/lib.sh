#!/usr/bin/env bash
# Shared plumbing for the convenience scripts in this folder.
#
# Source this, do not execute it. Everything here assumes the local dev stack
# from ./scripts/dev-up/dev-up.sh is already running.
#
# The one rule this file enforces above all others: a script must be
# physically incapable of touching production. See require_local_stack.

set -uo pipefail

SFLUV_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

API="${SFLUV_API:-http://localhost:8080}"
# HTTPS, and self-signed. dev-up serves the frontend from the pair in
# frontend/certificates/, so plain http gets connection-refused and every curl
# needs -k. Getting this wrong makes a running frontend look like a dead one.
WEB="${SFLUV_WEB:-https://localhost:3000}"
SITE="${SFLUV_SITE:-http://localhost:3002}"
RPC="${SFLUV_RPC:-http://127.0.0.1:8545}"
ADMIN_KEY="${DEV_ADMIN_KEY:-local-dev-admin-key}"

# The HTTP status of the last admin_api call.
#
# It lives in a file rather than a variable because callers capture bodies
# with out="$(admin_api GET /thing)" — a command substitution, which runs in a
# SUBSHELL. A variable set inside it never reaches the caller, so every status
# check silently read empty. A file write survives the subshell.
STATUS_FILE="$(mktemp -t sfluv-status)"
trap 'rm -f "$STATUS_FILE"' EXIT
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

# die stops the script. Reserved for conditions where continuing would be
# unsafe or meaningless.
die(){ c '1;31' "FATAL: $*"; exit 1; }

# discover_stack fills in the chain addresses from the running stack instead of
# asking a human to export them. Anything already exported wins, so a script
# can still be pointed somewhere deliberately.
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
  # The admin key, for the same reason and from the same file.
  #
  # Without this ADMIN_KEY falls back to the literal "local-dev-admin-key",
  # which stopped matching the moment dev-up regenerated the backend env. Every
  # admin_api call then 403s and returns HTML, and the script dies on a jq
  # parse error that says nothing about authentication.
  if [[ -z "${DEV_ADMIN_KEY:-}" && -f "$SFLUV_ROOT/tmp/backend.dev.env" ]]; then
    DEV_ADMIN_KEY="$(grep -E '^ADMIN_KEY=' "$SFLUV_ROOT/tmp/backend.dev.env" \
      | head -1 | cut -d= -f2- | tr -d "\"' [:space:]")"
    export DEV_ADMIN_KEY
    ADMIN_KEY="$DEV_ADMIN_KEY"
  fi
}

# --------------------------------------------------------------------------
# Safety
# --------------------------------------------------------------------------

# require_local_stack refuses to proceed against anything that is not the local
# dev stack. It fails closed: every check must actively prove locality, and an
# unreachable or unexpected answer stops the script.
#
# This exists because these scripts create events, approve merchants, redeem
# codes and move tokens. Pointed at production by a stray environment variable
# they would do all of that for real.
require_local_stack(){
  case "$API" in
    http://localhost:*|http://127.0.0.1:*) ;;
    *) die "API is $API — these scripts only ever run against localhost" ;;
  esac
  case "$RPC" in
    http://127.0.0.1:*|http://localhost:*) ;;
    *) die "RPC is $RPC — refusing to run against a non-local chain" ;;
  esac

  local health
  health="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$API/config" || echo 000)"
  [[ "$health" == "200" ]] || die "backend not answering on $API (got HTTP $health). Start it with ./scripts/dev-up/dev-up.sh"

  local chain
  chain="$(cast chain-id --rpc-url "$RPC" 2>/dev/null || echo "")"
  [[ -n "$chain" ]] || die "no chain on $RPC — anvil is not running"
  discover_stack
  info "backend $API · chain id $chain"
  [[ -n "${SFLUV_TOKEN_ADDRESS:-}" ]] && info "token ${SFLUV_TOKEN_ADDRESS} · faucet ${SFLUV_FAUCET_ADDRESS:-unknown}"
}

# --------------------------------------------------------------------------
# HTTP
# --------------------------------------------------------------------------

# admin_api METHOD PATH [body] — the X-Admin-Key path, for arranging state.
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

# --------------------------------------------------------------------------
# Chain
# --------------------------------------------------------------------------

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
  [[ $FAIL -eq 0 ]]
}
