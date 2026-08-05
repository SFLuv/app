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
#   - webpage:  public marketing site from ../webpage, on the first free port
#               from :3002 up, pointed at the local backend
#   - mobile:   Expo (pulled into ./tmp, branch via MOBILE_APP_BRANCH,
#               background — use the post-boot menu to open the iOS simulator)
#
# After boot, an interactive menu takes the foreground: open the iOS simulator,
# set admin by email, set/clear user pranks, tail logs, quit.
#
# Usage:
#   ./dev-up.sh                       # boot everything, then the post-boot menu
#   ./dev-up.sh --no-mobile           # skip Expo (menu still runs, minus simulator)
#   ./dev-up.sh --no-frontend         # skip the web app
#   ./dev-up.sh --no-webpage          # skip the public marketing site
#   ./dev-up.sh --no-backend          # skip the backend API
#   ./dev-up.sh --no-ponder           # skip the indexer
#   ./dev-up.sh --skip-db-clone       # reuse previously cloned local databases
#   ./dev-up.sh --mobile-branch <b>   # pull the mobile app from branch <b>
#                                     # (overrides MOBILE_APP_BRANCH in .dev.env)
#   ./dev-up.sh menu                  # dev utilities (set admin, set/clear user
#                                     # pranks) against the local db; boots nothing
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
RUN_BACKEND=1 RUN_PONDER=1 RUN_FRONTEND=1 RUN_MOBILE=1 RUN_WEBPAGE=1
SKIP_DB_CLONE_FLAG=""
MOBILE_BRANCH_OVERRIDE=""
MENU_MODE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    menu|--menu)    MENU_MODE=1 ;;
    --no-mobile)    RUN_MOBILE=0 ;;
    --no-frontend)  RUN_FRONTEND=0 ;;
    --no-webpage)   RUN_WEBPAGE=0 ;;
    --no-backend)   RUN_BACKEND=0 ;;
    --no-ponder)    RUN_PONDER=0 ;;
    --skip-db-clone) SKIP_DB_CLONE_FLAG=1 ;;
    --mobile-branch)
      [[ $# -ge 2 && -n "$2" ]] || { echo "--mobile-branch requires a branch name"; exit 1; }
      MOBILE_BRANCH_OVERRIDE="$2"
      shift ;;
    --mobile-branch=*) MOBILE_BRANCH_OVERRIDE="${1#--mobile-branch=}" ;;
    -h|--help)      awk 'NR > 1 && !/^#/ { exit } NR > 1 { sub(/^# ?/, ""); print }' "$0"; exit 0 ;;
    *) echo "unknown arg: $1 (try --help)"; exit 1 ;;
  esac
  shift
done

c_blue()  { printf "\033[1;34m%s\033[0m\n" "$1"; }
c_green() { printf "\033[1;32m%s\033[0m\n" "$1"; }
c_yellow(){ printf "\033[1;33m%s\033[0m\n" "$1"; }
c_red()   { printf "\033[1;31m%s\033[0m\n" "$1"; }
die()     { c_red "$1"; exit 1; }

# ----------------------------------------------------------------------------
# Dev utilities menu (./dev-up.sh menu) — does not boot any services. Operates
# on the local (cloned) app database only. All writes are gated behind explicit
# confirmation. The "prank" feature works with the backend's prank-forwarding
# middleware, which is compiled in only when IN_PRODUCTION!=true.
# ----------------------------------------------------------------------------

# pranks_active: succeeds when the pranks table exists AND holds at least one
# row. Existence is checked first so we never query a non-existent table.
pranks_active() {
  local exists count
  exists="$("${PSQL[@]}" -d "$APP_DB" -tAc "SELECT to_regclass('public.pranks') IS NOT NULL;" 2>/dev/null)"
  [[ "$exists" == "t" ]] || return 1
  count="$("${PSQL[@]}" -d "$APP_DB" -tAc "SELECT count(*) FROM pranks;" 2>/dev/null)"
  [[ "${count:-0}" =~ ^[0-9]+$ && "${count:-0}" -gt 0 ]]
}

