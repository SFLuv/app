package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bootstrap"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	defaultChainID = int64(80094)
	defaultLimit   = 100
	maxLimit       = 500
	queryTimeout   = 20 * time.Second
	zeroAddress    = "0x0000000000000000000000000000000000000000"
)

type adminMCP struct {
	appDB    *pgxpool.Pool
	botDB    *pgxpool.Pool
	ponderDB *pgxpool.Pool
	chainID  int64
}

func main() {
	bootstrap.LoadEnv()
	pools, err := bootstrap.OpenDBPools(true)
	if err != nil {
		log.Fatal(err)
	}
	defer pools.Close()

	admin := &adminMCP{
		appDB:    pools.App,
		botDB:    pools.Bot,
		ponderDB: pools.Ponder,
		chainID:  envInt64(defaultChainID, "SFLUV_CHAIN_ID", "ACTIVE_CHAIN_ID", "CHAIN_ID", "NEXT_PUBLIC_CHAIN_ID"),
	}

	mcpServer := admin.newServer()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ADMIN_MCP_TRANSPORT")), "http") || strings.TrimSpace(os.Getenv("MCP_HTTP_ADDR")) != "" {
		if err := serveHTTP(admin, mcpServer); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatal(err)
	}
}

func serveHTTP(admin *adminMCP, mcpServer *server.MCPServer) error {
	addr := envOrDefault("MCP_HTTP_ADDR", ":8090")
	mux := http.NewServeMux()
	streamable := server.NewStreamableHTTPServer(mcpServer)
	newOAuthServer(admin.appDB).register(mux, streamable)
	return http.ListenAndServe(addr, mux)
}

