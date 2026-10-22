package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/bridge"
	"github.com/SFLuv/app/backend/clientconfig"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/mcp"
	"github.com/SFLuv/app/backend/router"
	"github.com/SFLuv/app/backend/w9provider"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const deletedAccountPurgeRunTimeout = 30 * time.Minute

const (
	defaultBotDBName    = "bot"
	defaultAppDBName    = "app"
	botDBNameEnvKey     = "BOT_DB_NAME"
	appDBNameEnvKey     = "APP_DB_NAME"
	defaultPonderDBName = "ponder"
	ponderDBNameEnvKey  = "PONDER_DB_NAME"
)

type DBPools struct {
	Bot    *pgxpool.Pool
	App    *pgxpool.Pool
	Ponder *pgxpool.Pool
}

func LoadEnv() {
	if envFile := os.Getenv("ENV_FILE"); envFile != "" {
		_ = godotenv.Load(envFile)
	} else {
		godotenv.Load()
	}
}

func OpenDBPools(includePonder bool) (*DBPools, error) {
	pools := &DBPools{}
	botDBName, appDBName := resolveDBPoolNames()

	var err error
	pools.Bot, err = db.PgxDB(botDBName)
	if err != nil {
		pools.Close()
		return nil, fmt.Errorf("error initializing bot db: %w", err)
	}

	pools.App, err = db.PgxDB(appDBName)
	if err != nil {
		pools.Close()
		return nil, fmt.Errorf("error initializing app db: %w", err)
	}

	if includePonder {
		pools.Ponder, err = db.PgxDB(resolvePonderDBPoolName())
		if err != nil {
			pools.Close()
			return nil, fmt.Errorf("error initializing ponder db: %w", err)
		}
	}

	return pools, nil
}

func (p *DBPools) Close() {
	if p == nil {
		return
	}
	if p.Bot != nil {
		p.Bot.Close()
	}
	if p.App != nil {
		p.App.Close()
	}
	if p.Ponder != nil {
		p.Ponder.Close()
	}
}

func NewAppLogger() (*logger.LogCloser, error) {
	return logger.New("./logs/prod/app.log", "APP: ")
}

func resolveDBPoolNames() (string, string) {
	return envOrDefault(botDBNameEnvKey, defaultBotDBName), envOrDefault(appDBNameEnvKey, defaultAppDBName)
}

func resolvePonderDBPoolName() string {
	return envOrDefault(ponderDBNameEnvKey, defaultPonderDBName)
}

func envOrDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