# pick_user: searchable user chooser. $1 = label, $2 = initial search term.
# Emails can collide, so every row shows the user id; the caller selects the
# exact account. Result lands in PICKED_USER_ID / PICKED_USER_EMAIL.
PICKED_USER_ID=""
PICKED_USER_EMAIL=""
pick_user() {
  local label="$1" term="${2:-}"
  PICKED_USER_ID="" PICKED_USER_EMAIL=""
  while true; do
    if [[ -z "$term" ]]; then
      printf "  %s — search by email or user id (blank to cancel): " "$label"
      read -r term
      [[ -z "$term" ]] && return 1
    fi
    # Fed via stdin (not -c) so psql interpolates the :'q' variable; -v quoting
    # keeps the user-supplied search term injection-safe.
    local rows
    rows="$("${PSQL[@]}" -d "$APP_DB" -F $'\t' -v q="$term" <<'SQL' 2>/dev/null
SELECT id, COALESCE(NULLIF(TRIM(contact_email), ''), '(no email)')
  FROM users
 WHERE contact_email ILIKE '%' || :'q' || '%' OR id = :'q'
 ORDER BY LOWER(COALESCE(contact_email, '')), id
 LIMIT 50;
SQL
)"
    local ids=() emails=() id email
    while IFS=$'\t' read -r id email; do
      [[ -z "$id" ]] && continue
      ids+=("$id"); emails+=("$email")
    done <<< "$rows"
    if [[ ${#ids[@]} -eq 0 ]]; then
      c_yellow "    no users match \"$term\" — try again"
      term=""; continue
    fi
    echo
    local i
    for i in "${!ids[@]}"; do
      printf "    [%2d] %-32s %s\n" "$((i + 1))" "${emails[$i]}" "${ids[$i]}"
    done
    printf "  select 1-%d, 's' to search again (blank to cancel): " "${#ids[@]}"
    local choice; read -r choice
    [[ -z "$choice" ]] && return 1
    [[ "$choice" == "s" ]] && { term=""; continue; }
    if [[ "$choice" =~ ^[0-9]+$ && "$choice" -ge 1 && "$choice" -le ${#ids[@]} ]]; then
      PICKED_USER_ID="${ids[$((choice - 1))]}"
      PICKED_USER_EMAIL="${emails[$((choice - 1))]}"
      return 0
    fi
    c_yellow "    invalid selection"
  done
}

menu_set_admin() {
  local email
  printf "  email to grant admin: "; read -r email
  email="$(printf '%s' "$email" | tr -d '[:space:]')"
  [[ -z "$email" ]] && { c_yellow "  cancelled"; return; }

  local matches
  matches="$("${PSQL[@]}" -d "$APP_DB" -F $'\t' -v e="$email" <<'SQL' 2>/dev/null
SELECT id, COALESCE(NULLIF(TRIM(contact_email), ''), '(no email)'), is_admin
  FROM users
 WHERE LOWER(TRIM(COALESCE(contact_email, ''))) = LOWER(:'e')
    OR id IN (SELECT user_id FROM user_verified_emails
               WHERE LOWER(TRIM(email_normalized)) = LOWER(:'e') AND active = TRUE)
 ORDER BY id;
SQL
)"
  if [[ -z "$matches" ]]; then c_yellow "  no accounts with email $email"; return; fi
  echo "  accounts matching $email:"
  while IFS=$'\t' read -r id em adm; do
    [[ -z "$id" ]] && continue
    printf "    %-32s %s   admin=%s\n" "$em" "$id" "$adm"
  done <<< "$matches"
  printf "  set ALL of the above to admin? [y/N]: "; local ok; read -r ok
  [[ "$ok" =~ ^[Yy]$ ]] || { c_yellow "  cancelled"; return; }

  local updated
  updated="$("${PSQL[@]}" -d "$APP_DB" -v e="$email" <<'SQL' 2>/dev/null
WITH upd AS (
  UPDATE users SET is_admin = true
   WHERE (LOWER(TRIM(COALESCE(contact_email, ''))) = LOWER(:'e')
      OR id IN (SELECT user_id FROM user_verified_emails
                 WHERE LOWER(TRIM(email_normalized)) = LOWER(:'e') AND active = TRUE))
     AND is_admin = false
   RETURNING id
)
SELECT count(*) FROM upd;
SQL
)"
  c_green "  granted admin to ${updated:-0} account(s) (already-admin accounts left unchanged)."
}

menu_set_prank() {
  echo "  PRANKER — the logged-in developer account whose requests get forwarded:"
  local pranker_email
  printf "  pranker email: "; read -r pranker_email
  pranker_email="$(printf '%s' "$pranker_email" | tr -d '[:space:]')"
  [[ -z "$pranker_email" ]] && { c_yellow "  cancelled"; return; }
  pick_user "pranker" "$pranker_email" || { c_yellow "  cancelled"; return; }
  local pranker_id="$PICKED_USER_ID" pranker_disp="$PICKED_USER_EMAIL"

  echo "  PRANKEE — the account to impersonate (see exactly what they see):"
  pick_user "prankee" "" || { c_yellow "  cancelled"; return; }
  local prankee_id="$PICKED_USER_ID" prankee_disp="$PICKED_USER_EMAIL"

  if [[ "$pranker_id" == "$prankee_id" ]]; then
    c_yellow "  pranker and prankee are the same user — nothing to do."; return
  fi

  if "${PSQL[@]}" -d "$APP_DB" -v pr="$pranker_id" -v pe="$prankee_id" >/dev/null 2>&1 <<'SQL'
CREATE TABLE IF NOT EXISTS pranks(
  pranker_user_id TEXT PRIMARY KEY,
  prankee_user_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO pranks(pranker_user_id, prankee_user_id)
VALUES (:'pr', :'pe')
ON CONFLICT (pranker_user_id)
DO UPDATE SET prankee_user_id = EXCLUDED.prankee_user_id, created_at = NOW();
SQL
  then
    c_green "  prank set: $pranker_disp  →  $prankee_disp"
    echo "    ($pranker_id  →  $prankee_id)"
    c_yellow "  the running backend picks this up on the next request — no restart needed."
  else
    c_red "  failed to set prank"
  fi
}

menu_clear_pranks() {
  pranks_active || { c_yellow "  no active pranks to clear."; return; }
  echo "  current pranks:"
  "${PSQL[@]}" -d "$APP_DB" -tAc \
    "SELECT '    ' || pranker_user_id || '  ->  ' || prankee_user_id FROM pranks;" 2>/dev/null
  printf "  drop the pranks table and clear ALL pranks? [y/N]: "; local ok; read -r ok
  [[ "$ok" =~ ^[Yy]$ ]] || { c_yellow "  cancelled"; return; }
  if "${PSQL[@]}" -d "$APP_DB" -c "DROP TABLE IF EXISTS pranks;" >/dev/null 2>&1; then
    c_green "  pranks cleared."
  else
    c_red "  failed to drop pranks table."
  fi
}

run_menu() {
  APP_DB="${APP_DB_NAME:-app}"
  "${PSQL[@]}" -d "$APP_DB" -c "SELECT 1" >/dev/null 2>&1 \
    || die "local app database '$APP_DB' not reachable — run ./dev-up.sh first to clone it"
  while true; do
    echo
    c_blue "SFLUV dev utilities"
    echo "  1) Set admin by email"
    echo "  2) Set user prank"
    if pranks_active; then
      echo "  3) Clear pranks"
    fi
    echo "  q) Quit"
    printf "select: "; local sel; read -r sel
    case "$sel" in
      1) menu_set_admin ;;
      2) menu_set_prank ;;
      3) if pranks_active; then menu_clear_pranks; else c_yellow "  no active pranks."; fi ;;
      q|Q|"") break ;;
      *) c_yellow "  unknown selection" ;;
    esac
  done
}

