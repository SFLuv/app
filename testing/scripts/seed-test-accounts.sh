#!/usr/bin/env bash
# Creates the three role accounts the browser suites prank into.
#
# Why synthetic accounts rather than real ones: a prankee never authenticates.
# The admin's token authenticates and prankForwardingMiddleware swaps the user
# id in the request context, so the prankee only has to EXIST in the users
# table. That means these can be deterministic rows with fixed ids, instead of
# borrowing prod-derived accounts whose roles and data drift.
#
# What that buys:
#   - a spec can hardcode who it is pranking into and stay correct
#   - test data is obviously test data, by id and by email
#   - nothing writes to an account a real person also uses
#
# The limit: these have no Privy account and no wallet, so anything that signs
# or moves money on chain cannot be done as one of them. They are for panel and
# permission testing — which is what the affiliate and admin surfaces are.
#
#   ./testing/scripts/seed-test-accounts.sh          # create or update
#   ./testing/scripts/seed-test-accounts.sh --revert # remove them
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

# Lowercase alphanumeric only: the e2e harness validates ids against
# ^did:privy:[a-z0-9]+$ and refuses anything else, so no dashes here.
AFFILIATE_ID="did:privy:test0affiliate0sfluv"
MERCHANT_ID="did:privy:test0merchant0sfluv"
VOLUNTEER_ID="did:privy:test0volunteer0sfluv"

REVERT=0
[[ "${1:-}" == "--revert" ]] && REVERT=1

host="${DB_URL%%:*}"; port="${DB_URL##*:}"

if [[ "$REVERT" == "1" ]]; then
  PGPASSWORD="$DB_PASSWORD" psql -h "$host" -p "$port" -U "$DB_USER" \
    -d "${APP_DB_NAME:-app}" -v ON_ERROR_STOP=1 <<REVERTSQL
DELETE FROM locations           WHERE reference = 'test-account-seed';
DELETE FROM organization_roles WHERE organization_id IN (SELECT id FROM organizations WHERE name_normalized = 'sfluv test partner');
DELETE FROM organization_members WHERE user_id IN ('$AFFILIATE_ID','$MERCHANT_ID','$VOLUNTEER_ID');
DELETE FROM organizations        WHERE name_normalized = 'sfluv test partner';
DELETE FROM pranks               WHERE prankee_user_id IN ('$AFFILIATE_ID','$MERCHANT_ID','$VOLUNTEER_ID');
DELETE FROM users                WHERE id IN ('$AFFILIATE_ID','$MERCHANT_ID','$VOLUNTEER_ID');
SELECT 'removed' AS status;
REVERTSQL
  exit 0
fi

PGPASSWORD="$DB_PASSWORD" psql -h "$host" -p "$port" -U "$DB_USER" \
  -d "${APP_DB_NAME:-app}" -v ON_ERROR_STOP=1 <<SQL
DO \$\$
DECLARE
  org_id  int;
  loc_id  int;
  col     record;
