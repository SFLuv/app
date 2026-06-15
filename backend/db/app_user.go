package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func (a *AppDB) AddUser(ctx context.Context, id string) error {
	state, err := loadUserDeletionState(ctx, a.db, id)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil && !state.Active {
		return ErrUserPendingDeletion
	}

	_, err = a.db.Exec(ctx, `
		INSERT INTO users
			(id)
		VALUES
			($1)
		ON CONFLICT
			(id)
		DO NOTHING;
	`, id)
	if err != nil {
		return err
	}

	return nil
}

func (a *AppDB) UpdateUserInfo(ctx context.Context, user *structs.User) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			users
		SET
			contact_email = $1,
			contact_phone = $2,
			contact_name = $3
		WHERE
			id = $4
		AND
			active = TRUE;
	`, user.Email, user.Phone, user.Name, user.Id)
	if err != nil {
		return err
	}

	return nil
}

func (a *AppDB) UpdateUserPayPalEth(ctx context.Context, userId string, paypalEthAddress any) error {
	fmt.Println("update paypal controller reached")
	_, err := a.db.Exec(ctx, `
		UPDATE
			users
		SET
			paypal_eth = $1
		WHERE
			id = $2
		AND
			active = TRUE;
	`, paypalEthAddress, userId)
	if err != nil {
		return err
	}
	return nil
}

func (a *AppDB) UpdateUserPrimaryWallet(ctx context.Context, userId string, primaryWalletAddress string) (*structs.User, error) {
	normalizedAddress, err := normalizeEthereumAddressForField(primaryWalletAddress, "primary wallet")
	if err != nil {
		return nil, err
	}

	_, err = a.db.Exec(ctx, `
		UPDATE
			users
		SET
			primary_wallet_address = $1
		WHERE
			id = $2
		AND
			active = TRUE;
	`, normalizedAddress, userId)
	if err != nil {
		return nil, fmt.Errorf("error updating user primary wallet: %s", err)
	}

	return a.GetUserById(ctx, userId)
}

func (a *AppDB) UpdateUserRole(ctx context.Context, userId string, role string, value bool) error {
	roles := map[string]string{
		"admin":      "is_admin",
		"merchant":   "is_merchant",
		"organizer":  "is_organizer",
		"improver":   "is_improver",
		"proposer":   "is_proposer",
		"voter":      "is_voter",
		"issuer":     "is_issuer",
		"supervisor": "is_supervisor",
		"affiliate":  "is_affiliate",
	}

	role, ok := roles[role]
	if !ok {
		return fmt.Errorf("invalid role column name")
	}

	_, err := a.db.Exec(ctx, fmt.Sprintf(`
		UPDATE
			users
		SET
			%s = $1
		WHERE
			id = $2
		AND
			active = TRUE;
	`, role), value, userId)
	if err != nil {
		return fmt.Errorf("error updating user: %s", err)
	}

	if role == "is_admin" && value {
		_, err = a.db.Exec(ctx, `
			UPDATE
				users
			SET
				is_voter = true
			WHERE
				id = $1
			AND
				active = TRUE;
		`, userId)
		if err != nil {
			return fmt.Errorf("error defaulting admin to voter: %s", err)
		}
	}

	return nil
}

func normalizeUserVersionFilters(versionFilters []string) []string {
	normalized := []string{}
	seen := map[string]struct{}{}
	for _, raw := range versionFilters {
		for _, value := range strings.Split(raw, ",") {
			trimmed := strings.ToLower(strings.TrimSpace(value))
			if trimmed == "" {
				continue
			}
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			normalized = append(normalized, trimmed)
		}
	}
	return normalized
}

func appendUserListFilters(where []string, args []any, search string, versionFilters []string) ([]string, []any) {
	if trimmed := strings.TrimSpace(search); trimmed != "" {
		args = append(args, "%"+strings.ToLower(trimmed)+"%")
		param := len(args)
		where = append(where, fmt.Sprintf(`(
			LOWER(id) LIKE $%d
			OR LOWER(COALESCE(contact_email, '')) LIKE $%d
			OR LOWER(COALESCE(contact_phone, '')) LIKE $%d
			OR LOWER(COALESCE(contact_name, '')) LIKE $%d
			OR LOWER(primary_wallet_address) LIKE $%d
		)`, param, param, param, param, param))
	}

	versions := normalizeUserVersionFilters(versionFilters)
	if len(versions) > 0 {
		args = append(args, versions)
		param := len(args)
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT
				1
			FROM
				user_client_versions ucv
			WHERE
				ucv.user_id = users.id
			AND
				ucv.id = (
					SELECT
						latest_ucv.id
					FROM
						user_client_versions latest_ucv
					WHERE
						latest_ucv.user_id = users.id
					ORDER BY
						latest_ucv.last_seen_at DESC,
						latest_ucv.id DESC
					LIMIT 1
				)
			AND (
				LOWER(TRIM(ucv.version)) = ANY($%d)
				OR LOWER(
					CASE
						WHEN TRIM(ucv.version) = '' THEN 'unknown'
						WHEN TRIM(ucv.build) <> '' THEN TRIM(ucv.version) || ' (' || TRIM(ucv.build) || ')'
						ELSE TRIM(ucv.version)
					END
				) = ANY($%d)
				OR LOWER(TRIM(ucv.version) || ':' || TRIM(ucv.build)) = ANY($%d)
			)
		)`, param, param, param))
	}

	return where, args
}

