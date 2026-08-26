#!/usr/bin/env bash
# Wipes an account's W-9 history so a tier walkthrough starts from zero.
#
# Broader than `seed-w9-tier.sh --revert`, which only removes rows it seeded
# itself (idempotency_key LIKE 'w9-seed-%'). Money earned through a real
# redemption is not seeded, so revert leaves it behind and the next run starts
# part-way up the ladder — the tier you were aiming for fires early, or not at
# all. This removes everything the tier logic reads, whatever put it there.
#
# What it clears, and why each one matters:
#   payout_ledger              the annual total decidePayout compares against
#   w9_tier_notices            which modals have fired, and been acknowledged
#   w9_filings                 a cleared filing pays through every threshold
#   improver_notification_reads  the seen marks behind the bell badge
#
# What it does NOT clear: the on-chain balance. Transfers on the fork already
# happened and are not reversible, so the wallet keeps whatever it was sent.
# The tier maths reads payout_ledger, not the chain, so this is cosmetic — but
# it does mean the wallet can read 800 while the modal says "400 of 600".
#
#   ./scripts/reset-w9/reset-w9.sh <wallet-address|did>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
discover_stack   # needs the admin key to clear the stand-in's in-process state

TARGET="${1:-}"
if [[ ! "$TARGET" =~ ^0x[0-9a-fA-F]{40}$ && ! "$TARGET" =~ ^did:privy:[0-9a-z]+$ ]]; then
  echo "usage: $0 <wallet-address|did>" >&2
  exit 1
fi

eval "$(python3 - "$SFLUV_ROOT/tmp/backend.dev.env" <<'PY'
import re,sys,shlex
t=open(sys.argv[1]).read()
for k in ("DB_USER","DB_PASSWORD","DB_URL","APP_DB_NAME"):
    m=re.search(r'^%s=(.*)$'%k,t,re.M)
    if m: print("%s=%s"%(k,shlex.quote(m.group(1).strip())))
PY
)"
host="${DB_URL%%:*}"; port="${DB_URL##*:}"
YEAR="$(date -u +%Y)"
psql_app(){ PGPASSWORD="$DB_PASSWORD" psql -h "$host" -p "$port" -U "$DB_USER" -d "${APP_DB_NAME:-app}" -tAc "$1"; }

# Ambiguity is an error rather than a LIMIT 1 coin flip: while a prank is active
# the app writes the pranker's wallet into the prankee's row, so one address can
# genuinely belong to two accounts and "the first row" resets the wrong person.
if [[ "$TARGET" == did:privy:* ]]; then
  UID_="$(psql_app "SELECT id FROM users WHERE id='$TARGET';")"
else
  matches="$(psql_app "SELECT id FROM users WHERE primary_wallet_address ILIKE '$TARGET' ORDER BY id;")"
  if [[ "$(printf '%s\n' "$matches" | grep -c . || true)" -gt 1 ]]; then
    echo "  $TARGET is the primary wallet of more than one account:" >&2
    printf '%s\n' "$matches" | sed 's/^/    /' >&2
    echo "  pass the did instead so this is unambiguous." >&2
    exit 1
  fi
  UID_="$matches"
fi
[[ -n "$UID_" ]] || { echo "no account matches $TARGET" >&2; exit 1; }

before="$(psql_app "SELECT ROUND(COALESCE(SUM(amount_base),0)/1000000.0, 2) FROM payout_ledger WHERE user_id='$UID_' AND tax_year=$YEAR AND counts_toward_threshold AND state IN ('escrowed','releasing','paid');")"

psql_app "DELETE FROM payout_ledger WHERE user_id='$UID_' AND tax_year=$YEAR;" >/dev/null
psql_app "DELETE FROM w9_tier_notices WHERE user_id='$UID_' AND tax_year=$YEAR;" >/dev/null
psql_app "DELETE FROM w9_filings WHERE user_id='$UID_' AND tax_year=$YEAR;" >/dev/null
psql_app "DELETE FROM improver_notification_reads WHERE user_id='$UID_' AND notification_key LIKE 'w9_%';" >/dev/null

# The fake provider keeps its state in the backend process, not the database, and
# it is idempotent on the payee reference — so without this a person who signed
# the form before a reset gets the same signed submission handed back after one.
# The next crossing then releases the moment it is escrowed and the tiers clear
# themselves, which looks exactly like the tier logic breaking and is not.
#
# Quiet when the endpoint is absent: it only exists against the fake provider.
forgotten="$(admin_api POST "/w9/fake/forget?user_id=$UID_&tax_year=$YEAR" 2>/dev/null | jq -r '.forgotten // empty' 2>/dev/null || true)"
if [[ -n "$forgotten" ]]; then
  echo "  cleared $forgotten stand-in submission(s) held in the backend's memory"
fi

echo "  cleared W-9 history for $UID_ (year $YEAR)"
echo "  annual total was ${before} SFLUV, now 0 — on-chain balance is unchanged"
