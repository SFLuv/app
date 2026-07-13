package mcp

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerExtraTools adds the SFLUV-specific read-only reports that the ported
// base set does not cover: events (admin- and affiliate-created), affiliate
// program data, and a static app-structure guide for "where is the control
// for X" questions. All are read-only and expose no secrets.
func (a *reportCore) registerExtraTools(s *server.MCPServer) {
	a.addTool(s, mcp.NewTool("events_report",
		mcp.WithDescription("List redemption/faucet events with their creator (owner). Owner distinguishes admin-created from affiliate-created events; match owner against search_users or affiliate_report to attribute events."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("owner", mcp.Description("Optional creator user id to filter by (affiliate or admin).")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.eventsReport)

	a.addTool(s, mcp.NewTool("affiliate_report",
		mcp.WithDescription("List affiliate organizations with status, weekly allocation/balance, one-time balance, active state, and owner contact details."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("status", mcp.Description("Optional status filter (e.g. pending, approved).")),
		mcp.WithBoolean("active_only", mcp.Description("Only include active (non-deleted) affiliates. Defaults to false.")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.affiliateReport)

	a.addTool(s, mcp.NewTool("app_structure",
		mcp.WithDescription("Explain where features and admin controls live in the SFLUV app (e.g. where merchants are approved), and which report tool exposes which data. Use this to answer navigation/where-is questions."),
		mcp.WithReadOnlyHintAnnotation(true),
	), a.appStructure)
}

type eventRow struct {
	ID          string  `json:"id"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	StartAt     int64   `json:"start_at"`
	Expiration  *int64  `json:"expiration,omitempty"`
	Amount      int64   `json:"amount"`
	Owner       *string `json:"owner,omitempty"`
}

func (a *reportCore) eventsReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	owner := request.GetString("owner", "")
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	events := []eventRow{}
	err := withReadOnlyTx(ctx, a.botDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, title, description, start_at, expiration, amount, owner
			FROM events
			WHERE ($1 = '' OR owner = $1)
			ORDER BY start_at DESC, id ASC
			LIMIT $2 OFFSET $3;
		`, owner, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row eventRow
			if err := rows.Scan(&row.ID, &row.Title, &row.Description, &row.StartAt, &row.Expiration, &row.Amount, &row.Owner); err != nil {
				return err
			}
			events = append(events, row)
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
		"note":         "amount is denominated in the faucet's smallest token unit; owner is the creating user id (admin or affiliate).",
		"events":       events,
	}, nil
}

type affiliateRow struct {
	UserID           string  `json:"user_id"`
	Organization     string  `json:"organization"`
	Nickname         *string `json:"nickname,omitempty"`
	Status           string  `json:"status"`
	WeeklyAllocation int64   `json:"weekly_allocation"`
	WeeklyBalance    int64   `json:"weekly_balance"`
	OneTimeBalance   int64   `json:"one_time_balance"`
	Active           bool    `json:"active"`
	CreatedAt        string  `json:"created_at,omitempty"`
	ContactEmail     string  `json:"contact_email,omitempty"`
	ContactName      string  `json:"contact_name,omitempty"`
}

func (a *reportCore) affiliateReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	status := request.GetString("status", "")
	activeOnly := request.GetBool("active_only", false)
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	affiliates := []affiliateRow{}
	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				af.user_id,
				af.organization,
				af.nickname,
				af.status,
				COALESCE(af.weekly_allocation, 0),
				COALESCE(af.weekly_balance, 0),
				COALESCE(af.one_time_balance, 0),
				COALESCE(af.active, TRUE),
				af.created_at,
				COALESCE(u.contact_email, ''),
				COALESCE(u.contact_name, '')
			FROM affiliates af
			LEFT JOIN users u ON u.id = af.user_id
			WHERE ($1 = '' OR af.status = $1)
			AND ($2 = FALSE OR COALESCE(af.active, TRUE) = TRUE)
			ORDER BY af.created_at DESC, af.user_id ASC
			LIMIT $3 OFFSET $4;
		`, status, activeOnly, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row affiliateRow
			var createdAt time.Time
			if err := rows.Scan(
				&row.UserID, &row.Organization, &row.Nickname, &row.Status,
				&row.WeeklyAllocation, &row.WeeklyBalance, &row.OneTimeBalance,
				&row.Active, &createdAt, &row.ContactEmail, &row.ContactName,
			); err != nil {
				return err
			}
			if !createdAt.IsZero() {
				row.CreatedAt = createdAt.UTC().Format(time.RFC3339)
			}
			affiliates = append(affiliates, row)
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
		"note":         "weekly_allocation, weekly_balance, and one_time_balance are denominated in the smallest token unit.",
		"affiliates":   affiliates,
	}, nil
}

// appStructure returns a curated, static description of where things live in the
// SFLUV app so admins can ask "where is the control for X". It reads no data and
// exposes nothing sensitive.
func (a *reportCore) appStructure(ctx context.Context, _ mcp.CallToolRequest) (any, error) {
	return map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"overview":     "SFLUV is a local-currency platform (wrapped HONEY on the active chain) for merchants, improvers, proposers, voters, issuers, and affiliates. Admins manage everything from the /admin panel; role requests happen in /settings.",
		"admin_controls": []map[string]string{
			{"task": "Approve or reject a merchant", "where": "/settings merchant approval flow (admin view) and /admin Users/Merchants management; merchant locations are approved there.", "data_tool": "merchant_report"},
			{"task": "View / search users and roles", "where": "/admin Users tab.", "data_tool": "search_users"},
			{"task": "Grant or revoke credentials (dpw_certified, sfluv_verifier)", "where": "/issuer panel.", "data_tool": "search_users"},
			{"task": "Manage affiliates and their payouts", "where": "/admin Affiliates tab and /affiliates dashboard.", "data_tool": "affiliate_report"},
			{"task": "Create / manage redemption (faucet) events", "where": "/admin events management; affiliates create events from /affiliates.", "data_tool": "events_report"},
			{"task": "Review workflow / volunteer opportunities and payouts", "where": "/proposer (build), /voter (vote), /improver (claim/complete), /admin workflow data.", "data_tool": "workflow_report"},
			{"task": "W9 / tax compliance review", "where": "/admin W9 pending queue; merchants see /merchant-status.", "data_tool": "w9_report"},
			{"task": "Inspect a wallet's owner, balance, and W9 status", "where": "Wallet views; use the lookup tool for reporting.", "data_tool": "lookup_wallet"},
			{"task": "Financial totals over a date range", "where": "/admin analytics dashboard.", "data_tool": "financial_summary"},
			{"task": "Individual on-chain transfers", "where": "Transaction history views.", "data_tool": "transactions"},
		},
		"example_questions": []map[string]string{
			{"question": "How much did merchant \"Tilted Brim\" receive this month?", "how": "merchant_report to find the merchant's location payment wallet address(es), then transactions or financial_summary filtered to those addresses and the month's timestamp range."},
			{"question": "Which users have the improver role?", "how": "search_users, then filter roles for 'improver'."},
			{"question": "What events did affiliate X create?", "how": "affiliate_report to get the affiliate user_id, then events_report with that owner."},
		},
		"security_note": "This MCP surface is read-only and admin-gated via Privy JWT. It never returns API keys, private keys, encrypted OAuth credentials, push tokens, W9 documents, or PIN hashes.",
	}, nil
}
