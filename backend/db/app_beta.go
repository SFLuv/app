package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

// UserHasBetaFeature is the question every gate asks.
//
// Active means granted and not since revoked. A revoked row stays in the table
// — see the 1.69 migration — so this cannot be a plain existence check.
func (a *AppDB) UserHasBetaFeature(ctx context.Context, userId string, feature string) (bool, error) {
	userId = strings.TrimSpace(userId)
	feature = strings.TrimSpace(feature)
	if userId == "" || feature == "" {
		return false, nil
	}

	var active bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM beta_feature_access
			WHERE feature = $1 AND user_id = $2 AND revoked_at IS NULL
		);
	`, feature, userId).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("error checking beta feature %s for %s: %w", feature, userId, err)
	}
	return active, nil
}

// BetaFeaturesForUser lists the features a person currently holds, for the
// client to gate its own surfaces with. One round trip rather than one per
// feature, because the client asks for all of them at once.
func (a *AppDB) BetaFeaturesForUser(ctx context.Context, userId string) ([]string, error) {
	userId = strings.TrimSpace(userId)
	if userId == "" {
		return []string{}, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT feature FROM beta_feature_access
		WHERE user_id = $1 AND revoked_at IS NULL
		ORDER BY feature;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error listing beta features for %s: %w", userId, err)
	}
	defer rows.Close()

	features := []string{}
	for rows.Next() {
		var feature string
		if err := rows.Scan(&feature); err != nil {
			return nil, fmt.Errorf("error scanning beta feature for %s: %w", userId, err)
		}
		// A grant for a feature this build no longer declares is ignored rather
		// than returned: the client would not know what to do with it, and it
		// would read as access to something that does not exist.
		if structs.IsBetaFeature(feature) {
			features = append(features, feature)
		}
	}
	return features, rows.Err()
}

// UsersWithBetaFeature is the active membership, for deciding who may be
// messaged and who still gets pushes.
func (a *AppDB) UsersWithBetaFeature(ctx context.Context, feature string) (map[string]struct{}, error) {
	rows, err := a.db.Query(ctx, `
		SELECT user_id FROM beta_feature_access
		WHERE feature = $1 AND revoked_at IS NULL;
	`, strings.TrimSpace(feature))
	if err != nil {
		return nil, fmt.Errorf("error listing members of beta feature %s: %w", feature, err)
	}
	defer rows.Close()

	members := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning beta member: %w", err)
		}
		members[id] = struct{}{}
	}
	return members, rows.Err()
}

// GrantBetaFeature lets somebody in, or back in.
//
// The upsert clears revoked_at rather than inserting a second row, which is
// what makes a re-grant a restoration: the conversations and posts the person
// already had are still theirs, because nothing was ever detached from them.
// granted_at is refreshed so the panel orders by the current decision.
func (a *AppDB) GrantBetaFeature(ctx context.Context, feature string, userId string, grantedBy string) error {
	feature = strings.TrimSpace(feature)
	userId = strings.TrimSpace(userId)
	if !structs.IsBetaFeature(feature) {
		return fmt.Errorf("unknown beta feature %q", feature)
	}
	if userId == "" {
		return fmt.Errorf("no user to grant to")
	}

	var by any
	if trimmed := strings.TrimSpace(grantedBy); trimmed != "" {
		by = trimmed
	}

	_, err := a.db.Exec(ctx, `
		INSERT INTO beta_feature_access(feature, user_id, granted_at, granted_by)
		VALUES ($1, $2, NOW(), $3)
		ON CONFLICT (feature, user_id) DO UPDATE SET
			granted_at = NOW(),
			granted_by = $3,
			revoked_at = NULL,
			revoked_by = NULL;
	`, feature, userId, by)
	if err != nil {
		return fmt.Errorf("error granting beta feature %s to %s: %w", feature, userId, err)
	}
	return nil
}

// RevokeBetaFeature closes access without discarding the grant.
//
// Idempotent, and deliberately does not touch anything the person owns. Their
// conversations keep them as a member and their posts stay theirs; what
// changes is that every gate now refuses them and the people they were talking
// to see them as inactive. Re-granting undoes exactly this and nothing else.
func (a *AppDB) RevokeBetaFeature(ctx context.Context, feature string, userId string, revokedBy string) error {
	feature = strings.TrimSpace(feature)
	userId = strings.TrimSpace(userId)
	if !structs.IsBetaFeature(feature) {
		return fmt.Errorf("unknown beta feature %q", feature)
	}

	var by any
	if trimmed := strings.TrimSpace(revokedBy); trimmed != "" {
		by = trimmed
	}

	_, err := a.db.Exec(ctx, `
		UPDATE beta_feature_access
		SET revoked_at = NOW(), revoked_by = $3
		WHERE feature = $1 AND user_id = $2 AND revoked_at IS NULL;
	`, feature, userId, by)
	if err != nil {
		return fmt.Errorf("error revoking beta feature %s from %s: %w", feature, userId, err)
	}
	return nil
}

// ListBetaFeatureMembers is the admin panel's list for one feature, revoked
// members included and marked, so taking somebody out and putting them back is
// visible rather than silent.
func (a *AppDB) ListBetaFeatureMembers(ctx context.Context, feature string) ([]structs.BetaMember, error) {
	feature = strings.TrimSpace(feature)
	if !structs.IsBetaFeature(feature) {
		return nil, fmt.Errorf("unknown beta feature %q", feature)
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			b.user_id,
			COALESCE(NULLIF(TRIM(u.contact_email), ''), '') AS email,
			COALESCE(NULLIF(TRIM(u.contact_name), ''), '') AS name,
			b.granted_at,
			COALESCE(b.granted_by, '') AS granted_by,
			b.revoked_at,
			COALESCE(b.revoked_by, '') AS revoked_by
		FROM beta_feature_access b
		LEFT JOIN users u ON u.id = b.user_id
		WHERE b.feature = $1
		ORDER BY (b.revoked_at IS NULL) DESC, b.granted_at DESC;
	`, feature)
	if err != nil {
		return nil, fmt.Errorf("error listing beta members for %s: %w", feature, err)
	}
	defer rows.Close()

	members := []structs.BetaMember{}
	for rows.Next() {
		var member structs.BetaMember
		var revokedAt *time.Time
		if err := rows.Scan(
			&member.UserId, &member.Email, &member.Name,
			&member.GrantedAt, &member.GrantedBy, &revokedAt, &member.RevokedBy,
		); err != nil {
			return nil, fmt.Errorf("error scanning beta member for %s: %w", feature, err)
		}
		member.RevokedAt = revokedAt
		member.Active = revokedAt == nil
		members = append(members, member)
	}
	return members, rows.Err()
}

