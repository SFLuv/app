#!/usr/bin/env bash
#
# Boots the whole SFLUV stack for local development:
#   - chain:    anvil forking Celo (chain id preserved: 42220)
#   - engine:   Citizen Wallet engine (pulled into ./tmp) against the fork,
#               with a localized clone of the SFLUV Celo community config
#   - sponsor:  generates + funds a local paymaster payer key, whitelists it
#               on the forked paymaster (impersonated owner updateSponsor),
#               and seeds the engine's encrypted sponsor row
#   - data:     clones the production databases into local postgres — the ONLY
#               step that touches production, and it is read-only (pg_dump)
#   - ponder:   indexes the local fork into the cloned ponder db
#   - backend:  Go API on :8080 (local community config, no external sends)
#   - frontend: Next.js on :3000
#   - mobile:   Expo (pulled into ./tmp, branch via MOBILE_APP_BRANCH,
#               interactive, foreground last)
#
# Usage:
#   ./dev-up.sh                       # boot everything (Expo in foreground)
#   ./dev-up.sh --no-mobile           # skip Expo (tails service logs instead)
#   ./dev-up.sh --no-frontend         # skip the web app
#   ./dev-up.sh --no-backend          # skip the backend API
#   ./dev-up.sh --no-ponder           # skip the indexer
#   ./dev-up.sh --skip-db-clone       # reuse previously cloned local databases
#   ./dev-up.sh --mobile-branch <b>   # pull the mobile app from branch <b>
#                                     # (overrides MOBILE_APP_BRANCH in .dev.env)
#
# Configuration comes from ./.dev.env (auto-created from .dev.env.example).
# Logs live in tmp/logs/. Ctrl-C stops everything.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"
TMP_DIR="$ROOT/tmp"
LOG_DIR="$TMP_DIR/logs"
DUMP_DIR="$TMP_DIR/dumps"
mkdir -p "$TMP_DIR" "$LOG_DIR" "$DUMP_DIR"

# Flags are parsed BEFORE .dev.env is sourced, so flag values are captured in
# override variables and applied after the source (otherwise the env file's
# values would silently clobber them).
RUN_BACKEND=1 RUN_PONDER=1 RUN_FRONTEND=1 RUN_MOBILE=1
SKIP_DB_CLONE_FLAG=""
MOBILE_BRANCH_OVERRIDE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-mobile)    RUN_MOBILE=0 ;;
    --no-frontend)  RUN_FRONTEND=0 ;;
    --no-backend)   RUN_BACKEND=0 ;;
    --no-ponder)    RUN_PONDER=0 ;;
    --skip-db-clone) SKIP_DB_CLONE_FLAG=1 ;;
    --mobile-branch)
      [[ $# -ge 2 && -n "$2" ]] || { echo "--mobile-branch requires a branch name"; exit 1; }
      MOBILE_BRANCH_OVERRIDE="$2"
      shift ;;
    --mobile-branch=*) MOBILE_BRANCH_OVERRIDE="${1#--mobile-branch=}" ;;
    -h|--help)      sed -n '2,29p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown arg: $1 (try --help)"; exit 1 ;;
  esac
  shift
done

c_blue()  { printf "\033[1;34m%s\033[0m\n" "$1"; }
c_green() { printf "\033[1;32m%s\033[0m\n" "$1"; }
c_yellow(){ printf "\033[1;33m%s\033[0m\n" "$1"; }
c_red()   { printf "\033[1;31m%s\033[0m\n" "$1"; }
die()     { c_red "$1"; exit 1; }

ANVIL_PORT=8545
ANVIL_RPC="http://127.0.0.1:$ANVIL_PORT"
ENGINE_PORT=3001
ENGINE_URL="http://localhost:$ENGINE_PORT"
BACKEND_PORT=8080
PONDER_PORT=42069
FRONTEND_PORT=3000
CELO_CHAIN_ID=42220

# From backend/celo-community-config.json (accounts["42220:..."]).
PAYMASTER_ADDRESS="0x825b77eE3e3AB05c3a342EEE37223494b6c97a55"
ENGINE_DB_NAME="cw_engine"
LOCAL_CONFIG_FILE="$TMP_DIR/local-community-config.json"

PIDS=()
CLEANED=0
STARTED=0   # set once we begin launching things; gates the cleanup port sweep

