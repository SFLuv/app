package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const baselineDBVersion = "1.0"

type SchemaMigration struct {
	Version     string
	Description string
	Apply       func(context.Context, *DBPools, *logger.LogCloser) error
}

// Everything currently in CreateTables() is treated as baseline schema v1.0.
// Future schema changes should be added here in ascending version order.
var schemaMigrations = []SchemaMigration{
	{
		Version:     "1.1",
		Description: "add support indexes for list, location, wallet, and event queries",
		Apply: func(ctx context.Context, pools *DBPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE INDEX IF NOT EXISTS affiliates_created_idx
					ON affiliates(created_at DESC);
				CREATE INDEX IF NOT EXISTS proposers_created_idx
					ON proposers(created_at DESC);
				CREATE INDEX IF NOT EXISTS improvers_created_idx
					ON improvers(created_at DESC);
				CREATE INDEX IF NOT EXISTS supervisors_created_idx
					ON supervisors(created_at DESC);
				CREATE INDEX IF NOT EXISTS issuers_created_idx
					ON issuers(created_at DESC);
				CREATE INDEX IF NOT EXISTS wallets_owner_id_idx
					ON wallets(owner, id);
				CREATE INDEX IF NOT EXISTS wallets_owner_smart_default_idx
					ON wallets(owner, smart_index, id)
					WHERE is_eoa = FALSE;
				CREATE INDEX IF NOT EXISTS locations_approval_id_idx
					ON locations(approval, id);
				CREATE INDEX IF NOT EXISTS locations_owner_approval_id_idx
					ON locations(owner_id, approval, id);
				CREATE INDEX IF NOT EXISTS location_hours_location_weekday_idx
					ON location_hours(location_id, weekday);
				CREATE INDEX IF NOT EXISTS contacts_owner_favorite_id_idx
					ON contacts(owner, is_favorite DESC, id ASC);
				CREATE INDEX IF NOT EXISTS issuer_credential_scopes_credential_issuer_idx
					ON issuer_credential_scopes(credential_type, issuer_id);
				CREATE INDEX IF NOT EXISTS workflow_templates_owner_created_idx
					ON workflow_templates(owner_user_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS workflow_templates_default_created_idx
					ON workflow_templates(is_default, created_at DESC);
			`); err != nil {
				return err
			}

			if _, err := pools.Bot.Exec(ctx, `
				CREATE INDEX IF NOT EXISTS events_expiration_id_idx
					ON events(expiration, id);
				CREATE INDEX IF NOT EXISTS events_owner_expiration_id_idx
					ON events(owner, expiration, id);
				CREATE INDEX IF NOT EXISTS codes_event_id_idx
					ON codes(event, id);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.2",
		Description: "add location-owned payment wallets and tipping wallets for merchant payouts",
		Apply: func(ctx context.Context, pools *DBPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS tipping_wallet_address TEXT NOT NULL DEFAULT '';

				CREATE TABLE IF NOT EXISTS location_payment_wallets(
					id SERIAL PRIMARY KEY,
					location_id INTEGER NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
					wallet_address TEXT NOT NULL,
					is_default BOOLEAN NOT NULL DEFAULT false,
					UNIQUE (location_id, wallet_address)
				);

				CREATE INDEX IF NOT EXISTS location_payment_wallets_location_idx
					ON location_payment_wallets(location_id);

				CREATE UNIQUE INDEX IF NOT EXISTS location_payment_wallets_default_idx
					ON location_payment_wallets(location_id)
					WHERE is_default = TRUE;
			`); err != nil {
				return err
			}

			if _, err := pools.App.Exec(ctx, `
				DO $$
				BEGIN
					IF EXISTS (
						SELECT 1
						FROM information_schema.columns
						WHERE table_name = 'users'
						AND column_name = 'tipping_wallet_address'
					) THEN
						EXECUTE $sql$
							UPDATE locations l
							SET tipping_wallet_address = TRIM(COALESCE(u.tipping_wallet_address, ''))
							FROM users u
							WHERE l.owner_id = u.id
							AND TRIM(COALESCE(l.tipping_wallet_address, '')) = ''
							AND TRIM(COALESCE(u.tipping_wallet_address, '')) <> ''
						$sql$;
					END IF;
				END
				$$;
			`); err != nil {
				return err
			}

			return nil
		},
	},
}

type versionTarget struct {
	name          string
	pool          *pgxpool.Pool
	requiredTable string
}

func latestDBVersion() string {
	if len(schemaMigrations) == 0 {
		return baselineDBVersion
	}
	return schemaMigrations[len(schemaMigrations)-1].Version
}

func RunPendingMigrations(ctx context.Context, pools *DBPools, appLogger *logger.LogCloser) error {
	if pools == nil || pools.App == nil || pools.Bot == nil {
		return fmt.Errorf("app and bot db pools are required")
	}

	if err := validateMigrationSequence(); err != nil {
		return err
	}

	targets := []versionTarget{
		{name: "app", pool: pools.App, requiredTable: "users"},
		{name: "bot", pool: pools.Bot, requiredTable: "events"},
	}

	currentVersion, err := ensureConsistentDBVersion(ctx, targets, appLogger)
	if err != nil {
		return err
	}

	if err := ensureVersionIsKnown(currentVersion); err != nil {
		return err
	}

	latestVersion := latestDBVersion()
	cmp, err := compareVersions(currentVersion, latestVersion)
	if err != nil {
		return err
	}
	if cmp > 0 {
		return fmt.Errorf("database version %s is newer than server schema version %s", currentVersion, latestVersion)
	}
	if cmp == 0 {
		if appLogger != nil {
			appLogger.Logf("database schema already at version %s", currentVersion)
		}
		return nil
	}

	for _, migration := range schemaMigrations {
		isAfterCurrent, err := isVersionGreater(migration.Version, currentVersion)
		if err != nil {
			return err
		}
		if !isAfterCurrent {
			continue
		}

		if appLogger != nil {
			appLogger.Logf("applying schema migration %s: %s", migration.Version, migration.Description)
		}
		if err := migration.Apply(ctx, pools, appLogger); err != nil {
			return fmt.Errorf("error applying schema migration %s (%s): %w", migration.Version, migration.Description, err)
		}
		if err := setVersionForTargets(ctx, targets, migration.Version); err != nil {
			return fmt.Errorf("error updating database version to %s: %w", migration.Version, err)
		}
		currentVersion = migration.Version
	}

	if appLogger != nil {
		appLogger.Logf("database schema updated to version %s", currentVersion)
	}

	return nil
}

func validateMigrationSequence() error {
	previousVersion := baselineDBVersion
	seen := map[string]struct{}{}

	for _, migration := range schemaMigrations {
		if migration.Version == "" {
			return fmt.Errorf("schema migration is missing a version")
		}
		if migration.Description == "" {
			return fmt.Errorf("schema migration %s is missing a description", migration.Version)
		}
		if migration.Apply == nil {
			return fmt.Errorf("schema migration %s is missing an apply function", migration.Version)
		}
		if _, exists := seen[migration.Version]; exists {
			return fmt.Errorf("duplicate schema migration version %s", migration.Version)
		}
		seen[migration.Version] = struct{}{}

		cmp, err := compareVersions(migration.Version, previousVersion)
		if err != nil {
			return err
		}
		if cmp <= 0 {
			return fmt.Errorf("schema migration versions must be strictly increasing: %s came after %s", migration.Version, previousVersion)
		}
		previousVersion = migration.Version
	}

	return nil
}

func ensureVersionIsKnown(version string) error {
	if version == baselineDBVersion {
		return nil
	}
	for _, migration := range schemaMigrations {
		if migration.Version == version {
			return nil
		}
	}
	return fmt.Errorf("unsupported database version %s; run backend/cmd/init or add the missing migration batch", version)
}

func ensureConsistentDBVersion(ctx context.Context, targets []versionTarget, appLogger *logger.LogCloser) (string, error) {
	var currentVersion string

	for _, target := range targets {
		version, err := ensureTargetVersion(ctx, target)
		if err != nil {
			return "", err
		}
		if currentVersion == "" {
			currentVersion = version
			continue
		}
		if version != currentVersion {
			return "", fmt.Errorf("database version mismatch: %s=%s, expected %s", target.name, version, currentVersion)
		}
	}

	if currentVersion == "" {
		currentVersion = baselineDBVersion
	}

	if appLogger != nil {
		appLogger.Logf("database schema version is %s", currentVersion)
	}

	return currentVersion, nil
}

func ensureTargetVersion(ctx context.Context, target versionTarget) (string, error) {
	if err := ensureVersionTable(ctx, target.pool); err != nil {
		return "", fmt.Errorf("error ensuring db_version table in %s database: %w", target.name, err)
	}

	version, exists, err := getCurrentVersion(ctx, target.pool)
	if err != nil {
		return "", fmt.Errorf("error reading %s database version: %w", target.name, err)
	}
	if exists {
		return version, nil
	}

	hasBaselineSchema, err := tableExists(ctx, target.pool, target.requiredTable)
	if err != nil {
		return "", fmt.Errorf("error checking %s baseline schema table %s: %w", target.name, target.requiredTable, err)
	}
	if !hasBaselineSchema {
		return "", fmt.Errorf("%s database is missing baseline schema table %s and has no db_version row; run backend/cmd/init", target.name, target.requiredTable)
	}

	if err := setCurrentVersion(ctx, target.pool, baselineDBVersion); err != nil {
		return "", fmt.Errorf("error seeding %s database version to %s: %w", target.name, baselineDBVersion, err)
	}

	return baselineDBVersion, nil
}

func setVersionForTargets(ctx context.Context, targets []versionTarget, version string) error {
	for _, target := range targets {
		if err := setCurrentVersion(ctx, target.pool, version); err != nil {
			return fmt.Errorf("error updating %s database version: %w", target.name, err)
		}
	}
	return nil
}

func ensureVersionTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS db_version(
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			version TEXT NOT NULL,
			updated_at BIGINT NOT NULL
		);
	`)
	return err
}

func getCurrentVersion(ctx context.Context, pool *pgxpool.Pool) (string, bool, error) {
	row := pool.QueryRow(ctx, `
		SELECT
			version
		FROM
			db_version
		WHERE
			id = 1;
	`)

	var version string
	if err := row.Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return version, true, nil
}

func setCurrentVersion(ctx context.Context, pool *pgxpool.Pool, version string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO db_version(id, version, updated_at)
		VALUES (1, $1, $2)
		ON CONFLICT (id)
		DO UPDATE SET
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at;
	`, version, time.Now().Unix())
	return err
}

func tableExists(ctx context.Context, pool *pgxpool.Pool, tableName string) (bool, error) {
	row := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT
				1
			FROM
				information_schema.tables
			WHERE
				table_schema = 'public'
			AND
				table_name = $1
		);
	`, tableName)

	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func isVersionGreater(left, right string) (bool, error) {
	cmp, err := compareVersions(left, right)
	if err != nil {
		return false, err
	}
	return cmp > 0, nil
}

func compareVersions(left, right string) (int, error) {
	leftParts, err := parseVersion(left)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", left, err)
	}
	rightParts, err := parseVersion(right)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", right, err)
	}

	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}

	for index := 0; index < maxLen; index += 1 {
		leftValue := 0
		if index < len(leftParts) {
			leftValue = leftParts[index]
		}
		rightValue := 0
		if index < len(rightParts) {
			rightValue = rightParts[index]
		}
		if leftValue < rightValue {
			return -1, nil
		}
		if leftValue > rightValue {
			return 1, nil
		}
	}

	return 0, nil
}

func parseVersion(version string) ([]int, error) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return nil, fmt.Errorf("empty version")
	}

	parts := strings.Split(trimmed, ".")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("invalid empty version segment")
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid numeric segment %q", part)
		}
		if value < 0 {
			return nil, fmt.Errorf("negative version segment %q", part)
		}
		values = append(values, value)
	}
	return values, nil
}