ANVIL_PORT=8545
ANVIL_RPC="http://127.0.0.1:$ANVIL_PORT"
ENGINE_PORT=3001
ENGINE_URL="http://localhost:$ENGINE_PORT"
BACKEND_PORT=8080
PONDER_PORT=42069
FRONTEND_PORT=3000
# The webpage has no fixed-port requirement, so this is only where the search
# starts — the actual port is resolved at boot by pick_free_port. Starts above
# the frontend (:3000) and the engine (:3001).
WEBPAGE_PORT_BASE="${WEBPAGE_PORT_BASE:-3002}"
WEBPAGE_PORT=""
WEBPAGE_DIR="${WEBPAGE_DIR:-$(cd "$ROOT/.." && pwd)/webpage}"
CELO_CHAIN_ID=42220

# From backend/celo-community-config.json (accounts["42220:..."]).
PAYMASTER_ADDRESS="0x825b77eE3e3AB05c3a342EEE37223494b6c97a55"
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
  for p in "$ANVIL_PORT" "$ENGINE_PORT" "$PONDER_PORT" "$BACKEND_PORT" "$FRONTEND_PORT" ${WEBPAGE_PORT:+"$WEBPAGE_PORT"} 8081; do
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

pick_free_port() { # pick_free_port <start> [max_tries] — echo the first unused port.
  # Deliberately different from free_port(): that one CLAIMS a fixed port by
  # killing whoever holds it, which is right for services with a well-known
  # port (the frontend on :3000 is baked into Privy redirect URLs). The webpage
  # has no such constraint, so it takes whatever is free instead of killing a
  # process a developer may be relying on.
  local port="$1" tries="${2:-40}" i
  for ((i = 0; i < tries; i++)); do
    if [[ -z "$(lsof -ti tcp:"$port" 2>/dev/null || true)" ]]; then
      echo "$port"
      return 0
    fi
    port=$((port + 1))
  done
  return 1
}

start_bg() { # start_bg <name> <workdir> <logfile> <cmd...> — tracked background service.
  local name="$1" workdir="$2" logfile="$3"
  shift 3
  [[ -d "$workdir" ]] || die "  cannot start $name: missing directory $workdir"
  # </dev/null so no background service ever competes with the post-boot menu
  # for the terminal's stdin (Expo especially would otherwise grab keystrokes).
  ( cd "$workdir" && "$@" >"$logfile" 2>&1 </dev/null ) &
  PIDS+=($!)
  c_yellow "  $name logs: ${logfile#"$ROOT"/}"
}

export FOUNDRY_DISABLE_NIGHTLY_WARNING=1

# The menu subcommand only touches local psql, so it needs far less than a full
# boot: just psql (proven by the reachability probe below). A full boot needs
# the whole toolchain. Gating this here — before the .dev.env requirement — lets
# `./dev-up.sh menu` run on a machine without foundry/go/node and without the
# hand-set PROD_DB_*/Privy config it never reads.
if [[ "$MENU_MODE" -eq 1 ]]; then
  command -v psql >/dev/null || die "missing required tool: psql"
else
  for tool in git go node npm psql createdb dropdb pg_dump pg_restore anvil cast python3 curl lsof pgrep; do
    command -v "$tool" >/dev/null || die "missing required tool: $tool"
  done
fi

# ----------------------------------------------------------------------------
# 0. Boot config (.dev.env — auto-created; prod DB creds + Privy are hand-set)
# ----------------------------------------------------------------------------
# The menu reads only LOCAL_DB_* (all defaulted below), so a missing .dev.env is
# fine there; a full boot needs the hand-set PROD_DB_*/Privy values, so it stops
# to have them filled in first.
if [[ -f "$ROOT/.dev.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/.dev.env"
  set +a
elif [[ "$MENU_MODE" -ne 1 ]]; then
  cp "$ROOT/.dev.env.example" "$ROOT/.dev.env"
  c_yellow "created .dev.env from .dev.env.example — fill in PROD_DB_* and Privy values, then re-run"
  exit 1
fi

# CLI flags outrank .dev.env.
[[ -n "$SKIP_DB_CLONE_FLAG" ]] && SKIP_DB_CLONE=1
[[ -n "$MOBILE_BRANCH_OVERRIDE" ]] && MOBILE_APP_BRANCH="$MOBILE_BRANCH_OVERRIDE"

LOCAL_DB_USER="${LOCAL_DB_USER:-postgres}"
LOCAL_DB_HOST_PORT="${LOCAL_DB_HOST_PORT:-localhost:5432}"
LOCAL_DB_HOST="${LOCAL_DB_HOST_PORT%%:*}"
LOCAL_DB_PORT="${LOCAL_DB_HOST_PORT##*:}"
PSQL=(psql -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" -v ON_ERROR_STOP=1 -qAt)

# Local database names, taken straight from PROD_DB_NAMES so that list is the
# single place a database is named. Entries are positional — app, bot, ponder —
# matching the default "app bot ponder", so renaming one on prod (e.g. the Celo
# indexer living in migration_celo_ponder) needs no second variable.
read -r _prod_app _prod_bot _prod_ponder _ <<< "${PROD_DB_NAMES:-app bot ponder}"
APP_DB_NAME="${APP_DB_NAME:-${_prod_app:-app}}"
BOT_DB_NAME="${BOT_DB_NAME:-${_prod_bot:-bot}}"
PONDER_DB_NAME="${PONDER_DB_NAME:-${_prod_ponder:-ponder}}"
ENGINE_DB_NAME="${ENGINE_DB_NAME:-cw_engine}"
unset _prod_app _prod_bot _prod_ponder

# pg_dump refuses to dump from a server newer than itself, and Homebrew's
# postgresql@15 shadows any newer client on PATH. Prefer an explicit override,
# then the highest-versioned client Homebrew has, then whatever is on PATH.
# Major version of a postgres client binary, or 0 if it cannot be determined.
# Output varies by build — "pg_dump (PostgreSQL) 17.5" but also
# "pg_dump (PostgreSQL) 15.13 (Homebrew)" — so take the first field that is
# version-shaped rather than the last field, and never emit a non-number (an
# arithmetic comparison against one aborts the caller under `set -u`).
pg_client_major() {
  local raw
  raw="$("$1" --version 2>/dev/null |
    awk '{ for (i = 1; i <= NF; i++) if ($i ~ /^[0-9]+(\.[0-9]+)*$/) { print $i; exit } }' |
    cut -d. -f1)"
  case "$raw" in
    ''|*[!0-9]*) printf '0\n' ;;
    *)           printf '%s\n' "$raw" ;;
  esac
}

