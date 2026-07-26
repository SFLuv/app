package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

var (
	ErrOrgNameTaken          = errors.New("organization name is already taken")
	ErrOrgNotFound           = errors.New("organization not found")
	ErrOrgAlreadyMember      = errors.New("user already belongs to an organization")
	ErrOrgNotMember          = errors.New("user is not a member of this organization")
	ErrOrgSuperadminRequired = errors.New("organization must retain a superadmin")
	ErrOrgInviteInvalid      = errors.New("invite is invalid or expired")
	ErrOrgInviteEmail        = errors.New("invite was issued for a different email")
	ErrOrgInsufficientFunds  = errors.New("insufficient organization balance")
)

func NormalizeOrgName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}

// syncMemberRoleFlagsTx recomputes users.is_affiliate / is_proposer /
// is_supervisor for every member of the org (and, for removed users, clears
// them) based on the org's approved roles. Keeping the user flags in sync lets
// the existing frontend role UI keep working unchanged.
func syncMemberRoleFlagsTx(ctx context.Context, tx pgx.Tx, orgId int64, extraUsers ...string) error {
	_, err := tx.Exec(ctx, `
		WITH org_roles AS (
			SELECT role_type
			FROM organization_roles
			WHERE organization_id = $1 AND status = 'approved'
		),
		affected AS (
			SELECT user_id FROM organization_members WHERE organization_id = $1
			UNION
			SELECT UNNEST($2::TEXT[])
		)
		UPDATE users u
		SET
			is_affiliate = CASE
				WHEN om.user_id IS NOT NULL THEN EXISTS (SELECT 1 FROM org_roles WHERE role_type = 'affiliate')
				ELSE FALSE
			END,
			is_proposer = CASE
				WHEN om.user_id IS NOT NULL THEN EXISTS (SELECT 1 FROM org_roles WHERE role_type = 'proposer')
				ELSE FALSE
			END,
			is_supervisor = CASE
				WHEN om.user_id IS NOT NULL THEN EXISTS (SELECT 1 FROM org_roles WHERE role_type = 'supervisor')
				ELSE FALSE
			END,
			is_issuer = CASE
				WHEN om.user_id IS NOT NULL THEN EXISTS (SELECT 1 FROM org_roles WHERE role_type = 'issuer')
				ELSE FALSE
			END
		FROM affected a
		LEFT JOIN organization_members om
			ON om.user_id = a.user_id AND om.organization_id = $1
		WHERE u.id = a.user_id;
	`, orgId, extraUsers)
	if err != nil {
		return err
	}
	return syncMemberIssuerScopesTx(ctx, tx, orgId, extraUsers...)
}

