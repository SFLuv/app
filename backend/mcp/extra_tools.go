package mcp

import (
	"context"
	"errors"
	"regexp"
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

	a.addTool(s, mcp.NewTool("client_versions_report",
		mcp.WithDescription("Count signed-in people by the app version and build they last used, per platform, seen in the last 7 days, the last 30 days, and ever. Use it to see how far a store release has spread, or who an over-the-air update can reach. Counts only: no user ids, contact details, or device details."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("platform", mcp.Description("Optional platform filter (e.g. ios, android).")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.clientVersionsReport)

	a.addTool(s, mcp.NewTool("w9_person_detail",
		mcp.WithDescription("One person's W-9 state for a tax year, for answering \"what have they been told and what happened\": the filing (status, IRS TIN match result and failure count, provider status, timings), each tier prompt with when it was armed and dismissed, a payout summary with the newest payouts and any not yet paid, how many devices can get a push, and the app versions they last used. Never returns taxpayer numbers, form links, or provider request ids. Find the user id with search_users or w9_report."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("The person's user id.")),
		mcp.WithNumber("year", mcp.Description("Tax year. Defaults to the current year.")),
		mcp.WithBoolean("all_payouts", mcp.Description("List up to the newest 100 payouts instead of the newest 5 plus any not yet paid. Defaults to false.")),
	), a.w9PersonDetail)
}

type w9PersonFiling struct {
	Status              string  `json:"status"`
	Provider            string  `json:"provider,omitempty"`
	HasFormLink         bool    `json:"has_form_link"`
	TINType             string  `json:"tin_type,omitempty"`
	TINMatch            string  `json:"tin_match,omitempty"`
	TINMatchAt          *string `json:"tin_match_at,omitempty"`
	TINMismatches       int     `json:"tin_mismatches"`
	ThresholdCrossedAt  *string `json:"threshold_crossed_at,omitempty"`
	RequestedAt         *string `json:"requested_at,omitempty"`
	CompletedAt         *string `json:"completed_at,omitempty"`
	ClearedByUserID     string  `json:"cleared_by_user_id,omitempty"`
	ClearReason         string  `json:"clear_reason,omitempty"`
	LastProviderStatus  string  `json:"last_provider_status,omitempty"`
	LastProviderEventAt *string `json:"last_provider_event_at,omitempty"`
	LastPolledAt        *string `json:"last_polled_at,omitempty"`
	UpdatedAt           *string `json:"updated_at,omitempty"`
}

type w9PersonNotice struct {
	Tier           string  `json:"tier"`
	NotifiedAt     *string `json:"notified_at,omitempty"`
	AcknowledgedAt *string `json:"acknowledged_at,omitempty"`
}

type w9PersonPayout struct {
	Source                string  `json:"source"`
	RecipientAddress      string  `json:"recipient_address"`
	ChainID               int64   `json:"chain_id"`
	AmountSfluvBase       string  `json:"amount_sfluv_base"`
	State                 string  `json:"state"`
	CountsTowardThreshold bool    `json:"counts_toward_threshold"`
	ShadowDecision        string  `json:"shadow_decision,omitempty"`
	TxHash                string  `json:"tx_hash,omitempty"`
	LastError             string  `json:"last_error,omitempty"`
	CreatedAt             *string `json:"created_at,omitempty"`
	EscrowedAt            *string `json:"escrowed_at,omitempty"`
	ReleasedAt            *string `json:"released_at,omitempty"`
	PaidAt                *string `json:"paid_at,omitempty"`
}

type w9PersonClient struct {
	Platform   string  `json:"platform"`
	Version    string  `json:"version"`
	Build      string  `json:"build"`
	LastSeenAt *string `json:"last_seen_at,omitempty"`
}

type w9PersonPayoutState struct {
	State           string `json:"state"`
	Count           int    `json:"count"`
	AmountSfluvBase string `json:"amount_sfluv_base"`
}

type w9PersonPayoutSummary struct {
	Count            int                   `json:"count"`
	CountedSfluvBase string                `json:"counted_sfluv_base"`
	LastPayoutAt     *string               `json:"last_payout_at,omitempty"`
	ByState          []w9PersonPayoutState `json:"by_state"`
}

// A busy volunteer has dozens of payouts, nearly all small and paid, and the
// list is what makes this tool's answer long. By default it holds the newest
// few and whatever has not been paid, the rows a question is usually about.
const (
	w9PersonRecentPayouts = 5
	w9PersonPayoutLimit   = 100
)

// A failed send's error can quote the RPC endpoint, and that URL can carry a
// provider key, so URLs are taken out of last_error before it leaves here.
var errorURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"']+`)

func optionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.UTC().Format(time.RFC3339)
	return &formatted
}

func (a *reportCore) w9PersonDetail(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	userID := strings.TrimSpace(request.GetString("user_id", ""))
	if userID == "" {
		return nil, errors.New("user_id is required")
	}
	year := request.GetInt("year", time.Now().UTC().Year())
	allPayouts := request.GetBool("all_payouts", false)

	var filing *w9PersonFiling
	summary := w9PersonPayoutSummary{CountedSfluvBase: "0", ByState: []w9PersonPayoutState{}}
	notices := []w9PersonNotice{}
	payouts := []w9PersonPayout{}
	clients := []w9PersonClient{}
	pushDevices := 0
	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		var f w9PersonFiling
		var tinMatchAt, crossedAt, requestedAt, completedAt, providerEventAt, polledAt, updatedAt *time.Time
		err := tx.QueryRow(ctx, `
			SELECT
				status, provider, form_url <> '', tin_type, tin_match, tin_match_at,
				tin_mismatch_count, threshold_crossed_at, requested_at, completed_at,
				cleared_by_user_id, clear_reason, last_provider_status,
				last_provider_event_at, last_polled_at, updated_at
			FROM w9_filings
			WHERE user_id = $1 AND tax_year = $2;
		`, userID, year).Scan(
			&f.Status, &f.Provider, &f.HasFormLink, &f.TINType, &f.TINMatch, &tinMatchAt,
			&f.TINMismatches, &crossedAt, &requestedAt, &completedAt,
			&f.ClearedByUserID, &f.ClearReason, &f.LastProviderStatus,
			&providerEventAt, &polledAt, &updatedAt,
		)
		switch {
		case err == nil:
			f.TINMatchAt = optionalTime(tinMatchAt)
			f.ThresholdCrossedAt = optionalTime(crossedAt)
			f.RequestedAt = optionalTime(requestedAt)
			f.CompletedAt = optionalTime(completedAt)
			f.LastProviderEventAt = optionalTime(providerEventAt)
			f.LastPolledAt = optionalTime(polledAt)
			f.UpdatedAt = optionalTime(updatedAt)
			filing = &f
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}

		noticeRows, err := tx.Query(ctx, `
			SELECT tier, notified_at, acknowledged_at
			FROM w9_tier_notices
			WHERE user_id = $1 AND tax_year = $2
			ORDER BY notified_at ASC, tier ASC;
		`, userID, year)
		if err != nil {
			return err
		}
		defer noticeRows.Close()
		for noticeRows.Next() {
			var n w9PersonNotice
			var notifiedAt, acknowledgedAt *time.Time
			if err := noticeRows.Scan(&n.Tier, &notifiedAt, &acknowledgedAt); err != nil {
				return err
			}
			n.NotifiedAt = optionalTime(notifiedAt)
			n.AcknowledgedAt = optionalTime(acknowledgedAt)
			notices = append(notices, n)
		}
		if err := noticeRows.Err(); err != nil {
			return err
		}

		// Counted the way the payout gate counts, so failed and cancelled rows
		// stay out of the total.
		var lastPayoutAt *time.Time
		if err := tx.QueryRow(ctx, `
			SELECT
				COUNT(*)::int,
				COALESCE(SUM(amount_base) FILTER (
					WHERE counts_toward_threshold
					AND state IN ('pending','escrowed','releasing','paid')
				), 0)::text,
				MAX(created_at)
			FROM payout_ledger
			WHERE user_id = $1 AND tax_year = $2;
		`, userID, year).Scan(&summary.Count, &summary.CountedSfluvBase, &lastPayoutAt); err != nil {
			return err
		}
		summary.LastPayoutAt = optionalTime(lastPayoutAt)

		stateRows, err := tx.Query(ctx, `
			SELECT state, COUNT(*)::int, SUM(amount_base)::text
			FROM payout_ledger
			WHERE user_id = $1 AND tax_year = $2
			GROUP BY state
			ORDER BY state ASC;
		`, userID, year)
		if err != nil {
			return err
		}
		defer stateRows.Close()
		for stateRows.Next() {
			var s w9PersonPayoutState
			if err := stateRows.Scan(&s.State, &s.Count, &s.AmountSfluvBase); err != nil {
				return err
			}
			summary.ByState = append(summary.ByState, s)
		}
		if err := stateRows.Err(); err != nil {
			return err
		}

		payoutRows, err := tx.Query(ctx, `
			SELECT
				source, recipient_address, chain_id, amount_base::text, state,
				counts_toward_threshold, shadow_decision, tx_hash, last_error,
				created_at, escrowed_at, released_at, paid_at
			FROM payout_ledger
			WHERE user_id = $1 AND tax_year = $2
			AND ($3 OR state <> 'paid' OR id IN (
				SELECT recent.id
				FROM payout_ledger recent
				WHERE recent.user_id = $1 AND recent.tax_year = $2
				ORDER BY recent.created_at DESC, recent.id DESC
				LIMIT $4
			))
			ORDER BY created_at DESC, id DESC
			LIMIT $5;
		`, userID, year, allPayouts, w9PersonRecentPayouts, w9PersonPayoutLimit)
		if err != nil {
			return err
		}
		defer payoutRows.Close()
		for payoutRows.Next() {
			var p w9PersonPayout
			var createdAt, escrowedAt, releasedAt, paidAt *time.Time
			if err := payoutRows.Scan(
				&p.Source, &p.RecipientAddress, &p.ChainID, &p.AmountSfluvBase, &p.State,
				&p.CountsTowardThreshold, &p.ShadowDecision, &p.TxHash, &p.LastError,
				&createdAt, &escrowedAt, &releasedAt, &paidAt,
			); err != nil {
				return err
			}
			p.LastError = errorURL.ReplaceAllString(p.LastError, "[url]")
			p.CreatedAt = optionalTime(createdAt)
			p.EscrowedAt = optionalTime(escrowedAt)
			p.ReleasedAt = optionalTime(releasedAt)
			p.PaidAt = optionalTime(paidAt)
			payouts = append(payouts, p)
		}
		if err := payoutRows.Err(); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `
			SELECT COUNT(DISTINCT token)::int
			FROM mobile_push_subscriptions
			WHERE owner = $1 AND active AND device_registered;
		`, userID).Scan(&pushDevices); err != nil {
			return err
		}

		clientRows, err := tx.Query(ctx, `
			SELECT platform, version, build, last_seen_at
			FROM user_client_versions
			WHERE user_id = $1
			ORDER BY last_seen_at DESC;
		`, userID)
		if err != nil {
			return err
		}
		defer clientRows.Close()
		for clientRows.Next() {
			var c w9PersonClient
			var lastSeen *time.Time
			if err := clientRows.Scan(&c.Platform, &c.Version, &c.Build, &lastSeen); err != nil {
				return err
			}
			c.LastSeenAt = optionalTime(lastSeen)
			clients = append(clients, c)
		}
		return clientRows.Err()
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
		"user_id":        userID,
		"year":           year,
		"note":           "filing is null when no filing exists for the year. acknowledged_at on a tier prompt means it was dismissed, not that a W-9 was filed. _sfluv_base amounts are 6-decimal base units on every chain: divide by 1,000,000. counted_sfluv_base is the payout ledger only; w9_report's total can be higher where an older wallet total was never imported. payouts holds the newest 5 plus any not yet paid, unless all_payouts is set, which lists up to the newest 100.",
		"filing":         filing,
		"tier_notices":   notices,
		"payout_summary": summary,
		"payouts":        payouts,
		"push_devices":   pushDevices,
		"app_versions":   clients,
	}, nil
}