resolve_pg_client() {
  local binary="$1" override="$2" candidate best best_version version
  if [[ -n "$override" ]]; then
    printf '%s\n' "$override"
    return
  fi
  best="$(command -v "$binary" 2>/dev/null || true)"
  if [[ -n "$best" ]]; then
    best_version="$(pg_client_major "$best")"
  else
    best_version=0
  fi
  for candidate in /opt/homebrew/opt/libpq*/bin/"$binary" \
                   /opt/homebrew/opt/postgresql@*/bin/"$binary" \
                   /usr/local/opt/libpq*/bin/"$binary" \
                   /usr/local/opt/postgresql@*/bin/"$binary"; do
    [[ -x "$candidate" ]] || continue
    version="$(pg_client_major "$candidate")"
    if [[ "$version" -gt "$best_version" ]]; then
      best="$candidate"
      best_version="$version"
    fi
  done
  printf '%s\n' "${best:-$binary}"
}

PG_DUMP_BIN="$(resolve_pg_client pg_dump "${PG_DUMP_BIN:-}")"
PG_RESTORE_BIN="$(resolve_pg_client pg_restore "${PG_RESTORE_BIN:-}")"

"${PSQL[@]}" -d postgres -c "SELECT 1" >/dev/null 2>&1 \
  || die "cannot reach local postgres at $LOCAL_DB_HOST_PORT as $LOCAL_DB_USER"

# Dev utilities menu: operates on the already-cloned local database only. It
# boots nothing and never installs the shutdown trap, so it returns cleanly
# without tearing down a running stack.
if [[ "$MENU_MODE" -eq 1 ]]; then
  run_menu
  exit 0
fi

# All three Privy app ids must refer to the SAME Privy app: the backend
# rejects any token whose aud differs from PRIVY_APP_ID, so a mismatched
# mobile/web id means every authed request 403s.
for pair in "NEXT_PUBLIC_PRIVY_APP_ID:${NEXT_PUBLIC_PRIVY_APP_ID:-}" "EXPO_PUBLIC_PRIVY_APP_ID:${EXPO_PUBLIC_PRIVY_APP_ID:-}"; do
  pname="${pair%%:*}"; pval="${pair#*:}"
  if [[ -n "$pval" && -n "${PRIVY_APP_ID:-}" && "$pval" != "$PRIVY_APP_ID" ]]; then
    c_red "WARNING: $pname ($pval) != PRIVY_APP_ID (${PRIVY_APP_ID:-}) — that client's logins will 403 against this backend. Use ONE Privy app across .dev.env."
  fi
done

STARTED=1

# ----------------------------------------------------------------------------
# 1. Citizen Wallet engine source (into gitignored ./tmp)
# ----------------------------------------------------------------------------
ENGINE_REPO="${ENGINE_REPO:-https://github.com/citizenwallet/engine.git}"
ENGINE_BRANCH="${ENGINE_BRANCH:-main}"
ENGINE_DIR="$TMP_DIR/engine"

c_blue "[1/10] Citizen Wallet engine source ($ENGINE_BRANCH)"
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
c_blue "[2/10] Chain (anvil fork of Celo, :$ANVIL_PORT)"
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
c_blue "[3/10] Community config + engine (:$ENGINE_PORT)"
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

# The engine loads its -env file with godotenv, which NEVER overrides variables
# already present in the process environment — an inherited DB_NAME/DB_USER from
# the caller's shell would silently redirect it to the wrong database. So the
# config is passed as explicit process env (which always wins); the file is kept
# as a reference and for running engine-bin by hand.
# DB_PASSWORD must be NON-EMPTY: the engine builds a keyword/value DSN
# ("user=... password= dbname=...") and an empty password= derails the
# dbname token, silently landing the connection in the default `postgres`
# database (verified empirically). Trust-auth local postgres ignores the
# password value, so a placeholder is fine; set LOCAL_DB_PASSWORD in
# .dev.env if your postgres actually requires one.
ENGINE_ENV=(
  "CHAIN_NAME=celo"
  "RPC_URL=$ANVIL_RPC"
  "RPC_WS_URL=ws://127.0.0.1:$ANVIL_PORT"
  "DB_USER=$LOCAL_DB_USER"
  "DB_PASSWORD=${LOCAL_DB_PASSWORD:-devpassword}"
  "DB_NAME=$ENGINE_DB_NAME"
  "DB_HOST=$LOCAL_DB_HOST"
  "DB_READER_HOST=$LOCAL_DB_HOST"
  "DB_PORT=$LOCAL_DB_PORT"
  "DB_SECRET=$ENGINE_DB_SECRET"
)
ENGINE_ENV_FILE="$TMP_DIR/engine.env"
printf '%s\n' "${ENGINE_ENV[@]}" > "$ENGINE_ENV_FILE"