// workflowMaintenanceInterval controls how often the workflow maintenance
// sweep runs. Configurable so it can be tightened in production or slowed in
// development; defaults to the scheduler's own default when unset or invalid.
func workflowMaintenanceInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("WORKFLOW_MAINTENANCE_INTERVAL"))
	if raw == "" {
		return 0
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

// WarnOnMissingPublicConfig surfaces environment gaps that fail silently at
// runtime rather than at boot. PUBLIC_BACKEND_URL is the notable one: without
// it, volunteer event image URLs are emitted root-relative, which resolves fine
// for same-origin callers but 404s on the marketing site — a broken image on a
// public page with nothing in the logs to explain it.
func WarnOnMissingPublicConfig(appLogger *logger.LogCloser) {
	if appLogger == nil {
		return
	}

	if strings.TrimSpace(os.Getenv("PUBLIC_BACKEND_URL")) == "" &&
		strings.TrimSpace(os.Getenv("NEXT_PUBLIC_BACKEND_URL")) == "" {
		appLogger.Logf("warning: PUBLIC_BACKEND_URL is unset; volunteer event photo and organizer logo URLs will be root-relative and will not resolve for external clients")
	}

	if strings.TrimSpace(os.Getenv("VOLUNTEER_PROXY_KEY")) == "" {
		appLogger.Logf("warning: VOLUNTEER_PROXY_KEY is unset; forwarded client IPs will be ignored and proxied volunteer signups will share one rate-limit bucket")
	}
}

func InitializeDatabases(ctx context.Context, pools *DBPools, appLogger *logger.LogCloser) error {
	if pools == nil || pools.App == nil || pools.Bot == nil {
		return fmt.Errorf("app and bot db pools are required")
	}

	appDb := db.App(pools.App, appLogger)
	if err := appDb.CreateTables(); err != nil {
		return err
	}

	defaultAdminID, err := appDb.GetFirstAdminId(ctx)
	if err != nil && appLogger != nil {
		appLogger.Logf("error getting default admin id during init: %s", err)
	}

	botDb := db.Bot(pools.Bot)
	if err := botDb.CreateTables(defaultAdminID); err != nil {
		return err
	}

	return nil
}

func RunInitializationSyncs(ctx context.Context, pools *DBPools, appLogger *logger.LogCloser) error {
	if pools == nil || pools.App == nil {
		return fmt.Errorf("app db pool is required")
	}
	if appLogger == nil {
		return fmt.Errorf("app logger is required")
	}

	appDb := db.App(pools.App, appLogger)

	clientConfig, err := clientconfig.Load(ctx)
	if err != nil {
		return fmt.Errorf("error loading client config during init syncs: %w", err)
	}
	activeChainID := int64(clientConfig.ActiveChainID())

	if err := appDb.BackfillTransactionChainIDs(ctx, activeChainID); err != nil {
		return err
	}
	if pools.Bot != nil {
		botDb := db.Bot(pools.Bot)
		if err := botDb.BackfillTransactionChainIDs(ctx, activeChainID); err != nil {
			return err
		}
	}

	redeemer := handlers.NewRedeemerService(appDb, appLogger, clientConfig)
	if err := redeemer.SyncApprovedMerchants(ctx); err != nil {
		appLogger.Logf("error syncing redeemer roles during init: %s", err)
	}
	if err := redeemer.SyncAdmins(ctx); err != nil {
		appLogger.Logf("error syncing admin redeemer roles during init: %s", err)
	}

	minter := handlers.NewMinterService(appDb, appLogger, clientConfig)
	if err := minter.SyncWalletMinterStatuses(ctx); err != nil {
		appLogger.Logf("error syncing minter roles during init: %s", err)
	}

	appService := handlers.NewAppService(appDb, appLogger, nil, nil, clientConfig)
	if err := appService.SyncPrivyLinkedEmailsForAllUsers(ctx); err != nil {
		appLogger.Logf("error syncing Privy linked emails during init: %s", err)
	}

	return nil
}

func StartDeletedAccountPurgeLoop(ctx context.Context, appService *handlers.AppService, appLogger *logger.LogCloser) {
	if ctx == nil || appService == nil || appLogger == nil {
		return
	}

	go func() {
		runDeletedAccountPurge(ctx, appService, appLogger, "startup")
		if ctx.Err() != nil {
			return
		}
		appLogger.Logf("deleted account purge loop started; next run scheduled for %s", nextDeletedAccountPurgeRun(time.Now().UTC()).Format(time.RFC3339))

		for {
			nextRun := nextDeletedAccountPurgeRun(time.Now().UTC())
			timer := time.NewTimer(time.Until(nextRun))

			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				runDeletedAccountPurge(ctx, appService, appLogger, "daily")
			}
		}
	}()
}

// StartLocationRedeemerSync brings the REDEEMER_ROLE holders on chain back in
// line with the addresses locations are actually paid into.
//
// In a goroutine, and never returning an error to the caller, because it talks
// to the chain: a node that is slow, down or mid-reorg must cost the server a
// log line, not a boot. It runs once per start rather than on a timer — the set
// only changes when a location is created or its wallet swapped, and the next
// restart is soon enough for a role nobody can use before they have takings.
func StartLocationRedeemerSync(ctx context.Context, redeemer *handlers.RedeemerService, appLogger *logger.LogCloser) {
	if ctx == nil || redeemer == nil || appLogger == nil || !redeemer.IsEnabled() {
		return
	}

	go func() {
		if _, err := redeemer.SyncLocationWallets(ctx); err != nil && ctx.Err() == nil {
			appLogger.Logf("error syncing redeemer roles for location wallets during startup: %s", err)
		}
	}()
}