# kill_tree <SIG> <pid> — signal a process and all of its descendants, leaves
# first. This is what actually reaches the compiled binaries that `go run`,
# `next dev`, and expo spawn as grandchildren (a plain kill of the tracked pid
# leaves those orphaned and still holding their ports).
kill_tree() {
  local sig="$1" pid="$2" child
  for child in $(pgrep -P "$pid" 2>/dev/null); do
    kill_tree "$sig" "$child"
  done
  kill -"$sig" "$pid" 2>/dev/null || true
}

cleanup() {
  [[ "$CLEANED" -eq 1 ]] && return   # idempotent
  CLEANED=1
  trap - INT TERM EXIT               # disarm so we never re-enter
  [[ "$STARTED" -eq 0 ]] && exit 0   # nothing was launched — nothing to stop
  echo
  c_yellow "Shutting down…"
  # Polite TERM to every tracked subtree.
  for pid in ${PIDS[@]+"${PIDS[@]}"}; do
    kill_tree TERM "$pid"
  done
  # Give services a few seconds to exit gracefully (backend shuts down cleanly).
  for _ in 1 2 3; do
    local alive=0 pid
    for pid in ${PIDS[@]+"${PIDS[@]}"}; do
      kill -0 "$pid" 2>/dev/null && alive=1
    done
    [[ "$alive" -eq 0 ]] && break
    sleep 1
  done
  # Force-KILL anything still up, then free the known ports as a backstop.
  for pid in ${PIDS[@]+"${PIDS[@]}"}; do
    kill_tree KILL "$pid"
  done
  local p pids
  for p in "$ANVIL_PORT" "$ENGINE_PORT" "$PONDER_PORT" "$BACKEND_PORT" "$FRONTEND_PORT" 8081; do
    pids=$(lsof -ti tcp:"$p" 2>/dev/null || true)
    [[ -n "$pids" ]] && kill -9 $pids 2>/dev/null || true
  done
  c_green "Done. (tmp/ preserved — cloned DBs, payer key, and logs survive reruns)"
  exit 0
}
trap cleanup INT TERM EXIT

wait_for() { # wait_for <url> <name> <tries> [pid] [logfile] — up = HTTP responds.
  # When <pid> is given, stop early the moment that process exits instead of
  # polling a dead process for the full timeout.
  local url="$1" name="$2" tries="${3:-40}" pid="${4:-}" logfile="${5:-}"
  local i
  for ((i = 0; i < tries; i++)); do
    if curl -sk -o /dev/null "$url" 2>/dev/null; then
      c_green "  $name is up ($url)"
      return 0
    fi
    if [[ -n "$pid" ]] && ! kill -0 "$pid" 2>/dev/null; then
      c_red "  $name exited before becoming ready"
      [[ -n "$logfile" && -f "$logfile" ]] && tail -n 20 "$logfile"
      return 2
    fi
    sleep 1
  done
  c_red "  $name did not become ready at $url"
  [[ -n "$logfile" && -f "$logfile" ]] && tail -n 20 "$logfile"
  return 1
}

free_port() { # free_port <port> <name> — force-free a port, then WAIT until it
              # is actually released so the new bind can't race a dying process.
  local port="$1" name="$2" pids i announced=0
  for i in $(seq 1 20); do
    pids=$(lsof -ti tcp:"$port" 2>/dev/null || true)
    [[ -z "$pids" ]] && return 0
    if [[ "$announced" -eq 0 ]]; then
      c_yellow "  port $port in use — stopping existing $name"
      announced=1
    fi
    kill -9 $pids 2>/dev/null || true
    sleep 0.25
  done
  pids=$(lsof -ti tcp:"$port" 2>/dev/null || true)
  [[ -n "$pids" ]] && c_red "  could not free port $port (still held by: $pids)"
  return 0
}

start_bg() { # start_bg <name> <workdir> <logfile> <cmd...> — tracked background service.
  local name="$1" workdir="$2" logfile="$3"
  shift 3
  [[ -d "$workdir" ]] || die "  cannot start $name: missing directory $workdir"
  ( cd "$workdir" && "$@" >"$logfile" 2>&1 ) &
  PIDS+=($!)
  c_yellow "  $name logs: ${logfile#"$ROOT"/}"
}

export FOUNDRY_DISABLE_NIGHTLY_WARNING=1