( cd "$ENGINE_DIR" && go build -o "$TMP_DIR/engine-bin" ./cmd ) >"$LOG_DIR/engine-build.log" 2>&1 \
  || { c_red "  engine build failed:"; tail -n 20 "$LOG_DIR/engine-build.log"; exit 1; }
free_port "$ENGINE_PORT" engine
# shellcheck disable=SC2086
start_bg engine "$ENGINE_DIR" "$LOG_DIR/engine.log" env "${ENGINE_ENV[@]}" "$TMP_DIR/engine-bin" -env "$ENGINE_ENV_FILE" -port "$ENGINE_PORT" ${ENGINE_EXTRA_FLAGS:-}
wait_for "$ENGINE_URL/v1/rpc" "engine" 30 "${PIDS[${#PIDS[@]}-1]}" "$LOG_DIR/engine.log" || exit 1

# ----------------------------------------------------------------------------
# 4. Paymaster sponsor (local payer key: fund, whitelist, seed)
# ----------------------------------------------------------------------------
c_blue "[4/10] Paymaster sponsor (prank + whitelist)"

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

# The engine names its tables by chain id (t_sponsors_<chainId>), created in
# DB_NAME at startup.
SPONSOR_TABLE="t_sponsors_$CELO_CHAIN_ID"
FOUND_SPONSOR_TABLE=""
for _ in $(seq 1 15); do
  FOUND_SPONSOR_TABLE="$("${PSQL[@]}" -d "$ENGINE_DB_NAME" -c "SELECT tablename FROM pg_tables WHERE tablename = '$SPONSOR_TABLE' LIMIT 1" 2>/dev/null || true)"
  [[ -n "$FOUND_SPONSOR_TABLE" ]] && break
  sleep 2
done
[[ -n "$FOUND_SPONSOR_TABLE" ]] || die "  $SPONSOR_TABLE never appeared in $ENGINE_DB_NAME — see tmp/logs/engine.log (did the engine connect to the right database?)"
"${PSQL[@]}" -d "$ENGINE_DB_NAME" -c "DELETE FROM $SPONSOR_TABLE WHERE lower(contract) = lower('$PAYMASTER_ADDRESS')" >/dev/null
"${PSQL[@]}" -d "$ENGINE_DB_NAME" -c "INSERT INTO $SPONSOR_TABLE (contract, pk, created_at, updated_at) VALUES ('$PAYMASTER_ADDRESS', '$ENC_PK', now(), now())" >/dev/null \
  || die "  failed to seed sponsor row"
c_green "  engine sponsor row seeded ($SPONSOR_TABLE)"

# ----------------------------------------------------------------------------
# 5. Databases (clone production — READ-ONLY against prod)
# ----------------------------------------------------------------------------
if [[ "${SKIP_DB_CLONE:-0}" == "1" ]]; then
  c_blue "[5/10] Databases (skipped — reusing existing local clones)"
else
  c_blue "[5/10] Databases (cloning production: ${PROD_DB_NAMES:-app bot ponder})"
  [[ -n "${PROD_DB_HOST_PORT:-}" && -n "${PROD_DB_USER:-}" ]] \
    || die "  PROD_DB_HOST_PORT / PROD_DB_USER must be set in .dev.env (or use --skip-db-clone)"
  PROD_HOST="${PROD_DB_HOST_PORT%%:*}"
  PROD_PORT="${PROD_DB_HOST_PORT##*:}"
  # Entries are "prod_name" or "prod_name:local_name". The mapping form exists
  # because the prod database is not always named what the services read: e.g.
  # PROD_DB_NAMES="app bot migration_celo_ponder:ponder" clones the Celo ponder
  # database and restores it locally as `ponder`. Without the mapping the clone
  # lands in a database nothing is configured to read, and the stale local
  # `ponder` silently stays in use.
  for db_entry in ${PROD_DB_NAMES:-app bot ponder}; do
    prod_db="${db_entry%%:*}"
    local_db="${db_entry#*:}"
    [[ "$local_db" == "$db_entry" ]] && local_db="$prod_db"

    if [[ "$prod_db" == "$local_db" ]]; then
      c_yellow "  cloning ${prod_db}…"
    else
      c_yellow "  cloning ${prod_db} → local ${local_db}…"
    fi

    PGPASSWORD="${PROD_DB_PASSWORD:-}" "$PG_DUMP_BIN" -h "$PROD_HOST" -p "$PROD_PORT" -U "$PROD_DB_USER" \
      -d "$prod_db" -Fc --no-owner --no-acl -f "$DUMP_DIR/$local_db.dump" \
      || die "  pg_dump of $prod_db failed"
    dropdb  -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" --if-exists "$local_db"
    createdb -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" "$local_db"
    "$PG_RESTORE_BIN" -h "$LOCAL_DB_HOST" -p "$LOCAL_DB_PORT" -U "$LOCAL_DB_USER" \
      -d "$local_db" --no-owner --no-acl "$DUMP_DIR/$local_db.dump" 2>/dev/null \
      || c_yellow "  pg_restore for $local_db reported errors (often ignorable ownership noise)"
    c_green "  $local_db cloned"
  done
fi

# Sanitize cloned data that references production: the ponder_hooks table holds
# production callback URLs which local ponder would POST to during backfill.
if "${PSQL[@]}" -d "$PONDER_DB_NAME" -c "SELECT 1 FROM pg_tables WHERE tablename = 'ponder_hooks'" 2>/dev/null | grep -q 1; then
  "${PSQL[@]}" -d "$PONDER_DB_NAME" -c "UPDATE ponder_hooks SET url = 'http://localhost:$BACKEND_PORT/ponder/callback'" >/dev/null
  c_green "  ponder_hooks repointed at the local backend (no prod callbacks)"