// syncMemberIssuerScopesTx makes org-derived per-user issuer scope rows
// (organization_id set) mirror the org's issuance settings for its current
// members, but only while the org's issuer role is approved. Personally
// granted rows (organization_id NULL) are never touched, so a user keeps
// individually granted scopes even if the org's change. Runs from
// syncMemberRoleFlagsTx so every membership/role transition keeps scopes true.
func syncMemberIssuerScopesTx(ctx context.Context, tx pgx.Tx, orgId int64, extraUsers ...string) error {
	if _, err := tx.Exec(ctx, `
		WITH affected AS (
			SELECT user_id FROM organization_members WHERE organization_id = $1
			UNION
			SELECT UNNEST($2::TEXT[])
		),
		entitled AS (
			SELECT om.user_id, ois.credential_type
			FROM organization_members om
			JOIN organization_issuer_scopes ois ON ois.organization_id = om.organization_id
			JOIN organization_roles orr
				ON orr.organization_id = om.organization_id
				AND orr.role_type = 'issuer' AND orr.status = 'approved'
			WHERE om.organization_id = $1
		)
		DELETE FROM issuer_credential_scopes ics
		WHERE ics.organization_id = $1
		AND ics.issuer_id IN (SELECT user_id FROM affected)
		AND NOT EXISTS (
			SELECT 1 FROM entitled e
			WHERE e.user_id = ics.issuer_id AND e.credential_type = ics.credential_type
		);
	`, orgId, extraUsers); err != nil {
		return fmt.Errorf("error pruning org-derived issuer scopes: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO issuer_credential_scopes(issuer_id, credential_type, organization_id)
		SELECT om.user_id, ois.credential_type, $1
		FROM organization_members om
		JOIN organization_issuer_scopes ois ON ois.organization_id = om.organization_id
		JOIN organization_roles orr
			ON orr.organization_id = om.organization_id
			AND orr.role_type = 'issuer' AND orr.status = 'approved'
		WHERE om.organization_id = $1
		ON CONFLICT (issuer_id, credential_type) DO NOTHING;
	`, orgId); err != nil {
		return fmt.Errorf("error granting org-derived issuer scopes: %w", err)
	}
	return nil
}

// GetOrganizationByUser returns the caller's organization and membership role,
// or (nil, "", nil) when the user has no organization.
func (a *AppDB) GetOrganizationByUser(ctx context.Context, userId string) (*structs.Organization, string, error) {
	org := &structs.Organization{}
	var role string
	err := a.db.QueryRow(ctx, `
		SELECT o.id, o.name, o.logo, o.created_at, o.updated_at, om.role
		FROM organization_members om
		JOIN organizations o ON o.id = om.organization_id
		WHERE om.user_id = $1;
	`, userId).Scan(&org.Id, &org.Name, &org.Logo, &org.CreatedAt, &org.UpdatedAt, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("error getting organization for user: %w", err)
	}
	return org, role, nil
}

// UserOrgHasApprovedRole reports whether the user's organization holds an
// approved grant of roleType. This is the authoritative org-scoped auth check.
func (a *AppDB) UserOrgHasApprovedRole(ctx context.Context, userId string, roleType string) (bool, int64, error) {
	var orgId int64
	err := a.db.QueryRow(ctx, `
		SELECT om.organization_id
		FROM organization_members om
		JOIN organization_roles r
			ON r.organization_id = om.organization_id
			AND r.role_type = $2
			AND r.status = 'approved'
		WHERE om.user_id = $1;
	`, userId, roleType).Scan(&orgId)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return true, orgId, nil
}

func (a *AppDB) GetOrganizationMemberIDs(ctx context.Context, orgId int64) ([]string, error) {
	rows, err := a.db.Query(ctx, `SELECT user_id FROM organization_members WHERE organization_id = $1;`, orgId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetOrganizationView loads the full member-facing settings payload.
func (a *AppDB) GetOrganizationView(ctx context.Context, userId string) (*structs.OrganizationView, error) {
	org, myRole, err := a.GetOrganizationByUser(ctx, userId)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, nil
	}

	view := &structs.OrganizationView{Organization: *org, MyRole: myRole}

	rows, err := a.db.Query(ctx, `
		SELECT om.organization_id, om.user_id, om.role,
			COALESCE(u.contact_email, ''), COALESCE(u.contact_name, ''), om.created_at
		FROM organization_members om
		LEFT JOIN users u ON u.id = om.user_id
		WHERE om.organization_id = $1
		ORDER BY
			CASE om.role WHEN 'superadmin' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,
			om.created_at ASC;
	`, org.Id)
	if err != nil {
		return nil, fmt.Errorf("error listing organization members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m structs.OrganizationMember
		if err := rows.Scan(&m.OrganizationId, &m.UserId, &m.Role, &m.ContactEmail, &m.ContactName, &m.CreatedAt); err != nil {
			return nil, err
		}
		view.Members = append(view.Members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	roleRows, err := a.db.Query(ctx, `
		SELECT organization_id, role_type, status, COALESCE(email, ''),
			COALESCE(primary_rewards_account, ''), COALESCE(requested_by, ''), created_at, approved_at
		FROM organization_roles
		WHERE organization_id = $1
		ORDER BY role_type ASC;
	`, org.Id)
	if err != nil {
		return nil, fmt.Errorf("error listing organization roles: %w", err)
	}
	defer roleRows.Close()
	for roleRows.Next() {
		var r structs.OrganizationRole
		if err := roleRows.Scan(&r.OrganizationId, &r.RoleType, &r.Status, &r.Email, &r.PrimaryRewardsAccount, &r.RequestedBy, &r.CreatedAt, &r.ApprovedAt); err != nil {
			return nil, err
		}
		view.Roles = append(view.Roles, r)
	}
	if err := roleRows.Err(); err != nil {
		return nil, err
	}

	allocations, err := a.ListOrganizationAllocations(ctx, org.Id)
	if err != nil {
		return nil, err
	}
	view.Allocations = allocations

	scopes, err := a.ListOrganizationIssuerScopes(ctx, org.Id)
	if err != nil {
		return nil, err
	}
	view.IssuerScopes = scopes

	if myRole == structs.OrgRoleAdmin || myRole == structs.OrgRoleSuperadmin {
		inviteRows, err := a.db.Query(ctx, `
			SELECT id, organization_id, email, COALESCE(invited_by, ''), expires_at, accepted_at, created_at
			FROM organization_invites
			WHERE organization_id = $1 AND accepted_at IS NULL AND expires_at > NOW()
			ORDER BY created_at DESC;
		`, org.Id)
		if err != nil {
			return nil, fmt.Errorf("error listing organization invites: %w", err)
		}
		defer inviteRows.Close()
		for inviteRows.Next() {
			var inv structs.OrganizationInvite
			if err := inviteRows.Scan(&inv.Id, &inv.OrganizationId, &inv.Email, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt); err != nil {
				return nil, err
			}
			view.Invites = append(view.Invites, inv)
		}
		if err := inviteRows.Err(); err != nil {
			return nil, err
		}
	}

	return view, nil
}

// CreateOrganizationWithSuperadmin creates a new org with the user as its
// superadmin. Fails if the (normalized) name is taken or the user already
// belongs to an organization.
func (a *AppDB) CreateOrganizationWithSuperadmin(ctx context.Context, name string, userId string) (*structs.Organization, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("organization name is required")
	}
	normalized := NormalizeOrgName(trimmed)

	var org *structs.Organization
	err := pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		var existing int64
		err := tx.QueryRow(ctx, `SELECT organization_id FROM organization_members WHERE user_id = $1;`, userId).Scan(&existing)
		if err == nil {
			return ErrOrgAlreadyMember
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		org = &structs.Organization{}
		err = tx.QueryRow(ctx, `
			INSERT INTO organizations(name, name_normalized)
			VALUES ($1, $2)
			ON CONFLICT (name_normalized) DO NOTHING
			RETURNING id, name, logo, created_at, updated_at;
		`, trimmed, normalized).Scan(&org.Id, &org.Name, &org.Logo, &org.CreatedAt, &org.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrgNameTaken
		}
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_members(organization_id, user_id, role)
			VALUES ($1, $2, 'superadmin');
		`, org.Id, userId); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return org, nil
}

func (a *AppDB) UpdateOrganizationName(ctx context.Context, orgId int64, name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("organization name is required")
	}
	tag, err := a.db.Exec(ctx, `
		UPDATE organizations
		SET name = $2, name_normalized = $3, updated_at = NOW()
		WHERE id = $1
		AND NOT EXISTS (
			SELECT 1 FROM organizations WHERE name_normalized = $3 AND id <> $1
		);
	`, orgId, trimmed, NormalizeOrgName(trimmed))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrOrgNameTaken
	}
	return nil
}