BEGIN
  -- Accounts. Upserted so re-running re-asserts the roles rather than failing,
  -- which matters because a spec can leave a role toggled.
  INSERT INTO users (id, contact_email, contact_name, is_affiliate, account_type, active,
                     accepted_privacy_policy)
  VALUES ('$AFFILIATE_ID', 'test-affiliate@sfluv.test', 'Test Affiliate', TRUE, 'regular', TRUE, TRUE)
  ON CONFLICT (id) DO UPDATE
    SET is_affiliate = TRUE, active = TRUE, accepted_privacy_policy = TRUE;

  INSERT INTO users (id, contact_email, contact_name, is_merchant, account_type, active,
                     accepted_privacy_policy, merchant_onboarding_completed_at)
  VALUES ('$MERCHANT_ID', 'test-merchant@sfluv.test', 'Test Merchant', TRUE, 'merchant', TRUE, TRUE, NOW())
  ON CONFLICT (id) DO UPDATE
    SET is_merchant = TRUE, account_type = 'merchant', active = TRUE,
        accepted_privacy_policy = TRUE, merchant_onboarding_completed_at = NOW();

  INSERT INTO users (id, contact_email, contact_name, account_type, active, accepted_privacy_policy)
  VALUES ('$VOLUNTEER_ID', 'test-volunteer@sfluv.test', 'Test Volunteer', 'regular', TRUE, TRUE)
  ON CONFLICT (id) DO UPDATE
    SET active = TRUE, accepted_privacy_policy = TRUE;

  -- The affiliate needs an organization: AffiliateRequestVolunteerEvent refuses
  -- with "you must belong to an organization to request an event" without one,
  -- so an affiliate account alone cannot reach the surface being tested.
  SELECT id INTO org_id FROM organizations WHERE name_normalized = 'sfluv test partner';
  IF org_id IS NULL THEN
    INSERT INTO organizations (name, name_normalized)
    VALUES ('SFLuv Test Partner', 'sfluv test partner')
    RETURNING id INTO org_id;
  END IF;

  INSERT INTO organization_members (organization_id, user_id, role)
  VALUES (org_id, '$AFFILIATE_ID', 'admin')
  ON CONFLICT DO NOTHING;

  -- The affiliate GUARD does not read users.is_affiliate. IsAffiliate joins
  -- organization_members to organization_roles and requires role_type
  -- 'affiliate' with status 'approved' on the org — so the boolean alone
  -- passes every DB check you would think to write and still 403s at the route.
  -- The boolean is kept above because panels and reports do read it; this row
  -- is what actually opens /affiliates/*.
  INSERT INTO organization_roles (organization_id, role_type, status, email, requested_by, approved_at)
  VALUES (org_id, 'affiliate', 'approved', 'test-affiliate@sfluv.test', '$AFFILIATE_ID', NOW())
  ON CONFLICT DO NOTHING;

  -- An approved location, so the merchant account reaches merchant mode rather
  -- than the onboarding gate.
  SELECT id INTO loc_id FROM locations WHERE reference = 'test-account-seed';
  IF loc_id IS NULL THEN
    INSERT INTO locations
      (owner_id, name, description, type, approval, approved_at, street, city, state, zip,
       lat, lng, phone, email, admin_phone, admin_email, contact_firstname, contact_lastname,
       contact_phone, pos_system, sole_proprietorship, tipping_policy, tipping_division,
       location_kind, listing_source, reference, active, website, image_url, maps_page, rating)
    VALUES
      ('$MERCHANT_ID', 'Test Merchant Shop', 'Seeded for browser tests', 'restaurant', TRUE, NOW(),
       '3 Test St', 'San Francisco', 'CA', '94110', 37.7601, -122.4150, '4155550102',
       'test-merchant@sfluv.test', '4155550102', 'test-merchant@sfluv.test', 'Test', 'Merchant',
       '4155550102', 'other', TRUE, 'pooled', 'even', 'merchant', 'manual', 'test-account-seed',
       TRUE, '', '', '', 0)
    RETURNING id INTO loc_id;
  END IF;

  -- google_id stays NULL: the partial unique index covers google_id IS NOT NULL,
  -- so blanking it collides with any other seeded row.
  FOR col IN
    SELECT column_name, data_type FROM information_schema.columns
     WHERE table_name = 'locations' AND is_nullable = 'YES'
       AND data_type IN ('text','numeric','integer')
       AND column_name <> 'google_id'
  LOOP
    EXECUTE format('UPDATE locations SET %I = COALESCE(%I, %s) WHERE id = \$1',
      col.column_name, col.column_name,
      CASE WHEN col.data_type = 'text' THEN '''''' ELSE '0' END) USING loc_id;
  END LOOP;

  RAISE NOTICE 'affiliate % (org %), merchant % (location %), volunteer %',
    '$AFFILIATE_ID', org_id, '$MERCHANT_ID', loc_id, '$VOLUNTEER_ID';
END
\$\$;

SELECT id, contact_email, is_affiliate, is_merchant, account_type
FROM users WHERE id IN ('$AFFILIATE_ID','$MERCHANT_ID','$VOLUNTEER_ID') ORDER BY id;
SQL
