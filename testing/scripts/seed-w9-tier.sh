#!/usr/bin/env bash
# Puts an account at a W-9 warning tier, so the mobile tier modal can be tested.
#
# The modal needs two things and neither can be faked with one of them:
#   - payout_ledger rows summing past the tier, because the progress meter is
#     drawn from the real annual total across every wallet and chain
#   - an UNACKNOWLEDGED row in w9_tier_notices, because that is what
#     GET /w9/status reports as the outstanding tier
# Seeding only the ledger gets a status response with no tier and no modal.
#
# Amounts are token BASE units: SFLUV has 6 decimals, so 400 SFLUV is 400000000.
#
#   ./testing/scripts/seed-w9-tier.sh 0x<address> notice_400
#   ./testing/scripts/seed-w9-tier.sh 0x<address> warning_500
#   ./testing/scripts/seed-w9-tier.sh 0x<address> escrow_600
#   ./testing/scripts/seed-w9-tier.sh 0x<address> blocked
#   ./testing/scripts/seed-w9-tier.sh --revert 0x<address>
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT/tmp/backend.dev.env"
[[ -f "$ENV_FILE" ]] || { echo "no $ENV_FILE — has dev-up ever run?" >&2; exit 1; }

eval "$(python3 - "$ENV_FILE" <<'PY'
import re,sys,shlex
t=open(sys.argv[1]).read()
for k in ("DB_USER","DB_PASSWORD","DB_URL","APP_DB_NAME"):
    m=re.search(r'^%s=(.*)$'%k,t,re.M)
    if m: print("%s=%s"%(k,shlex.quote(m.group(1).strip())))
PY
)"

REVERT=0
[[ "${1:-}" == "--revert" ]] && { REVERT=1; shift; }
TARGET="${1:-}"; TIER="${2:-notice_400}"

if [[ ! "$TARGET" =~ ^0x[0-9a-fA-F]{40}$ && ! "$TARGET" =~ ^did:privy:[0-9a-z]+$ ]]; then
  echo "usage: $0 [--revert] <wallet-address|did> [notice_400|warning_500|escrow_600|blocked]" >&2
  exit 1
fi

# One SFLUV over the tier it is meant to trigger, so the modal is unambiguously
# past the line rather than sitting exactly on it.
#
# blocked is 601, NOT some larger number. A blocked account cannot have a bigger
# annual total than one sitting at the crossing: the payment that crosses 600 is
# ESCROWED and still counts, and every payment after it is refused outright, so
# it never becomes a ledger row in a counting state. The total stops moving
# precisely because nothing more is being paid. Seeding 701 here showed a figure
# the system cannot produce.
case "$TIER" in
  notice_400)  AMOUNT=401000000; STATE=paid     ;;
  warning_500) AMOUNT=501000000; STATE=paid     ;;
  escrow_600)  AMOUNT=601000000; STATE=escrowed ;;
  blocked)     AMOUNT=601000000; STATE=escrowed ;;
  *) echo "unknown tier: $TIER" >&2; exit 1 ;;
esac

host="${DB_URL%%:*}"; port="${DB_URL##*:}"
YEAR="$(date -u +%Y)"

PGPASSWORD="$DB_PASSWORD" psql -h "$host" -p "$port" -U "$DB_USER" \
  -d "${APP_DB_NAME:-app}" -v ON_ERROR_STOP=1 <<SQL
DO \$\$
DECLARE
  uid text;
  addr text;
BEGIN
  SELECT u.id, COALESCE(u.primary_wallet_address, '') INTO uid, addr
  FROM users u
  WHERE u.id = '$TARGET' OR u.primary_wallet_address ILIKE '$TARGET'
  LIMIT 1;
  IF uid IS NULL THEN RAISE EXCEPTION 'no account matches %', '$TARGET'; END IF;

  DELETE FROM payout_ledger   WHERE user_id = uid AND idempotency_key LIKE 'w9-seed-%';
  DELETE FROM w9_tier_notices WHERE user_id = uid AND tax_year = $YEAR;

  IF $REVERT = 1 THEN
    RAISE NOTICE 'cleared seeded W-9 state for %', uid;
    RETURN;
  END IF;

  -- State matters as much as the amount. The first two tiers are warnings that
  -- arrive while money is STILL BEING PAID, so they are seeded 'paid'; seeding
  -- them held would describe a state those tiers are specifically not in. The
  -- last two are past the crossing, where the payment is held, so 'escrowed'.
  INSERT INTO payout_ledger
    (idempotency_key, user_id, recipient_address, chain_id, tax_year, source,
     source_ref, amount_base, state, paid_at, escrowed_at, counts_toward_threshold)
  VALUES
    ('w9-seed-' || uid || '-$TIER', uid, addr, 42220, $YEAR, 'redemption_code',
     'w9-seed', $AMOUNT, '$STATE',
     CASE WHEN '$STATE' = 'paid' THEN NOW() ELSE NULL END,
     CASE WHEN '$STATE' = 'escrowed' THEN NOW() ELSE NULL END,
     TRUE);

  -- acknowledged_at NULL is the whole point: an acknowledged tier is exactly
  -- the state where the modal must NOT reappear.
  INSERT INTO w9_tier_notices (user_id, tax_year, tier, notified_at, acknowledged_at)
  VALUES (uid, $YEAR, '$TIER', NOW(), NULL);

  RAISE NOTICE 'account % seeded at tier % (% base units)', uid, '$TIER', $AMOUNT;
END
\$\$;
SQL