func (a *AppDB) UpdateOrganizationLogo(ctx context.Context, orgId int64, logo string) error {
	_, err := a.db.Exec(ctx, `UPDATE organizations SET logo = $2, updated_at = NOW() WHERE id = $1;`, orgId, logo)
	return err
}

// AddOrganizationMember adds a user as a plain member and syncs role flags.
func (a *AppDB) AddOrganizationMember(ctx context.Context, orgId int64, userId string) error {
	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		var existing int64
		err := tx.QueryRow(ctx, `SELECT organization_id FROM organization_members WHERE user_id = $1;`, userId).Scan(&existing)
		if err == nil {
			return ErrOrgAlreadyMember
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_members(organization_id, user_id, role)
			VALUES ($1, $2, 'member');
		`, orgId, userId); err != nil {
			return err
		}
		return syncMemberRoleFlagsTx(ctx, tx, orgId)
	})
}

// RemoveOrganizationMember removes a member (never the superadmin) and clears
// their role flags.
func (a *AppDB) RemoveOrganizationMember(ctx context.Context, orgId int64, userId string) error {
	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		var role string
		err := tx.QueryRow(ctx, `
			SELECT role FROM organization_members WHERE organization_id = $1 AND user_id = $2;
		`, orgId, userId).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrgNotMember
		}
		if err != nil {
			return err
		}
		if role == structs.OrgRoleSuperadmin {
			return ErrOrgSuperadminRequired
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2;
		`, orgId, userId); err != nil {
			return err
		}
		return syncMemberRoleFlagsTx(ctx, tx, orgId, userId)
	})
}