func (a *adminMCP) newServer() *server.MCPServer {
	s := server.NewMCPServer(
		"sfluv-admin-readonly",
		"0.1.0",
		server.WithInstructions("Read-only SFLUV admin data access. Use named report tools only. Do not ask for raw SQL. Every response includes generated_at when useful; financial data is denominated in wei unless labeled otherwise."),
		server.WithToolCapabilities(false),
	)

	a.addTool(s, mcp.NewTool("admin_report_catalog",
		mcp.WithDescription("List available read-only SFLUV admin report tools and their intended use."),
		mcp.WithReadOnlyHintAnnotation(true),
	), a.reportCatalog)

	a.addTool(s, mcp.NewTool("search_users",
		mcp.WithDescription("Search active users with roles, contact fields, primary wallet, and all active wallet addresses."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("search", mcp.Description("Optional case-insensitive search over user id, contact fields, primary wallet, and wallet addresses.")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.searchUsers)

	a.addTool(s, mcp.NewTool("lookup_wallet",
		mcp.WithDescription("Look up a wallet address across user wallets, merchant location payment wallets, balances, and W9 rows."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("address", mcp.Description("Wallet address to inspect."), mcp.Required()),
		mcp.WithNumber("year", mcp.Description("Optional W9/reporting year; defaults to current UTC year.")),
	), a.lookupWallet)

	a.addTool(s, mcp.NewTool("financial_summary",
		mcp.WithDescription("Summarize token transfers, rewards, merchant payments, redemptions, workflow costs, and volunteer events for a date range."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithNumber("start_timestamp", mcp.Description("Inclusive Unix timestamp. Defaults to 0.")),
		mcp.WithNumber("end_timestamp", mcp.Description("Exclusive Unix timestamp. Defaults to now.")),
		mcp.WithNumber("chain_id", mcp.Description("Chain id for analytics role history. Defaults to active SFLUV chain.")),
	), a.financialSummary)

	a.addTool(s, mcp.NewTool("transactions",
		mcp.WithDescription("List indexed token transfers from the Ponder database by address, hash, and time range."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("address", mcp.Description("Optional sender or recipient address.")),
		mcp.WithString("hash", mcp.Description("Optional transaction hash.")),
		mcp.WithNumber("start_timestamp", mcp.Description("Inclusive Unix timestamp. Defaults to 0.")),
		mcp.WithNumber("end_timestamp", mcp.Description("Exclusive Unix timestamp. Defaults to now.")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.transactions)

	a.addTool(s, mcp.NewTool("w9_report",
		mcp.WithDescription("Report W9 wallet earnings and submission status without exposing stored W9 document URLs."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithNumber("year", mcp.Description("Optional tax year.")),
		mcp.WithString("wallet_address", mcp.Description("Optional wallet address.")),
		mcp.WithString("user_id", mcp.Description("Optional user id.")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.w9Report)

	a.addTool(s, mcp.NewTool("merchant_report",
		mcp.WithDescription("List merchant locations, owner contact details, approval state, and configured payment wallets."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithBoolean("approved_only", mcp.Description("Only include approved locations. Defaults to false.")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.merchantReport)

	a.addTool(s, mcp.NewTool("workflow_report",
		mcp.WithDescription("List workflow financial and lifecycle rows, including proposer/improver attribution and payout status."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("status", mcp.Description("Optional workflow status filter.")),
		mcp.WithNumber("start_timestamp", mcp.Description("Inclusive created_at Unix timestamp. Defaults to 0.")),
		mcp.WithNumber("end_timestamp", mcp.Description("Exclusive created_at Unix timestamp. Defaults to now.")),
		mcp.WithNumber("page", mcp.Description("Zero-based page number.")),
		mcp.WithNumber("count", mcp.Description("Rows per page, capped at 500.")),
	), a.workflowReport)

	return s
}

func (a *adminMCP) addTool(s *server.MCPServer, tool mcp.Tool, handler func(context.Context, mcp.CallToolRequest) (any, error)) {
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, queryTimeout)
		defer cancel()

		data, err := handler(ctx, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultJSON(data)
	})
}

func (a *adminMCP) reportCatalog(ctx context.Context, _ mcp.CallToolRequest) (any, error) {
	return map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"tools": []map[string]string{
			{"name": "search_users", "use": "active users, roles, emails, phones, primary wallet, wallet list"},
			{"name": "lookup_wallet", "use": "owner, merchant association, balance, W9 status for one wallet"},
			{"name": "financial_summary", "use": "date-range transfer volume, rewards, merchant spend, redemptions, workflow costs"},
			{"name": "transactions", "use": "indexed transfer rows by address, hash, or date range"},
			{"name": "w9_report", "use": "wallet earnings and W9 submission state without W9 document URLs"},
			{"name": "merchant_report", "use": "merchant locations and payment wallets"},
			{"name": "workflow_report", "use": "workflow lifecycle and payout reporting"},
		},
	}, nil
}

type userRow struct {
	ID                   string        `json:"id"`
	Roles                []string      `json:"roles"`
	ContactEmail         string        `json:"contact_email,omitempty"`
	ContactPhone         string        `json:"contact_phone,omitempty"`
	ContactName          string        `json:"contact_name,omitempty"`
	PrimaryWalletAddress string        `json:"primary_wallet_address"`
	MailingListOptIn     bool          `json:"mailing_list_opt_in"`
	Wallets              []walletBrief `json:"wallets"`
}

type walletBrief struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	IsEOA        bool   `json:"is_eoa"`
	IsRedeemer   bool   `json:"is_redeemer"`
	IsMinter     bool   `json:"is_minter"`
	EOAAddress   string `json:"eoa_address"`
	SmartAddress string `json:"smart_address,omitempty"`
	SmartIndex   int    `json:"smart_index,omitempty"`
}

func (a *adminMCP) searchUsers(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	search := strings.TrimSpace(request.GetString("search", ""))
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	var users []userRow
	var total int
	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		var err error
		total, err = countUsers(ctx, tx, search)
		if err != nil {
			return err
		}

		rows, err := tx.Query(ctx, `
			WITH filtered AS (
				SELECT
					u.id,
					u.is_admin,
					u.is_merchant,
					u.is_organizer,
					u.is_improver,
					u.is_proposer,
					u.is_voter,
					u.is_issuer,
					u.is_supervisor,
					u.is_affiliate,
					COALESCE(u.contact_email, '') AS contact_email,
					COALESCE(u.contact_phone, '') AS contact_phone,
					COALESCE(u.contact_name, '') AS contact_name,
					COALESCE(u.primary_wallet_address, '') AS primary_wallet_address,
					u.mailing_list_opt_in
				FROM users u
				WHERE COALESCE(u.active, TRUE) = TRUE
				AND (
					$1 = ''
					OR u.id ILIKE '%' || $1 || '%'
					OR COALESCE(u.contact_email, '') ILIKE '%' || $1 || '%'
					OR COALESCE(u.contact_phone, '') ILIKE '%' || $1 || '%'
					OR COALESCE(u.contact_name, '') ILIKE '%' || $1 || '%'
					OR COALESCE(u.primary_wallet_address, '') ILIKE '%' || $1 || '%'
					OR EXISTS (
						SELECT 1
						FROM wallets w
						WHERE w.owner = u.id
						AND COALESCE(w.active, TRUE) = TRUE
						AND (
							COALESCE(w.eoa_address, '') ILIKE '%' || $1 || '%'
							OR COALESCE(w.smart_address, '') ILIKE '%' || $1 || '%'
						)
					)
				)
				ORDER BY u.id ASC
				LIMIT $2
				OFFSET $3
			),
			wallet_rows AS (
				SELECT
					w.owner,
					jsonb_agg(jsonb_build_object(
						'id', w.id,
						'name', w.name,
						'is_eoa', w.is_eoa,
						'is_redeemer', w.is_redeemer,
						'is_minter', w.is_minter,
						'eoa_address', LOWER(TRIM(w.eoa_address)),
						'smart_address', LOWER(TRIM(COALESCE(w.smart_address, ''))),
						'smart_index', COALESCE(w.smart_index, 0)
					) ORDER BY w.id) AS wallets
				FROM wallets w
				WHERE COALESCE(w.active, TRUE) = TRUE
				AND w.owner IN (SELECT id FROM filtered)
				GROUP BY w.owner
			)
			SELECT
				f.*,
				COALESCE(w.wallets, '[]'::jsonb)
			FROM filtered f
			LEFT JOIN wallet_rows w ON w.owner = f.id
			ORDER BY f.id ASC;
		`, search, limit, offset)
		if err != nil {
			return fmt.Errorf("query users: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var row userRow
			var booleans [10]bool
			var walletsJSON []byte
			if err := rows.Scan(
				&row.ID,
				&booleans[0],
				&booleans[1],
				&booleans[2],
				&booleans[3],
				&booleans[4],
				&booleans[5],
				&booleans[6],
				&booleans[7],
				&booleans[8],
				&row.ContactEmail,
				&row.ContactPhone,
				&row.ContactName,
				&row.PrimaryWalletAddress,
				&row.MailingListOptIn,
				&walletsJSON,
			); err != nil {
				return fmt.Errorf("scan user: %w", err)
			}
			row.Roles = rolesFromBools(booleans)
			if err := json.Unmarshal(walletsJSON, &row.Wallets); err != nil {
				return fmt.Errorf("decode wallets: %w", err)
			}
			users = append(users, row)
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
		"total":        total,
		"users":        users,
	}, nil
}

func countUsers(ctx context.Context, tx pgx.Tx, search string) (int, error) {
	var total int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM users u
		WHERE COALESCE(u.active, TRUE) = TRUE
		AND (
			$1 = ''
			OR u.id ILIKE '%' || $1 || '%'
			OR COALESCE(u.contact_email, '') ILIKE '%' || $1 || '%'
			OR COALESCE(u.contact_phone, '') ILIKE '%' || $1 || '%'
			OR COALESCE(u.contact_name, '') ILIKE '%' || $1 || '%'
			OR COALESCE(u.primary_wallet_address, '') ILIKE '%' || $1 || '%'
			OR EXISTS (
				SELECT 1
				FROM wallets w
				WHERE w.owner = u.id
				AND COALESCE(w.active, TRUE) = TRUE
				AND (
					COALESCE(w.eoa_address, '') ILIKE '%' || $1 || '%'
					OR COALESCE(w.smart_address, '') ILIKE '%' || $1 || '%'
				)
			)
		);
	`, search).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return total, nil
}

type walletLookupMatch struct {
	Source       string `json:"source"`
	UserID       string `json:"user_id"`
	ContactEmail string `json:"contact_email,omitempty"`
	ContactName  string `json:"contact_name,omitempty"`
	LocationID   int    `json:"location_id,omitempty"`
	LocationName string `json:"location_name,omitempty"`
	WalletID     int    `json:"wallet_id,omitempty"`
	WalletName   string `json:"wallet_name,omitempty"`
	EOAAddress   string `json:"eoa_address,omitempty"`
	SmartAddress string `json:"smart_address,omitempty"`
	IsDefault    bool   `json:"is_default,omitempty"`
}

type walletBalance struct {
	Address    string `json:"address"`
	BalanceWei string `json:"balance_wei"`
}

type w9Status struct {
	WalletAddress   string `json:"wallet_address"`
	ChainID         int64  `json:"chain_id"`
	Year            int    `json:"year"`
	AmountReceived  string `json:"amount_received"`
	UserID          string `json:"user_id,omitempty"`
	W9Required      bool   `json:"w9_required"`
	W9RequiredAt    string `json:"w9_required_at,omitempty"`
	SubmissionEmail string `json:"submission_email,omitempty"`
	SubmittedAt     string `json:"submitted_at,omitempty"`
	PendingApproval *bool  `json:"pending_approval,omitempty"`
	ApprovedAt      string `json:"approved_at,omitempty"`
	RejectedAt      string `json:"rejected_at,omitempty"`
	LastTxHash      string `json:"last_tx_hash,omitempty"`
	LastTxTimestamp int64  `json:"last_tx_timestamp,omitempty"`
}

func (a *adminMCP) lookupWallet(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	address := normalizeAddress(request.GetString("address", ""))
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}
	year := request.GetInt("year", time.Now().UTC().Year())

	matches := []walletLookupMatch{}
	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				source,
				user_id,
				contact_email,
				contact_name,
				location_id,
				location_name,
				wallet_id,
				wallet_name,
				eoa_address,
				smart_address,
				is_default
			FROM (
				SELECT
					'user_wallet' AS source,
					u.id AS user_id,
					COALESCE(u.contact_email, '') AS contact_email,
					COALESCE(u.contact_name, '') AS contact_name,
					0 AS location_id,
					'' AS location_name,
					w.id AS wallet_id,
					w.name AS wallet_name,
					LOWER(TRIM(w.eoa_address)) AS eoa_address,
					LOWER(TRIM(COALESCE(w.smart_address, ''))) AS smart_address,
					FALSE AS is_default
				FROM wallets w
				JOIN users u ON u.id = w.owner
				WHERE COALESCE(w.active, TRUE) = TRUE
				AND COALESCE(u.active, TRUE) = TRUE
				AND (LOWER(w.eoa_address) = $1 OR LOWER(COALESCE(w.smart_address, '')) = $1)
				UNION ALL
				SELECT
					'location_payment_wallet' AS source,
					u.id AS user_id,
					COALESCE(u.contact_email, '') AS contact_email,
					COALESCE(u.contact_name, '') AS contact_name,
					l.id AS location_id,
					COALESCE(l.name, '') AS location_name,
					lpw.id AS wallet_id,
					'' AS wallet_name,
					LOWER(TRIM(lpw.wallet_address)) AS eoa_address,
					'' AS smart_address,
					lpw.is_default AS is_default
				FROM location_payment_wallets lpw
				JOIN locations l ON l.id = lpw.location_id
				JOIN users u ON u.id = l.owner_id
				WHERE COALESCE(lpw.active, TRUE) = TRUE
				AND COALESCE(l.active, TRUE) = TRUE
				AND COALESCE(u.active, TRUE) = TRUE
				AND LOWER(lpw.wallet_address) = $1
				UNION ALL
				SELECT
					'location_tipping_wallet' AS source,
					u.id AS user_id,
					COALESCE(u.contact_email, '') AS contact_email,
					COALESCE(u.contact_name, '') AS contact_name,
					l.id AS location_id,
					COALESCE(l.name, '') AS location_name,
					0 AS wallet_id,
					'' AS wallet_name,
					LOWER(TRIM(l.tipping_wallet_address)) AS eoa_address,
					'' AS smart_address,
					FALSE AS is_default
				FROM locations l
				JOIN users u ON u.id = l.owner_id
				WHERE COALESCE(l.active, TRUE) = TRUE
				AND COALESCE(u.active, TRUE) = TRUE
				AND LOWER(COALESCE(l.tipping_wallet_address, '')) = $1
			) matches
			ORDER BY source, user_id, location_id, wallet_id;
		`, address)
		if err != nil {
			return fmt.Errorf("query wallet matches: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var match walletLookupMatch
			if err := rows.Scan(
				&match.Source,
				&match.UserID,
				&match.ContactEmail,
				&match.ContactName,
				&match.LocationID,
				&match.LocationName,
				&match.WalletID,
				&match.WalletName,
				&match.EOAAddress,
				&match.SmartAddress,
				&match.IsDefault,
			); err != nil {
				return fmt.Errorf("scan wallet match: %w", err)
			}
			matches = append(matches, match)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	var balance *walletBalance
	if err := withReadOnlyTx(ctx, a.ponderDB, func(tx pgx.Tx) error {
		var b walletBalance
		err := tx.QueryRow(ctx, `
			SELECT LOWER(address), COALESCE(SUM(balance), 0)::text
			FROM transfer_account
			WHERE LOWER(address) = $1
			GROUP BY LOWER(address);
		`, address).Scan(&b.Address, &b.BalanceWei)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		balance = &b
		return nil
	}); err != nil {
		return nil, fmt.Errorf("balance lookup: %w", err)
	}

	w9Rows, err := a.loadW9(ctx, address, "", year, 0, maxLimit)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"address":      address,
		"matches":      matches,
		"balance":      balance,
		"w9":           w9Rows,
	}, nil
}

type financialTransferSummary struct {
	TransactionCount     int    `json:"transaction_count"`
	TransactionVolumeWei string `json:"transaction_volume_wei"`
	RewardCount          int    `json:"reward_count"`
	RewardsWei           string `json:"rewards_wei"`
	MerchantPaymentCount int    `json:"merchant_payment_count"`
	MerchantPaymentsWei  string `json:"merchant_payments_wei"`
	RedemptionCount      int    `json:"redemption_count"`
	RedemptionsWei       string `json:"redemptions_wei"`
	CirculatingSFLUVWei  string `json:"circulating_sfluv_wei"`
}

type workflowCostSummary struct {
	WorkflowCount int    `json:"workflow_count"`
	CostWei       string `json:"cost_wei"`
}

type volunteerEventSummary struct {
	EventCount       int    `json:"event_count"`
	CodeCount        int    `json:"code_count"`
	RedeemedCount    int    `json:"redeemed_count"`
	PlannedRewardWei string `json:"planned_reward_wei"`
}

func (a *adminMCP) financialSummary(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	start, end := unixRangeFromRequest(request)
	chainID := int64(request.GetInt("chain_id", int(a.chainID)))
	if chainID <= 0 {
		chainID = a.chainID
	}

	roles, err := a.loadRoleIndex(ctx, chainID)
	if err != nil {
		return nil, err
	}

	transfers, err := a.summarizeTransfers(ctx, roles, start, end)
	if err != nil {
		return nil, fmt.Errorf("transfer summary: %w", err)
	}

	var workflowCosts workflowCostSummary
	if err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			WITH workflow_costs AS (
				SELECT
					w.id,
					COALESCE(w.manager_paid_out_at, MAX(ws.completed_at), w.updated_at, w.start_at, w.created_at) AS completed_at,
					(
						CASE
							WHEN COALESCE(w.total_bounty, 0) > 0 THEN COALESCE(w.total_bounty, 0)
							ELSE COALESCE(SUM(ws.bounty), 0)
						END
						+ COALESCE(w.manager_bounty, 0)
					)::numeric AS cost_wei
				FROM workflows w
				LEFT JOIN workflow_steps ws ON ws.workflow_id = w.id
				WHERE w.status IN ('completed', 'paid_out')
				GROUP BY w.id
			)
			SELECT
				COUNT(*)::int,
				COALESCE(SUM(cost_wei), 0)::text
			FROM workflow_costs
			WHERE completed_at >= $1
			AND completed_at < $2;
		`, start, end).Scan(&workflowCosts.WorkflowCount, &workflowCosts.CostWei)
	}); err != nil {
		return nil, fmt.Errorf("workflow cost summary: %w", err)
	}

	var events volunteerEventSummary
	if err := withReadOnlyTx(ctx, a.botDB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			WITH event_rows AS (
				SELECT
					e.id,
					COALESCE(e.amount, 0) AS amount,
					COUNT(c.id)::int AS code_count,
					COUNT(c.id) FILTER (WHERE COALESCE(c.redeemed, FALSE) = TRUE)::int AS redeemed_count
				FROM events e
				LEFT JOIN codes c ON c.event = e.id
				WHERE COALESCE(NULLIF(e.start_at, 0), e.expiration, 0) >= $1
				AND COALESCE(NULLIF(e.start_at, 0), e.expiration, 0) < $2
				GROUP BY e.id
			)
			SELECT
				COUNT(*)::int,
				COALESCE(SUM(code_count), 0)::int,
				COALESCE(SUM(redeemed_count), 0)::int,
				COALESCE(SUM(amount), 0)::text
			FROM event_rows;
		`, start, end).Scan(&events.EventCount, &events.CodeCount, &events.RedeemedCount, &events.PlannedRewardWei)
	}); err != nil {
		return nil, fmt.Errorf("volunteer event summary: %w", err)
	}

	return map[string]any{
		"generated_at":     time.Now().UTC().Format(time.RFC3339),
		"chain_id":         chainID,
		"start_timestamp":  start,
		"end_timestamp":    end,
		"transfers":        transfers,
		"workflow_costs":   workflowCosts,
		"volunteer_events": events,
		"notes":            []string{"circulating_sfluv_wei is rewards minus redemptions inside the requested range, floored at zero"},
	}, nil
}

type roleRecord struct {
	Address   string
	Role      string
	StartedAt int64
	EndedAt   int64
}

type roleIndex map[string][]roleRecord

func (a *adminMCP) loadRoleIndex(ctx context.Context, chainID int64) (roleIndex, error) {
	index := make(roleIndex)
	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				LOWER(address),
				role,
				EXTRACT(EPOCH FROM started_at)::bigint,
				COALESCE(EXTRACT(EPOCH FROM ended_at)::bigint, 0)
			FROM analytics_wallet_role_history
			WHERE chain_id = $1;
		`, chainID)
		if err != nil {
			return fmt.Errorf("query analytics role history: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var record roleRecord
			if err := rows.Scan(&record.Address, &record.Role, &record.StartedAt, &record.EndedAt); err != nil {
				return fmt.Errorf("scan analytics role history: %w", err)
			}
			index[record.Address] = append(index[record.Address], record)
		}
		return rows.Err()
	})
	return index, err
}

func (a *adminMCP) summarizeTransfers(ctx context.Context, roles roleIndex, start int64, end int64) (financialTransferSummary, error) {
	totalVolume := big.NewInt(0)
	rewards := big.NewInt(0)
	payments := big.NewInt(0)
	redemptions := big.NewInt(0)
	var summary financialTransferSummary

	err := withReadOnlyTx(ctx, a.ponderDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				amount::text,
				timestamp,
				LOWER("from"),
				LOWER("to")
			FROM transfer_event
			WHERE timestamp >= $1
			AND timestamp < $2
			ORDER BY timestamp ASC, id ASC;
		`, start, end)
		if err != nil {
			return fmt.Errorf("query analytics transfers: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var amountRaw string
			var timestamp int64
			var from string
			var to string
			if err := rows.Scan(&amountRaw, &timestamp, &from, &to); err != nil {
				return fmt.Errorf("scan analytics transfer: %w", err)
			}
			amount, ok := new(big.Int).SetString(amountRaw, 10)
			if !ok || amount.Sign() <= 0 || from == zeroAddress || to == zeroAddress {
				continue
			}

			fromRoles := roles.rolesAt(from, timestamp)
			toRoles := roles.rolesAt(to, timestamp)
			fromRewardSource := fromRoles.has("admin") || fromRoles.has("faucet")
			fromMerchant := fromRoles.has("merchant")
			toMerchant := toRoles.has("merchant")
			toAdminOrZapper := toRoles.has("admin") || toRoles.has("zapper")
			fromUserWallet := fromRoles.isUserWallet()
			toUserWallet := toRoles.isUserWallet()

			summary.TransactionCount++
			totalVolume.Add(totalVolume, amount)
			if fromRewardSource && toUserWallet {
				summary.RewardCount++
				rewards.Add(rewards, amount)
			}
			if fromUserWallet && toMerchant {
				summary.MerchantPaymentCount++
				payments.Add(payments, amount)
			}
			if fromMerchant && toAdminOrZapper {
				summary.RedemptionCount++
				redemptions.Add(redemptions, amount)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return summary, err
	}

	circulating := new(big.Int).Sub(rewards, redemptions)
	if circulating.Sign() < 0 {
		circulating.SetInt64(0)
	}
	summary.TransactionVolumeWei = totalVolume.String()
	summary.RewardsWei = rewards.String()
	summary.MerchantPaymentsWei = payments.String()
	summary.RedemptionsWei = redemptions.String()
	summary.CirculatingSFLUVWei = circulating.String()
	return summary, nil
}

type roleSet map[string]struct{}

func (index roleIndex) rolesAt(address string, timestamp int64) roleSet {
	roles := make(roleSet)
	for _, record := range index[normalizeAddress(address)] {
		if record.StartedAt > timestamp {
			continue
		}
		if record.EndedAt > 0 && record.EndedAt <= timestamp {
			continue
		}
		roles[record.Role] = struct{}{}
	}
	return roles
}

func (roles roleSet) has(role string) bool {
	_, ok := roles[role]
	return ok
}

func (roles roleSet) isUserWallet() bool {
	return !roles.has("admin") && !roles.has("merchant") && !roles.has("faucet") && !roles.has("zapper")
}

type transactionRow struct {
	ID        string `json:"id"`
	ChainID   int64  `json:"chain_id"`
	Hash      string `json:"hash"`
	AmountWei string `json:"amount_wei"`
	Timestamp int64  `json:"timestamp"`
	From      string `json:"from"`
	To        string `json:"to"`
}

func (a *adminMCP) transactions(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	address := normalizeAddress(request.GetString("address", ""))
	hash := normalizeAddress(request.GetString("hash", ""))
	start, end := unixRangeFromRequest(request)
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit

	items := []transactionRow{}
	err := withReadOnlyTx(ctx, a.ponderDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				id,
				$7::bigint AS chain_id,
				hash,
				amount::text,
				timestamp,
				LOWER("from"),
				LOWER("to")
			FROM transfer_event
			WHERE timestamp >= $1
			AND timestamp < $2
			AND ($3 = '' OR LOWER("from") = $3 OR LOWER("to") = $3)
			AND ($4 = '' OR LOWER(hash) = $4)
			ORDER BY timestamp DESC, id DESC
			LIMIT $5
			OFFSET $6;
		`, start, end, address, hash, limit, offset, a.chainID)
		if err != nil {
			return fmt.Errorf("query transactions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row transactionRow
			if err := rows.Scan(&row.ID, &row.ChainID, &row.Hash, &row.AmountWei, &row.Timestamp, &row.From, &row.To); err != nil {
				return fmt.Errorf("scan transaction: %w", err)
			}
			items = append(items, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"generated_at":    time.Now().UTC().Format(time.RFC3339),
		"page":            page,
		"count":           limit,
		"start_timestamp": start,
		"end_timestamp":   end,
		"transactions":    items,
	}, nil
}

func (a *adminMCP) w9Report(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	year := request.GetInt("year", 0)
	wallet := normalizeAddress(request.GetString("wallet_address", ""))
	userID := strings.TrimSpace(request.GetString("user_id", ""))
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	rows, err := a.loadW9(ctx, wallet, userID, year, page*limit, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"page":         page,
		"count":        limit,
		"w9":           rows,
	}, nil
}

func (a *adminMCP) loadW9(ctx context.Context, wallet string, userID string, year int, offset int, limit int) ([]w9Status, error) {
	out := []w9Status{}
	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				LOWER(e.wallet_address),
				COALESCE(e.chain_id, $6)::bigint,
				e.year,
				e.amount_received::text,
				COALESCE(e.user_id, ''),
				e.w9_required,
				e.w9_required_at,
				COALESCE(e.last_tx_hash, ''),
				COALESCE(e.last_tx_timestamp, 0)::bigint,
				COALESCE(s.email, ''),
				s.submitted_at,
				s.pending_approval,
				s.approved_at,
				s.rejected_at
			FROM w9_wallet_earnings e
			LEFT JOIN w9_submissions s
				ON LOWER(s.wallet_address) = LOWER(e.wallet_address)
				AND s.year = e.year
			WHERE ($1 = '' OR LOWER(e.wallet_address) = $1)
			AND ($2 = '' OR e.user_id = $2)
			AND ($3 = 0 OR e.year = $3)
			ORDER BY e.year DESC, e.amount_received DESC, e.wallet_address ASC
			LIMIT $4
			OFFSET $5;
		`, wallet, userID, year, limit, offset, a.chainID)
		if err != nil {
			return fmt.Errorf("query w9 report: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row w9Status
			var requiredAt sql.NullTime
			var submittedAt sql.NullTime
			var pending sql.NullBool
			var approvedAt sql.NullTime
			var rejectedAt sql.NullTime
			if err := rows.Scan(
				&row.WalletAddress,
				&row.ChainID,
				&row.Year,
				&row.AmountReceived,
				&row.UserID,
				&row.W9Required,
				&requiredAt,
				&row.LastTxHash,
				&row.LastTxTimestamp,
				&row.SubmissionEmail,
				&submittedAt,
				&pending,
				&approvedAt,
				&rejectedAt,
			); err != nil {
				return fmt.Errorf("scan w9 report: %w", err)
			}
			row.W9RequiredAt = formatNullTime(requiredAt)
			row.SubmittedAt = formatNullTime(submittedAt)
			if pending.Valid {
				value := pending.Bool
				row.PendingApproval = &value
			}
			row.ApprovedAt = formatNullTime(approvedAt)
			row.RejectedAt = formatNullTime(rejectedAt)
			out = append(out, row)
		}
		return rows.Err()
	})
	return out, err
}

type merchantRow struct {
	LocationID           int      `json:"location_id"`
	OwnerID              string   `json:"owner_id"`
	OwnerEmail           string   `json:"owner_email,omitempty"`
	OwnerName            string   `json:"owner_name,omitempty"`
	Name                 string   `json:"name"`
	Approval             bool     `json:"approval"`
	ApprovedAt           string   `json:"approved_at,omitempty"`
	Street               string   `json:"street,omitempty"`
	City                 string   `json:"city,omitempty"`
	State                string   `json:"state,omitempty"`
	Zip                  string   `json:"zip,omitempty"`
	Email                string   `json:"email,omitempty"`
	AdminEmail           string   `json:"admin_email,omitempty"`
	TippingWalletAddress string   `json:"tipping_wallet_address,omitempty"`
	PaymentWallets       []string `json:"payment_wallets"`
}

func (a *adminMCP) merchantReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	approvedOnly := request.GetBool("approved_only", false)
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit
	merchants := []merchantRow{}

	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				l.id,
				COALESCE(l.owner_id, ''),
				COALESCE(u.contact_email, ''),
				COALESCE(u.contact_name, ''),
				COALESCE(l.name, ''),
				COALESCE(l.approval, FALSE),
				l.approved_at,
				COALESCE(l.street, ''),
				COALESCE(l.city, ''),
				COALESCE(l.state, ''),
				COALESCE(l.zip, ''),
				COALESCE(l.email, ''),
				COALESCE(l.admin_email, ''),
				LOWER(TRIM(COALESCE(l.tipping_wallet_address, ''))),
				COALESCE(
					jsonb_agg(LOWER(TRIM(lpw.wallet_address)) ORDER BY lpw.is_default DESC, lpw.id)
						FILTER (WHERE lpw.wallet_address IS NOT NULL),
					'[]'::jsonb
				)
			FROM locations l
			LEFT JOIN users u ON u.id = l.owner_id
			LEFT JOIN location_payment_wallets lpw
				ON lpw.location_id = l.id
				AND COALESCE(lpw.active, TRUE) = TRUE
			WHERE COALESCE(l.active, TRUE) = TRUE
			AND ($1 = FALSE OR COALESCE(l.approval, FALSE) = TRUE)
			GROUP BY l.id, u.contact_email, u.contact_name
			ORDER BY COALESCE(l.approval, FALSE) DESC, COALESCE(l.name, '') ASC, l.id ASC
			LIMIT $2
			OFFSET $3;
		`, approvedOnly, limit, offset)
		if err != nil {
			return fmt.Errorf("query merchant report: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row merchantRow
			var approvedAt sql.NullTime
			var walletsJSON []byte
			if err := rows.Scan(
				&row.LocationID,
				&row.OwnerID,
				&row.OwnerEmail,
				&row.OwnerName,
				&row.Name,
				&row.Approval,
				&approvedAt,
				&row.Street,
				&row.City,
				&row.State,
				&row.Zip,
				&row.Email,
				&row.AdminEmail,
				&row.TippingWalletAddress,
				&walletsJSON,
			); err != nil {
				return fmt.Errorf("scan merchant report: %w", err)
			}
			row.ApprovedAt = formatNullTime(approvedAt)
			if err := json.Unmarshal(walletsJSON, &row.PaymentWallets); err != nil {
				return fmt.Errorf("decode merchant wallets: %w", err)
			}
			merchants = append(merchants, row)
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
		"merchants":    merchants,
	}, nil
}

type workflowRow struct {
	ID                  string `json:"id"`
	SeriesID            string `json:"series_id"`
	Status              string `json:"status"`
	ProposerID          string `json:"proposer_id"`
	ProposerEmail       string `json:"proposer_email,omitempty"`
	StartAt             int64  `json:"start_at"`
	CreatedAt           int64  `json:"created_at"`
	ApprovedAt          int64  `json:"approved_at,omitempty"`
	ManagerImproverID   string `json:"manager_improver_id,omitempty"`
	ManagerBountyWei    string `json:"manager_bounty_wei"`
	TotalBountyWei      string `json:"total_bounty_wei"`
	StepBountyWei       string `json:"step_bounty_wei"`
	StepCount           int    `json:"step_count"`
	CompletedStepCount  int    `json:"completed_step_count"`
	PaidOutStepCount    int    `json:"paid_out_step_count"`
	ManagerPaidOutAt    int64  `json:"manager_paid_out_at,omitempty"`
	ManagerPayoutTxHash string `json:"manager_payout_tx_hash,omitempty"`
}

func (a *adminMCP) workflowReport(ctx context.Context, request mcp.CallToolRequest) (any, error) {
	status := strings.TrimSpace(request.GetString("status", ""))
	start, end := unixRangeFromRequest(request)
	page := max(0, request.GetInt("page", 0))
	limit := safeLimit(request.GetInt("count", defaultLimit))
	offset := page * limit
	workflows := []workflowRow{}

	err := withReadOnlyTx(ctx, a.appDB, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				w.id,
				w.series_id,
				w.status,
				w.proposer_id,
				COALESCE(u.contact_email, ''),
				w.start_at,
				w.created_at,
				COALESCE(w.approved_at, 0),
				COALESCE(w.manager_improver_id, ''),
				COALESCE(w.manager_bounty, 0)::text,
				COALESCE(w.total_bounty, 0)::text,
				COALESCE(SUM(ws.bounty), 0)::text,
				COUNT(ws.id)::int,
				COUNT(ws.id) FILTER (WHERE ws.status IN ('completed', 'paid_out'))::int,
				COUNT(ws.id) FILTER (WHERE ws.status = 'paid_out')::int,
				COALESCE(w.manager_paid_out_at, 0),
				COALESCE(w.manager_payout_tx_hash, '')
			FROM workflows w
			LEFT JOIN users u ON u.id = w.proposer_id
			LEFT JOIN workflow_steps ws ON ws.workflow_id = w.id
			WHERE w.created_at >= $1
			AND w.created_at < $2
			AND ($3 = '' OR w.status = $3)
			GROUP BY w.id, u.contact_email
			ORDER BY w.created_at DESC, w.id DESC
			LIMIT $4
			OFFSET $5;
		`, start, end, status, limit, offset)
		if err != nil {
			return fmt.Errorf("query workflow report: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row workflowRow
			if err := rows.Scan(
				&row.ID,
				&row.SeriesID,
				&row.Status,
				&row.ProposerID,
				&row.ProposerEmail,
				&row.StartAt,
				&row.CreatedAt,
				&row.ApprovedAt,
				&row.ManagerImproverID,
				&row.ManagerBountyWei,
				&row.TotalBountyWei,
				&row.StepBountyWei,
				&row.StepCount,
				&row.CompletedStepCount,
				&row.PaidOutStepCount,
				&row.ManagerPaidOutAt,
				&row.ManagerPayoutTxHash,
			); err != nil {
				return fmt.Errorf("scan workflow report: %w", err)
			}
			workflows = append(workflows, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"generated_at":    time.Now().UTC().Format(time.RFC3339),
		"page":            page,
		"count":           limit,
		"start_timestamp": start,
		"end_timestamp":   end,
		"workflows":       workflows,
	}, nil
}

func withReadOnlyTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// ponytail: local timeout is enough; add per-tool tuning if reports outgrow it.
	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '20s'"); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func unixRangeFromRequest(request mcp.CallToolRequest) (int64, int64) {
	start := int64(request.GetInt("start_timestamp", 0))
	end := int64(request.GetInt("end_timestamp", int(time.Now().UTC().Unix())))
	if end <= 0 {
		end = time.Now().UTC().Unix()
	}
	if start < 0 {
		start = 0
	}
	if end <= start {
		end = start + 1
	}
	return start, end
}

func rolesFromBools(values [10]bool) []string {
	names := []string{"admin", "merchant", "organizer", "improver", "proposer", "voter", "issuer", "supervisor", "affiliate"}
	roles := make([]string, 0, len(names))
	for i, name := range names {
		if values[i] {
			roles = append(roles, name)
		}
	}
	return roles
}

func safeLimit(value int) int {
	if value <= 0 {
		return defaultLimit
	}
	if value > maxLimit {
		return maxLimit
	}
	return value
}

func normalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

func envInt64(defaultValue int64, keys ...string) int64 {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultValue
}

func formatNullTime(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format(time.RFC3339)
}