func runDeletedAccountPurge(ctx context.Context, appService *handlers.AppService, appLogger *logger.LogCloser, runType string) {
	if ctx == nil || appService == nil || appLogger == nil || ctx.Err() != nil {
		return
	}

	runCtx, cancel := context.WithTimeout(ctx, deletedAccountPurgeRunTimeout)
	defer cancel()

	purged, err := appService.PurgeDeletedAccounts(runCtx, time.Now().UTC())
	if err != nil {
		if ctx.Err() == nil {
			appLogger.Logf("error running deleted account purge during %s pass: %s", runType, err)
		}
		return
	}

	if purged > 0 {
		appLogger.Logf("purged %d deleted accounts during %s pass", purged, runType)
	}
}

func nextDeletedAccountPurgeRun(now time.Time) time.Time {
	now = now.UTC()
	nextMidnightUTC := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	if !nextMidnightUTC.After(now) {
		return nextMidnightUTC.Add(24 * time.Hour)
	}
	return nextMidnightUTC
}

func NewServerHandler(ctx context.Context, pools *DBPools, appLogger *logger.LogCloser) (http.Handler, error) {
	if pools == nil || pools.Bot == nil || pools.App == nil || pools.Ponder == nil {
		return nil, fmt.Errorf("bot, app, and ponder db pools are required")
	}
	if appLogger == nil {
		return nil, fmt.Errorf("app logger is required")
	}

	if err := pools.Bot.Ping(ctx); err != nil {
		return nil, fmt.Errorf("error pinging bot db: %w", err)
	}
	if err := pools.App.Ping(ctx); err != nil {
		return nil, fmt.Errorf("error pinging app db: %w", err)
	}

	appDb := db.App(pools.App, appLogger)
	botDb := db.Bot(pools.Bot)
	ponderDb := db.Ponder(pools.Ponder, appLogger)
	if err := ponderDb.Ping(); err != nil {
		return nil, err
	}

	clientConfig, err := clientconfig.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading client config: %w", err)
	}
	appLogger.Logf("loaded client config from %s for alias %s on chain %d", clientConfig.Source(), clientConfig.Community.Alias, clientConfig.ActiveChainID())
	activeChainID := int64(clientConfig.ActiveChainID())
	if err := appDb.BackfillTransactionChainIDs(ctx, activeChainID); err != nil {
		return nil, err
	}
	if err := botDb.BackfillTransactionChainIDs(ctx, activeChainID); err != nil {
		return nil, err
	}
	// NOTE: the Ponder database is owned by the Ponder indexer and must not be
	// altered or written to from the backend — doing so (ALTER ADD chain_id,
	// SET DEFAULT, indexes, UPDATE) changes Ponder's schema out from under the
	// running indexer and trips its live-query triggers ("live_query_tables does
	// not exist"), which halts indexing. Ponder tags chain ids itself via its
	// schema; the cross-chain migration handles legacy rows on a clone only.

	botClient, err := bot.Init(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("error initializing bot service: %w", err)
	}

	// The tax provider holds the TIN; nothing on our side ever sees one. An
	// unconfigured provider yields a disabled adapter rather than a boot failure
	// — money is still held correctly without it, only the route out is missing.
	w9Provider := w9provider.New(w9provider.Config{
		Provider: strings.TrimSpace(os.Getenv("W9_PROVIDER")),
		BaseURL:  strings.TrimSpace(os.Getenv("W9_PROVIDER_BASE_URL")),
		// Sandbox unless this says otherwise. Defaulting the other way would
		// mean a misconfigured deploy files real tax forms.
		Environment: strings.TrimSpace(os.Getenv("W9_PROVIDER_ENV")),

		// TaxBandits: a client credential triple, the payer GUID, and the API
		// version as a path segment.
		ClientID:     strings.TrimSpace(os.Getenv("W9_PROVIDER_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("W9_PROVIDER_CLIENT_SECRET")),
		UserToken:    strings.TrimSpace(os.Getenv("W9_PROVIDER_USER_TOKEN")),
		BusinessID:   strings.TrimSpace(os.Getenv("W9_PROVIDER_BUSINESS_ID")),
		WebhookRef:   strings.TrimSpace(os.Getenv("W9_PROVIDER_WEBHOOK_REF")),
		APIVersion:   strings.TrimSpace(os.Getenv("W9_PROVIDER_API_VERSION")),
		AuthURL:      strings.TrimSpace(os.Getenv("W9_PROVIDER_AUTH_URL")),

		// Track1099, kept only as the fallback until TaxBandits goes live.
		APIKey:    strings.TrimSpace(os.Getenv("W9_PROVIDER_API_KEY")),
		TeamAPIID: strings.TrimSpace(os.Getenv("W9_PROVIDER_TEAM_ID")),
	})

	// The adapter complains out loud about anything it was not able to verify
	// against a live call — an unrecognised status, an unparseable timestamp, a
	// second submission where one was expected. Those messages are the whole
	// early-warning system for this integration, so they must reach the log.
	if tb, ok := w9Provider.(*w9provider.TaxBandits); ok {
		tb.SetWarningLogger(func(message string) { appLogger.Logf("%s", message) })
	}

	// Every payout in the system goes through this one service, which is what
	// makes the tax gate impossible to bypass by adding another send path.
	payouts := handlers.NewPayoutService(appDb, botClient, w9Provider, appLogger, activeChainID)

	redeemer := handlers.NewRedeemerService(appDb, appLogger, clientConfig)
	minter := handlers.NewMinterService(appDb, appLogger, clientConfig)

	s := handlers.NewBotService(botDb, appDb, botClient, payouts, activeChainID, clientConfig.ReadRPCURL())
	a := handlers.NewAppService(appDb, appLogger, payouts, w9Provider, clientConfig)
	a.SetBotService(s)
	s.SetAppService(a)
	payouts.SetAppService(a)
	a.SetRedeemerService(redeemer)
	a.SetMinterService(minter)

	// The backend's own Safes. Constructed even when it cannot act: the panel
	// has to be able to say why, and LogConfig below says it at boot.
	protocolSvc := handlers.NewProtocolService(appDb, appLogger, clientConfig)
	a.SetProtocolService(protocolSvc)

	// Who counted as the faucet when a payout happened. Installed whatever
	// state the migration is in — with no history rows the resolver finds
	// nothing and the verifier falls back to the key in hand, which is the
	// behaviour it had before any of this existed.
	botClient.SetSenderResolver(protocolSvc.FaucetSenderResolver())
	botClient.SetTxSenderResolver(protocolSvc.TxSenderResolver())

	// Bridge is the off-ramp behind merchant bank payouts. Without a key the
	// client is disabled and every payout route answers "not available yet";
	// the rest of the server is unaffected. Sandbox unless told otherwise, for
	// the same reason as the tax provider: a misconfigured deploy must not be
	// able to mint real liquidation addresses.
	bridgeRedirect := strings.TrimSpace(os.Getenv("BRIDGE_KYB_REDIRECT_URL"))
	if bridgeRedirect == "" {
		if base := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/"); base != "" {
			bridgeRedirect = base + "/settings?tab=merchant"
		}
	}
	bridgeClient := bridge.New(bridge.Config{
		Environment:         strings.TrimSpace(os.Getenv("BRIDGE_ENV")),
		APIKey:              strings.TrimSpace(os.Getenv("BRIDGE_API_KEY")),
		BaseURL:             strings.TrimSpace(os.Getenv("BRIDGE_BASE_URL")),
		WebhookPublicKeyPEM: os.Getenv("BRIDGE_WEBHOOK_PUBLIC_KEY"),
		RedirectURL:         bridgeRedirect,
	})
	a.SetBridgeClient(bridgeClient)
	if bridgeClient.Enabled() {
		env := "sandbox"
		if bridgeClient.Production() {
			env = "PRODUCTION"
		}
		appLogger.Logf("bridge payouts enabled (%s)", env)
	} else {
		appLogger.Logf("bridge payouts disabled: BRIDGE_API_KEY not set")
	}
	a.SetPonderDB(ponderDb)

	p := handlers.NewPonderService(ponderDb, appDb, botDb, appLogger, activeChainID)

	// Everything below moves money, signs transactions or writes on a timer, so
	// it runs only in the process holding the jobs lock. See StartJobsAsLeader.
	jobsLeader := StartJobsAsLeader(ctx, pools.App, appLogger, func(jobsCtx context.Context) {
		// Compare our webhook bookkeeping against the indexer's. The two drift
		// silently and always toward the same failure — notifications stop with
		// nothing logged — so the check runs where someone will see it.
		// Recreating hooks reaches out to Ponder, so it is opt-in; the check
		// itself only ever reads Ponder and writes to our own database.
		a.LogPonderHookReconciliation(jobsCtx, strings.TrimSpace(os.Getenv("PONDER_HOOK_AUTO_REPAIR")) == "true")

		// The protocol accounts come before the analytics sync, because the
		// sync reads the registry the first step seeds: run the other way
		// round, the first boot would classify nothing as a reward and the
		// window it opened for today's faucet would be a day late.
		StartProtocolAccounts(jobsCtx, protocolSvc, botClient, redeemer, appLogger)

		if err := p.SyncCurrentAnalyticsWalletRoleHistory(jobsCtx); err != nil {
			appLogger.Logf("error syncing analytics wallet role history during startup: %s", err)
		}

		// Backfills that read the indexer run here, not in a migration: the
		// server comes up whether or not ponder is reachable, and a slow or dead
		// indexer can no longer keep the backend from listening.
		SeedPonderNotifiedTransfersOnBoot(jobsCtx, pools, appLogger)

		// Merchant opening hours are refreshed from Google nightly. Listings
		// switched to manual are excluded, and a poll that returns nothing
		// usable leaves the existing hours alone rather than clearing them.
		handlers.NewLocationHoursScheduler(a).Start(jobsCtx)

		StartDeletedAccountPurgeLoop(jobsCtx, a, appLogger)

		// Tills are derived per location, so the account-level grant a merchant
		// got when their first shop was approved does not reach the address
		// their second shop is paid into. This catches every one of them up.
		StartLocationRedeemerSync(jobsCtx, redeemer, appLogger)
		handlers.StartMerchantPayoutSweep(jobsCtx, a, appLogger)

		// Find or register our Bridge webhook endpoint and adopt the signing
		// key Bridge issues for it, so BRIDGE_WEBHOOK_PUBLIC_KEY does not have
		// to be pasted in by hand. Runs in the background: a third party must
		// never be able to hold up the server starting.
		handlers.StartBridgeWebhookResolution(jobsCtx, a, appLogger)

		// Workflow upkeep (recurrence catch-up, payout reconciliation, paid_out
		// finalization) previously ran only as a side effect of user requests,
		// so it stalled whenever nobody hit the right endpoint. Running it on a
		// timer makes it independent of traffic.
		handlers.NewWorkflowMaintenanceScheduler(a, workflowMaintenanceInterval()).Start(jobsCtx)
	})

	// Read-only admin MCP endpoint, gated by the existing Privy JWT + admin check.
	// It reuses the running pools; the read-only guarantee comes from per-query
	// read-only transactions inside the mcp package.
	mcpService := mcp.New(pools.App, pools.Bot, pools.Ponder, a, activeChainID)

	return withHealthz(router.New(s, a, p, mcpService.RegisterRoutes), jobsLeader), nil
}

// withHealthz answers GET /healthz ahead of the router, for a platform's
// deploy healthcheck: no auth, no middleware, no database work, nothing
// recorded. It reports whether this process runs the background jobs, but is
// healthy either way: a new deployment must pass its check while the old
// process still holds the jobs lock, or neither would ever give way.
func withHealthz(next http.Handler, leader *JobsLeader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, `{"status":"ok","jobs_leader":%t}`, leader.Held())
	})
}
