package bootstrap

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// migrateOrganizations is the one-time 1.22 migration body. It:
//  1. creates the organization tables,
//  2. merges every existing affiliate/proposer/supervisor organization (grouped
//     case-insensitively) into a single organizations row,
//  3. makes every previous role-row owner an org admin, choosing the superadmin
//     as: platform admin first, else the earliest role-row creator,
//  4. converts affiliate weekly/one-time allocations to org allocations,
//  5. tags legacy role rows and bot events with the new organization_id so no
//     references to pre-merge organizations remain.
func migrateOrganizations(ctx context.Context, pools *DBPools) error {
	if _, err := pools.App.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS organizations(
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			name_normalized TEXT NOT NULL UNIQUE,
			logo TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS organization_members(
			organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			user_id TEXT PRIMARY KEY REFERENCES users(id),
			role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin', 'superadmin')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS organization_members_org_idx ON organization_members(organization_id);

		CREATE TABLE IF NOT EXISTS organization_roles(
			organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			role_type TEXT NOT NULL CHECK (role_type IN ('affiliate', 'proposer', 'supervisor', 'issuer')),
			status TEXT NOT NULL DEFAULT 'pending',
			email TEXT NOT NULL DEFAULT '',
			primary_rewards_account TEXT NOT NULL DEFAULT '',
			requested_by TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			approved_at TIMESTAMPTZ,
			PRIMARY KEY (organization_id, role_type)
		);

		CREATE TABLE IF NOT EXISTS organization_invites(
			id BIGSERIAL PRIMARY KEY,
			organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			email TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			invited_by TEXT,
			expires_at TIMESTAMPTZ NOT NULL,
			accepted_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS organization_invites_org_idx ON organization_invites(organization_id);

		CREATE TABLE IF NOT EXISTS organization_allocations(
			id BIGSERIAL PRIMARY KEY,
			organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			cycle TEXT NOT NULL CHECK (cycle IN ('daily', 'weekly', 'monthly', 'one_time')),
			allocation BIGINT NOT NULL DEFAULT 0,
			balance BIGINT NOT NULL DEFAULT 0,
			last_reset_at TIMESTAMPTZ,
			UNIQUE (organization_id, cycle)
		);

		ALTER TABLE affiliates ADD COLUMN IF NOT EXISTS organization_id BIGINT;
		ALTER TABLE proposers ADD COLUMN IF NOT EXISTS organization_id BIGINT;
		ALTER TABLE supervisors ADD COLUMN IF NOT EXISTS organization_id BIGINT;
		ALTER TABLE issuers ADD COLUMN IF NOT EXISTS organization_id BIGINT;
	`); err != nil {
		return fmt.Errorf("error creating organization tables: %w", err)
	}

	type roleRow struct {
		userID    string
		orgName   string
		roleType  string
		status    string
		email     string
		rewards   string
		weekly    uint64
		weeklyBal uint64
		oneTime   uint64
		createdAt time.Time
		isAdmin   bool
		active    bool
	}

	rows := []roleRow{}
	collect := func(query string, roleType string) error {
		res, err := pools.App.Query(ctx, query)
		if err != nil {
			return err
		}
		defer res.Close()
		for res.Next() {
			r := roleRow{roleType: roleType}
			if err := res.Scan(&r.userID, &r.orgName, &r.status, &r.email, &r.rewards, &r.weekly, &r.weeklyBal, &r.oneTime, &r.createdAt, &r.isAdmin, &r.active); err != nil {
				return err
			}
			if strings.TrimSpace(r.orgName) == "" || !r.active {
				continue
			}
			rows = append(rows, r)
		}
		return res.Err()
	}

	if err := collect(`
		SELECT a.user_id, a.organization, a.status, '', '',
			COALESCE(a.weekly_allocation, 0), COALESCE(a.weekly_balance, 0), COALESCE(a.one_time_balance, 0),
			a.created_at, COALESCE(u.is_admin, FALSE), COALESCE(a.active, TRUE)
		FROM affiliates a JOIN users u ON u.id = a.user_id;
	`, "affiliate"); err != nil {
		return fmt.Errorf("error collecting affiliates: %w", err)
	}
	if err := collect(`
		SELECT p.user_id, p.organization, p.status, COALESCE(p.email, ''), '',
			0, 0, 0, p.created_at, COALESCE(u.is_admin, FALSE), TRUE
		FROM proposers p JOIN users u ON u.id = p.user_id;
	`, "proposer"); err != nil {
		return fmt.Errorf("error collecting proposers: %w", err)
	}
	if err := collect(`
		SELECT s.user_id, s.organization, s.status, COALESCE(s.email, ''), COALESCE(s.primary_rewards_account, ''),
			0, 0, 0, s.created_at, COALESCE(u.is_admin, FALSE), TRUE
		FROM supervisors s JOIN users u ON u.id = s.user_id;
	`, "supervisor"); err != nil {
		return fmt.Errorf("error collecting supervisors: %w", err)
	}
	if err := collect(`
		SELECT i.user_id, i.organization, i.status, COALESCE(i.email, ''), '',
			0, 0, 0, i.created_at, COALESCE(u.is_admin, FALSE), COALESCE(i.active, TRUE)
		FROM issuers i JOIN users u ON u.id = i.user_id;
	`, "issuer"); err != nil {
		return fmt.Errorf("error collecting issuers: %w", err)
	}

	normalize := func(name string) string {
		return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
	}

	groups := map[string][]roleRow{}
	for _, r := range rows {
		key := normalize(r.orgName)
		groups[key] = append(groups[key], r)
	}

	// A user can appear under several org names (e.g. affiliate for "A",
	// proposer for "B") but may only belong to ONE org: their earliest row wins.
	memberOrgKey := map[string]string{}
	memberEarliest := map[string]time.Time{}
	for key, group := range groups {
		for _, r := range group {
			if prev, ok := memberEarliest[r.userID]; !ok || r.createdAt.Before(prev) {
				memberEarliest[r.userID] = r.createdAt
				memberOrgKey[r.userID] = key
			}
		}
	}

	sortedKeys := make([]string, 0, len(groups))
	for key := range groups {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	orgIDByKey := map[string]int64{}

	err := pgx.BeginFunc(ctx, pools.App, func(tx pgx.Tx) error {
		for _, key := range sortedKeys {
			group := groups[key]
			sort.Slice(group, func(i, j int) bool { return group[i].createdAt.Before(group[j].createdAt) })

			// Members of this org: users whose single-org assignment is here.
			memberSet := map[string]roleRow{}
			for _, r := range group {
				if memberOrgKey[r.userID] != key {
					continue
				}
				if existing, ok := memberSet[r.userID]; !ok || r.createdAt.Before(existing.createdAt) {
					memberSet[r.userID] = r
				}
			}
			if len(memberSet) == 0 {
				continue // every candidate ended up in another org; skip empty org
			}

			// Superadmin: platform admin first, else earliest creator; ties broken
			// by earliest createdAt within each class.
			var superadmin string
			var superadminAt time.Time
			for id, r := range memberSet {
				if !r.isAdmin {
					continue
				}
				if superadmin == "" || r.createdAt.Before(superadminAt) {
					superadmin = id
					superadminAt = r.createdAt
				}
			}
			if superadmin == "" {
				for id, r := range memberSet {
					if superadmin == "" || r.createdAt.Before(superadminAt) {
						superadmin = id
						superadminAt = r.createdAt
					}
				}
			}

			// Canonical display name: from the superadmin's row, else first row.
			displayName := strings.TrimSpace(group[0].orgName)
			for _, r := range group {
				if r.userID == superadmin && memberOrgKey[r.userID] == key {
					displayName = strings.TrimSpace(r.orgName)
					break
				}
			}

			var orgID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO organizations(name, name_normalized) VALUES ($1, $2)
				ON CONFLICT (name_normalized) DO UPDATE SET name = organizations.name
				RETURNING id;
			`, displayName, key).Scan(&orgID); err != nil {
				return fmt.Errorf("error creating organization %q: %w", displayName, err)
			}
			orgIDByKey[key] = orgID

			for id := range memberSet {
				role := "admin" // every previous role-row owner becomes an org admin
				if id == superadmin {
					role = "superadmin"
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO organization_members(organization_id, user_id, role)
					VALUES ($1, $2, $3)
					ON CONFLICT (user_id) DO NOTHING;
				`, orgID, id, role); err != nil {
					return fmt.Errorf("error adding member %s to org %d: %w", id, orgID, err)
				}
			}

			// Org roles: best status per role type across the merged rows.
			type roleAgg struct {
				status    string
				email     string
				rewards   string
				createdAt time.Time
			}
			roleAggs := map[string]*roleAgg{}
			for _, r := range group {
				agg, ok := roleAggs[r.roleType]
				if !ok {
					agg = &roleAgg{status: r.status, email: r.email, rewards: r.rewards, createdAt: r.createdAt}
					roleAggs[r.roleType] = agg
					continue
				}
				if r.status == "approved" {
					agg.status = "approved"
				}
				if agg.email == "" {
					agg.email = r.email
				}
				if agg.rewards == "" {
					agg.rewards = r.rewards
				}
				if r.createdAt.Before(agg.createdAt) {
					agg.createdAt = r.createdAt
				}
			}
			for roleType, agg := range roleAggs {
				if _, err := tx.Exec(ctx, `
					INSERT INTO organization_roles(organization_id, role_type, status, email, primary_rewards_account, created_at, approved_at)
					VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $3 = 'approved' THEN NOW() ELSE NULL END)
					ON CONFLICT (organization_id, role_type) DO NOTHING;
				`, orgID, roleType, agg.status, agg.email, agg.rewards, agg.createdAt); err != nil {
					return fmt.Errorf("error creating org role %s for org %d: %w", roleType, orgID, err)
				}
			}

			// Allocations: MAX weekly allocation (avoid inflating spend on merge),
			// SUM of one-time balances (real held funds).
			var weeklyAllocation, weeklyBalance, oneTime uint64
			for _, r := range group {
				if r.roleType != "affiliate" {
					continue
				}
				if r.weekly > weeklyAllocation {
					weeklyAllocation = r.weekly
				}
				weeklyBalance += r.weeklyBal
				oneTime += r.oneTime
			}
			if weeklyBalance > weeklyAllocation {
				weeklyBalance = weeklyAllocation
			}
			if weeklyAllocation > 0 {
				if _, err := tx.Exec(ctx, `
					INSERT INTO organization_allocations(organization_id, cycle, allocation, balance, last_reset_at)
					VALUES ($1, 'weekly', $2, $3, NOW())
					ON CONFLICT (organization_id, cycle) DO NOTHING;
				`, orgID, weeklyAllocation, weeklyBalance); err != nil {
					return fmt.Errorf("error creating weekly allocation for org %d: %w", orgID, err)
				}
			}
			if oneTime > 0 {
				if _, err := tx.Exec(ctx, `
					INSERT INTO organization_allocations(organization_id, cycle, allocation, balance)
					VALUES ($1, 'one_time', $2, $2)
					ON CONFLICT (organization_id, cycle) DO NOTHING;
				`, orgID, oneTime, oneTime); err != nil {
					return fmt.Errorf("error creating one-time allocation for org %d: %w", orgID, err)
				}
			}
		}

		// Point legacy role rows at their merged organization and canonical name so
		// no references to pre-merge org spellings survive.
		for _, table := range []string{"affiliates", "proposers", "supervisors", "issuers"} {
			if _, err := tx.Exec(ctx, fmt.Sprintf(`
				UPDATE %s t
				SET organization_id = o.id, organization = o.name
				FROM organizations o
				WHERE o.name_normalized = LOWER(REGEXP_REPLACE(TRIM(t.organization), '\s+', ' ', 'g'));
			`, table)); err != nil {
				return fmt.Errorf("error linking %s to organizations: %w", table, err)
			}
		}

		// Re-sync user role flags from org-approved roles for all members.
		if _, err := tx.Exec(ctx, `
			UPDATE users u
			SET
				is_affiliate = EXISTS (
					SELECT 1 FROM organization_members om
					JOIN organization_roles r ON r.organization_id = om.organization_id
						AND r.role_type = 'affiliate' AND r.status = 'approved'
					WHERE om.user_id = u.id
				),
				is_proposer = EXISTS (
					SELECT 1 FROM organization_members om
					JOIN organization_roles r ON r.organization_id = om.organization_id
						AND r.role_type = 'proposer' AND r.status = 'approved'
					WHERE om.user_id = u.id
				),
				is_supervisor = EXISTS (
					SELECT 1 FROM organization_members om
					JOIN organization_roles r ON r.organization_id = om.organization_id
						AND r.role_type = 'supervisor' AND r.status = 'approved'
					WHERE om.user_id = u.id
				),
				is_issuer = EXISTS (
					SELECT 1 FROM organization_members om
					JOIN organization_roles r ON r.organization_id = om.organization_id
						AND r.role_type = 'issuer' AND r.status = 'approved'
					WHERE om.user_id = u.id
				)
			WHERE EXISTS (SELECT 1 FROM organization_members om WHERE om.user_id = u.id)
			OR u.is_affiliate = TRUE OR u.is_proposer = TRUE OR u.is_supervisor = TRUE OR u.is_issuer = TRUE;
		`); err != nil {
			return fmt.Errorf("error syncing user role flags: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Bot DB: tag events with the owning user's organization.
	if _, err := pools.Bot.Exec(ctx, `
		ALTER TABLE events ADD COLUMN IF NOT EXISTS organization_id BIGINT;
		CREATE INDEX IF NOT EXISTS events_organization_idx ON events(organization_id);
	`); err != nil {
		return fmt.Errorf("error adding organization_id to events: %w", err)
	}

	ownerRows, err := pools.App.Query(ctx, `SELECT user_id, organization_id FROM organization_members;`)
	if err != nil {
		return fmt.Errorf("error loading member org map: %w", err)
	}
	memberOrg := map[string]int64{}
	for ownerRows.Next() {
		var userID string
		var orgID int64
		if err := ownerRows.Scan(&userID, &orgID); err != nil {
			ownerRows.Close()
			return err
		}
		memberOrg[userID] = orgID
	}
	ownerRows.Close()
	if err := ownerRows.Err(); err != nil {
		return err
	}

	for userID, orgID := range memberOrg {
		if _, err := pools.Bot.Exec(ctx, `
			UPDATE events SET organization_id = $2
			WHERE owner = $1 AND organization_id IS NULL;
		`, userID, orgID); err != nil {
			return fmt.Errorf("error tagging events for owner %s: %w", userID, err)
		}
	}

	return nil
}