fi

# ----------------------------------------------------------------------------
# 6. Ponder (indexes the fork into the cloned db)
# ----------------------------------------------------------------------------
if [[ "$RUN_PONDER" -eq 1 ]]; then
  c_blue "[6/10] Ponder (:$PONDER_PORT)"
  free_port "$PONDER_PORT" ponder
  if [[ ! -d "$ROOT/ponder/node_modules" ]]; then
    c_yellow "  installing ponder deps…"
    ( cd "$ROOT/ponder" && npm install --no-audit --no-fund >"$LOG_DIR/ponder-install.log" 2>&1 )
  fi
  PONDER_ENV=(
    "DATABASE_URL=postgresql://$LOCAL_DB_USER:@$LOCAL_DB_HOST_PORT/$PONDER_DB_NAME"
    "PONDER_RPC_URL_1=$ANVIL_RPC"
    "ADMIN_KEY=${DEV_PONDER_KEY:-local-dev-ponder-key}"
    # `ponder dev` defaults this to "public"; `ponder start` requires it stated.
    "DATABASE_SCHEMA=${PONDER_DATABASE_SCHEMA:-public}"
  )
  [[ -n "${PONDER_START_BLOCK:-}" ]] && PONDER_ENV+=("PONDER_START_BLOCK=$PONDER_START_BLOCK")
  # npx ponder directly, NOT `npm run dev`: the npm script shell-sources
  # ./ponder/.env, which would override the explicit local env above (ponder's
  # own dotenv loading respects process-env precedence, so this stays local).
  #
  # `start`, not `dev`. The local ponder database is a clone of production, and
  # its tables hold Berachain history that was backfilled rather than indexed —
  # nothing can regenerate those rows. Ponder refuses to reuse a populated
  # schema when the command is `dev`, unconditionally, because dev mode drops
  # and recreates tables on every reload; that would destroy the backfill.
  # `start` instead compares build ids, and the build id is a hash of
  # {ordering, contracts, accounts, blocks} — `chains` is excluded, so pointing
  # ponder at the anvil fork does not change it. A matching build id lets ponder
  # attach to the existing tables and append.
  start_bg ponder "$ROOT/ponder" "$LOG_DIR/ponder.log" env "${PONDER_ENV[@]}" npx ponder start
  c_yellow "  note: ponder's chain id is hardcoded to $CELO_CHAIN_ID (anvil matches); it attaches to the cloned schema and indexes forward from the configured start block"
else
  c_blue "[6/10] Ponder (skipped)"
fi

# ----------------------------------------------------------------------------
# 7. Backend (local community config; no external sends)
# ----------------------------------------------------------------------------
if [[ "$RUN_BACKEND" -eq 1 ]]; then
  c_blue "[7/10] Backend (:$BACKEND_PORT)"
  free_port "$BACKEND_PORT" backend
  # Passed as explicit process env (wins over any dotenv file); ENV_FILE points
  # at /dev/null so godotenv can never fall back to backend/.env, which may
  # carry production values. tmp/backend.dev.env is a reference copy only.
  BACKEND_ENV=(
    "PORT=$BACKEND_PORT"
    "IN_PRODUCTION=false"
    "APP_BASE_URL=http://localhost:$FRONTEND_PORT"
    "DB_USER=$LOCAL_DB_USER"
    "DB_PASSWORD="
    "DB_BASE_URL=$LOCAL_DB_HOST_PORT"
    "DB_URL=$LOCAL_DB_HOST_PORT"
    "APP_DB_NAME=$APP_DB_NAME"
    "BOT_DB_NAME=$BOT_DB_NAME"
    "PONDER_DB_NAME=$PONDER_DB_NAME"
    "CLIENT_CONFIG_LOCAL_ONLY=true"
    "CLIENT_CONFIG_FALLBACK_PATH=$LOCAL_CONFIG_FILE"
    "RPC_URL=$ANVIL_RPC"
    "ENGINE_RPC_URL=$ENGINE_URL"
    "ENGINE_WS_URL=ws://localhost:$ENGINE_PORT"
    "PRIVY_APP_ID=${PRIVY_APP_ID:-}"
    "PRIVY_VKEY=${PRIVY_VKEY:-}"
    # Server-side Google Places verification for merchant locations. Falls back
    # to the browser key, which works as long as it is not referrer-restricted.
    # Empty is safe: verification is skipped and local validation still applies.
    "GOOGLE_MAPS_SERVER_API_KEY=${GOOGLE_MAPS_SERVER_API_KEY:-${NEXT_PUBLIC_GOOGLE_MAPS_API_KEY:-}}"
    "ADMIN_KEY=${DEV_ADMIN_KEY:-local-dev-admin-key}"
    "PONDER_SERVER_BASE_URL=http://localhost:$PONDER_PORT"
    "PONDER_KEY=${DEV_PONDER_KEY:-local-dev-ponder-key}"
    "PONDER_CALLBACK_URL=http://localhost:$BACKEND_PORT/ponder/callback"
    "NOTIFICATION_TEST_MODE=true"
    "ENV_FILE=/dev/null"
  )
  printf '%s\n' "${BACKEND_ENV[@]}" > "$TMP_DIR/backend.dev.env"
  start_bg backend "$ROOT/backend" "$LOG_DIR/backend.log" env "${BACKEND_ENV[@]}" go run ./cmd/server
  wait_for "http://localhost:$BACKEND_PORT/config" "backend" 180 "${PIDS[${#PIDS[@]}-1]}" "$LOG_DIR/backend.log" || exit 1
else
  c_blue "[7/10] Backend (skipped)"
fi

