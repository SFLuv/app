package mcp

import (
	"context"
	"strings"
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
		mcp.WithDescription("List redemption/faucet events with their creator (owner) and per-event fine-grained metrics: redemption code counts (total/redeemed/unredeemed) and $SFLUV amounts (per code, total granted, redeemed). Owner distinguishes admin-created from affiliate-created events; match owner against search_users or affiliate_report to attribute events. All amounts here are WHOLE $SFLUV (not base units)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("owner", mcp.Description("Optional creator user id to filter by (affiliate or admin).")),
		mcp.WithString("event_id", mcp.Description("Optional exact event id to fetch a single event.")),
		mcp.WithString("search", mcp.Description("Optional case-insensitive search over event title and description.")),
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
	// AmountSfluv is the whole-$SFLUV value of a single redemption code.
	AmountSfluv int64   `json:"amount_sfluv"`
	Owner       *string `json:"owner,omitempty"`
	// Per-event fine-grained metrics (whole $SFLUV; not base units).
	CodeCount           int   `json:"code_count"`
	RedeemedCount       int   `json:"redeemed_count"`
	UnredeemedCount     int   `json:"unredeemed_count"`
	RedeemedAmountSfluv int64 `json:"redeemed_amount_sfluv"`
	TotalAmountSfluv    int64 `json:"total_amount_sfluv"`
}

func (a *reportCore) eventsReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	owner := strings.TrimSpace(request.GetString("owner", ""))
	eventID := strings.TrimSpace(request.GetString("event_id", ""))
	search := strings.TrimSpace(request.GetString("search", ""))
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	searchPattern := ""
	if search != "" {
		searchPattern = "%" + search + "%"
	}

	events := []eventRow{}
	err := withReadOnlyTx(ctx, a.botDB, func(tx pgx.Tx) error {
		// One code == one redemption of `amount` whole $SFLUV, so per-event
		// totals derive from the codes join: redeemed_count * amount is the
		// $SFLUV actually handed out, code_count * amount is the amount funded.
		rows, err := tx.Query(ctx, `
			SELECT
				e.id,
				e.title,
				e.description,
				e.start_at,
				e.expiration,
				COALESCE(e.amount, 0) AS amount,
				e.owner,
				COUNT(c.id)::int AS code_count,
				COUNT(c.id) FILTER (WHERE COALESCE(c.redeemed, FALSE))::int AS redeemed_count
			FROM events e
			LEFT JOIN codes c ON c.event = e.id
			WHERE ($1 = '' OR e.owner = $1)
			AND ($2 = '' OR e.id = $2)
			AND ($3 = '' OR e.title ILIKE $3 OR e.description ILIKE $3)
			GROUP BY e.id
			ORDER BY e.start_at DESC, e.id ASC
			LIMIT $4 OFFSET $5;
		`, owner, eventID, searchPattern, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row eventRow
			if err := rows.Scan(
				&row.ID, &row.Title, &row.Description, &row.StartAt, &row.Expiration,
				&row.AmountSfluv, &row.Owner, &row.CodeCount, &row.RedeemedCount,
			); err != nil {
				return err
			}
			row.UnredeemedCount = row.CodeCount - row.RedeemedCount
			row.RedeemedAmountSfluv = int64(row.RedeemedCount) * row.AmountSfluv
			row.TotalAmountSfluv = int64(row.CodeCount) * row.AmountSfluv
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
		"note":         "All amounts are WHOLE $SFLUV (not base units). amount_sfluv is the value of one redemption code; redeemed_amount_sfluv = redeemed_count * amount_sfluv; total_amount_sfluv = code_count * amount_sfluv. owner is the creating user id (admin or affiliate).",
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
		"note":         "weekly_allocation, weekly_balance, and one_time_balance are whole $SFLUV (not base units).",
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
			{"question": "How many codes were redeemed for a given event and how much $SFLUV went out?", "how": "events_report with the event_id (or search); read redeemed_count and redeemed_amount_sfluv (whole $SFLUV)."},
			{"question": "Find a workflow by name and see what an improver reported.", "how": "workflow_report with search=<title>, then workflow_detail with that workflow_id to read each step's submission (written/dropdown responses, step-not-possible notes, photo metadata)."},
		},
		"amount_units_note": "Amounts are $SFLUV, never wei. Whole-$SFLUV fields end in _sfluv (event amounts, redemption totals). Base-unit fields end in _sfluv_base and use 6 decimals — divide by 1,000,000 for whole $SFLUV (transactions, financial_summary, workflow bounties, balances).",
		"security_note":     "This MCP surface is read-only and admin-gated via Privy JWT. It never returns API keys, private keys, encrypted OAuth credentials, push tokens, W9 documents, PIN hashes, or workflow photo image bytes.",
	}, nil
}