type clientVersionRow struct {
	Platform       string `json:"platform"`
	Version        string `json:"version"`
	Build          string `json:"build"`
	LegacyInferred bool   `json:"legacy_inferred"`
	Seen7d         int    `json:"seen_7d"`
	Seen30d        int    `json:"seen_30d"`
	Ever           int    `json:"ever"`
	LastSeenAt     string `json:"last_seen_at"`
}

func (a *reportCore) clientVersionsReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	platform := strings.ToLower(strings.TrimSpace(request.GetString("platform", "")))
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	versions := []clientVersionRow{}
	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				platform,
				version,
				build,
				legacy_inferred,
				COUNT(*) FILTER (WHERE last_seen_at > NOW() - INTERVAL '7 days')::int AS seen_7d,
				COUNT(*) FILTER (WHERE last_seen_at > NOW() - INTERVAL '30 days')::int AS seen_30d,
				COUNT(*)::int AS ever,
				MAX(last_seen_at)
			FROM user_client_versions
			WHERE ($1 = '' OR platform = $1)
			GROUP BY platform, version, build, legacy_inferred
			ORDER BY seen_30d DESC, ever DESC, platform ASC, version ASC, build ASC
			LIMIT $2 OFFSET $3;
		`, platform, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row clientVersionRow
			var lastSeen time.Time
			if err := rows.Scan(
				&row.Platform, &row.Version, &row.Build, &row.LegacyInferred,
				&row.Seen7d, &row.Seen30d, &row.Ever, &lastSeen,
			); err != nil {
				return err
			}
			row.LastSeenAt = lastSeen.UTC().Format(time.RFC3339)
			versions = append(versions, row)
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
		"note":         "One count per person per platform, at the version they used most recently, so seen_* are people, not installs or downloads. Signed-in requests only: someone who never signed in is not counted. Version is the installed store build; over-the-air updates do not change it. legacy_inferred rows are old apps that send no version, recorded as 1.0.0 from their user agent.",
		"versions":     versions,
	}, nil
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
			{"task": "Approve, set up or reject a merchant", "where": "/settings merchant approval flow (admin view) and /admin Users/Merchants management; merchant locations are approved there. Setup approves a location but keeps it off the map and public lists.", "data_tool": "merchant_report"},
			{"task": "View / search users and roles", "where": "/admin Users tab.", "data_tool": "search_users"},
			{"task": "Manage organizations (members, role approvals, allocations, issuer scopes)", "where": "/admin Organizations tab. Orgs own affiliate/proposer/supervisor/issuer roles.", "data_tool": "organizations_report"},
			{"task": "Grant or revoke credentials; configure who can issue what", "where": "/issuer panel; org issuer scopes in /admin Organizations. Credential type catalog in /admin Credential Types.", "data_tool": "credentials_report"},
			{"task": "Review improver volunteers and their credentials", "where": "/admin Improvers tab.", "data_tool": "improvers_report"},
			{"task": "Review proposer/supervisor/issuer role holders", "where": "role approvals live in /admin Organizations now.", "data_tool": "role_holders_report"},
			{"task": "Review workflow governance votes", "where": "/voter queue; admins force-approve from /admin workflow data.", "data_tool": "workflow_votes"},
			{"task": "Manage affiliates and their payouts", "where": "/admin Organizations (affiliate role) and /affiliates dashboard.", "data_tool": "affiliate_report"},
			{"task": "Create / manage redemption (faucet) events", "where": "/admin events management; affiliates create events from /affiliates.", "data_tool": "events_report"},
			{"task": "Review workflow / volunteer opportunities and payouts", "where": "/proposer (build), /voter (vote), /improver (claim/complete), /admin workflow data.", "data_tool": "workflow_report"},
			{"task": "W9 / tax compliance review", "where": "/admin W9 pending queue; merchants see /merchant-status.", "data_tool": "w9_report"},
			{"task": "Inspect a wallet's owner, balance, and W9 status", "where": "Wallet views; use the lookup tool for reporting.", "data_tool": "lookup_wallet"},
			{"task": "Financial totals over a date range", "where": "/admin analytics dashboard.", "data_tool": "financial_summary"},
			{"task": "Individual on-chain transfers", "where": "Transaction history views.", "data_tool": "transactions"},
			{"task": "Debug a mobile app error or a reported bug", "where": "Sentry (one login, ops@sfluv.org); read it here instead.", "data_tool": "sentry_issues, then sentry_issue and sentry_events"},
			{"task": "See which app versions people are running", "where": "Not shown in one place in the app; read it here.", "data_tool": "client_versions_report"},
			{"task": "See how a new release is spreading and whether it crashes", "where": "Sentry; read it here instead.", "data_tool": "sentry_release_health"},
			{"task": "Work out what one person was told about their W-9", "where": "/admin Tax & Escrow shows part of it; the full trail is here.", "data_tool": "w9_person_detail"},
		},
		"example_questions": []map[string]string{
			{"question": "How much did merchant \"Tilted Brim\" receive this month?", "how": "merchant_report to find the merchant's location payment wallet address(es), then transactions or financial_summary filtered to those addresses and the month's timestamp range."},
			{"question": "Which users have the improver role?", "how": "search_users, then filter roles for 'improver'."},
			{"question": "What events did affiliate X create?", "how": "affiliate_report to get the affiliate user_id, then events_report with that owner."},
			{"question": "How many codes were redeemed for a given event and how much $SFLUV went out?", "how": "events_report with the event_id (or search); read redeemed_count and redeemed_amount_sfluv (whole $SFLUV)."},
			{"question": "Find a workflow by name and see what an improver reported.", "how": "workflow_report with search=<title>, then workflow_detail with that workflow_id to read each step's submission (written/dropdown responses, step-not-possible notes, photo metadata)."},
			{"question": "Someone says the app broke for them. What happened?", "how": "search_users for their id (a Privy DID), then sentry_issues (optionally version=, environment=preview|production); sentry_events on a likely issue shows whether it hit that DID, and sentry_issue shows the stack and the requests before it. A request_id there can be searched for in the backend's Railway logs."},
		},
		"amount_units_note": "Amounts are $SFLUV, never wei. Whole-$SFLUV fields end in _sfluv (event amounts, redemption totals). Base-unit fields end in _sfluv_base and use 6 decimals — divide by 1,000,000 for whole $SFLUV (transactions, financial_summary, workflow bounties, balances).",
		"security_note":     "This MCP surface is read-only and admin-gated via Privy JWT. It never returns API keys, private keys, encrypted OAuth credentials, push tokens, W9 documents, PIN hashes, or workflow photo image bytes.",
	}, nil
}