// SetOrganizationMemberRole toggles a member between member and admin.
// Superadmin assignment goes through TransferOrganizationSuperadmin only.
func (a *AppDB) SetOrganizationMemberRole(ctx context.Context, orgId int64, userId string, role string) error {
	if role != structs.OrgRoleMember && role != structs.OrgRoleAdmin {
		return fmt.Errorf("invalid member role %q", role)
	}
	tag, err := a.db.Exec(ctx, `
		UPDATE organization_members
		SET role = $3
		WHERE organization_id = $1 AND user_id = $2 AND role <> 'superadmin';
	`, orgId, userId, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrOrgNotMember
	}
	return nil
}

// TransferOrganizationSuperadmin atomically demotes the current superadmin to
// admin and promotes the target member.
func (a *AppDB) TransferOrganizationSuperadmin(ctx context.Context, orgId int64, fromUserId string, toUserId string) error {
	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE organization_members SET role = 'admin'
			WHERE organization_id = $1 AND user_id = $2 AND role = 'superadmin';
		`, orgId, fromUserId)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrOrgSuperadminRequired
		}
		tag, err = tx.Exec(ctx, `
			UPDATE organization_members SET role = 'superadmin'
			WHERE organization_id = $1 AND user_id = $2;
		`, orgId, toUserId)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrOrgNotMember
		}
		return nil
	})
}

// LeaveOrganization removes the caller. A superadmin must transfer first unless
// they are the only member, in which case the organization is deleted.
func (a *AppDB) LeaveOrganization(ctx context.Context, userId string) error {
	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		var orgId int64
		var role string
		err := tx.QueryRow(ctx, `
			SELECT organization_id, role FROM organization_members WHERE user_id = $1;
		`, userId).Scan(&orgId, &role)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrgNotMember
		}
		if err != nil {
			return err
		}

		if role == structs.OrgRoleSuperadmin {
			var others int
			if err := tx.QueryRow(ctx, `
				SELECT COUNT(*) FROM organization_members
				WHERE organization_id = $1 AND user_id <> $2;
			`, orgId, userId).Scan(&others); err != nil {
				return err
			}
			if others > 0 {
				return ErrOrgSuperadminRequired
			}
			// Sole member: dissolve the organization entirely.
			if _, err := tx.Exec(ctx, `DELETE FROM organization_members WHERE organization_id = $1;`, orgId); err != nil {
				return err
			}
			if err := syncMemberRoleFlagsTx(ctx, tx, orgId, userId); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `DELETE FROM organizations WHERE id = $1;`, orgId)
			return err
		}

		if _, err := tx.Exec(ctx, `
			DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2;
		`, orgId, userId); err != nil {
			return err
		}
		return syncMemberRoleFlagsTx(ctx, tx, orgId, userId)
	})
}

// --- Invites -----------------------------------------------------------------

func (a *AppDB) CreateOrganizationInvite(ctx context.Context, orgId int64, email string, tokenHash string, invitedBy string, ttl time.Duration) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO organization_invites(organization_id, email, token_hash, invited_by, expires_at)
		VALUES ($1, LOWER(TRIM($2)), $3, $4, $5);
	`, orgId, email, tokenHash, invitedBy, time.Now().UTC().Add(ttl))
	return err
}

