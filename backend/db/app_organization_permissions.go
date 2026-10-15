package db

import (
	"context"
	"fmt"
	"slices"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// Fine-grained organization permissions.
//
// The resolution rule, in one place so the guards cannot disagree with the
// dashboard:
//
//	global admin        -> everything (handled by the caller, unchanged)
//	org lacks the role  -> nothing (unchanged: the org-level approval still gates)
//	org admin/superadmin-> everything the org holds
//	mode = inherit      -> everything the org holds   (the pre-existing behaviour)
//	mode = custom       -> only what is granted
//
// Everyone is "inherit" until an admin says otherwise, so this file changes no
// one's access on the day it ships.

// MemberAccess is what the resolver needs about a member, in one round trip.
type MemberAccess struct {
	OrganizationId int64
	// MemberRole is member | admin | superadmin.
	MemberRole string
	// Mode is inherit | custom.
	Mode string
	// Found is false when the user belongs to no organization.
	Found bool
}

// GetMemberAccess reads the membership facts behind every capability check.
func (a *AppDB) GetMemberAccess(ctx context.Context, userId string) (MemberAccess, error) {
	var access MemberAccess
	row := a.db.QueryRow(ctx, `
		SELECT om.organization_id, om.role, om.permissions_mode
		FROM organization_members om
		JOIN users u ON u.id = om.user_id
		WHERE om.user_id = $1 AND u.active = TRUE;
	`, userId)
	err := row.Scan(&access.OrganizationId, &access.MemberRole, &access.Mode)
	if err == pgx.ErrNoRows {
		return access, nil
	}
	if err != nil {
		return access, err
	}
	access.Found = true
	return access, nil
}

// HasOrgCapability resolves one capability for one user.
//
// It deliberately re-checks the org-level role approval rather than trusting
// the caller, so a capability can never outlive the role it belongs to: if an
// organization's affiliate status is revoked, every affiliate grant inside it
// stops resolving immediately, without anything having to clean up the rows.
func (a *AppDB) HasOrgCapability(ctx context.Context, userId string, capability string) (bool, error) {
	roleType, known := structs.CapabilityRoleType(capability)
	if !known {
		return false, fmt.Errorf("unknown capability %q", capability)
	}

	access, err := a.GetMemberAccess(ctx, userId)
	if err != nil || !access.Found {
		return false, err
	}

	// The administrative set is SFLuv-only and never inherited: it is granted
	// one capability at a time by the superadmin, so an org admin who was
	// never given it does not get it by being an admin.
	if roleType == structs.OrgRoleTypeAdministrative {
		return a.hasGrant(ctx, access.OrganizationId, userId, capability)
	}

	approved, err := a.orgHoldsRole(ctx, access.OrganizationId, roleType)
	if err != nil || !approved {
		return false, err
	}

	// The people who hand out grants are not limited by them.
	if access.MemberRole == structs.OrgRoleAdmin || access.MemberRole == structs.OrgRoleSuperadmin {
		return true, nil
	}
	if access.Mode != structs.OrgPermissionsCustom {
		return true, nil
	}
	return a.hasGrant(ctx, access.OrganizationId, userId, capability)
}

func (a *AppDB) orgHoldsRole(ctx context.Context, orgId int64, roleType string) (bool, error) {
	var ok bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organization_roles
			WHERE organization_id = $1 AND role_type = $2 AND status = 'approved'
		);
	`, orgId, roleType).Scan(&ok)
	return ok, err
}

func (a *AppDB) hasGrant(ctx context.Context, orgId int64, userId string, capability string) (bool, error) {
	var ok bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organization_member_permissions
			WHERE organization_id = $1 AND user_id = $2 AND capability = $3
		);
	`, orgId, userId, capability).Scan(&ok)
	return ok, err
}

// GetMemberGrants lists a member's granted capability keys.
func (a *AppDB) GetMemberGrants(ctx context.Context, orgId int64, userId string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT capability FROM organization_member_permissions
		WHERE organization_id = $1 AND user_id = $2
		ORDER BY capability;
	`, orgId, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	granted := []string{}
	for rows.Next() {
		var capability string
		if err := rows.Scan(&capability); err != nil {
			return nil, err
		}
		granted = append(granted, capability)
	}
	return granted, rows.Err()
}

// GetOrganizationGrants returns every member's grants in one query, keyed by
// user id, so the dashboard does not fan out per member.
func (a *AppDB) GetOrganizationGrants(ctx context.Context, orgId int64) (map[string][]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT user_id, capability FROM organization_member_permissions
		WHERE organization_id = $1
		ORDER BY user_id, capability;
	`, orgId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byUser := map[string][]string{}
	for rows.Next() {
		var userId, capability string
		if err := rows.Scan(&userId, &capability); err != nil {
			return nil, err
		}
		byUser[userId] = append(byUser[userId], capability)
	}
	return byUser, rows.Err()
}

