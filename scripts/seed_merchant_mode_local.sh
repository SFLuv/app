#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
BACKEND_ENV="$PROJECT_ROOT/backend/.env"

env_file_value() {
  key="$1"
  if [ ! -f "$BACKEND_ENV" ]; then
    return 0
  fi
  awk -v key="$key" '
    $0 ~ "^[[:space:]]*" key "=" {
      sub("^[[:space:]]*" key "=", "")
      gsub(/^[[:space:]]+|[[:space:]]+$/, "")
      gsub(/^"|"$/, "")
      print
    }
  ' "$BACKEND_ENV" | tail -n 1
}

CONFIRM="${CONFIRM_LOCAL_MERCHANT_MODE_SEED:-}"
if [ "$CONFIRM" != "YES" ]; then
  cat >&2 <<'MSG'
Refusing to seed without confirmation.

Run with:
  CONFIRM_LOCAL_MERCHANT_MODE_SEED=YES ./scripts/seed_merchant_mode_local.sh

Optional:
  TEST_USER_EMAIL=sanchez@oleary.com
  TEST_USER_ID=did:privy:...
MSG
  exit 2
fi

DB_USER_VALUE="${DB_USER:-$(env_file_value DB_USER)}"
DB_PASSWORD_VALUE="${DB_PASSWORD:-$(env_file_value DB_PASSWORD)}"
DB_BASE_VALUE="${DB_BASE_URL:-$(env_file_value DB_BASE_URL)}"
DB_URL_VALUE="${DB_URL:-$(env_file_value DB_URL)}"
APP_DB_NAME_VALUE="${APP_DB_NAME:-$(env_file_value APP_DB_NAME)}"

DB_USER_VALUE="${DB_USER_VALUE:-postgres}"
DB_PATH_VALUE="${DB_BASE_VALUE:-${DB_URL_VALUE:-localhost:5432}}"
APP_DB_NAME_VALUE="${APP_DB_NAME_VALUE:-app}"

case "$DB_PATH_VALUE" in
  localhost|localhost:*|127.0.0.1|127.0.0.1:*|::1|::1:*)
    ;;
  *)
    if [ "${ALLOW_NONLOCAL_DB:-}" != "YES" ]; then
      echo "Refusing to seed non-local DB target: $DB_PATH_VALUE" >&2
      echo "Set ALLOW_NONLOCAL_DB=YES only if this is a disposable dev database." >&2
      exit 2
    fi
    ;;
esac

DB_HOST="$DB_PATH_VALUE"
DB_PORT="5432"
case "$DB_PATH_VALUE" in
  *:*)
    DB_HOST="${DB_PATH_VALUE%%:*}"
    DB_PORT="${DB_PATH_VALUE##*:}"
    ;;
esac

TEST_USER_EMAIL_VALUE="${TEST_USER_EMAIL:-sanchez@oleary.com}"
TEST_USER_ID_VALUE="${TEST_USER_ID:-}"

export PGPASSWORD="$DB_PASSWORD_VALUE"

psql \
  -h "$DB_HOST" \
  -p "$DB_PORT" \
  -U "$DB_USER_VALUE" \
  -d "$APP_DB_NAME_VALUE" \
  -v ON_ERROR_STOP=1 \
  -v test_user_email="$TEST_USER_EMAIL_VALUE" \
  -v test_user_id="$TEST_USER_ID_VALUE" <<'SQL'
CREATE TEMP TABLE merchant_mode_seed_config AS
SELECT
  NULLIF(:'test_user_id', '')::TEXT AS user_id_override,
  LOWER(NULLIF(:'test_user_email', ''))::TEXT AS test_user_email;

DO $$
DECLARE
  owner_id TEXT;
  owner_email TEXT;
  primary_wallet TEXT;
  fallback_eoa TEXT := '0x1000000000000000000000000000000000000001';
  fallback_smart TEXT := '0x2000000000000000000000000000000000000001';
