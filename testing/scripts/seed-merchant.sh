#!/usr/bin/env bash
# Promotes a signed-in account to a merchant that owns an approved location, so
# the mobile merchant surfaces can be tested.
#
# Why this exists: the app database holds several rows for the same person, one
# per Privy environment they have ever signed in from. Only one of them is a
# merchant, and it is not the one a local mobile build can log in as — signing in
# against the local stack's Privy app mints a DIFFERENT did, and therefore a new,
# plain account. So the merchant row that exists cannot be reached from the
# simulator, and the account that can be reached is not a merchant.
#
# Rather than move rows between environments, this promotes whichever account you
# are actually signed in as. Find it from the app: Wallet -> Receive shows the
# address, and that is what this takes.
#
# Idempotent: re-running updates in place rather than adding a second location.
#
#   ./testing/scripts/seed-merchant.sh 0xc869764da6222e6a70FF2Fa7264E95b4e9F34Ab2
#   ./testing/scripts/seed-merchant.sh did:privy:cmlie8xyp039hjp0csmrnivxt
#
# Pass --onboarding-pending to leave merchant_onboarding_completed_at NULL, which
# is how you exercise the forced-onboarding gate instead of merchant mode.
#
# Pass --second-shop to add a second approved location, which is what you need to
# exercise switching a till between counters — the Switch location control only
# appears when there is somewhere else to go.
#
# Pass --revert to put the account back to a plain one and drop the seeded shops.
# The mobile suite is split by account state — the volunteer flows need a plain
# account and the merchant flows need a seeded one — so switching between them
# means running this either way round.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh" 2>/dev/null || true

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT/tmp/backend.dev.env"
[[ -f "$ENV_FILE" ]] || { echo "no $ENV_FILE — has dev-up ever run?" >&2; exit 1; }

# Parsed with python, not `source`: PRIVY_VKEY is a multi-line PEM and sourcing
# truncates it at the first newline.
eval "$(python3 - "$ENV_FILE" <<'PY'
import re,sys,shlex
t=open(sys.argv[1]).read()
for k in ("DB_USER","DB_PASSWORD","DB_URL","APP_DB_NAME"):
    m=re.search(r'^%s=(.*)$'%k,t,re.M)
    if m: print("%s=%s"%(k,shlex.quote(m.group(1).strip())))
PY
)"

REVERT=0; SECOND=0
for arg in "$@"; do
  [[ "$arg" == "--revert" ]] && REVERT=1
  [[ "$arg" == "--second-shop" ]] && SECOND=1
done

TARGET="${1:-}"
[[ "$TARGET" == "--revert" || "$TARGET" == "--second-shop" ]] && TARGET="${2:-}"
ONBOARDING_DONE="now()"
[[ "${2:-}" == "--onboarding-pending" || "${1:-}" == "--onboarding-pending" ]] && ONBOARDING_DONE="NULL"
[[ "$TARGET" == "--onboarding-pending" ]] && TARGET=""
[[ -n "$TARGET" ]] || { echo "usage: $0 <wallet-address|did> [--onboarding-pending]" >&2; exit 1; }

host="${DB_URL%%:*}"; port="${DB_URL##*:}"

if [[ "$REVERT" == "1" ]]; then
  PGPASSWORD="$DB_PASSWORD" psql -h "$host" -p "$port" -U "$DB_USER" \
    -d "${APP_DB_NAME:-app}" -v ON_ERROR_STOP=1 <<REVERTSQL
DELETE FROM locations WHERE reference IN ('maestro-seed','maestro-seed-2')
  AND owner_id = (SELECT id FROM users
                  WHERE id = '$TARGET' OR primary_wallet_address ILIKE '$TARGET'
                  LIMIT 1);
UPDATE users SET account_type = 'regular', is_merchant = false,
                 merchant_onboarding_completed_at = NULL
 WHERE id = '$TARGET' OR primary_wallet_address ILIKE '$TARGET';
SELECT id, account_type, is_merchant FROM users
 WHERE id = '$TARGET' OR primary_wallet_address ILIKE '$TARGET';
REVERTSQL
  exit 0
fi

# psql does not substitute :'vars' inside a DO $$ ... $$ body, so the target is
# interpolated by the shell. Constrain it to the two shapes it can legitimately
# be, so nothing else can ride in.
if [[ ! "$TARGET" =~ ^0x[0-9a-fA-F]{40}$ && ! "$TARGET" =~ ^did:privy:[0-9a-z]+$ ]]; then
  echo "target must be a 0x address or a did:privy:... id, got: $TARGET" >&2
  exit 1
fi

PGPASSWORD="$DB_PASSWORD" psql -h "$host" -p "$port" -U "$DB_USER" -d "${APP_DB_NAME:-app}" \
  -v ON_ERROR_STOP=1 <<SQL
\\set QUIET on
DO \$\$
DECLARE
  uid text;
  addr text;
  loc_id int;
  loc2_id int;
  col record;