// AcceptOrganizationInvite consumes an invite token: the accepting user must not
// already be in an org, and their contact email or a verified email must match
// the invited address. Adds them as a member and syncs flags.
func (a *AppDB) AcceptOrganizationInvite(ctx context.Context, tokenHash string, userId string) (int64, error) {
	var orgId int64
	err := pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		var email string
		err := tx.QueryRow(ctx, `
			UPDATE organization_invites
			SET accepted_at = NOW()
			WHERE token_hash = $1 AND accepted_at IS NULL AND expires_at > NOW()
			RETURNING organization_id, email;
		`, tokenHash).Scan(&orgId, &email)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrgInviteInvalid
		}
		if err != nil {
			return err
		}

		var emailMatches bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM users
				WHERE id = $1 AND LOWER(TRIM(COALESCE(contact_email, ''))) = $2
			) OR EXISTS (
				SELECT 1 FROM user_verified_emails
				WHERE user_id = $1 AND active = TRUE AND verified_at IS NOT NULL AND email_normalized = $2
			);
		`, userId, strings.ToLower(strings.TrimSpace(email))).Scan(&emailMatches); err != nil {
			return err
		}
		if !emailMatches {
			return ErrOrgInviteEmail
		}

		var existing int64
		err = tx.QueryRow(ctx, `SELECT organization_id FROM organization_members WHERE user_id = $1;`, userId).Scan(&existing)
		if err == nil {
			return ErrOrgAlreadyMember
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_members(organization_id, user_id, role)
			VALUES ($1, $2, 'member');
		`, orgId, userId); err != nil {
			return err
		}
		return syncMemberRoleFlagsTx(ctx, tx, orgId)
	})
	return orgId, err
}

// --- Org roles ---------------------------------------------------------------

// UpsertOrganizationRoleRequest records a pending role request for the org.
// Approved roles are never downgraded by a re-request.
func (a *AppDB) UpsertOrganizationRoleRequest(ctx context.Context, orgId int64, req *structs.OrganizationRoleRequest, requestedBy string) error {
	switch req.RoleType {
	case structs.OrgRoleTypeAffiliate, structs.OrgRoleTypeProposer, structs.OrgRoleTypeSupervisor, structs.OrgRoleTypeIssuer:
	default:
		return fmt.Errorf("invalid role type %q", req.RoleType)
	}
	_, err := a.db.Exec(ctx, `
		INSERT INTO organization_roles(organization_id, role_type, status, email, primary_rewards_account, requested_by)
		VALUES ($1, $2, 'pending', $3, $4, $5)
		ON CONFLICT (organization_id, role_type) DO UPDATE SET
			email = CASE WHEN EXCLUDED.email <> '' THEN EXCLUDED.email ELSE organization_roles.email END,
			primary_rewards_account = CASE WHEN EXCLUDED.primary_rewards_account <> '' THEN EXCLUDED.primary_rewards_account ELSE organization_roles.primary_rewards_account END,
			requested_by = EXCLUDED.requested_by,
			status = CASE WHEN organization_roles.status = 'approved' THEN 'approved' ELSE 'pending' END;
	`, orgId, req.RoleType, req.Email, req.PrimaryRewardsAccount, requestedBy)
	return err
}

// SetOrganizationRoleStatus is the admin approval path; it also re-syncs every
// member's user flags.
func (a *AppDB) SetOrganizationRoleStatus(ctx context.Context, orgId int64, roleType string, status string) error {
	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE organization_roles
			SET status = $3, approved_at = CASE WHEN $3 = 'approved' THEN NOW() ELSE approved_at END
			WHERE organization_id = $1 AND role_type = $2;
		`, orgId, roleType, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrOrgNotFound
		}
		return syncMemberRoleFlagsTx(ctx, tx, orgId)
	})
}

// --- Allocations -------------------------------------------------------------

func (a *AppDB) ListOrganizationAllocations(ctx context.Context, orgId int64) ([]structs.OrganizationAllocation, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, organization_id, cycle, allocation, balance, last_reset_at
		FROM organization_allocations
		WHERE organization_id = $1
		ORDER BY CASE cycle WHEN 'daily' THEN 0 WHEN 'weekly' THEN 1 WHEN 'monthly' THEN 2 ELSE 3 END;
	`, orgId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allocations := []structs.OrganizationAllocation{}
	for rows.Next() {
		var al structs.OrganizationAllocation
		if err := rows.Scan(&al.Id, &al.OrganizationId, &al.Cycle, &al.Allocation, &al.Balance, &al.LastResetAt); err != nil {
			return nil, err
		}
		allocations = append(allocations, al)
	}
	return allocations, rows.Err()
}

