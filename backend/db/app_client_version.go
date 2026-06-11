package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

func normalizeClientVersionLabel(version string, build string) string {
	version = strings.TrimSpace(version)
	build = strings.TrimSpace(build)
	if version == "" {
		version = "unknown"
	}
	if build == "" {
		return version
	}
	return fmt.Sprintf("%s (%s)", version, build)
}

func (a *AppDB) RecordClientVersionObservation(ctx context.Context, observation structs.ClientVersionObservation) error {
	if strings.TrimSpace(observation.ClientKey) == "" {
		return nil
	}

	seenAt := observation.SeenAt
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}

	var userID any
	if strings.TrimSpace(observation.UserId) != "" {
		userID = strings.TrimSpace(observation.UserId)
	}

	_, err := a.db.Exec(ctx, `
		INSERT INTO user_client_versions(
			user_id,
			client_key,
			platform,
			version,
			build,
			build_number,
			user_agent,
			source,
			legacy_inferred,
			first_seen_at,
			last_seen_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
		ON CONFLICT (client_key)
		DO UPDATE SET
			user_id = COALESCE(EXCLUDED.user_id, user_client_versions.user_id),
			platform = CASE
				WHEN EXCLUDED.platform <> '' THEN EXCLUDED.platform
				ELSE user_client_versions.platform
			END,
			version = CASE
				WHEN EXCLUDED.version <> '' THEN EXCLUDED.version
				ELSE user_client_versions.version
			END,
			build = CASE
				WHEN EXCLUDED.build <> '' THEN EXCLUDED.build
				ELSE user_client_versions.build
			END,
			build_number = CASE
				WHEN EXCLUDED.build_number > 0 THEN EXCLUDED.build_number
				ELSE user_client_versions.build_number
			END,
			user_agent = CASE
				WHEN EXCLUDED.user_agent <> '' THEN EXCLUDED.user_agent
				ELSE user_client_versions.user_agent
			END,
			source = CASE
				WHEN EXCLUDED.source <> '' THEN EXCLUDED.source
				ELSE user_client_versions.source
			END,
			legacy_inferred = EXCLUDED.legacy_inferred,
			first_seen_at = LEAST(user_client_versions.first_seen_at, EXCLUDED.first_seen_at),
			last_seen_at = GREATEST(user_client_versions.last_seen_at, EXCLUDED.last_seen_at);
	`,
		userID,
		strings.TrimSpace(observation.ClientKey),
		strings.TrimSpace(observation.Platform),
		strings.TrimSpace(observation.Version),
		strings.TrimSpace(observation.Build),
		observation.BuildNumber,
		strings.TrimSpace(observation.UserAgent),
		strings.TrimSpace(observation.Source),
		observation.LegacyInferred,
		seenAt,
	)
	if err != nil {
		return fmt.Errorf("error recording client version observation: %s", err)
	}

	return nil
}