// CountBetaFeatureMembers is the active count per feature, for the catalog.
func (a *AppDB) CountBetaFeatureMembers(ctx context.Context) (map[string]int, error) {
	rows, err := a.db.Query(ctx, `
		SELECT feature, COUNT(*)
		FROM beta_feature_access
		WHERE revoked_at IS NULL
		GROUP BY feature;
	`)
	if err != nil {
		return nil, fmt.Errorf("error counting beta members: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var feature string
		var count int
		if err := rows.Scan(&feature, &count); err != nil {
			return nil, fmt.Errorf("error scanning beta member count: %w", err)
		}
		counts[feature] = count
	}
	return counts, rows.Err()
}

// ActiveBetaMembersAmong narrows a known set of users to those currently
// holding the feature.
//
// Bounded by the ids asked about rather than by the feature's whole membership,
// because the caller is shaping one page of conversations and only needs the
// people on it.
func (a *AppDB) ActiveBetaMembersAmong(ctx context.Context, feature string, userIds []string) (map[string]struct{}, error) {
	members := map[string]struct{}{}
	feature = strings.TrimSpace(feature)
	if feature == "" || len(userIds) == 0 {
		return members, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT user_id FROM beta_feature_access
		WHERE feature = $1 AND revoked_at IS NULL AND user_id = ANY($2);
	`, feature, userIds)
	if err != nil {
		return nil, fmt.Errorf("error narrowing beta members for %s: %w", feature, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning narrowed beta member: %w", err)
		}
		members[id] = struct{}{}
	}
	return members, rows.Err()
}