func (a *AppDB) GetUsers(ctx context.Context, page int, count int, search string, versionFilters []string) ([]*structs.User, error) {
	var users []*structs.User
	offset := page * count
	where := []string{"active = TRUE"}
	args := []any{}
	where, args = appendUserListFilters(where, args, search, versionFilters)
	args = append(args, count, offset)
	limitParam := len(args) - 1
	offsetParam := len(args)

	rows, err := a.db.Query(ctx, fmt.Sprintf(`
		SELECT
			id,
			is_admin,
			is_merchant,
			is_organizer,
			is_improver,
			is_proposer,
			is_voter,
			is_issuer,
			is_supervisor,
			is_affiliate,
			contact_email,
			contact_phone,
			contact_name,
			primary_wallet_address,
			paypal_eth,
			last_redemption,
			accepted_privacy_policy,
			accepted_privacy_policy_at,
			privacy_policy_version,
			mailing_list_opt_in,
			mailing_list_opt_in_at,
			mailing_list_policy_version
		FROM
			users
		WHERE
			%s
		LIMIT $%d
		OFFSET $%d;
	`, strings.Join(where, "\n\t\tAND "), limitParam, offsetParam), args...)
	if err != nil {
		return nil, fmt.Errorf("error getting users: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		user := structs.User{}
		user.Exists = true
		err = rows.Scan(
			&user.Id,
			&user.IsAdmin,
			&user.IsMerchant,
			&user.IsOrganizer,
			&user.IsImprover,
			&user.IsProposer,
			&user.IsVoter,
			&user.IsIssuer,
			&user.IsSupervisor,
			&user.IsAffiliate,
			&user.Email,
			&user.Phone,
			&user.Name,
			&user.PrimaryWalletAddress,
			&user.PayPalEth,
			&user.LastRedemption,
			&user.AcceptedPrivacyPolicy,
			&user.AcceptedPrivacyPolicyAt,
			&user.PrivacyPolicyVersion,
			&user.MailingListOptIn,
			&user.MailingListOptInAt,
			&user.MailingListPolicyVersion,
		)
		if err != nil {
			continue
		}

		users = append(users, &user)
	}

	return users, nil
}

func (a *AppDB) CountUsers(ctx context.Context, search string, versionFilters []string) (int, error) {
	var total int
	where := []string{"active = TRUE"}
	args := []any{}
	where, args = appendUserListFilters(where, args, search, versionFilters)

	err := a.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT
			COUNT(*)
		FROM
			users
		WHERE
			%s;
	`, strings.Join(where, "\n\t\tAND ")), args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("error counting users: %s", err)
	}

	return total, nil
}

func (a *AppDB) GetMailingListEmails(ctx context.Context) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		WITH candidate_emails AS (
			SELECT
				TRIM(contact_email) AS email
			FROM
				users
			WHERE
				active = TRUE
				AND mailing_list_opt_in = TRUE
				AND TRIM(COALESCE(contact_email, '')) <> ''

			UNION ALL

			SELECT
				TRIM(ve.email) AS email
			FROM
				users u
			INNER JOIN
				user_verified_emails ve
			ON
				ve.user_id = u.id
			WHERE
				u.active = TRUE
				AND u.mailing_list_opt_in = TRUE
				AND ve.verified_at IS NOT NULL
				AND TRIM(COALESCE(ve.email, '')) <> ''
		)
		SELECT DISTINCT ON (LOWER(email))
			email
		FROM
			candidate_emails
		ORDER BY
			LOWER(email),
			email;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying mailing list emails: %s", err)
	}
	defer rows.Close()

	emails := []string{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("error scanning mailing list email: %s", err)
		}
		emails = append(emails, email)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading mailing list emails: %s", err)
	}

	return emails, nil
}

func (a *AppDB) GetUserById(ctx context.Context, userId string) (*structs.User, error) {
	return a.getUserById(ctx, userId, false)
}

func (a *AppDB) GetUserByIdIncludingInactive(ctx context.Context, userId string) (*structs.User, error) {
	return a.getUserById(ctx, userId, true)
}

func (a *AppDB) getUserById(ctx context.Context, userId string, includeInactive bool) (*structs.User, error) {
	var user structs.User
	query := `
		SELECT
			id,
			is_admin,
			is_merchant,
			is_organizer,
			is_improver,
			is_proposer,
			is_voter,
			is_issuer,
			is_supervisor,
			is_affiliate,
			contact_email,
			contact_phone,
			contact_name,
			primary_wallet_address,
			paypal_eth,
			last_redemption,
			accepted_privacy_policy,
			accepted_privacy_policy_at,
			privacy_policy_version,
			mailing_list_opt_in,
			mailing_list_opt_in_at,
			mailing_list_policy_version
		FROM
			users
		WHERE
			id = $1`
	if !includeInactive {
		query += `
		AND
			active = TRUE`
	}
	query += `;`
	row := a.db.QueryRow(ctx, query, userId)
	err := row.Scan(
		&user.Id,
		&user.IsAdmin,
		&user.IsMerchant,
		&user.IsOrganizer,
		&user.IsImprover,
		&user.IsProposer,
		&user.IsVoter,
		&user.IsIssuer,
		&user.IsSupervisor,
		&user.IsAffiliate,
		&user.Email,
		&user.Phone,
		&user.Name,
		&user.PrimaryWalletAddress,
		&user.PayPalEth,
		&user.LastRedemption,
		&user.AcceptedPrivacyPolicy,
		&user.AcceptedPrivacyPolicyAt,
		&user.PrivacyPolicyVersion,
		&user.MailingListOptIn,
		&user.MailingListOptInAt,
		&user.MailingListPolicyVersion,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (a *AppDB) GetUserPolicyStatus(ctx context.Context, userId string) (*structs.UserPolicyStatusResponse, error) {
	status := &structs.UserPolicyStatusResponse{}
	row := a.db.QueryRow(ctx, `
		SELECT
			id,
			active,
			accepted_privacy_policy,
			accepted_privacy_policy_at,
			privacy_policy_version,
			mailing_list_opt_in,
			mailing_list_opt_in_at,
			mailing_list_policy_version
		FROM
			users
		WHERE
			id = $1;
	`, userId)
	if err := row.Scan(
		&status.UserId,
		&status.Active,
		&status.AcceptedPrivacyPolicy,
		&status.AcceptedPrivacyPolicyAt,
		&status.PrivacyPolicyVersion,
		&status.MailingListOptIn,
		&status.MailingListOptInAt,
		&status.MailingListPolicyVersion,
	); err != nil {
		return nil, err
	}

	return status, nil
}

func (a *AppDB) UserHasAcceptedPrivacyPolicy(ctx context.Context, userId string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			accepted_privacy_policy
		FROM
			users
		WHERE
			id = $1
		AND
			active = TRUE;
	`, userId)

	var accepted bool
	if err := row.Scan(&accepted); err != nil {
		return false, err
	}

	return accepted, nil
}