func (a *AppDB) AttachClientVersionDevices(ctx context.Context, users []*structs.User) error {
	if len(users) == 0 {
		return nil
	}

	userIDs := make([]string, 0, len(users))
	userByID := make(map[string]*structs.User, len(users))
	for _, user := range users {
		if user == nil || user.Id == "" {
			continue
		}
		userIDs = append(userIDs, user.Id)
		user.ClientDevices = []*structs.ClientVersionDevice{}
		userByID[user.Id] = user
	}
	if len(userIDs) == 0 {
		return nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			COALESCE(user_id, ''),
			platform,
			version,
			build,
			source,
			legacy_inferred,
			first_seen_at,
			last_seen_at
		FROM
			user_client_versions
		WHERE
			user_id = ANY($1)
		ORDER BY
			user_id ASC,
			last_seen_at DESC,
			id DESC;
	`, userIDs)
	if err != nil {
		return fmt.Errorf("error getting client version devices: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		device := &structs.ClientVersionDevice{}
		if err := rows.Scan(
			&device.Id,
			&device.UserId,
			&device.Platform,
			&device.Version,
			&device.Build,
			&device.Source,
			&device.LegacyInferred,
			&device.FirstSeenAt,
			&device.LastSeenAt,
		); err != nil {
			return fmt.Errorf("error scanning client version device: %s", err)
		}
		device.VersionLabel = normalizeClientVersionLabel(device.Version, device.Build)
		if user := userByID[device.UserId]; user != nil {
			user.ClientDevices = append(user.ClientDevices, device)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error reading client version devices: %s", err)
	}

	return nil
}

func (a *AppDB) GetClientVersionFilterOptions(ctx context.Context) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			CASE
				WHEN TRIM(version) = '' THEN 'unknown'
				WHEN TRIM(build) <> '' THEN TRIM(version) || ' (' || TRIM(build) || ')'
				ELSE TRIM(version)
			END AS version_label
		FROM
			user_client_versions
		WHERE
			user_id IS NOT NULL
		GROUP BY
			1
		ORDER BY
			MAX(last_seen_at) DESC
		LIMIT 100;
	`)
	if err != nil {
		return nil, fmt.Errorf("error getting client version filter options: %s", err)
	}
	defer rows.Close()

	options := []string{}
	for rows.Next() {
		var option string
		if err := rows.Scan(&option); err != nil {
			return nil, fmt.Errorf("error scanning client version filter option: %s", err)
		}
		options = append(options, option)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading client version filter options: %s", err)
	}

	return options, nil
}

func (a *AppDB) GetClientVersionUserCounts(ctx context.Context) ([]*structs.ClientVersionUserCount, error) {
	rows, err := a.db.Query(ctx, `
		WITH active_users AS (
			SELECT
				id
			FROM
				users
			WHERE
				active = TRUE
		),
		latest_versions AS (
			SELECT DISTINCT ON (ucv.user_id)
				ucv.user_id,
				TRIM(ucv.version) AS version,
				TRIM(ucv.build) AS build,
				ucv.legacy_inferred,
				ucv.last_seen_at
			FROM
				user_client_versions ucv
			INNER JOIN
				active_users au
			ON
				au.id = ucv.user_id
			ORDER BY
				ucv.user_id,
				ucv.last_seen_at DESC,
				ucv.id DESC
		),
		version_counts AS (
			SELECT
				CASE
					WHEN COALESCE(lv.version, '') = '' THEN 'Unknown / no version'
					WHEN COALESCE(lv.build, '') <> '' THEN lv.version || ' (' || lv.build || ')'
					ELSE lv.version
				END AS version_label,
				COALESCE(lv.version, '') AS version,
				COALESCE(lv.build, '') AS build,
				COALESCE(lv.legacy_inferred, FALSE) AS legacy_inferred,
				lv.user_id IS NULL AS unknown,
				COUNT(*)::INTEGER AS user_count,
				MAX(lv.last_seen_at) AS latest_seen
			FROM
				active_users au
			LEFT JOIN
				latest_versions lv
			ON
				lv.user_id = au.id
			GROUP BY
				1, 2, 3, 4, 5
		)
		SELECT
			version_label,
			version,
			build,
			user_count,
			legacy_inferred,
			unknown
		FROM
			version_counts
		ORDER BY
			unknown ASC,
			legacy_inferred DESC,
			latest_seen DESC NULLS LAST,
			user_count DESC,
			version_label ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error getting client version user counts: %s", err)
	}
	defer rows.Close()

	counts := []*structs.ClientVersionUserCount{}
	for rows.Next() {
		count := &structs.ClientVersionUserCount{}
		if err := rows.Scan(
			&count.VersionLabel,
			&count.Version,
			&count.Build,
			&count.UserCount,
			&count.LegacyInferred,
			&count.Unknown,
		); err != nil {
			return nil, fmt.Errorf("error scanning client version user count: %s", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading client version user counts: %s", err)
	}

	return counts, nil
}