// SetMemberPermissions replaces a member's configuration for one section.
//
// `roleTypes` bounds what is being replaced: the organization dashboard sends
// the platform roles, and the SFLuv administrative section sends only
// "administrative". Without that bound, saving the organization tab would
// silently wipe administrative grants it never showed.
//
// Mode is only written when `mode` is non-empty, so the administrative section
// — which has no mode of its own — cannot flip a member back to inherit.
func (a *AppDB) SetMemberPermissions(
	ctx context.Context,
	orgId int64,
	userId string,
	mode string,
	roleTypes []string,
	granted []string,
	grantedBy string,
) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// The member must belong to this organization. Checked inside the
	// transaction so a concurrent removal cannot leave orphan grants.
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM organization_members WHERE organization_id = $1 AND user_id = $2);
	`, orgId, userId).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("user %s is not a member of organization %d", userId, orgId)
	}

	if mode != "" {
		if mode != structs.OrgPermissionsInherit && mode != structs.OrgPermissionsCustom {
			return fmt.Errorf("unknown permissions mode %q", mode)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE organization_members SET permissions_mode = $3
			WHERE organization_id = $1 AND user_id = $2;
		`, orgId, userId, mode); err != nil {
			return err
		}
	}

	if len(roleTypes) > 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM organization_member_permissions
			WHERE organization_id = $1 AND user_id = $2 AND role_type = ANY($3);
		`, orgId, userId, roleTypes); err != nil {
			return err
		}
	}

	for _, capability := range granted {
		roleType, known := structs.CapabilityRoleType(capability)
		if !known {
			return fmt.Errorf("unknown capability %q", capability)
		}
		// Only write what this call is allowed to write, so a payload cannot
		// reach past the section it came from.
		if !slices.Contains(roleTypes, roleType) {
			return fmt.Errorf("capability %q is outside the roles being edited", capability)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_member_permissions (organization_id, user_id, role_type, capability, granted_by)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (organization_id, user_id, capability) DO NOTHING;
		`, orgId, userId, roleType, capability, grantedBy); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// OrganizationApprovedRoleTypes lists the roles an organization actually holds,
// so the dashboard only offers capabilities the organization can delegate.
func (a *AppDB) OrganizationApprovedRoleTypes(ctx context.Context, orgId int64) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT role_type FROM organization_roles
		WHERE organization_id = $1 AND status = 'approved'
		ORDER BY role_type;
	`, orgId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := []string{}
	for rows.Next() {
		var roleType string
		if err := rows.Scan(&roleType); err != nil {
			return nil, err
		}
		roles = append(roles, roleType)
	}
	return roles, rows.Err()
}

// GetOrganizationMembersWithPermissions lists members with the two fields the
// permissions dashboard resolves against.
func (a *AppDB) GetOrganizationMembersWithPermissions(ctx context.Context, orgId int64) ([]structs.OrganizationMember, error) {
	rows, err := a.db.Query(ctx, `
		SELECT om.organization_id, om.user_id, om.role, om.permissions_mode,
			COALESCE(u.contact_email, ''), COALESCE(u.contact_name, ''), om.created_at
		FROM organization_members om
		LEFT JOIN users u ON u.id = om.user_id
		WHERE om.organization_id = $1
		ORDER BY
			CASE om.role WHEN 'superadmin' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,
			om.created_at ASC;
	`, orgId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []structs.OrganizationMember{}
	for rows.Next() {
		var m structs.OrganizationMember
		if err := rows.Scan(&m.OrganizationId, &m.UserId, &m.Role, &m.PermissionsMode,
			&m.ContactEmail, &m.ContactName, &m.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// GetOrganizationMemberRole returns a member's role, or "" when the user is not
// a member of that organization.
func (a *AppDB) GetOrganizationMemberRole(ctx context.Context, orgId int64, userId string) (string, error) {
	var role string
	err := a.db.QueryRow(ctx, `
		SELECT role FROM organization_members
		WHERE organization_id = $1 AND user_id = $2;
	`, orgId, userId).Scan(&role)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return role, err
}

// SetUserSiteCredential grants or revokes one website credential directly.
//
// It deliberately bypasses the issuer-scope check that IssueCredential applies,
// because the authorization here is different: the caller is the house
// organization's superadmin configuring staff, not an issuer working inside a
// scope their organization was granted. The route is the only caller, and it
// checks that before reaching this.
//
// Writes are shaped to the partial unique index on (user_id, credential_type)
// WHERE is_revoked = false, so granting twice is a no-op rather than a second
// live row.
func (a *AppDB) SetUserSiteCredential(ctx context.Context, userId string, credentialType string, granted bool, actor string) error {
	if granted {
		_, err := a.db.Exec(ctx, `
			INSERT INTO user_credentials (user_id, credential_type, issued_by)
			SELECT $1, $2, $3
			WHERE NOT EXISTS (
				SELECT 1 FROM user_credentials
				WHERE user_id = $1 AND credential_type = $2 AND is_revoked = false
			);
		`, userId, credentialType, actor)
		return err
	}
	_, err := a.db.Exec(ctx, `
		UPDATE user_credentials
		SET is_revoked = true, revoked_at = NOW()
		WHERE user_id = $1 AND credential_type = $2 AND is_revoked = false;
	`, userId, credentialType)
	return err
}
