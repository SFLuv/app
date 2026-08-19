#!/usr/bin/env bash
# Pushes every cloned event's QR window into the future.
#
# Databases are restored from production dumps, so every event's qr_expires_at
# is already in the past and every redemption returns "code expired" — a failure
# that looks like broken redemption code and is not.
#
# Local databases only. The connection is refused if it is not pointed at
# localhost.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

DB_HOST="${DB_HOST:-localhost}"
DB_USER="${DB_USER:-postgres}"
BOT_DB="${BOT_DB_NAME:-bot}"
DAYS="${1:-30}"

case "$DB_HOST" in
  localhost|127.0.0.1) ;;
  *) die "DB_HOST is $DB_HOST — this only ever runs against a local database" ;;
esac

step "Extending QR windows by $DAYS days"

# Column names differ across schema versions, so discover rather than assume.
cols="$(psql -h "$DB_HOST" -U "$DB_USER" -d "$BOT_DB" -tAc \
  "SELECT column_name FROM information_schema.columns
   WHERE table_name='events' AND column_name IN
   ('qr_expires_at','expires_at','end_at','qr_starts_at','starts_at','start_at');" 2>/dev/null)"

if [[ -z "$cols" ]]; then
  fail "no recognisable date columns on events in '$BOT_DB' — check DB_HOST/DB_USER/BOT_DB_NAME"
  summary "QR windows"; exit 1
fi
info "columns found: $(echo "$cols" | tr '\n' ' ')"

for col in $cols; do
  case "$col" in
    *start*) sql="UPDATE events SET $col = NOW() - INTERVAL '1 day' WHERE $col IS NOT NULL;" ;;
    *)       sql="UPDATE events SET $col = NOW() + INTERVAL '$DAYS days' WHERE $col IS NOT NULL;" ;;
  esac
  if out="$(psql -h "$DB_HOST" -U "$DB_USER" -d "$BOT_DB" -tAc "$sql" 2>&1)"; then
    pass "$col updated ($out)"
  else
    fail "$col — $out"
  fi
done

summary "QR windows"