// ReplaceOrganizationAllocations makes the given list the organization's FULL
// allocation set in one transaction: listed cycles are upserted, unlisted
// cycles are deleted. For upserts, balance is clamped to the new allocation for
// recurring cycles; one_time sets balance directly.
func (a *AppDB) ReplaceOrganizationAllocations(ctx context.Context, orgId int64, items []structs.AdminOrganizationAllocationItem) error {
	cycles := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		switch item.Cycle {
		case structs.AllocationCycleDaily, structs.AllocationCycleWeekly, structs.AllocationCycleMonthly, structs.AllocationCycleOneTime:
		default:
			return fmt.Errorf("invalid allocation cycle %q", item.Cycle)
		}
		if seen[item.Cycle] {
			return fmt.Errorf("duplicate allocation cycle %q", item.Cycle)
		}
		seen[item.Cycle] = true
		cycles = append(cycles, item.Cycle)
	}

	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM organization_allocations
			WHERE organization_id = $1 AND cycle != ALL($2::text[]);
		`, orgId, cycles); err != nil {
			return fmt.Errorf("error removing unlisted allocations: %w", err)
		}
		for _, item := range items {
			// one_time balance only resets when its allocation actually CHANGES:
			// replace-all saves re-submit untouched rows, and an unconditional
			// reset would silently re-credit already-spent one_time funds every
			// time an unrelated cycle is edited. Unchanged rows (and recurring
			// cycles) just clamp balance to the allocation.
			if _, err := tx.Exec(ctx, `
				INSERT INTO organization_allocations(organization_id, cycle, allocation, balance, last_reset_at)
				VALUES ($1, $2, $3, $3, NOW())
				ON CONFLICT (organization_id, cycle) DO UPDATE SET
					allocation = EXCLUDED.allocation,
					balance = CASE
						WHEN organization_allocations.cycle = 'one_time'
							AND organization_allocations.allocation IS DISTINCT FROM EXCLUDED.allocation
							THEN EXCLUDED.allocation
						ELSE LEAST(organization_allocations.balance, EXCLUDED.allocation)
					END;
			`, orgId, item.Cycle, item.Allocation); err != nil {
				return fmt.Errorf("error upserting %s allocation: %w", item.Cycle, err)
			}
		}
		return nil
	})
}

// ReserveOrganizationBalance debits `total` from the org's balances in fixed
// deduction order: one_time -> daily -> weekly -> monthly. One-time funds are
// consumed first (they never refresh, so they should be drawn down before any
// recurring budget), then recurring cycles shortest-first.
func (a *AppDB) ReserveOrganizationBalance(ctx context.Context, orgId int64, total uint64) error {
	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, balance FROM organization_allocations
			WHERE organization_id = $1
			ORDER BY CASE cycle WHEN 'one_time' THEN 0 WHEN 'daily' THEN 1 WHEN 'weekly' THEN 2 ELSE 3 END
			FOR UPDATE;
		`, orgId)
		if err != nil {
			return err
		}
		type row struct {
			id      int64
			balance uint64
		}
		var alloc []row
		var available uint64
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.balance); err != nil {
				rows.Close()
				return err
			}
			alloc = append(alloc, r)
			available += r.balance
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if available < total {
			return ErrOrgInsufficientFunds
		}
		remaining := total
		for _, r := range alloc {
			if remaining == 0 {
				break
			}
			debit := min(r.balance, remaining)
			if debit == 0 {
				continue
			}
			if _, err := tx.Exec(ctx, `
				UPDATE organization_allocations SET balance = balance - $2 WHERE id = $1;
			`, r.id, debit); err != nil {
				return err
			}
			remaining -= debit
		}
		return nil
	})
}

