#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing required command '$1'" >&2
    exit 1
  fi
}

read_env_value() {
  local key="$1"
  local file="$2"
  if [[ ! -f "$file" ]]; then
    return 1
  fi
  grep -E "^${key}=" "$file" | tail -n 1 | cut -d'=' -f2- | tr -d '[:space:]'
}

is_deployed_at_block() {
  local contract="$1"
  local rpc_url="$2"
  local block="$3"
  local code
  code="$(cast code "$contract" --rpc-url "$rpc_url" --block "$block" 2>/dev/null || true)"
  [[ -n "$code" && "$code" != "0x" ]]
}

find_deployment_block() {
  local contract="$1"
  local rpc_url="$2"
  local latest="$3"
  local low=0
  local high="$latest"
  local mid

  while (( low < high )); do
    mid=$(( (low + high) / 2 ))
    if is_deployed_at_block "$contract" "$rpc_url" "$mid"; then
      high="$mid"
    else
      low=$(( mid + 1 ))
    fi
  done

  echo "$low"
}

require_cmd cast
require_cmd jq

DEFAULT_ENV_FILE="${ROOT_DIR}/backend/.env"

SFLUV_ADDRESS="${1:-${SFLUV_ADDRESS:-${TOKEN_ID:-}}}"
RPC_URL="${2:-${RPC_URL:-${ETH_RPC_URL:-}}}"
FROM_BLOCK="${3:-${FROM_BLOCK:-}}"

if [[ -z "$SFLUV_ADDRESS" ]]; then
  SFLUV_ADDRESS="$(read_env_value "TOKEN_ID" "$DEFAULT_ENV_FILE" || true)"
fi

if [[ -z "$RPC_URL" ]]; then
  RPC_URL="$(read_env_value "RPC_URL" "$DEFAULT_ENV_FILE" || true)"
fi

if [[ -z "$SFLUV_ADDRESS" || -z "$RPC_URL" ]]; then
  cat <<'EOF' >&2
usage: scripts/check_redeemer_roles.sh [SFLUV_ADDRESS] [RPC_URL] [FROM_BLOCK]

Resolution order:
1) CLI args
2) environment variables SFLUV_ADDRESS/TOKEN_ID and RPC_URL/ETH_RPC_URL
3) backend/.env (TOKEN_ID and RPC_URL)
EOF
  exit 1
fi

LATEST_BLOCK="$(cast block-number --rpc-url "$RPC_URL")"

if [[ -z "$FROM_BLOCK" ]]; then
  if ! is_deployed_at_block "$SFLUV_ADDRESS" "$RPC_URL" "$LATEST_BLOCK"; then
    echo "error: no contract code at ${SFLUV_ADDRESS} on latest block ${LATEST_BLOCK}" >&2
    exit 1
  fi
  FROM_BLOCK="$(find_deployment_block "$SFLUV_ADDRESS" "$RPC_URL" "$LATEST_BLOCK")"
fi

ROLE_HASH="$(cast call "$SFLUV_ADDRESS" "REDEEMER_ROLE()(bytes32)" --rpc-url "$RPC_URL" | tr '[:upper:]' '[:lower:]')"
GRANT_SIG="$(cast sig "RoleGranted(bytes32,address,address)" | tr '[:upper:]' '[:lower:]')"
REVOKE_SIG="$(cast sig "RoleRevoked(bytes32,address,address)" | tr '[:upper:]' '[:lower:]')"

CHUNK_SIZE=9999
current="$FROM_BLOCK"
events_file="$(mktemp)"
active_file="$(mktemp)"
trap 'rm -f "$events_file" "$active_file"' EXIT

echo "SFLUV: ${SFLUV_ADDRESS}"
echo "RPC: ${RPC_URL}"
echo "REDEEMER_ROLE: ${ROLE_HASH}"
echo "Scanning blocks ${FROM_BLOCK}..${LATEST_BLOCK}"
echo

while (( current <= LATEST_BLOCK )); do
  to_block=$(( current + CHUNK_SIZE ))
  if (( to_block > LATEST_BLOCK )); then
    to_block="$LATEST_BLOCK"
  fi

  echo "  - range ${current}..${to_block}"

  granted_json="$(cast logs --json --rpc-url "$RPC_URL" --address "$SFLUV_ADDRESS" \
    "RoleGranted(bytes32,address,address)" "$ROLE_HASH" \
    --from-block "$current" --to-block "$to_block")"
  revoked_json="$(cast logs --json --rpc-url "$RPC_URL" --address "$SFLUV_ADDRESS" \
    "RoleRevoked(bytes32,address,address)" "$ROLE_HASH" \
    --from-block "$current" --to-block "$to_block")"

  while IFS='|' read -r block_hex log_hex topic2; do
    [[ -z "$block_hex" ]] && continue
    block_dec=$((16#${block_hex#0x}))
    log_dec=$((16#${log_hex#0x}))
    account="0x${topic2: -40}"
    account="$(printf '%s' "$account" | tr '[:upper:]' '[:lower:]')"
    printf "%012d|%06d|grant|%s\n" "$block_dec" "$log_dec" "$account" >>"$events_file"
  done < <(jq -r '.[] | "\(.blockNumber)|\(.logIndex)|\(.topics[2])"' <<<"$granted_json")

  while IFS='|' read -r block_hex log_hex topic2; do
    [[ -z "$block_hex" ]] && continue
    block_dec=$((16#${block_hex#0x}))
    log_dec=$((16#${log_hex#0x}))
    account="0x${topic2: -40}"
    account="$(printf '%s' "$account" | tr '[:upper:]' '[:lower:]')"
    printf "%012d|%06d|revoke|%s\n" "$block_dec" "$log_dec" "$account" >>"$events_file"
  done < <(jq -r '.[] | "\(.blockNumber)|\(.logIndex)|\(.topics[2])"' <<<"$revoked_json")

  current=$(( to_block + 1 ))
done

sort -t'|' -k1,1n -k2,2n "$events_file" \
  | awk -F'|' '{ state[$4]=$3 } END { for (account in state) if (state[account] == "grant") print account }' \
  | sort >"$active_file"

if [[ ! -s "$active_file" ]]; then
  echo
  echo "No active REDEEMER_ROLE holders found."
  exit 0
fi

candidate_count="$(wc -l < "$active_file" | tr -d '[:space:]')"
verified_count=0
stale_count=0

echo
echo "Active REDEEMER_ROLE holders:"
while IFS= read -r account; do
  has_role="$(cast call "$SFLUV_ADDRESS" "hasRole(bytes32,address)(bool)" "$ROLE_HASH" "$account" --rpc-url "$RPC_URL" | tr -d '[:space:]')"
  if [[ "$has_role" == "true" ]]; then
    printf "  - %s\n" "$account"
    verified_count=$((verified_count + 1))
  else
    stale_count=$((stale_count + 1))
  fi
done <"$active_file"

echo
echo "Total active: ${verified_count}"
if (( stale_count > 0 )); then
  echo "Warning: ${stale_count} candidate(s) were excluded by on-chain hasRole check."
  echo "Hint: use an earlier FROM_BLOCK for complete historical reconstruction."
fi
echo "Candidates from events: ${candidate_count}"