BEGIN
  SELECT user_id_override, test_user_email
  INTO owner_id, owner_email
  FROM merchant_mode_seed_config;

  IF owner_id IS NULL THEN
    SELECT u.id
    INTO owner_id
    FROM users u
    WHERE u.active = TRUE
      AND LOWER(TRIM(COALESCE(u.contact_email, ''))) = owner_email
    ORDER BY u.id
    LIMIT 1;
  END IF;

  IF owner_id IS NULL THEN
    SELECT uve.user_id
    INTO owner_id
    FROM user_verified_emails uve
    JOIN users u ON u.id = uve.user_id AND u.active = TRUE
    WHERE uve.active = TRUE
      AND LOWER(TRIM(uve.email)) = owner_email
    ORDER BY uve.verified_at DESC NULLS LAST, uve.created_at DESC
    LIMIT 1;
  END IF;

  IF owner_id IS NULL THEN
    RAISE EXCEPTION 'No local user found for %. Sign in once against the local backend first, or rerun with TEST_USER_ID=did:privy:...', owner_email;
  END IF;

  INSERT INTO users (id, contact_email, contact_name)
  VALUES (owner_id, owner_email, 'Sanchez O''Leary')
  ON CONFLICT (id) DO NOTHING;

  UPDATE users
  SET
    is_merchant = TRUE,
    accepted_privacy_policy = TRUE,
    accepted_privacy_policy_at = COALESCE(accepted_privacy_policy_at, NOW()),
    privacy_policy_version = CASE WHEN privacy_policy_version = '' THEN '2026-04-15' ELSE privacy_policy_version END,
    contact_email = COALESCE(NULLIF(TRIM(contact_email), ''), owner_email),
    contact_name = COALESCE(NULLIF(TRIM(contact_name), ''), 'Sanchez O''Leary')
  WHERE id = owner_id;

  SELECT COALESCE(
    NULLIF(TRIM(u.primary_wallet_address), ''),
    (
      SELECT NULLIF(TRIM(w.smart_address), '')
      FROM wallets w
      WHERE w.owner = owner_id
        AND w.active = TRUE
        AND w.is_eoa = FALSE
        AND w.smart_address IS NOT NULL
      ORDER BY w.smart_index NULLS LAST, w.id
      LIMIT 1
    ),
    (
      SELECT NULLIF(TRIM(w.eoa_address), '')
      FROM wallets w
      WHERE w.owner = owner_id
        AND w.active = TRUE
      ORDER BY w.id
      LIMIT 1
    )
  )
  INTO primary_wallet
  FROM users u
  WHERE u.id = owner_id;

  IF COALESCE(primary_wallet, '') = '' THEN
    INSERT INTO wallets (owner, name, is_eoa, is_hidden, eoa_address, smart_address, smart_index)
    VALUES
      (owner_id, 'Owner EOA', TRUE, TRUE, fallback_eoa, NULL, NULL),
      (owner_id, 'Tenderloin Till', FALSE, FALSE, fallback_eoa, fallback_smart, 0)
    ON CONFLICT (owner, is_eoa, eoa_address, smart_index) WHERE active = TRUE
    DO UPDATE SET name = EXCLUDED.name;

    primary_wallet := fallback_smart;
  END IF;

  UPDATE users
  SET primary_wallet_address = primary_wallet
  WHERE id = owner_id
    AND COALESCE(NULLIF(TRIM(primary_wallet_address), ''), '') = '';

  DELETE FROM location_payment_wallets lpw
  USING locations l
  WHERE lpw.location_id = l.id
    AND l.google_id LIKE 'local-merchant-mode-%';

  DELETE FROM location_hours lh
  USING locations l
  WHERE lh.location_id = l.id
    AND l.google_id LIKE 'local-merchant-mode-%';

  DELETE FROM locations
  WHERE google_id LIKE 'local-merchant-mode-%';

  DELETE FROM wallets
  WHERE owner LIKE 'local-merchant-mode-owner-%';

  DELETE FROM users
  WHERE id LIKE 'local-merchant-mode-owner-%';

  INSERT INTO users (id, is_merchant, contact_email, contact_name, primary_wallet_address, accepted_privacy_policy, accepted_privacy_policy_at, privacy_policy_version)
  VALUES
    ('local-merchant-mode-owner-1', TRUE, 'civic-books@sfluv.local', 'Civic Center Books', '0x3000000000000000000000000000000000000001', TRUE, NOW(), '2026-04-15'),
    ('local-merchant-mode-owner-2', TRUE, 'little-saigon@sfluv.local', 'Little Saigon Hardware', '0x3000000000000000000000000000000000000002', TRUE, NOW(), '2026-04-15')
  ON CONFLICT (id) DO UPDATE SET
    is_merchant = TRUE,
    contact_email = EXCLUDED.contact_email,
    contact_name = EXCLUDED.contact_name,
    primary_wallet_address = EXCLUDED.primary_wallet_address,
    active = TRUE,
    delete_date = NULL,
    delete_reason = NULL;

  INSERT INTO wallets (owner, name, is_eoa, is_hidden, eoa_address, smart_address, smart_index)
  VALUES
    ('local-merchant-mode-owner-1', 'Civic Books Till', FALSE, FALSE, '0x4000000000000000000000000000000000000001', '0x3000000000000000000000000000000000000001', 0),
    ('local-merchant-mode-owner-2', 'Hardware Counter', FALSE, FALSE, '0x4000000000000000000000000000000000000002', '0x3000000000000000000000000000000000000002', 0)
  ON CONFLICT (owner, is_eoa, eoa_address, smart_index) WHERE active = TRUE
  DO UPDATE SET
    name = EXCLUDED.name,
    smart_address = EXCLUDED.smart_address,
    is_hidden = FALSE,
    active = TRUE,
    delete_date = NULL,
    delete_reason = NULL;

  INSERT INTO locations (
    google_id, owner_id, name, description, type, approval, approved_at,
    street, city, state, zip, lat, lng, phone, email, admin_phone, admin_email,
    website, image_url, rating, maps_page, contact_firstname, contact_lastname,
    contact_phone, pos_system, sole_proprietorship, tipping_policy, tipping_division,
    table_coverage, service_stations, tablet_model, messaging_service, reference
  )
  VALUES
    (
      'local-merchant-mode-sfluv-corner-market', owner_id, 'SFLuv Corner Market',
      'A neighborhood market set up for local merchant-mode testing.', 'Market', TRUE, NOW(),
      '100 Turk St', 'San Francisco', 'CA', '94102', 37.783960, -122.410930,
      '(415) 555-0101', 'corner-market@sfluv.local', '(415) 555-0199', owner_email,
      'https://sfluv.org', '', 4.7, 'https://maps.google.com/?q=100+Turk+St+San+Francisco',
      'Sanchez', 'O''Leary', '(415) 555-0199', 'Square', 'No',
      'Tips are shared by the shift team.', 'Shift pool', 'Counter service', 2,
      'iPad 10th generation', 'SMS', 'local merchant mode owner location'
    ),
    (
      'local-merchant-mode-sfluv-community-cafe', owner_id, 'SFLuv Community Cafe',
      'A realistic second location for testing location-scoped merchant mode.', 'Cafe', TRUE, NOW(),
      '525 Golden Gate Ave', 'San Francisco', 'CA', '94102', 37.781440, -122.418580,
      '(415) 555-0102', 'community-cafe@sfluv.local', '(415) 555-0199', owner_email,
      'https://sfluv.org', '', 4.8, 'https://maps.google.com/?q=525+Golden+Gate+Ave+San+Francisco',
      'Sanchez', 'O''Leary', '(415) 555-0199', 'Toast', 'No',
      'Tips go to the cafe staff on duty.', 'Shift pool', 'Counter service', 3,
      'iPad mini', 'Email', 'local merchant mode second owner location'
    ),
    (
      'local-merchant-mode-civic-center-books', 'local-merchant-mode-owner-1', 'Civic Center Books & Cafe',
      'Independent books, coffee, and community events near Civic Center.', 'Bookstore', TRUE, NOW(),
      '1164 Market St', 'San Francisco', 'CA', '94102', 37.779650, -122.413310,
      '(415) 555-0111', 'hello@civicbooks.local', '(415) 555-0112', 'ops@civicbooks.local',
      'https://example.com/civic-books', '', 4.6, 'https://maps.google.com/?q=1164+Market+St+San+Francisco',
      'Maya', 'Rivera', '(415) 555-0112', 'Clover', 'No',
      'Tips are split across cafe staff.', 'Shift pool', 'Counter service', 2,
      'iPad Air', 'Email', 'local merchant mode public map seed'
    ),
    (
      'local-merchant-mode-little-saigon-hardware', 'local-merchant-mode-owner-2', 'Little Saigon Hardware',
      'A practical neighborhood hardware shop with household essentials.', 'Retail', TRUE, NOW(),
      '645 Larkin St', 'San Francisco', 'CA', '94109', 37.783610, -122.417940,
      '(415) 555-0121', 'counter@lshardware.local', '(415) 555-0122', 'owner@lshardware.local',
      'https://example.com/little-saigon-hardware', '', 4.5, 'https://maps.google.com/?q=645+Larkin+St+San+Francisco',
      'An', 'Nguyen', '(415) 555-0122', 'Shopify POS', 'No',
      'No tipping expected.', 'N/A', 'Single register', 1,
      'Android tablet', 'SMS', 'local merchant mode public map seed'
    );

  INSERT INTO location_payment_wallets (location_id, wallet_address, is_default)
  SELECT id, primary_wallet, TRUE
  FROM locations
  WHERE google_id IN (
    'local-merchant-mode-sfluv-corner-market',
    'local-merchant-mode-sfluv-community-cafe'
  );

  INSERT INTO location_payment_wallets (location_id, wallet_address, is_default)
  SELECT l.id, u.primary_wallet_address, TRUE
  FROM locations l
  JOIN users u ON u.id = l.owner_id
  WHERE l.google_id IN (
    'local-merchant-mode-civic-center-books',
    'local-merchant-mode-little-saigon-hardware'
  );

  INSERT INTO location_hours (location_id, weekday, hours)
  SELECT l.id, day.weekday, day.hours
  FROM locations l
  CROSS JOIN (
    VALUES
      (0, 'Monday 8:00 AM - 8:00 PM'),
      (1, 'Tuesday 8:00 AM - 8:00 PM'),
      (2, 'Wednesday 8:00 AM - 8:00 PM'),
      (3, 'Thursday 8:00 AM - 8:00 PM'),
      (4, 'Friday 8:00 AM - 9:00 PM'),
      (5, 'Saturday 9:00 AM - 9:00 PM'),
      (6, 'Sunday 9:00 AM - 6:00 PM')
  ) AS day(weekday, hours)
  WHERE l.google_id LIKE 'local-merchant-mode-%';

  RAISE NOTICE 'Seeded merchant mode owner %, wallet %, email %', owner_id, primary_wallet, owner_email;
END
$$;

SELECT
  l.id,
  l.name,
  l.owner_id,
  COALESCE(lpw.wallet_address, '') AS default_payment_wallet
FROM locations l
LEFT JOIN location_payment_wallets lpw
  ON lpw.location_id = l.id
  AND lpw.active = TRUE
  AND lpw.is_default = TRUE
WHERE l.google_id LIKE 'local-merchant-mode-%'
ORDER BY l.owner_id, l.name;
SQL