// RefundOrganizationBalance returns funds after a failed/deleted event, capped
// at each cycle's allocation (recurring) and uncapped for one_time.
func (a *AppDB) RefundOrganizationBalance(ctx context.Context, orgId int64, total uint64) error {
	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		remaining := total
		rows, err := tx.Query(ctx, `
			SELECT id, cycle, allocation, balance FROM organization_allocations
			WHERE organization_id = $1
			ORDER BY CASE cycle WHEN 'one_time' THEN 0 WHEN 'monthly' THEN 1 WHEN 'weekly' THEN 2 ELSE 3 END
			FOR UPDATE;
		`, orgId)
		if err != nil {
			return err
		}
		type row struct {
			id                  int64
			cycle               string
			allocation, balance uint64
		}
		var alloc []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.cycle, &r.allocation, &r.balance); err != nil {
				rows.Close()
				return err
			}
			alloc = append(alloc, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i, r := range alloc {
			if remaining == 0 {
				break
			}
			var credit uint64
			if r.cycle == structs.AllocationCycleOneTime || i == len(alloc)-1 {
				credit = remaining
			} else {
				headroom := uint64(0)
				if r.allocation > r.balance {
					headroom = r.allocation - r.balance
				}
				credit = min(headroom, remaining)
			}
			if credit == 0 {
				continue
			}
			if _, err := tx.Exec(ctx, `
				UPDATE organization_allocations SET balance = balance + $2 WHERE id = $1;
			`, r.id, credit); err != nil {
				return err
			}
			remaining -= credit
		}
		return nil
	})
}

// ResetOrganizationAllocations refills every allocation of the given cycle to
// its full amount, less any outstanding reserved amounts the caller passes in.
func (a *AppDB) ResetOrganizationAllocations(ctx context.Context, cycle string, reservedByOrg map[int64]uint64) error {
	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, organization_id, allocation FROM organization_allocations
			WHERE cycle = $1 FOR UPDATE;
		`, cycle)
		if err != nil {
			return err
		}
		type row struct {
			id, orgId  int64
			allocation uint64
		}
		var alloc []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.orgId, &r.allocation); err != nil {
				rows.Close()
				return err
			}
			alloc = append(alloc, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, r := range alloc {
			reserved := reservedByOrg[r.orgId]
			next := uint64(0)
			if r.allocation > reserved {
				next = r.allocation - reserved
			}
			if _, err := tx.Exec(ctx, `
				UPDATE organization_allocations SET balance = $2, last_reset_at = NOW() WHERE id = $1;
			`, r.id, next); err != nil {
				return err
			}
		}
		return nil
	})
}

// --- Admin -------------------------------------------------------------------

func (a *AppDB) ListOrganizations(ctx context.Context) ([]*structs.OrganizationView, error) {
	rows, err := a.db.Query(ctx, `
		SELECT o.id, o.name, o.logo, o.created_at, o.updated_at
		FROM organizations o
		ORDER BY o.name ASC;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []*structs.OrganizationView{}
	for rows.Next() {
		var org structs.Organization
		if err := rows.Scan(&org.Id, &org.Name, &org.Logo, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, err
		}
		views = append(views, &structs.OrganizationView{Organization: org})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, view := range views {
		memberRows, err := a.db.Query(ctx, `
			SELECT om.organization_id, om.user_id, om.role,
				COALESCE(u.contact_email, ''), COALESCE(u.contact_name, ''), om.created_at
			FROM organization_members om
			LEFT JOIN users u ON u.id = om.user_id
			WHERE om.organization_id = $1
			ORDER BY CASE om.role WHEN 'superadmin' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, om.created_at ASC;
		`, view.Organization.Id)
		if err != nil {
			return nil, err
		}
		for memberRows.Next() {
			var m structs.OrganizationMember
			if err := memberRows.Scan(&m.OrganizationId, &m.UserId, &m.Role, &m.ContactEmail, &m.ContactName, &m.CreatedAt); err != nil {
				memberRows.Close()
				return nil, err
			}
			view.Members = append(view.Members, m)
		}
		memberRows.Close()
		if err := memberRows.Err(); err != nil {
			return nil, err
		}

		roleRows, err := a.db.Query(ctx, `
			SELECT organization_id, role_type, status, COALESCE(email, ''), COALESCE(primary_rewards_account, ''), COALESCE(requested_by, ''), created_at, approved_at
			FROM organization_roles WHERE organization_id = $1 ORDER BY role_type ASC;
		`, view.Organization.Id)
		if err != nil {
			return nil, err
		}
		for roleRows.Next() {
			var role structs.OrganizationRole
			if err := roleRows.Scan(&role.OrganizationId, &role.RoleType, &role.Status, &role.Email, &role.PrimaryRewardsAccount, &role.RequestedBy, &role.CreatedAt, &role.ApprovedAt); err != nil {
				roleRows.Close()
				return nil, err
			}
			view.Roles = append(view.Roles, role)
		}
		roleRows.Close()
		if err := roleRows.Err(); err != nil {
			return nil, err
		}

		allocations, err := a.ListOrganizationAllocations(ctx, view.Organization.Id)
		if err != nil {
			return nil, err
		}
		view.Allocations = allocations

		scopes, err := a.ListOrganizationIssuerScopes(ctx, view.Organization.Id)
		if err != nil {
			return nil, err
		}
		view.IssuerScopes = scopes
	}
	return views, nil
}

// ListOrganizationIssuerScopes returns the credential types the organization's
// members may issue (effective only while the org's issuer role is approved).
func (a *AppDB) ListOrganizationIssuerScopes(ctx context.Context, orgId int64) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT credential_type FROM organization_issuer_scopes
		WHERE organization_id = $1 ORDER BY credential_type ASC;
	`, orgId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scopes := []string{}
	for rows.Next() {
		var scope string
		if err := rows.Scan(&scope); err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, rows.Err()
}

// SetOrganizationIssuerScopes replaces the organization's issuance settings and
// re-syncs the org-derived per-user scope rows for every member, so the change
// takes effect immediately across all issuer runtime checks.
func (a *AppDB) SetOrganizationIssuerScopes(ctx context.Context, orgId int64, credentialTypes []string, adminId string) error {
	normalized := make([]string, 0, len(credentialTypes))
	seen := map[string]struct{}{}
	for _, credential := range credentialTypes {
		credential = strings.ToLower(strings.TrimSpace(credential))
		if credential == "" {
			continue
		}
		valid, err := a.IsGlobalCredentialType(ctx, credential)
		if err != nil {
			return fmt.Errorf("error validating credential type: %w", err)
		}
		if !valid {
			return fmt.Errorf("invalid credential type: %s", credential)
		}
		if _, exists := seen[credential]; exists {
			continue
		}
		seen[credential] = struct{}{}
		normalized = append(normalized, credential)
	}

	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM organization_issuer_scopes
			WHERE organization_id = $1 AND credential_type != ALL($2::text[]);
		`, orgId, normalized); err != nil {
			return fmt.Errorf("error removing unlisted issuer scopes: %w", err)
		}
		for _, credential := range normalized {
			if _, err := tx.Exec(ctx, `
				INSERT INTO organization_issuer_scopes(organization_id, credential_type, created_by)
				VALUES ($1, $2, $3)
				ON CONFLICT (organization_id, credential_type) DO NOTHING;
			`, orgId, credential, adminId); err != nil {
				return fmt.Errorf("error adding issuer scope %s: %w", credential, err)
			}
		}
		return syncMemberIssuerScopesTx(ctx, tx, orgId)
	})
}

