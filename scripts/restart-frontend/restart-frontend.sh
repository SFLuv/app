#!/usr/bin/env bash
# Restarts the dev frontend from current source, with the env dev-up gave it.
#
# Needed because dev-up starts the frontend once, and a dev server left running
# for days accumulates hot-reload state until individual routes deadlock — a
# page that compiled in milliseconds yesterday simply never answers today,
# which reads as the app being broken rather than the compiler being tired.
#
# The env below mirrors dev-up.sh's FRONTEND_ENV exactly; if that list changes,
# change this one to match.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"

step "Restart frontend"

FRONTEND_PORT=3000
BACKEND_PORT=8080

# Values dev-up read from .dev.env and the generated backend env.
if [[ -f "$SFLUV_ROOT/.dev.env" ]]; then
  NEXT_PUBLIC_PRIVY_APP_ID="${NEXT_PUBLIC_PRIVY_APP_ID:-$(grep -E '^NEXT_PUBLIC_PRIVY_APP_ID=' "$SFLUV_ROOT/.dev.env" | head -1 | cut -d= -f2-)}"
  NEXT_PUBLIC_GOOGLE_MAPS_API_KEY="${NEXT_PUBLIC_GOOGLE_MAPS_API_KEY:-$(grep -E '^NEXT_PUBLIC_GOOGLE_MAPS_API_KEY=' "$SFLUV_ROOT/.dev.env" | head -1 | cut -d= -f2-)}"
  NEXT_PUBLIC_MAP_ID="${NEXT_PUBLIC_MAP_ID:-$(grep -E '^NEXT_PUBLIC_MAP_ID=' "$SFLUV_ROOT/.dev.env" | head -1 | cut -d= -f2-)}"
fi
FAUCET_ADDRESS_LOCAL="$(grep -E '^BOT_ADDRESS=' "$SFLUV_ROOT/tmp/backend.dev.env" 2>/dev/null | head -1 | cut -d= -f2-)"

# ALL pids on the port, not the first: npm and next-server hold it as a pair,
# and killing one leaves the other answering health probes while `next dev`
# quietly comes up on :3003 instead — a restart that reports success and
# changes nothing.
old_pids="$(lsof -ti tcp:$FRONTEND_PORT 2>/dev/null)"
if [[ -n "$old_pids" ]]; then
  kill -9 $old_pids 2>/dev/null || true
  for _ in $(seq 1 20); do
    [[ -z "$(lsof -ti tcp:$FRONTEND_PORT 2>/dev/null)" ]] && break
    sleep 0.5
  done
  pass "old frontend stopped"
else
  info "nothing was listening on :$FRONTEND_PORT"
fi

cd "$SFLUV_ROOT/frontend"
nohup env \
  "IN_PRODUCTION=false" \
  "NEXT_PUBLIC_BACKEND_URL=http://localhost:$BACKEND_PORT" \
  "NEXT_PUBLIC_FRONTEND_URL=https://localhost:$FRONTEND_PORT" \
  "NEXT_PUBLIC_CHAIN_RPC_URL=http://127.0.0.1:8545" \
  "NEXT_PUBLIC_FAUCET_ADDRESS=$FAUCET_ADDRESS_LOCAL" \
  "NEXT_PUBLIC_ENGINE_URL=http://localhost:3001" \
  "NEXT_PUBLIC_PRIVY_APP_ID=${NEXT_PUBLIC_PRIVY_APP_ID:-}" \
  "NEXT_PUBLIC_GOOGLE_MAPS_API_KEY=${NEXT_PUBLIC_GOOGLE_MAPS_API_KEY:-}" \
  "NEXT_PUBLIC_MAP_ID=${NEXT_PUBLIC_MAP_ID:-}" \
  "NEXT_PUBLIC_CSP_EXTRA_IMG_SRC=http://localhost:$BACKEND_PORT" \
  npm run dev >> "$SFLUV_ROOT/tmp/logs/frontend.log" 2>&1 &

started_pid=$!
for i in $(seq 1 60); do
  code="$(curl -sk -o /dev/null -w '%{http_code}' --max-time 5 "https://localhost:$FRONTEND_PORT" 2>/dev/null || echo 000)"
  if [[ "$code" =~ ^(200|3..)$ ]]; then
    # The port must be held by the process tree we just started; a 200 from a
    # survivor of the old server is exactly the failure this script exists for.
    holder="$(lsof -ti tcp:$FRONTEND_PORT 2>/dev/null | head -1)"
    if [[ -n "$holder" ]] && ! pgrep -P "$started_pid" >/dev/null 2>&1 && [[ "$holder" != "$started_pid" ]]; then
      root_of() { local pid="$1" parent; while :; do parent="$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d ' ')"; [[ -z "$parent" || "$parent" -le 1 ]] && break; pid="$parent"; done; echo "$pid"; }
      [[ "$(root_of "$holder")" != "$(root_of "$started_pid")" ]] && {
        fail ":$FRONTEND_PORT answered but is held by pid $holder, not the server just started — old process survived"
        summary "Restart frontend"; exit 1; }
    fi
    pass "frontend up after ~$((i * 2))s"
    summary "Restart frontend"
    exit 0
  fi
  sleep 2
done
fail "frontend did not answer within 120s — check tmp/logs/frontend.log"
summary "Restart frontend"
