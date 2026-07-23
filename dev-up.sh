#!/usr/bin/env bash
# =============================================================================
# dev-up.sh — boot a complete local SFLUV development environment.
#
#   1. Pull the Citizen Wallet engine into ./tmp (gitignored)
#   2. Fork Celo into a local anvil instance (chain id preserved: 42220)
#   3. Run the CW engine against anvil with a localized clone of the SFLUV
#      Celo community config
#   4. Generate a local paymaster payer (sponsor) key, fund it, and whitelist
#      it on the forked paymaster via an impersonated owner call
#   5. Clone the production databases (app/bot/ponder) into local postgres
#      — the ONLY step that touches production, and it is read-only
#   6. Start ponder against the cloned db + local chain
#   7. Start the backend using the local community config (never the remote)
#   8. Start the frontend
#   9. Pull + start the mobile app with Expo (branch via MOBILE_APP_BRANCH)
#
# All configuration comes from ./.dev.env (see .dev.env.example).
# Logs:  tmp/logs/<service>.log      PIDs: tmp/pids/<service>.pid
# Stop everything with ./dev-down.sh
# =============================================================================
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP_DIR="$ROOT_DIR/tmp"
LOG_DIR="$TMP_DIR/logs"
PID_DIR="$TMP_DIR/pids"
DUMP_DIR="$TMP_DIR/dumps"
DEV_ENV_FILE="$ROOT_DIR/.dev.env"

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

log()  { printf '\n\033[1;36m[dev-up]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[dev-up][warn]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[dev-up][fail]\033[0m %s\n' "$*" >&2; exit 1; }

# --- preflight ---------------------------------------------------------------
[[ -f "$DEV_ENV_FILE" ]] || die "missing $DEV_ENV_FILE — copy .dev.env.example to .dev.env and fill it in"
set -a
# shellcheck disable=SC1090
source "$DEV_ENV_FILE"
set +a

for tool in git go node npm psql createdb dropdb pg_dump pg_restore anvil cast python3 curl lsof; do
  command -v "$tool" >/dev/null 2>&1 || die "required tool not found: $tool"
done

mkdir -p "$TMP_DIR" "$LOG_DIR" "$PID_DIR" "$DUMP_DIR"