func (a *AppDB) AcceptUserPolicies(
	ctx context.Context,
	userId string,
	mailingListOptIn bool,
	now time.Time,
) (*structs.UserPolicyStatusResponse, error) {
	row := a.db.QueryRow(ctx, `
		UPDATE
			users
		SET
			accepted_privacy_policy = TRUE,
			accepted_privacy_policy_at = COALESCE(accepted_privacy_policy_at, $2),
			privacy_policy_version = $3,
			mailing_list_opt_in = $4,
			mailing_list_opt_in_at = CASE
				WHEN $4 THEN COALESCE(mailing_list_opt_in_at, $2)
				ELSE NULL
			END,
			mailing_list_policy_version = CASE
				WHEN $4 THEN $5
				ELSE ''
			END
		WHERE
			id = $1
		AND
			active = TRUE
		RETURNING
			id,
			active,
			accepted_privacy_policy,
			accepted_privacy_policy_at,
			privacy_policy_version,
			mailing_list_opt_in,
			mailing_list_opt_in_at,
			mailing_list_policy_version;
	`, userId, now.UTC(), structs.CurrentPrivacyPolicyVersion, mailingListOptIn, structs.CurrentMailingListPolicyVersion)

	status := &structs.UserPolicyStatusResponse{}
	if err := row.Scan(
		&status.UserId,
		&status.Active,
		&status.AcceptedPrivacyPolicy,
		&status.AcceptedPrivacyPolicyAt,
		&status.PrivacyPolicyVersion,
		&status.MailingListOptIn,
		&status.MailingListOptInAt,
		&status.MailingListPolicyVersion,
	); err != nil {
		return nil, err
	}

	return status, nil
}

func (a *AppDB) GetAllUserIDs(ctx context.Context) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			users
		WHERE
			active = TRUE;
	`)
	if err != nil {
		return nil, fmt.Errorf("error getting all user ids: %s", err)
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	return ids, nil
}