for tool in git go node npm psql createdb dropdb pg_dump pg_restore anvil cast python3 curl lsof pgrep; do
  command -v "$tool" >/dev/null || die "missing required tool: $tool"
done

# ----------------------------------------------------------------------------
# 0. Boot config (.dev.env — auto-created; prod DB creds + Privy are hand-set)
# ----------------------------------------------------------------------------
if [[ ! -f "$ROOT/.dev.env" ]]; then
  cp "$ROOT/.dev.env.example" "$ROOT/.dev.env"
  c_yellow "created .dev.env from .dev.env.example — fill in PROD_DB_* and Privy values, then re-run"
  exit 1
fi
set -a
# shellcheck disable=SC1091
source "$ROOT/.dev.env"
set +a

# CLI flags outrank .dev.env.
[[ -n "$SKIP_DB_CLONE_FLAG" ]] && SKIP_DB_CLONE=1
[[ -n "$MOBILE_BRANCH_OVERRIDE" ]] && MOBILE_APP_BRANCH="$MOBILE_BRANCH_OVERRIDE"

LOCAL_DB_USER="${LOCAL_DB_USER:-postgres}"
LOCAL_DB_HOST_PORT="${LOCAL_DB_HOST_PORT:-localhost:5432}"
LOCAL_DB_HOST="${LOCAL_DB_HOST_PORT%%:*}"
LOCAL_DB_PORT="${LOCAL_DB_HOST_PORT##*:}"
PSQL=(psql -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" -v ON_ERROR_STOP=1 -qAt)

"${PSQL[@]}" -d postgres -c "SELECT 1" >/dev/null 2>&1 \
  || die "cannot reach local postgres at $LOCAL_DB_HOST_PORT as $LOCAL_DB_USER"

STARTED=1

# ----------------------------------------------------------------------------
# 1. Citizen Wallet engine source (into gitignored ./tmp)
# ----------------------------------------------------------------------------
ENGINE_REPO="${ENGINE_REPO:-https://github.com/citizenwallet/engine.git}"
ENGINE_BRANCH="${ENGINE_BRANCH:-main}"
ENGINE_DIR="$TMP_DIR/engine"

c_blue "[1/9] Citizen Wallet engine source ($ENGINE_BRANCH)"
if [[ -d "$ENGINE_DIR/.git" ]]; then
  ( git -C "$ENGINE_DIR" fetch origin "$ENGINE_BRANCH" \
      && git -C "$ENGINE_DIR" checkout "$ENGINE_BRANCH" \
      && git -C "$ENGINE_DIR" reset --hard "origin/$ENGINE_BRANCH" ) >/dev/null 2>&1 \
    || die "  failed to update engine checkout in tmp/engine"
  c_green "  engine updated (tmp/engine @ $ENGINE_BRANCH)"
else
  git clone --depth 1 --branch "$ENGINE_BRANCH" "$ENGINE_REPO" "$ENGINE_DIR" >/dev/null 2>&1 \
    || die "  failed to clone $ENGINE_REPO"
  c_green "  engine cloned (tmp/engine @ $ENGINE_BRANCH)"
fi

# ----------------------------------------------------------------------------
# 2. Chain (anvil fork of Celo, chain id preserved)
# ----------------------------------------------------------------------------
c_blue "[2/9] Chain (anvil fork of Celo, :$ANVIL_PORT)"
free_port "$ANVIL_PORT" anvil
ANVIL_ARGS=(--fork-url "${CELO_FORK_RPC_URL:-https://forno.celo.org}" --chain-id "$CELO_CHAIN_ID" --host 127.0.0.1 --port "$ANVIL_PORT")
[[ -n "${ANVIL_FORK_BLOCK:-}" ]] && ANVIL_ARGS+=(--fork-block-number "$ANVIL_FORK_BLOCK")
anvil "${ANVIL_ARGS[@]}" >"$LOG_DIR/anvil.log" 2>&1 &
PIDS+=($!)
c_yellow "  anvil logs: tmp/logs/anvil.log"
for ((i = 0; i < 120; i++)); do
  cast block-number --rpc-url "$ANVIL_RPC" >/dev/null 2>&1 && break
  [[ $i -eq 119 ]] && { c_red "  anvil did not come up"; tail -n 20 "$LOG_DIR/anvil.log"; exit 1; }
  sleep 1
done
c_green "  chain is up ($ANVIL_RPC, chain id $CELO_CHAIN_ID, forked from Celo)"

# ----------------------------------------------------------------------------
# 3. Community config + engine service
# ----------------------------------------------------------------------------
c_blue "[3/9] Community config + engine (:$ENGINE_PORT)"
ENGINE_URL_LOCAL="$ENGINE_URL" ENGINE_WS_LOCAL="ws://localhost:$ENGINE_PORT" ANVIL_RPC_LOCAL="$ANVIL_RPC" \
python3 - "$ROOT/backend/celo-community-config.json" "$LOCAL_CONFIG_FILE" <<'PYEOF' || die "  failed to localize community config"
import json, os, sys

src, dst = sys.argv[1], sys.argv[2]
with open(src) as f:
    envelope = json.load(f)

cfg = envelope.get("json", envelope)
engine = os.environ["ENGINE_URL_LOCAL"]
engine_ws = os.environ["ENGINE_WS_LOCAL"]
anvil = os.environ["ANVIL_RPC_LOCAL"]

# Point every chain node at the local engine; add the anvil read RPC as the
# extras.rpc_url the backend/frontend prefer for read methods.
for chain in cfg.get("chains", {}).values():
    node = chain.setdefault("node", {})
    node["url"] = engine
    node["ws_url"] = engine_ws
extras = cfg.setdefault("extras", {})
extras["rpc_url"] = anvil

with open(dst, "w") as f:
    json.dump(envelope, f, indent=2)
PYEOF
c_green "  localized config → tmp/local-community-config.json (chains → engine, extras.rpc_url → anvil)"

if ! "${PSQL[@]}" -d postgres -c "SELECT 1 FROM pg_database WHERE datname = '$ENGINE_DB_NAME'" 2>/dev/null | grep -q 1; then
  createdb -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" "$ENGINE_DB_NAME" \
    || die "  failed to create $ENGINE_DB_NAME database"
  c_green "  created local engine database ($ENGINE_DB_NAME)"
fi

# Persist a DB secret across reruns (encrypts sponsor private keys at rest).
# The engine parses it with crypto.HexToECDSA, so it MUST be 64 hex chars.
ENGINE_SECRET_FILE="$TMP_DIR/engine-db-secret"
if [[ ! -f "$ENGINE_SECRET_FILE" ]] || [[ "$(tr -d '[:space:]' < "$ENGINE_SECRET_FILE" | wc -c | tr -d ' ')" != "64" ]]; then
  python3 -c "import secrets; print(secrets.token_hex(32))" > "$ENGINE_SECRET_FILE"
fi
ENGINE_DB_SECRET="$(tr -d '[:space:]' < "$ENGINE_SECRET_FILE")"

ENGINE_ENV_FILE="$TMP_DIR/engine.env"
cat > "$ENGINE_ENV_FILE" <<EOF
CHAIN_NAME=celo
RPC_URL=$ANVIL_RPC
RPC_WS_URL=ws://127.0.0.1:$ANVIL_PORT
DB_USER=$LOCAL_DB_USER
DB_PASSWORD=
DB_NAME=$ENGINE_DB_NAME
DB_HOST=$LOCAL_DB_HOST
DB_READER_HOST=$LOCAL_DB_HOST
DB_PORT=$LOCAL_DB_PORT
DB_SECRET=$ENGINE_DB_SECRET
EOF

( cd "$ENGINE_DIR" && go build -o "$TMP_DIR/engine-bin" ./cmd ) >"$LOG_DIR/engine-build.log" 2>&1 \
  || { c_red "  engine build failed:"; tail -n 20 "$LOG_DIR/engine-build.log"; exit 1; }
free_port "$ENGINE_PORT" engine
# shellcheck disable=SC2086
start_bg engine "$ENGINE_DIR" "$LOG_DIR/engine.log" "$TMP_DIR/engine-bin" -env "$ENGINE_ENV_FILE" -port "$ENGINE_PORT" ${ENGINE_EXTRA_FLAGS:-}
wait_for "$ENGINE_URL/v1/rpc" "engine" 30 "${PIDS[${#PIDS[@]}-1]}" "$LOG_DIR/engine.log" || exit 1

# ----------------------------------------------------------------------------
# 4. Paymaster sponsor (local payer key: fund, whitelist, seed)
# ----------------------------------------------------------------------------
c_blue "[4/9] Paymaster sponsor (prank + whitelist)"

# Persist the payer key across reruns.
PAYER_KEY_FILE="$TMP_DIR/payer.key"
if [[ ! -f "$PAYER_KEY_FILE" ]]; then
  OUT="$(cast wallet new)"
  PAYER_ADDRESS="$(printf '%s' "$OUT" | grep -oE '0x[a-fA-F0-9]{40}' | head -1)"
  PAYER_PK="$(printf '%s' "$OUT" | grep -oE '0x[a-fA-F0-9]{64}' | head -1)"
  printf '%s\n%s\n' "$PAYER_ADDRESS" "$PAYER_PK" > "$PAYER_KEY_FILE"
fi
PAYER_ADDRESS="$(sed -n 1p "$PAYER_KEY_FILE")"
PAYER_PK="$(sed -n 2p "$PAYER_KEY_FILE")"
[[ "$PAYER_ADDRESS" =~ ^0x[a-fA-F0-9]{40}$ ]] || die "  could not derive payer address (see tmp/payer.key)"

# Fund the payer with native CELO for gas (1000 ether).
FUND_WEI="0x3635C9ADC5DEA00000"
cast rpc anvil_setBalance "$PAYER_ADDRESS" "$FUND_WEI" --rpc-url "$ANVIL_RPC" >/dev/null \
  || die "  failed to fund payer"
c_green "  payer $PAYER_ADDRESS funded (1000 CELO)"

# Whitelist: impersonate the paymaster owner and call updateSponsor(payer).
PM_OWNER="$(cast call "$PAYMASTER_ADDRESS" 'owner()(address)' --rpc-url "$ANVIL_RPC")" \
  || die "  failed to read paymaster owner"
cast rpc anvil_impersonateAccount "$PM_OWNER" --rpc-url "$ANVIL_RPC" >/dev/null
cast rpc anvil_setBalance "$PM_OWNER" "$FUND_WEI" --rpc-url "$ANVIL_RPC" >/dev/null
cast send "$PAYMASTER_ADDRESS" 'updateSponsor(address)' "$PAYER_ADDRESS" \
  --from "$PM_OWNER" --unlocked --rpc-url "$ANVIL_RPC" >/dev/null \
  || die "  updateSponsor reverted (owner: $PM_OWNER)"
NEW_SPONSOR="$(cast call "$PAYMASTER_ADDRESS" 'sponsor()(address)' --rpc-url "$ANVIL_RPC")"
[[ "$(printf '%s' "$NEW_SPONSOR" | tr '[:upper:]' '[:lower:]')" == "$(printf '%s' "$PAYER_ADDRESS" | tr '[:upper:]' '[:lower:]')" ]] \
  || die "  paymaster sponsor() is $NEW_SPONSOR, expected $PAYER_ADDRESS"
c_green "  paymaster sponsor updated on-chain (owner $PM_OWNER impersonated)"

# Seed the engine's sponsor table. The engine's key parser rejects a 0x prefix,
# and cmd/encrypt logs its result to stderr as "key: <value>".
PAYER_PK_NOPREFIX="${PAYER_PK#0x}"
ENC_PK="$(cd "$ENGINE_DIR" && go run ./cmd/encrypt -s "$ENGINE_DB_SECRET" -v "$PAYER_PK_NOPREFIX" 2>&1 | sed -n 's/.*key: //p' | tail -1 | tr -d '[:space:]')"
[[ -n "$ENC_PK" ]] || die "  engine cmd/encrypt produced no output"

SPONSOR_TABLE=""
for _ in $(seq 1 15); do
  SPONSOR_TABLE="$("${PSQL[@]}" -d "$ENGINE_DB_NAME" -c "SELECT tablename FROM pg_tables WHERE tablename LIKE 't_sponsors%' LIMIT 1" 2>/dev/null || true)"
  [[ -n "$SPONSOR_TABLE" ]] && break
  sleep 2
done
[[ -n "$SPONSOR_TABLE" ]] || die "  engine sponsors table never appeared in $ENGINE_DB_NAME — see tmp/logs/engine.log"
"${PSQL[@]}" -d "$ENGINE_DB_NAME" -c "DELETE FROM $SPONSOR_TABLE WHERE lower(contract) = lower('$PAYMASTER_ADDRESS')" >/dev/null
"${PSQL[@]}" -d "$ENGINE_DB_NAME" -c "INSERT INTO $SPONSOR_TABLE (contract, pk, created_at, updated_at) VALUES ('$PAYMASTER_ADDRESS', '$ENC_PK', now(), now())" >/dev/null \
  || die "  failed to seed sponsor row"
c_green "  engine sponsor row seeded ($SPONSOR_TABLE)"

# ----------------------------------------------------------------------------
# 5. Databases (clone production — READ-ONLY against prod)
# ----------------------------------------------------------------------------
if [[ "${SKIP_DB_CLONE:-0}" == "1" ]]; then
  c_blue "[5/9] Databases (skipped — reusing existing local clones)"
else
  c_blue "[5/9] Databases (cloning production: ${PROD_DB_NAMES:-app bot ponder})"
  [[ -n "${PROD_DB_HOST_PORT:-}" && -n "${PROD_DB_USER:-}" ]] \
    || die "  PROD_DB_HOST_PORT / PROD_DB_USER must be set in .dev.env (or use --skip-db-clone)"
  PROD_HOST="${PROD_DB_HOST_PORT%%:*}"
  PROD_PORT="${PROD_DB_HOST_PORT##*:}"
  for dbname in ${PROD_DB_NAMES:-app bot ponder}; do
    c_yellow "  cloning $dbname…"
    PGPASSWORD="${PROD_DB_PASSWORD:-}" pg_dump -h "$PROD_HOST" -p "$PROD_PORT" -U "$PROD_DB_USER" \
      -d "$dbname" -Fc --no-owner --no-acl -f "$DUMP_DIR/$dbname.dump" \
      || die "  pg_dump of $dbname failed"
    dropdb  -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" --if-exists "$dbname"
    createdb -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" "$dbname"
    pg_restore -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" \
      -d "$dbname" --no-owner --no-acl "$DUMP_DIR/$dbname.dump" 2>/dev/null \
      || c_yellow "  pg_restore for $dbname reported errors (often ignorable ownership noise)"
    c_green "  $dbname cloned"
  done
fi

# Sanitize cloned data that references production: the ponder_hooks table holds
# production callback URLs which local ponder would POST to during backfill.
if "${PSQL[@]}" -d ponder -c "SELECT 1 FROM pg_tables WHERE tablename = 'ponder_hooks'" 2>/dev/null | grep -q 1; then
  "${PSQL[@]}" -d ponder -c "UPDATE ponder_hooks SET url = 'http://localhost:$BACKEND_PORT/ponder/callback'" >/dev/null
  c_green "  ponder_hooks repointed at the local backend (no prod callbacks)"
fi

# ----------------------------------------------------------------------------
# 6. Ponder (indexes the fork into the cloned db)
# ----------------------------------------------------------------------------
if [[ "$RUN_PONDER" -eq 1 ]]; then
  c_blue "[6/9] Ponder (:$PONDER_PORT)"
  free_port "$PONDER_PORT" ponder
  if [[ ! -d "$ROOT/ponder/node_modules" ]]; then
    c_yellow "  installing ponder deps…"
    ( cd "$ROOT/ponder" && npm install --no-audit --no-fund >"$LOG_DIR/ponder-install.log" 2>&1 )
  fi
  PONDER_ENV=(
    "DATABASE_URL=postgresql://$LOCAL_DB_USER:@$LOCAL_DB_HOST_PORT/ponder"
    "PONDER_RPC_URL_1=$ANVIL_RPC"
    "ADMIN_KEY=${DEV_PONDER_KEY:-local-dev-ponder-key}"
  )
  [[ -n "${PONDER_START_BLOCK:-}" ]] && PONDER_ENV+=("PONDER_START_BLOCK=$PONDER_START_BLOCK")
  start_bg ponder "$ROOT/ponder" "$LOG_DIR/ponder.log" env "${PONDER_ENV[@]}" npm run dev
  c_yellow "  note: ponder's chain id is hardcoded to $CELO_CHAIN_ID (anvil matches); if it re-syncs, it reindexes from the configured start block — expected on a fresh fork"
else
  c_blue "[6/9] Ponder (skipped)"
fi

# ----------------------------------------------------------------------------
# 7. Backend (local community config; no external sends)
# ----------------------------------------------------------------------------
if [[ "$RUN_BACKEND" -eq 1 ]]; then
  c_blue "[7/9] Backend (:$BACKEND_PORT)"
  free_port "$BACKEND_PORT" backend
  BACKEND_ENV_FILE="$TMP_DIR/backend.dev.env"
  cat > "$BACKEND_ENV_FILE" <<EOF
# Generated by dev-up.sh — local dev only. Nothing here points at production.
PORT=$BACKEND_PORT
IN_PRODUCTION=false
APP_BASE_URL=http://localhost:$FRONTEND_PORT

DB_USER=$LOCAL_DB_USER
DB_PASSWORD=
DB_BASE_URL=$LOCAL_DB_HOST_PORT
APP_DB_NAME=app
BOT_DB_NAME=bot
PONDER_DB_NAME=ponder

# Local chain + local engine via the localized community config.
CLIENT_CONFIG_LOCAL_ONLY=true
CLIENT_CONFIG_FALLBACK_PATH=$LOCAL_CONFIG_FILE
RPC_URL=$ANVIL_RPC
ENGINE_RPC_URL=$ENGINE_URL
ENGINE_WS_URL=ws://localhost:$ENGINE_PORT

PRIVY_APP_ID=${PRIVY_APP_ID:-}
PRIVY_VKEY="${PRIVY_VKEY:-}"

ADMIN_KEY=${DEV_ADMIN_KEY:-local-dev-admin-key}
PONDER_SERVER_BASE_URL=http://localhost:$PONDER_PORT
PONDER_KEY=${DEV_PONDER_KEY:-local-dev-ponder-key}
PONDER_CALLBACK_URL=http://localhost:$BACKEND_PORT/ponder/callback

# Hard guarantee: no external notifications from the dev environment.
NOTIFICATION_TEST_MODE=true
EOF
  start_bg backend "$ROOT/backend" "$LOG_DIR/backend.log" env ENV_FILE="$BACKEND_ENV_FILE" go run ./cmd/server
  wait_for "http://localhost:$BACKEND_PORT/config" "backend" 180 "${PIDS[${#PIDS[@]}-1]}" "$LOG_DIR/backend.log" || exit 1
else
  c_blue "[7/9] Backend (skipped)"
fi

# ----------------------------------------------------------------------------
# 8. Frontend
# ----------------------------------------------------------------------------
if [[ "$RUN_FRONTEND" -eq 1 ]]; then
  c_blue "[8/9] Frontend (:$FRONTEND_PORT)"
  free_port "$FRONTEND_PORT" frontend
  if [[ ! -d "$ROOT/frontend/node_modules" ]]; then
    c_yellow "  installing frontend deps…"
    ( cd "$ROOT/frontend" && npm install --no-audit --no-fund >"$LOG_DIR/frontend-install.log" 2>&1 )
  fi
  FRONTEND_ENV=(
    "IN_PRODUCTION=false"
    "NEXT_PUBLIC_BACKEND_URL=http://localhost:$BACKEND_PORT"
    "NEXT_PUBLIC_FRONTEND_URL=http://localhost:$FRONTEND_PORT"
    "NEXT_PUBLIC_CHAIN_RPC_URL=$ANVIL_RPC"
    "NEXT_PUBLIC_ENGINE_URL=$ENGINE_URL"
    "NEXT_PUBLIC_PRIVY_APP_ID=${NEXT_PUBLIC_PRIVY_APP_ID:-}"
    "NEXT_PUBLIC_GOOGLE_MAPS_API_KEY=${NEXT_PUBLIC_GOOGLE_MAPS_API_KEY:-}"
    "NEXT_PUBLIC_MAP_ID=${NEXT_PUBLIC_MAP_ID:-}"
  )
  start_bg frontend "$ROOT/frontend" "$LOG_DIR/frontend.log" env "${FRONTEND_ENV[@]}" npm run dev
  wait_for "https://localhost:$FRONTEND_PORT" "frontend" 90 "${PIDS[${#PIDS[@]}-1]}" "$LOG_DIR/frontend.log" \
    || c_yellow "  frontend still starting — check tmp/logs/frontend.log"
else
  c_blue "[8/9] Frontend (skipped)"
fi

echo
c_green "Stack is up:"
echo "  Chain      $ANVIL_RPC (chain id $CELO_CHAIN_ID, Celo fork)"
echo "  Engine     $ENGINE_URL"
echo "  Paymaster  $PAYMASTER_ADDRESS (sponsor: $PAYER_ADDRESS)"
[[ "$RUN_PONDER" -eq 1 ]]   && echo "  Ponder     http://localhost:$PONDER_PORT"
[[ "$RUN_BACKEND" -eq 1 ]]  && echo "  Backend    http://localhost:$BACKEND_PORT"
[[ "$RUN_FRONTEND" -eq 1 ]] && echo "  Frontend   https://localhost:$FRONTEND_PORT"
echo "  Config     tmp/local-community-config.json"
echo "  Logs       tmp/logs/"
echo

# ----------------------------------------------------------------------------
# 9. Mobile (interactive) — foreground so Expo's menu/QR shows.
# ----------------------------------------------------------------------------
if [[ "$RUN_MOBILE" -eq 1 ]]; then
  MOBILE_APP_REPO="${MOBILE_APP_REPO:-https://github.com/SFLuv/mobile-app.git}"
  MOBILE_APP_BRANCH="${MOBILE_APP_BRANCH:-main}"
  MOBILE_DIR="$TMP_DIR/mobile-app"

  c_blue "[9/9] Mobile (Expo @ $MOBILE_APP_BRANCH) — press q to quit Expo, then Ctrl-C to stop the rest"
  if [[ -d "$MOBILE_DIR/.git" ]]; then
    ( git -C "$MOBILE_DIR" fetch origin "$MOBILE_APP_BRANCH" \
        && git -C "$MOBILE_DIR" checkout "$MOBILE_APP_BRANCH" \
        && git -C "$MOBILE_DIR" reset --hard "origin/$MOBILE_APP_BRANCH" ) >/dev/null 2>&1 \
      || die "  failed to update mobile checkout in tmp/mobile-app"
  else
    git clone --branch "$MOBILE_APP_BRANCH" "$MOBILE_APP_REPO" "$MOBILE_DIR" >/dev/null 2>&1 \
      || die "  failed to clone $MOBILE_APP_REPO"
  fi

  MOBILE_BACKEND_HOST="${DEV_LAN_IP:-localhost}"
  cat > "$MOBILE_DIR/mobile/.env" <<EOF
# Generated by dev-up.sh — local dev only.
EXPO_PUBLIC_APP_BACKEND_URL=http://$MOBILE_BACKEND_HOST:$BACKEND_PORT
EXPO_PUBLIC_APP_ORIGIN=http://localhost:$FRONTEND_PORT
EXPO_PUBLIC_PRIVY_APP_ID=${EXPO_PUBLIC_PRIVY_APP_ID:-}
EXPO_PUBLIC_PRIVY_CLIENT_ID=${EXPO_PUBLIC_PRIVY_CLIENT_ID:-}
EXPO_PUBLIC_GOOGLE_MAPS_API_KEY=${EXPO_PUBLIC_GOOGLE_MAPS_API_KEY:-}
EXPO_PUBLIC_MAP_ID=${EXPO_PUBLIC_MAP_ID:-}
EXPO_PUBLIC_EAS_PROJECT_ID=${EXPO_PUBLIC_EAS_PROJECT_ID:-}
EOF
  if [[ ! -d "$MOBILE_DIR/mobile/node_modules" ]]; then
    c_yellow "  installing mobile deps…"
    ( cd "$MOBILE_DIR/mobile" && npm install --no-audit --no-fund >"$LOG_DIR/mobile-install.log" 2>&1 )
  fi
  ( cd "$MOBILE_DIR/mobile" && npm run start )
else
  c_blue "[9/9] Running (no mobile). Press Ctrl-C to stop."
  TAIL_LOGS=()
  [[ "$RUN_BACKEND" -eq 1 ]]  && TAIL_LOGS+=("$LOG_DIR/backend.log")
  [[ "$RUN_PONDER" -eq 1 ]]   && TAIL_LOGS+=("$LOG_DIR/ponder.log")
  [[ "$RUN_FRONTEND" -eq 1 ]] && TAIL_LOGS+=("$LOG_DIR/frontend.log")
  TAIL_LOGS+=("$LOG_DIR/engine.log")
  tail -f ${TAIL_LOGS[@]+"${TAIL_LOGS[@]}"}
fi