BEGIN
  SELECT u.id, u.primary_wallet_address INTO uid, addr
  FROM users u
  WHERE u.id = '$TARGET'
     OR u.primary_wallet_address ILIKE '$TARGET'
     OR u.id IN (SELECT w.owner FROM wallets w
                 WHERE w.eoa_address ILIKE '$TARGET'
                    OR coalesce(w.smart_address,'') ILIKE '$TARGET')
  LIMIT 1;

  IF uid IS NULL THEN
    RAISE EXCEPTION 'no account matches %', '$TARGET';
  END IF;

  UPDATE users
     SET account_type = 'merchant',
         is_merchant  = true,
         merchant_onboarding_completed_at = $ONBOARDING_DONE
   WHERE id = uid;

  -- One seeded location per account, reused on re-run so repeated seeding does
  -- not leave a trail of shops behind.
  SELECT id INTO loc_id FROM locations
   WHERE owner_id = uid AND reference = 'maestro-seed' LIMIT 1;

  IF loc_id IS NULL THEN
    INSERT INTO locations
      (owner_id, name, description, type, approval, approved_at, street, city,
       state, zip, lat, lng, phone, email, admin_phone, admin_email,
       contact_firstname, contact_lastname, contact_phone, pos_system,
       sole_proprietorship, tipping_policy, tipping_division, location_kind,
       listing_source, reference, active, payment_wallet_address,
       -- Populated because these columns are nullable in the schema while the Go
       -- structs scan them into plain strings. A NULL here does not just break
       -- this shop: GET /locations reads every row for the public map, so one
       -- NULL website 500s the map for everyone. Matching what the API writes
       -- keeps the seed from creating a state the app cannot render.
       website, image_url, maps_page, rating)
    VALUES
      (uid, 'Maestro Test Shop', 'Seeded for mobile merchant-mode tests',
       'restaurant', true, now(), '1 Test St', 'San Francisco', 'CA', '94110',
       37.7599, -122.4148, '4155550100', 'test@example.com', '4155550100',
       'test@example.com', 'Test', 'Merchant', '4155550100', 'other',
       true, 'pooled', 'even', 'merchant', 'manual', 'maestro-seed', true, addr,
       '', '', '', 0)
    RETURNING id INTO loc_id;
  ELSE
    UPDATE locations
       SET approval = true, approved_at = now(), active = true,
           payment_wallet_address = addr,
           website   = coalesce(website, ''),
           image_url = coalesce(image_url, ''),
           maps_page = coalesce(maps_page, ''),
           rating    = coalesce(rating, 0)
     WHERE id = loc_id;
  END IF;

  -- Fill every remaining NULL scalar on the seeded row.
  --
  -- Done generically rather than column by column because almost the whole
  -- locations table is nullable (name, type, lat, lng included) while the Go
  -- structs scan most of it into plain strings and numbers. A NULL anywhere the
  -- scan touches fails the whole query, and fixing them one at a time turned
  -- into whack-a-mole: website, then table_coverage, then whatever is next.
  FOR col IN
    SELECT column_name, data_type FROM information_schema.columns
     WHERE table_name = 'locations' AND is_nullable = 'YES'
       AND data_type IN ('text','numeric','integer')
  LOOP
    EXECUTE format(
      'UPDATE locations SET %I = COALESCE(%I, %s) WHERE id = \$1',
      col.column_name, col.column_name,
      CASE WHEN col.data_type = 'text' THEN '''''' ELSE '0' END
    ) USING loc_id;
  END LOOP;

  IF $SECOND = 1 THEN
    SELECT id INTO loc2_id FROM locations
     WHERE owner_id = uid AND reference = 'maestro-seed-2' LIMIT 1;
    IF loc2_id IS NULL THEN
      -- A second counter needs a DIFFERENT till wallet, or switching between the
      -- two would not change where the money lands and the test would prove
      -- nothing.
      INSERT INTO locations
        (owner_id, name, description, type, approval, approved_at, street, city,
         state, zip, lat, lng, phone, email, admin_phone, admin_email,
         contact_firstname, contact_lastname, contact_phone, pos_system,
         sole_proprietorship, tipping_policy, tipping_division, location_kind,
         listing_source, reference, active, payment_wallet_address,
         website, image_url, maps_page, rating)
      VALUES
        (uid, 'Maestro Second Shop', 'Second counter for switch tests',
         'restaurant', true, now(), '2 Test St', 'San Francisco', 'CA', '94110',
         37.7620, -122.4180, '4155550101', 'test2@example.com', '4155550101',
         'test2@example.com', 'Test', 'Merchant', '4155550101', 'other',
         true, 'pooled', 'even', 'merchant', 'manual', 'maestro-seed-2', true,
         '0x000000000000000000000000000000000000dEaD', '', '', '', 0)
      RETURNING id INTO loc2_id;
    END IF;

    FOR col IN
      SELECT column_name, data_type FROM information_schema.columns
       WHERE table_name = 'locations' AND is_nullable = 'YES'
         AND data_type IN ('text','numeric','integer')
         -- google_id stays NULL. There is a unique index on (google_id, active),
         -- and rows created through the API leave it NULL, so blanking it to ''
         -- collides as soon as a second shop exists.
         AND column_name <> 'google_id'
    LOOP
      EXECUTE format(
        'UPDATE locations SET %I = COALESCE(%I, %s) WHERE id = \$1',
        col.column_name, col.column_name,
        CASE WHEN col.data_type = 'text' THEN '''''' ELSE '0' END
      ) USING loc2_id;
    END LOOP;
  END IF;

  RAISE NOTICE 'account % is now a merchant (onboarding: %)', uid,
    CASE WHEN $ONBOARDING_DONE IS NULL THEN 'pending' ELSE 'complete' END;
END
\$\$;

SELECT u.id, u.account_type, u.is_merchant,
       u.merchant_onboarding_completed_at IS NOT NULL AS onboarded,
       l.name, l.approval, l.payment_wallet_address
FROM users u
JOIN locations l ON l.owner_id = u.id AND l.reference = 'maestro-seed'
WHERE u.id = (SELECT id FROM users
              WHERE id = '$TARGET'
                 OR primary_wallet_address ILIKE '$TARGET'
                 OR id IN (SELECT owner FROM wallets
                           WHERE eoa_address ILIKE '$TARGET'
                              OR coalesce(smart_address,'') ILIKE '$TARGET')
              LIMIT 1);
SQL