# ----------------------------------------------------------------------------
# 8. Frontend
# ----------------------------------------------------------------------------
if [[ "$RUN_FRONTEND" -eq 1 ]]; then
  c_blue "[8/10] Frontend (:$FRONTEND_PORT)"
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

  # Warm-build every route in the background so the dev compiler does its work
  # now instead of on your first click (a cold /map compile takes 2+ minutes).
  # Runs concurrently with the rest of the boot; progress in frontend-warm.log.
  FRONTEND_ROUTES=(
    / /map /settings /wallets /admin /contacts /calendar /voter /proposer
    /improver /issuer /supervisor /affiliates /opportunities /your-opportunities
    /role-management /merchant-status /update /verify /addcontact
  )
  (
    for route in "${FRONTEND_ROUTES[@]}"; do
      printf '%s ' "$route"
      curl -sk -o /dev/null -m 300 "https://localhost:$FRONTEND_PORT$route" && printf 'ok\n' || printf 'failed\n'
    done
    echo "route warm-up complete"
  ) >"$LOG_DIR/frontend-warm.log" 2>&1 &
  PIDS+=($!)
  c_yellow "  warming all routes in the background (tmp/logs/frontend-warm.log)"
else
  c_blue "[8/10] Frontend (skipped)"
fi

# ----------------------------------------------------------------------------
# 9. Webpage (public marketing site, ../webpage)
# ----------------------------------------------------------------------------
if [[ "$RUN_WEBPAGE" -eq 1 && -d "$WEBPAGE_DIR" ]]; then
  # Takes the first free port rather than claiming a fixed one — nothing points
  # at the webpage by hard-coded URL, so there is no reason to evict whatever a
  # developer already has running.
  WEBPAGE_PORT="$(pick_free_port "$WEBPAGE_PORT_BASE")" \
    || { c_yellow "  no free port found from $WEBPAGE_PORT_BASE up — skipping webpage"; RUN_WEBPAGE=0; }
fi

if [[ "$RUN_WEBPAGE" -eq 1 && -n "$WEBPAGE_PORT" ]]; then
  c_blue "[9/10] Webpage (:$WEBPAGE_PORT)"
  if [[ ! -d "$WEBPAGE_DIR/node_modules" ]]; then
    c_yellow "  installing webpage deps…"
    ( cd "$WEBPAGE_DIR" && npm install --no-audit --no-fund >"$LOG_DIR/webpage-install.log" 2>&1 ) \
      || c_yellow "  webpage dep install failed — see tmp/logs/webpage-install.log"
  fi

  # SFLUV_API_BASE_URL is the only variable the webpage reads. Unset, it falls
  # back to built-in fixtures; pointed at the local backend it renders live
  # volunteer events. Set it here so a dev boot always exercises the real API.
  WEBPAGE_ENV=(
    "NODE_ENV=development"
    "PORT=$WEBPAGE_PORT"
    "SFLUV_API_BASE_URL=http://localhost:$BACKEND_PORT"
  )
  # Forwarded only when configured, so the signup proxy can present the shared
  # secret exactly as it will in production (see VOLUNTEER_PROXY_KEY in the
  # backend env). Harmless to omit.
  [[ -n "${VOLUNTEER_PROXY_KEY:-}" ]] && WEBPAGE_ENV+=("SFLUV_PROXY_KEY=$VOLUNTEER_PROXY_KEY")

  if [[ "$RUN_BACKEND" -ne 1 ]]; then
    c_yellow "  backend is not running — the webpage will show fixture data for volunteer events"
  fi

  start_bg webpage "$WEBPAGE_DIR" "$LOG_DIR/webpage.log" \
    env "${WEBPAGE_ENV[@]}" npm run dev -- --port "$WEBPAGE_PORT"
  wait_for "http://localhost:$WEBPAGE_PORT" "webpage" 90 "${PIDS[${#PIDS[@]}-1]}" "$LOG_DIR/webpage.log" \
    || c_yellow "  webpage still starting — check tmp/logs/webpage.log"
elif [[ "$RUN_WEBPAGE" -eq 1 ]]; then
  c_blue "[9/10] Webpage (skipped — no repo at ${WEBPAGE_DIR})"
  c_yellow "  clone it beside this repo, or set WEBPAGE_DIR in .dev.env"
  RUN_WEBPAGE=0
else
  c_blue "[9/10] Webpage (skipped)"
fi

echo
c_green "Stack is up:"
echo "  Chain      $ANVIL_RPC (chain id $CELO_CHAIN_ID, Celo fork)"
echo "  Engine     $ENGINE_URL"
echo "  Paymaster  $PAYMASTER_ADDRESS (sponsor: $PAYER_ADDRESS)"
[[ "$RUN_PONDER" -eq 1 ]]   && echo "  Ponder     http://localhost:$PONDER_PORT"
[[ "$RUN_BACKEND" -eq 1 ]]  && echo "  Backend    http://localhost:$BACKEND_PORT"
[[ "$RUN_FRONTEND" -eq 1 ]] && echo "  Frontend   https://localhost:$FRONTEND_PORT"
[[ "$RUN_WEBPAGE" -eq 1 ]]  && echo "  Webpage    http://localhost:$WEBPAGE_PORT  (volunteers: /volunteers)"
echo "  Config     tmp/local-community-config.json"
echo "  Logs       tmp/logs/"
echo

# ----------------------------------------------------------------------------
# 9. Mobile (interactive) — foreground so Expo's menu/QR shows.
# ----------------------------------------------------------------------------
EXPO_URL="exp://${DEV_LAN_IP:-127.0.0.1}:8081"

