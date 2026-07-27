package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerAdminDataTools adds read-only coverage for the remaining admin
// surfaces the base/extra tools didn't reach: organizations, the improver role,
// the credential system (holdings, issuer scopes, requests, catalog), the
// legacy proposer/supervisor/issuer role tables, and workflow votes. All are
// read-only and expose no secrets (no logos/badge bytes/document urls).
func (a *reportCore) registerAdminDataTools(s *server.MCPServer) {
	a.addTool(s, mcp.NewTool("organizations_report",
		mcp.WithDescription("List organizations with members (id, role, email, name), platform role grants (affiliate/proposer/supervisor/issuer + status/email/rewards account/requested_by), cycle allocations and balances (whole $SFLUV), and issuer credential scopes. Organizations own affiliate/proposer/supervisor/issuer roles; this is the source of truth for those role approvals. Logos are omitted (only has_logo)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("search", mcp.Description("Optional case-insensitive search over organization name.")),
		mcp.WithString("role_type", mcp.Description("Optional: only orgs holding this role (affiliate|proposer|supervisor|issuer).")),
		mcp.WithString("role_status", mcp.Description("Optional role status filter used with role_type (pending|approved|rejected).")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.organizationsReport)

	a.addTool(s, mcp.NewTool("improvers_report",
		mcp.WithDescription("List improvers (the volunteer role that is per-user, not org-owned) with name, email, status, primary rewards account, and the credentials each currently holds."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("status", mcp.Description("Optional status filter (pending|approved|rejected).")),
		mcp.WithString("search", mcp.Description("Optional case-insensitive search over user id, name, and email.")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.improversReport)

	a.addTool(s, mcp.NewTool("role_holders_report",
		mcp.WithDescription("List holders of a legacy per-user role table: proposer, supervisor, or issuer. Returns user id, organization, email, nickname, status, and (supervisor) primary rewards account. Role approvals are org-owned now (see organizations_report); this exposes the underlying per-user rows."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("role_type", mcp.Description("Which role: proposer, supervisor, or issuer."), mcp.Required()),
		mcp.WithString("status", mcp.Description("Optional status filter (pending|approved|rejected).")),
		mcp.WithString("search", mcp.Description("Optional case-insensitive search over user id, organization, email, and nickname.")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.roleHoldersReport)

	a.addTool(s, mcp.NewTool("credentials_report",
		mcp.WithDescription("The credential system: the global credential-type catalog (value, label, visibility; never badge image bytes), who currently holds which credentials (user_credentials), which issuers/orgs are scoped to grant which credential types, and pending/resolved credential requests. Use section to fetch one part."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("section", mcp.Description("One of: catalog, holders, scopes, requests, all (default all).")),
		mcp.WithString("credential_type", mcp.Description("Optional credential type value to filter holders/scopes/requests.")),
		mcp.WithString("user_id", mcp.Description("Optional user id to filter holders/requests.")),
		mcp.WithString("request_status", mcp.Description("Optional credential-request status filter (pending|approved|rejected).")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number (holders/requests).")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.credentialsReport)

	a.addTool(s, mcp.NewTool("workflow_votes",
		mcp.WithDescription("Governance votes for a workflow: each voter's decision (approve/deny) and comment, with tallies, plus the workflow's recorded vote decision and finalization. Voting drives workflow approval."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("workflow_id", mcp.Description("The workflow id to fetch votes for."), mcp.Required()),
	), a.workflowVotes)
}

// ---- organizations ---------------------------------------------------------

type orgMemberRow struct {
	UserID       string `json:"user_id"`
	Role         string `json:"role"`
	ContactEmail string `json:"contact_email,omitempty"`
	ContactName  string `json:"contact_name,omitempty"`
}

type orgRoleRow struct {
	RoleType              string `json:"role_type"`
	Status                string `json:"status"`
	Email                 string `json:"email,omitempty"`
	PrimaryRewardsAccount string `json:"primary_rewards_account,omitempty"`
	RequestedBy           string `json:"requested_by,omitempty"`
}

type orgAllocationRow struct {
	Cycle           string `json:"cycle"`
	AllocationSfluv int64  `json:"allocation_sfluv"`
	BalanceSfluv    int64  `json:"balance_sfluv"`
}

type organizationRow struct {
	ID           int64              `json:"id"`
	Name         string             `json:"name"`
	HasLogo      bool               `json:"has_logo"`
	CreatedAt    string             `json:"created_at,omitempty"`
	Members      []orgMemberRow     `json:"members"`
	Roles        []orgRoleRow       `json:"roles"`
	Allocations  []orgAllocationRow `json:"allocations"`
	IssuerScopes []string           `json:"issuer_scopes"`
}

func (a *reportCore) organizationsReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	search := strings.TrimSpace(request.GetString("search", ""))
	roleType := strings.TrimSpace(request.GetString("role_type", ""))
	roleStatus := strings.TrimSpace(request.GetString("role_status", ""))
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	searchPattern := ""
	if search != "" {
		searchPattern = "%" + search + "%"
	}

	orgs := []organizationRow{}
	byID := map[int64]*organizationRow{}

	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT o.id, o.name, (COALESCE(TRIM(o.logo), '') <> ''), o.created_at
			FROM organizations o
			WHERE ($1 = '' OR o.name ILIKE $1)
			AND ($2 = '' OR EXISTS (
				SELECT 1 FROM organization_roles r
				WHERE r.organization_id = o.id AND r.role_type = $2
				AND ($3 = '' OR r.status = $3)
			))
			ORDER BY o.name ASC
			LIMIT $4 OFFSET $5;
		`, searchPattern, roleType, roleStatus, limit, offset)
		if err != nil {
			return fmt.Errorf("query organizations: %w", err)
		}
		defer rows.Close()
		ids := []int64{}
		for rows.Next() {
			var org organizationRow
			var createdAt time.Time
			if err := rows.Scan(&org.ID, &org.Name, &org.HasLogo, &createdAt); err != nil {
				return err
			}
			org.CreatedAt = createdAt.UTC().Format(time.RFC3339)
			org.Members = []orgMemberRow{}
			org.Roles = []orgRoleRow{}
			org.Allocations = []orgAllocationRow{}
			org.IssuerScopes = []string{}
			orgs = append(orgs, org)
			ids = append(ids, org.ID)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range orgs {
			byID[orgs[i].ID] = &orgs[i]
		}
		if len(ids) == 0 {
			return nil
		}

		memberRows, err := tx.Query(ctx, `
			SELECT om.organization_id, om.user_id, om.role,
				COALESCE(u.contact_email, ''), COALESCE(u.contact_name, '')
			FROM organization_members om
			LEFT JOIN users u ON u.id = om.user_id
			WHERE om.organization_id = ANY($1::bigint[])
			ORDER BY CASE om.role WHEN 'superadmin' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, om.created_at ASC;
		`, ids)
		if err != nil {
			return fmt.Errorf("query org members: %w", err)
		}
		for memberRows.Next() {
			var orgID int64
			var m orgMemberRow
			if err := memberRows.Scan(&orgID, &m.UserID, &m.Role, &m.ContactEmail, &m.ContactName); err != nil {
				memberRows.Close()
				return err
			}
			if org := byID[orgID]; org != nil {
				org.Members = append(org.Members, m)
			}
		}
		memberRows.Close()
		if err := memberRows.Err(); err != nil {
			return err
		}

		roleRows, err := tx.Query(ctx, `
			SELECT organization_id, role_type, status,
				COALESCE(email, ''), COALESCE(primary_rewards_account, ''), COALESCE(requested_by, '')
			FROM organization_roles
			WHERE organization_id = ANY($1::bigint[])
			ORDER BY role_type ASC;
		`, ids)
		if err != nil {
			return fmt.Errorf("query org roles: %w", err)
		}
		for roleRows.Next() {
			var orgID int64
			var r orgRoleRow
			if err := roleRows.Scan(&orgID, &r.RoleType, &r.Status, &r.Email, &r.PrimaryRewardsAccount, &r.RequestedBy); err != nil {
				roleRows.Close()
				return err
			}
			if org := byID[orgID]; org != nil {
				org.Roles = append(org.Roles, r)
			}
		}
		roleRows.Close()
		if err := roleRows.Err(); err != nil {
			return err
		}

		allocRows, err := tx.Query(ctx, `
			SELECT organization_id, cycle, allocation, balance
			FROM organization_allocations
			WHERE organization_id = ANY($1::bigint[])
			ORDER BY cycle ASC;
		`, ids)
		if err != nil {
			return fmt.Errorf("query org allocations: %w", err)
		}
		for allocRows.Next() {
			var orgID int64
			var al orgAllocationRow
			if err := allocRows.Scan(&orgID, &al.Cycle, &al.AllocationSfluv, &al.BalanceSfluv); err != nil {
				allocRows.Close()
				return err
			}
			if org := byID[orgID]; org != nil {
				org.Allocations = append(org.Allocations, al)
			}
		}
		allocRows.Close()
		if err := allocRows.Err(); err != nil {
			return err
		}

		scopeRows, err := tx.Query(ctx, `
			SELECT organization_id, credential_type
			FROM organization_issuer_scopes
			WHERE organization_id = ANY($1::bigint[])
			ORDER BY credential_type ASC;
		`, ids)
		if err != nil {
			return fmt.Errorf("query org issuer scopes: %w", err)
		}
		for scopeRows.Next() {
			var orgID int64
			var scope string
			if err := scopeRows.Scan(&orgID, &scope); err != nil {
				scopeRows.Close()
				return err
			}
			if org := byID[orgID]; org != nil {
				org.IssuerScopes = append(org.IssuerScopes, scope)
			}
		}
		scopeRows.Close()
		return scopeRows.Err()
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"generated_at":  time.Now().UTC().Format(time.RFC3339),
		"page":          page,
		"count":         limit,
		"note":          "Allocations/balances are whole $SFLUV. Organizations own affiliate/proposer/supervisor/issuer role approvals (the roles array).",
		"organizations": orgs,
	}, nil
}

// ---- improvers -------------------------------------------------------------

type improverRow struct {
	UserID                string   `json:"user_id"`
	FirstName             string   `json:"first_name,omitempty"`
	LastName              string   `json:"last_name,omitempty"`
	Email                 string   `json:"email,omitempty"`
	Status                string   `json:"status"`
	PrimaryRewardsAccount string   `json:"primary_rewards_account,omitempty"`
	Credentials           []string `json:"credentials"`
}

func (a *reportCore) improversReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	status := strings.TrimSpace(request.GetString("status", ""))
	search := strings.TrimSpace(request.GetString("search", ""))
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	searchPattern := ""
	if search != "" {
		searchPattern = "%" + search + "%"
	}

	improvers := []improverRow{}
	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				i.user_id,
				COALESCE(i.first_name, ''),
				COALESCE(i.last_name, ''),
				COALESCE(i.email, ''),
				i.status,
				COALESCE(i.primary_rewards_account, ''),
				COALESCE(ARRAY_AGG(uc.credential_type ORDER BY uc.credential_type)
					FILTER (WHERE uc.credential_type IS NOT NULL AND NOT uc.is_revoked), '{}') AS credentials
			FROM improvers i
			LEFT JOIN user_credentials uc ON uc.user_id = i.user_id
			WHERE ($1 = '' OR i.status = $1)
			AND ($2 = '' OR i.user_id ILIKE $2 OR i.email ILIKE $2
				OR (COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')) ILIKE $2)
			GROUP BY i.user_id
			ORDER BY i.created_at DESC, i.user_id ASC
			LIMIT $3 OFFSET $4;
		`, status, searchPattern, limit, offset)
		if err != nil {
			return fmt.Errorf("query improvers: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row improverRow
			if err := rows.Scan(
				&row.UserID, &row.FirstName, &row.LastName, &row.Email,
				&row.Status, &row.PrimaryRewardsAccount, &row.Credentials,
			); err != nil {
				return err
			}
			improvers = append(improvers, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"page":         page,
		"count":        limit,
		"improvers":    improvers,
	}, nil
}

// ---- legacy per-user role tables (proposer/supervisor/issuer) --------------

type roleHolderRow struct {
	UserID                string `json:"user_id"`
	Organization          string `json:"organization,omitempty"`
	Email                 string `json:"email,omitempty"`
	Nickname              string `json:"nickname,omitempty"`
	Status                string `json:"status"`
	PrimaryRewardsAccount string `json:"primary_rewards_account,omitempty"`
	ContactEmail          string `json:"contact_email,omitempty"`
	ContactName           string `json:"contact_name,omitempty"`
}

func (a *reportCore) roleHoldersReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	roleType := strings.ToLower(strings.TrimSpace(request.GetString("role_type", "")))
	status := strings.TrimSpace(request.GetString("status", ""))
	search := strings.TrimSpace(request.GetString("search", ""))
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	// Whitelist the table + whether it has a rewards column; never interpolate
	// arbitrary input into SQL.
	var table string
	hasRewards := false
	switch roleType {
	case "proposer":
		table = "proposers"
	case "supervisor":
		table = "supervisors"
		hasRewards = true
	case "issuer":
		table = "issuers"
	default:
		return nil, fmt.Errorf("role_type must be one of: proposer, supervisor, issuer")
	}

	rewardsSelect := "''"
	if hasRewards {
		rewardsSelect = "COALESCE(t.primary_rewards_account, '')"
	}

	searchPattern := ""
	if search != "" {
		searchPattern = "%" + search + "%"
	}

	holders := []roleHolderRow{}
	query := fmt.Sprintf(`
		SELECT
			t.user_id,
			COALESCE(t.organization, ''),
			COALESCE(t.email, ''),
			COALESCE(t.nickname, ''),
			t.status,
			%s,
			COALESCE(u.contact_email, ''),
			COALESCE(u.contact_name, '')
		FROM %s t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE ($1 = '' OR t.status = $1)
		AND ($2 = '' OR t.user_id ILIKE $2 OR t.organization ILIKE $2 OR t.email ILIKE $2 OR COALESCE(t.nickname, '') ILIKE $2)
		ORDER BY t.created_at DESC, t.user_id ASC
		LIMIT $3 OFFSET $4;
	`, rewardsSelect, table)

	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, status, searchPattern, limit, offset)
		if err != nil {
			return fmt.Errorf("query %s: %w", table, err)
		}
		defer rows.Close()
		for rows.Next() {
			var row roleHolderRow
			if err := rows.Scan(
				&row.UserID, &row.Organization, &row.Email, &row.Nickname, &row.Status,
				&row.PrimaryRewardsAccount, &row.ContactEmail, &row.ContactName,
			); err != nil {
				return err
			}
			holders = append(holders, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"role_type":    roleType,
		"page":         page,
		"count":        limit,
		"note":         "Role approvals are org-owned (organizations_report). This exposes the underlying per-user role rows.",
		"holders":      holders,
	}, nil
}

// ---- credentials -----------------------------------------------------------

func (a *reportCore) credentialsReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	section := strings.ToLower(strings.TrimSpace(request.GetString("section", "all")))
	credType := strings.TrimSpace(request.GetString("credential_type", ""))
	userID := strings.TrimSpace(request.GetString("user_id", ""))
	requestStatus := strings.TrimSpace(request.GetString("request_status", ""))
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	want := func(name string) bool { return section == "all" || section == name }

	out := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"note":         "Credential badge image bytes are never returned. Holders are active (non-revoked) grants.",
	}

	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		if want("catalog") {
			catalog := []map[string]any{}
			rows, err := tx.Query(ctx, `
				SELECT value, label, COALESCE(visibility, 'public'), (badge_data IS NOT NULL)
				FROM credential_type_definitions
				ORDER BY value ASC;
			`)
			if err != nil {
				return fmt.Errorf("query credential catalog: %w", err)
			}
			for rows.Next() {
				var value, label, visibility string
				var hasBadge bool
				if err := rows.Scan(&value, &label, &visibility, &hasBadge); err != nil {
					rows.Close()
					return err
				}
				catalog = append(catalog, map[string]any{
					"value": value, "label": label, "visibility": visibility, "has_badge": hasBadge,
				})
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			out["catalog"] = catalog
		}

		if want("holders") {
			holders := []map[string]any{}
			rows, err := tx.Query(ctx, `
				SELECT uc.user_id, COALESCE(u.contact_email, ''), uc.credential_type,
					COALESCE(uc.issued_by, ''), uc.issued_at
				FROM user_credentials uc
				LEFT JOIN users u ON u.id = uc.user_id
				WHERE NOT uc.is_revoked
				AND ($1 = '' OR uc.credential_type = $1)
				AND ($2 = '' OR uc.user_id = $2)
				ORDER BY uc.issued_at DESC
				LIMIT $3 OFFSET $4;
			`, credType, userID, limit, offset)
			if err != nil {
				return fmt.Errorf("query credential holders: %w", err)
			}
			for rows.Next() {
				var uid, email, ctype, issuedBy string
				var issuedAt time.Time
				if err := rows.Scan(&uid, &email, &ctype, &issuedBy, &issuedAt); err != nil {
					rows.Close()
					return err
				}
				holders = append(holders, map[string]any{
					"user_id": uid, "contact_email": email, "credential_type": ctype,
					"issued_by": issuedBy, "issued_at": issuedAt.UTC().Format(time.RFC3339),
				})
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			out["holders"] = holders
		}

		if want("scopes") {
			// Which users/orgs are scoped to issue which credential types.
			scopes := []map[string]any{}
			rows, err := tx.Query(ctx, `
				SELECT ics.issuer_id, COALESCE(u.contact_email, ''), ics.credential_type,
					ics.organization_id IS NOT NULL AS org_derived, COALESCE(ics.organization_id, 0)
				FROM issuer_credential_scopes ics
				LEFT JOIN users u ON u.id = ics.issuer_id
				WHERE ($1 = '' OR ics.credential_type = $1)
				ORDER BY ics.credential_type ASC, ics.issuer_id ASC;
			`, credType)
			if err != nil {
				return fmt.Errorf("query issuer scopes: %w", err)
			}
			for rows.Next() {
				var issuerID, email, ctype string
				var orgDerived bool
				var orgID int64
				if err := rows.Scan(&issuerID, &email, &ctype, &orgDerived, &orgID); err != nil {
					rows.Close()
					return err
				}
				entry := map[string]any{
					"issuer_id": issuerID, "contact_email": email, "credential_type": ctype,
					"org_derived": orgDerived,
				}
				if orgID > 0 {
					entry["organization_id"] = orgID
				}
				scopes = append(scopes, entry)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			out["scopes"] = scopes
		}

		if want("requests") {
			reqs := []map[string]any{}
			rows, err := tx.Query(ctx, `
				SELECT cr.id, cr.user_id, COALESCE(u.contact_email, ''), cr.credential_type,
					cr.status, cr.requested_at, cr.resolved_at, COALESCE(cr.resolved_by, '')
				FROM credential_requests cr
				LEFT JOIN users u ON u.id = cr.user_id
				WHERE ($1 = '' OR cr.credential_type = $1)
				AND ($2 = '' OR cr.user_id = $2)
				AND ($3 = '' OR cr.status = $3)
				ORDER BY cr.requested_at DESC
				LIMIT $4 OFFSET $5;
			`, credType, userID, requestStatus, limit, offset)
			if err != nil {
				return fmt.Errorf("query credential requests: %w", err)
			}
			for rows.Next() {
				var id, uid, email, ctype, st, resolvedBy string
				var requestedAt time.Time
				var resolvedAt *time.Time
				if err := rows.Scan(&id, &uid, &email, &ctype, &st, &requestedAt, &resolvedAt, &resolvedBy); err != nil {
					rows.Close()
					return err
				}
				entry := map[string]any{
					"id": id, "user_id": uid, "contact_email": email, "credential_type": ctype,
					"status": st, "requested_at": requestedAt.UTC().Format(time.RFC3339), "resolved_by": resolvedBy,
				}
				if resolvedAt != nil {
					entry["resolved_at"] = resolvedAt.UTC().Format(time.RFC3339)
				}
				reqs = append(reqs, entry)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			out["requests"] = reqs
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---- workflow votes --------------------------------------------------------

func (a *reportCore) workflowVotes(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	workflowID := strings.TrimSpace(request.GetString("workflow_id", ""))
	if workflowID == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}

	votes := []map[string]any{}
	var decision, finalizedBy string
	var voteDecision, quorumReachedAt, finalizeAt, finalizedAt *int64

	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		var vd *string
		err := tx.QueryRow(ctx, `
			SELECT w.vote_decision, w.vote_quorum_reached_at, w.vote_finalize_at,
				w.vote_finalized_at, COALESCE(w.vote_finalized_by_user_id, '')
			FROM workflows w WHERE w.id = $1;
		`, workflowID).Scan(&vd, &quorumReachedAt, &finalizeAt, &finalizedAt, &finalizedBy)
		if err == pgx.ErrNoRows {
			return fmt.Errorf("workflow %q not found", workflowID)
		}
		if err != nil {
			return fmt.Errorf("query workflow vote header: %w", err)
		}
		if vd != nil {
			decision = *vd
		}
		_ = voteDecision

		rows, err := tx.Query(ctx, `
			SELECT wv.voter_id, COALESCE(u.contact_email, ''), wv.decision, COALESCE(wv.comment, ''), wv.created_at
			FROM workflow_votes wv
			LEFT JOIN users u ON u.id = wv.voter_id
			WHERE wv.workflow_id = $1
			ORDER BY wv.created_at ASC;
		`, workflowID)
		if err != nil {
			return fmt.Errorf("query workflow votes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var voterID, email, dec, comment string
			var createdAt int64
			if err := rows.Scan(&voterID, &email, &dec, &comment, &createdAt); err != nil {
				return err
			}
			votes = append(votes, map[string]any{
				"voter_id": voterID, "contact_email": email, "decision": dec,
				"comment": comment, "created_at": createdAt,
			})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	approve, deny := 0, 0
	for _, v := range votes {
		switch v["decision"] {
		case "approve":
			approve++
		case "deny":
			deny++
		}
	}

	return map[string]any{
		"generated_at":           time.Now().UTC().Format(time.RFC3339),
		"workflow_id":            workflowID,
		"vote_decision":          decision,
		"vote_quorum_reached_at": quorumReachedAt,
		"vote_finalize_at":       finalizeAt,
		"vote_finalized_at":      finalizedAt,
		"vote_finalized_by":      finalizedBy,
		"approve_count":          approve,
		"deny_count":             deny,
		"total_votes":            len(votes),
		"votes":                  votes,
	}, nil
}
