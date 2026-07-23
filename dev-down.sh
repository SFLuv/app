#!/usr/bin/env bash
# dev-down.sh — stop every service started by dev-up.sh.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_DIR="$ROOT_DIR/tmp/pids"

log() { printf '\033[1;36m[dev-down]\033[0m %s\n' "$*"; }

if [[ -d "$PID_DIR" ]]; then
  for pidfile in "$PID_DIR"/*.pid; do
    [[ -e "$pidfile" ]] || continue
    name="$(basename "$pidfile" .pid)"
    pid="$(cat "$pidfile")"
    if kill -0 "$pid" 2>/dev/null; then
      log "stopping $name (pid group $pid)"
      # dev-up starts each service in its own process group (pgid == pid), so
      # kill the whole tree; fall back to the single pid for stale pid files.
      kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
    fi
    rm -f "$pidfile"
  done
fi

# Belt and braces: free the well-known dev ports (children of npm/go run etc.).
for port in 8545 3001 42069 8080 3000 8081; do
  pids="$(lsof -ti ":$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    log "freeing :$port"
    for pid in $pids; do
      pgid="$(ps -o pgid= -p "$pid" 2>/dev/null | tr -d ' ')"
      if [[ -n "$pgid" ]]; then
        kill -TERM -- "-$pgid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
      else
        kill -TERM "$pid" 2>/dev/null || true
      fi
    done
  fi
done

log "done"