if [[ "$RUN_MOBILE" -eq 1 ]]; then
  MOBILE_APP_REPO="${MOBILE_APP_REPO:-https://github.com/SFLuv/mobile-app.git}"
  MOBILE_APP_BRANCH="${MOBILE_APP_BRANCH:-main}"
  MOBILE_DIR="$TMP_DIR/mobile-app"

  c_blue "[10/10] Mobile (Expo @ $MOBILE_APP_BRANCH, background)"
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
  # Simulators can't reliably open the LAN URL (exp://<lan-ip>:8081 times out,
  # typically because the macOS firewall blocks Metro on the LAN interface), so
  # default to localhost mode. Setting DEV_LAN_IP in .dev.env switches to LAN
  # mode for testing on a physical device.
  if [[ -n "${DEV_LAN_IP:-}" ]]; then
    c_yellow "  DEV_LAN_IP set — Expo in LAN mode ($EXPO_URL; allow node through the macOS firewall if devices time out)"
    start_bg mobile "$MOBILE_DIR/mobile" "$LOG_DIR/expo.log" npm run start
  else
    c_yellow "  Expo in localhost mode for simulators (set DEV_LAN_IP in .dev.env for physical devices)"
    start_bg mobile "$MOBILE_DIR/mobile" "$LOG_DIR/expo.log" npm run start -- --host localhost
  fi
else
  c_blue "[10/10] Running (no mobile)."
fi

TAIL_LOGS=()
[[ "$RUN_BACKEND" -eq 1 ]]  && TAIL_LOGS+=("$LOG_DIR/backend.log")
[[ "$RUN_PONDER" -eq 1 ]]   && TAIL_LOGS+=("$LOG_DIR/ponder.log")
[[ "$RUN_FRONTEND" -eq 1 ]] && TAIL_LOGS+=("$LOG_DIR/frontend.log")
[[ "$RUN_WEBPAGE" -eq 1 ]]  && TAIL_LOGS+=("$LOG_DIR/webpage.log")
[[ "$RUN_MOBILE" -eq 1 ]]   && TAIL_LOGS+=("$LOG_DIR/expo.log")
TAIL_LOGS+=("$LOG_DIR/engine.log")

# open_ios_simulator: what Expo's interactive "i" key does, without needing
# Expo to own the terminal — boot a simulator, ensure Expo Go is installed
# (Expo caches the .app under ~/.expo), then deep-link the dev-server URL.
open_ios_simulator() {
  command -v xcrun >/dev/null || { c_red "  xcrun not found — install Xcode"; return; }
  open -a Simulator 2>/dev/null
  local waited=0
  until xcrun simctl list devices booted 2>/dev/null | grep -q "(Booted)"; do
    waited=$((waited + 1))
    [[ "$waited" -gt 60 ]] && { c_red "  no simulator booted after 60s — boot one in Simulator.app and retry"; return; }
    sleep 1
  done
  if ! xcrun simctl listapps booted 2>/dev/null | grep -q "host.exp.Exponent"; then
    local cached
    cached="$(ls -td "$HOME/.expo/ios-simulator-app-cache/"*.app 2>/dev/null | head -1)"
    if [[ -n "$cached" ]]; then
      c_yellow "  installing Expo Go from cache…"
      xcrun simctl install booted "$cached" >/dev/null 2>&1 || true
    else
      c_red "  Expo Go is not installed on this simulator and no cached copy exists."
      c_red "  One-time setup: cd tmp/mobile-app/mobile && npx expo start --ios (then re-run ./dev-up.sh)"
      return
    fi
  fi
  # Simulators always reach the host via loopback regardless of Expo's host mode.
  local url="exp://127.0.0.1:8081"
  if xcrun simctl openurl booted "$url" 2>/dev/null; then
    c_green "  opened $url in the iOS simulator"
  else
    c_red "  failed to open $url — is the simulator finished booting? (retry in a few seconds)"
  fi
}

# Post-boot menu — the resident foreground. Ctrl-C anywhere still tears the
# whole stack down via the trap; "tail logs" shields itself so Ctrl-C there
# returns to the menu instead.
APP_DB="$APP_DB_NAME"
c_green "All services up."
while true; do
  echo
  c_blue "SFLUV dev — running (Ctrl-C or q shuts everything down)"
  [[ "$RUN_FRONTEND" -eq 1 ]] && echo "  web    https://localhost:$FRONTEND_PORT"
  [[ "$RUN_BACKEND" -eq 1 ]]  && echo "  api    http://localhost:$BACKEND_PORT"
  [[ "$RUN_MOBILE" -eq 1 ]]   && echo "  expo   $EXPO_URL   (QR + logs: tmp/logs/expo.log)"
  echo
  [[ "$RUN_MOBILE" -eq 1 ]] && echo "  i) Open iOS simulator"
  echo "  a) Set admin by email"
  echo "  p) Set user prank"
  if pranks_active; then
    echo "  c) Clear pranks"
  fi
  echo "  l) Tail service logs (Ctrl-C returns here)"
  echo "  q) Quit (shut everything down)"
  printf "select: "
  MENU_SEL=""
  read -r MENU_SEL || MENU_SEL="q"
  case "$MENU_SEL" in
    i|I) if [[ "$RUN_MOBILE" -eq 1 ]]; then open_ios_simulator; else c_yellow "  mobile not running (--no-mobile)"; fi ;;
    a|A) menu_set_admin ;;
    p|P) menu_set_prank ;;
    c|C) if pranks_active; then menu_clear_pranks; else c_yellow "  no active pranks."; fi ;;
    l|L)
      c_yellow "  tailing ${#TAIL_LOGS[@]} logs — Ctrl-C to return to the menu"
      # A no-op INT *handler* (not '' ignore — ignores are inherited by
      # children, handlers are not): Ctrl-C hits the whole foreground process
      # group, killing tail (default disposition) while the script just runs
      # the no-op and returns to the menu.
      trap ':' INT
      tail -f ${TAIL_LOGS[@]+"${TAIL_LOGS[@]}"} || true
      trap cleanup INT
      ;;
    q|Q) exit 0 ;;
    "") ;;
    *) c_yellow "  unknown selection" ;;
  esac
done
