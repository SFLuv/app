#!/usr/bin/env bash
# Pushes every cloned event's QR redemption window into the future.
#
# Databases are restored from production dumps, so every event's window is
# already in the past and every redemption returns "code expired" — a failure
# that looks like broken redemption code and is not.
#
# The window, per db/faucet_bot.go: redemption opens at COALESCE(qr_live_at,
# start_at) and closes at expiration, all epoch seconds. This opens the window
# an hour ago and closes it DAYS days out (default 30) for every event that is
# not cancelled. Note the side effect: the volunteer panel calls an event
# "upcoming" while its expiration is in the future, so refreshed old events
# reappear in the upcoming list. That is inherent to making them redeemable.
#
# Local databases only. The connection is refused if it is not pointed at
# localhost.
#
#   ./scripts/refresh-qr-windows/refresh-qr-windows.sh [days]
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"

DB_HOST="${DB_HOST:-localhost}"
# The role comes from the backend's own env, so this needs no configuration on
# a machine where the stack already runs; $USER is the postgres default on mac.
if [[ -z "${DB_USER:-}" && -f "$SFLUV_ROOT/backend/.env" ]]; then
  DB_USER="$(grep -E '^DB_USER=' "$SFLUV_ROOT/backend/.env" | head -1 | cut -d= -f2- | tr -d "\"' ")"
fi
DB_USER="${DB_USER:-$USER}"
BOT_DB="${BOT_DB_NAME:-bot}"
DAYS="${1:-30}"

case "$DB_HOST" in
  localhost|127.0.0.1) ;;
  *) die "DB_HOST is $DB_HOST — this only ever runs against a local database" ;;
esac

step "Extending QR windows by $DAYS days"

out="$(psql -h "$DB_HOST" -U "$DB_USER" -d "$BOT_DB" -tAc "
  UPDATE events
     SET expiration = EXTRACT(EPOCH FROM NOW())::bigint + ${DAYS} * 86400,
         qr_live_at = EXTRACT(EPOCH FROM NOW())::bigint - 3600
   WHERE cancelled_at IS NULL
     AND expiration < EXTRACT(EPOCH FROM NOW());" 2>&1)"
if [[ "$out" == UPDATE* ]]; then
  pass "expired events reopened (${out})"
else
  fail "update failed: $out"
fi

summary "QR windows"