// AdminSetOrganizationSuperadminByEmail lets a platform admin reassign the
// superadmin (e.g. the previous one left the company). The target is matched by
// contact or verified email; they must already be an org member.
func (a *AppDB) AdminSetOrganizationSuperadminByEmail(ctx context.Context, orgId int64, email string) error {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return fmt.Errorf("email is required")
	}
	return pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
		var targetUserId string
		err := tx.QueryRow(ctx, `
			SELECT om.user_id
			FROM organization_members om
			JOIN users u ON u.id = om.user_id
			WHERE om.organization_id = $1
			AND (
				LOWER(TRIM(COALESCE(u.contact_email, ''))) = $2
				OR EXISTS (
					SELECT 1 FROM user_verified_emails uve
					WHERE uve.user_id = u.id AND uve.active = TRUE
					AND uve.verified_at IS NOT NULL AND uve.email_normalized = $2
				)
			)
			LIMIT 1;
		`, orgId, normalized).Scan(&targetUserId)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrgNotMember
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE organization_members SET role = 'admin'
			WHERE organization_id = $1 AND role = 'superadmin';
		`, orgId); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE organization_members SET role = 'superadmin'
			WHERE organization_id = $1 AND user_id = $2;
		`, orgId, targetUserId)
		return err
	})
}