LOCAL_DB_USER="${LOCAL_DB_USER:-postgres}"
LOCAL_DB_HOST_PORT="${LOCAL_DB_HOST_PORT:-localhost:5432}"
LOCAL_DB_HOST="${LOCAL_DB_HOST_PORT%%:*}"
LOCAL_DB_PORT="${LOCAL_DB_HOST_PORT##*:}"
PSQL=(psql -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" -v ON_ERROR_STOP=1 -qAt)

"${PSQL[@]}" -d postgres -c "SELECT 1" >/dev/null 2>&1 \
  || die "cannot reach local postgres at $LOCAL_DB_HOST_PORT as $LOCAL_DB_USER"

kill_port() {
  local port="$1"
  local pids
  pids="$(lsof -ti ":$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    warn "killing existing listener(s) on :$port"
    kill $pids 2>/dev/null || true
    sleep 1
  fi
}

start_service() {
  # start_service <name> <workdir> <command...>
  # The async job is a plain simple command in its own process group (set -m,
  # pgid == pid) so dev-down.sh can kill the whole service tree; </dev/null is
  # required under job control so backgrounded npm/expo don't stop on SIGTTIN.
  local name="$1" workdir="$2"
  shift 2
  log "starting $name → $LOG_DIR/$name.log"
  (
    cd "$workdir" || exit 1
    set -m
    nohup "$@" < /dev/null > "$LOG_DIR/$name.log" 2>&1 &
    echo $! > "$PID_DIR/$name.pid"
  ) || die "failed to start $name (bad workdir $workdir?)"
}

wait_for() {
  # wait_for <description> <timeout_s> <command...>
  local desc="$1" timeout="$2"
  shift 2
  local waited=0
  until "$@" >/dev/null 2>&1; do
    (( waited >= timeout )) && die "timed out waiting for $desc (${timeout}s)"
    sleep 2
    waited=$((waited + 2))
  done
  log "$desc is ready"
}

# =============================================================================
# 1. Pull the Citizen Wallet engine into ./tmp
# =============================================================================
ENGINE_REPO="${ENGINE_REPO:-https://github.com/citizenwallet/engine.git}"
ENGINE_BRANCH="${ENGINE_BRANCH:-main}"
ENGINE_DIR="$TMP_DIR/engine"

log "step 1/9: syncing citizenwallet engine ($ENGINE_BRANCH)"
if [[ -d "$ENGINE_DIR/.git" ]]; then
  git -C "$ENGINE_DIR" fetch origin "$ENGINE_BRANCH"
  git -C "$ENGINE_DIR" checkout "$ENGINE_BRANCH"
  git -C "$ENGINE_DIR" reset --hard "origin/$ENGINE_BRANCH"
else
  git clone --depth 1 --branch "$ENGINE_BRANCH" "$ENGINE_REPO" "$ENGINE_DIR"
fi

# =============================================================================
# 2. Fork Celo into a local anvil instance
# =============================================================================
log "step 2/9: starting anvil fork of Celo (chain id $CELO_CHAIN_ID)"
kill_port "$ANVIL_PORT"
ANVIL_ARGS=(--fork-url "${CELO_FORK_RPC_URL:-https://forno.celo.org}" --chain-id "$CELO_CHAIN_ID" --host 127.0.0.1 --port "$ANVIL_PORT")
if [[ -n "${ANVIL_FORK_BLOCK:-}" ]]; then
  ANVIL_ARGS+=(--fork-block-number "$ANVIL_FORK_BLOCK")
fi
start_service anvil "$ROOT_DIR" anvil "${ANVIL_ARGS[@]}"
wait_for "anvil" 120 cast block-number --rpc-url "$ANVIL_RPC"

# =============================================================================
# 3a. Localize the SFLUV Celo community config
# =============================================================================
log "step 3/9: writing localized community config → $LOCAL_CONFIG_FILE"
ENGINE_URL_LOCAL="$ENGINE_URL" ENGINE_WS_LOCAL="ws://localhost:$ENGINE_PORT" ANVIL_RPC_LOCAL="$ANVIL_RPC" \
python3 - "$ROOT_DIR/backend/celo-community-config.json" "$LOCAL_CONFIG_FILE" <<'PYEOF'
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
print(f"localized config written: chains -> {engine}, extras.rpc_url -> {anvil}")
PYEOF

# =============================================================================
# 3b. Engine database + env, then run the engine against anvil
# =============================================================================
log "step 3/9 (cont): preparing engine database + env"
if ! "${PSQL[@]}" -d postgres -c "SELECT 1 FROM pg_database WHERE datname = '$ENGINE_DB_NAME'" | grep -q 1; then
  createdb -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" "$ENGINE_DB_NAME"
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

log "building + starting the engine on :$ENGINE_PORT"
kill_port "$ENGINE_PORT"
(cd "$ENGINE_DIR" && go build -o "$TMP_DIR/engine-bin" ./cmd) \
  || die "engine build failed — check go version vs $ENGINE_DIR/go.mod"
# shellcheck disable=SC2086
start_service engine "$ENGINE_DIR" "$TMP_DIR/engine-bin" -env "$ENGINE_ENV_FILE" -port "$ENGINE_PORT" ${ENGINE_EXTRA_FLAGS:-}
wait_for "engine port" 60 bash -c "lsof -ti :$ENGINE_PORT -sTCP:LISTEN"

# =============================================================================
# 4. Local payer key: fund it, whitelist it on the paymaster, seed the engine
# =============================================================================
log "step 4/9: provisioning the local paymaster sponsor"

# Persist the payer key across reruns.
PAYER_KEY_FILE="$TMP_DIR/payer.key"
if [[ ! -f "$PAYER_KEY_FILE" ]]; then
  cast wallet new --json > "$PAYER_KEY_FILE.json" 2>/dev/null || true
  if [[ -s "$PAYER_KEY_FILE.json" ]]; then
    python3 - "$PAYER_KEY_FILE.json" "$PAYER_KEY_FILE" <<'PYEOF'
import json, sys
data = json.load(open(sys.argv[1]))
entry = data[0] if isinstance(data, list) else data
addr = entry.get("address")
pk = entry.get("private_key") or entry.get("privateKey")
with open(sys.argv[2], "w") as f:
    f.write(f"{addr}\n{pk}\n")
PYEOF
    rm -f "$PAYER_KEY_FILE.json"
  else
    # older foundry without --json: parse the text output
    OUT="$(cast wallet new)"
    ADDR="$(printf '%s' "$OUT" | grep -oE '0x[a-fA-F0-9]{40}' | head -1)"
    PK="$(printf '%s' "$OUT" | grep -oE '0x[a-fA-F0-9]{64}' | head -1)"
    printf '%s\n%s\n' "$ADDR" "$PK" > "$PAYER_KEY_FILE"
  fi
fi
PAYER_ADDRESS="$(sed -n 1p "$PAYER_KEY_FILE")"
PAYER_PK="$(sed -n 2p "$PAYER_KEY_FILE")"
[[ "$PAYER_ADDRESS" =~ ^0x[a-fA-F0-9]{40}$ ]] || die "could not derive payer address (see $PAYER_KEY_FILE)"
log "local payer/sponsor: $PAYER_ADDRESS"

# Fund the payer with native CELO for gas (1000 ether).
FUND_WEI="0x3635C9ADC5DEA00000"
cast rpc anvil_setBalance "$PAYER_ADDRESS" "$FUND_WEI" --rpc-url "$ANVIL_RPC" >/dev/null

# Whitelist: impersonate the paymaster owner and call updateSponsor(payer).
PM_OWNER="$(cast call "$PAYMASTER_ADDRESS" 'owner()(address)' --rpc-url "$ANVIL_RPC")"
log "paymaster owner on fork: $PM_OWNER (impersonating)"
cast rpc anvil_impersonateAccount "$PM_OWNER" --rpc-url "$ANVIL_RPC" >/dev/null
cast rpc anvil_setBalance "$PM_OWNER" "$FUND_WEI" --rpc-url "$ANVIL_RPC" >/dev/null
cast send "$PAYMASTER_ADDRESS" 'updateSponsor(address)' "$PAYER_ADDRESS" \
  --from "$PM_OWNER" --unlocked --rpc-url "$ANVIL_RPC" >/dev/null
NEW_SPONSOR="$(cast call "$PAYMASTER_ADDRESS" 'sponsor()(address)' --rpc-url "$ANVIL_RPC")"
[[ "$(printf '%s' "$NEW_SPONSOR" | tr '[:upper:]' '[:lower:]')" == "$(printf '%s' "$PAYER_ADDRESS" | tr '[:upper:]' '[:lower:]')" ]] \
  || die "paymaster sponsor() is $NEW_SPONSOR, expected $PAYER_ADDRESS"
log "paymaster sponsor updated on-chain ✔"

# Seed the engine's sponsor table (pk encrypted with the engine DB_SECRET).
# The engine's key parser rejects a 0x prefix, and cmd/encrypt logs its result
# to stderr as "key: <value>" — strip/extract both accordingly.
log "seeding engine sponsor row"
PAYER_PK_NOPREFIX="${PAYER_PK#0x}"
ENC_PK="$(cd "$ENGINE_DIR" && go run ./cmd/encrypt -s "$ENGINE_DB_SECRET" -v "$PAYER_PK_NOPREFIX" 2>&1 | sed -n 's/.*key: //p' | tail -1 | tr -d '[:space:]')"
[[ -n "$ENC_PK" ]] || die "engine cmd/encrypt produced no output"

SPONSOR_TABLE=""
for _ in $(seq 1 15); do
  SPONSOR_TABLE="$("${PSQL[@]}" -d "$ENGINE_DB_NAME" -c "SELECT tablename FROM pg_tables WHERE tablename LIKE 't_sponsors%' LIMIT 1" || true)"
  [[ -n "$SPONSOR_TABLE" ]] && break
  sleep 2
done
[[ -n "$SPONSOR_TABLE" ]] || die "engine sponsors table never appeared in $ENGINE_DB_NAME — check $LOG_DIR/engine.log"
"${PSQL[@]}" -d "$ENGINE_DB_NAME" -c "DELETE FROM $SPONSOR_TABLE WHERE lower(contract) = lower('$PAYMASTER_ADDRESS')"
"${PSQL[@]}" -d "$ENGINE_DB_NAME" -c "INSERT INTO $SPONSOR_TABLE (contract, pk, created_at, updated_at) VALUES ('$PAYMASTER_ADDRESS', '$ENC_PK', now(), now())"
log "engine sponsor row seeded in $SPONSOR_TABLE ✔"

# =============================================================================
# 5. Clone the production databases (READ-ONLY against prod)
# =============================================================================
if [[ "${SKIP_DB_CLONE:-0}" == "1" ]]; then
  log "step 5/9: SKIP_DB_CLONE=1 — reusing existing local databases"
else
  log "step 5/9: cloning production databases (${PROD_DB_NAMES:-app bot ponder})"
  [[ -n "${PROD_DB_HOST_PORT:-}" && -n "${PROD_DB_USER:-}" ]] \
    || die "PROD_DB_HOST_PORT / PROD_DB_USER must be set in .dev.env (or set SKIP_DB_CLONE=1)"
  PROD_HOST="${PROD_DB_HOST_PORT%%:*}"
  PROD_PORT="${PROD_DB_HOST_PORT##*:}"
  for dbname in ${PROD_DB_NAMES:-app bot ponder}; do
    log "  dumping $dbname from production..."
    PGPASSWORD="${PROD_DB_PASSWORD:-}" pg_dump -h "$PROD_HOST" -p "$PROD_PORT" -U "$PROD_DB_USER" \
      -d "$dbname" -Fc --no-owner --no-acl -f "$DUMP_DIR/$dbname.dump"
    log "  restoring $dbname locally..."
    dropdb  -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" --if-exists "$dbname"
    createdb -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" "$dbname"
    pg_restore -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" \
      -d "$dbname" --no-owner --no-acl "$DUMP_DIR/$dbname.dump" \
      || warn "pg_restore for $dbname reported errors (often ignorable ownership/extension noise) — inspect if $dbname misbehaves"
  done
fi

# Sanitize cloned data that references production: the ponder_hooks table holds
# production callback URLs which local ponder would POST to during backfill.
# Repoint every hook at the local backend so nothing calls out to prod.
if "${PSQL[@]}" -d ponder -c "SELECT 1 FROM pg_tables WHERE tablename = 'ponder_hooks'" 2>/dev/null | grep -q 1; then
  log "repointing cloned ponder_hooks at the local backend"
  "${PSQL[@]}" -d ponder -c "UPDATE ponder_hooks SET url = 'http://localhost:$BACKEND_PORT/ponder/callback'"
fi

# =============================================================================
# 6. Ponder against the cloned db + local chain
# =============================================================================
log "step 6/9: starting ponder on :$PONDER_PORT"
kill_port "$PONDER_PORT"
(cd "$ROOT_DIR/ponder" && npm install --no-audit --no-fund >/dev/null 2>&1 || npm install)
PONDER_ENV=(
  "DATABASE_URL=postgresql://$LOCAL_DB_USER:@$LOCAL_DB_HOST_PORT/ponder"
  "PONDER_RPC_URL_1=$ANVIL_RPC"
  "ADMIN_KEY=${DEV_PONDER_KEY:-local-dev-ponder-key}"
)
if [[ -n "${PONDER_START_BLOCK:-}" ]]; then
  PONDER_ENV+=("PONDER_START_BLOCK=$PONDER_START_BLOCK")
fi
start_service ponder "$ROOT_DIR/ponder" env "${PONDER_ENV[@]}" npm run dev
warn "ponder chain id is hardcoded to $CELO_CHAIN_ID and anvil forks with the same id — if ponder still rejects the fork or re-syncs, that is the known rpc-mismatch caveat; it will reindex from the configured start block."

# =============================================================================
# 7. Backend using the LOCAL community config
# =============================================================================
log "step 7/9: starting backend on :$BACKEND_PORT"
kill_port "$BACKEND_PORT"
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
start_service backend "$ROOT_DIR/backend" env ENV_FILE="$BACKEND_ENV_FILE" go run ./cmd/server
wait_for "backend /config" 180 curl -sf "http://localhost:$BACKEND_PORT/config"

# =============================================================================
# 8. Frontend
# =============================================================================
log "step 8/9: starting frontend on :$FRONTEND_PORT"
kill_port "$FRONTEND_PORT"
(cd "$ROOT_DIR/frontend" && npm install --no-audit --no-fund >/dev/null 2>&1 || npm install)
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
start_service frontend "$ROOT_DIR/frontend" env "${FRONTEND_ENV[@]}" npm run dev

# =============================================================================
# 9. Mobile app via Expo (branch-selectable clone in ./tmp)
# =============================================================================
MOBILE_APP_REPO="${MOBILE_APP_REPO:-https://github.com/SFLuv/mobile-app.git}"
MOBILE_APP_BRANCH="${MOBILE_APP_BRANCH:-main}"
MOBILE_DIR="$TMP_DIR/mobile-app"

log "step 9/9: syncing mobile app ($MOBILE_APP_BRANCH) + starting Expo"
if [[ -d "$MOBILE_DIR/.git" ]]; then
  git -C "$MOBILE_DIR" fetch origin "$MOBILE_APP_BRANCH"
  git -C "$MOBILE_DIR" checkout "$MOBILE_APP_BRANCH"
  git -C "$MOBILE_DIR" reset --hard "origin/$MOBILE_APP_BRANCH"
else
  git clone --branch "$MOBILE_APP_BRANCH" "$MOBILE_APP_REPO" "$MOBILE_DIR"
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
(cd "$MOBILE_DIR/mobile" && npm install --no-audit --no-fund >/dev/null 2>&1 || npm install)
start_service mobile "$MOBILE_DIR/mobile" npm run start

# =============================================================================
log "local dev environment is up"
cat <<EOF

  anvil (Celo fork)   $ANVIL_RPC                (log: tmp/logs/anvil.log)
  cw engine           $ENGINE_URL               (log: tmp/logs/engine.log)
  ponder              http://localhost:$PONDER_PORT  (log: tmp/logs/ponder.log)
  backend             http://localhost:$BACKEND_PORT   (log: tmp/logs/backend.log)
  frontend            https://localhost:$FRONTEND_PORT   (log: tmp/logs/frontend.log)
  mobile (expo)       see tmp/logs/mobile.log for the QR code

  payer/sponsor       $PAYER_ADDRESS (key: tmp/payer.key)
  local config        $LOCAL_CONFIG_FILE
  stop everything     ./dev-down.sh

EOF
