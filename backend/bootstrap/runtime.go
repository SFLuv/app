package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/clientconfig"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/mcp"
	"github.com/SFLuv/app/backend/router"
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

	appService := handlers.NewAppService(appDb, appLogger, nil, clientConfig)
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

	w9 := handlers.NewW9Service(appDb, ponderDb, appLogger, activeChainID)
	affiliateScheduler := handlers.NewAffiliateScheduler(appDb, botDb, appLogger)
	affiliateScheduler.Start(ctx)

	redeemer := handlers.NewRedeemerService(appDb, appLogger, clientConfig)
	minter := handlers.NewMinterService(appDb, appLogger, clientConfig)

	s := handlers.NewBotService(botDb, appDb, botClient, w9, affiliateScheduler, activeChainID, clientConfig.ReadRPCURL())
	a := handlers.NewAppService(appDb, appLogger, w9, clientConfig)
	a.SetBotService(s)
	a.SetRedeemerService(redeemer)
	a.SetMinterService(minter)
	StartDeletedAccountPurgeLoop(ctx, a, appLogger)

	// Workflow upkeep (recurrence catch-up, payout reconciliation, paid_out
	// finalization) previously ran only as a side effect of user requests, so it
	// stalled whenever nobody hit the right endpoint. Running it on a timer makes
	// it independent of traffic.
	handlers.NewWorkflowMaintenanceScheduler(a, workflowMaintenanceInterval()).Start(ctx)

	p := handlers.NewPonderService(ponderDb, appDb, botDb, appLogger, activeChainID)
	if err := p.SyncCurrentAnalyticsWalletRoleHistory(ctx); err != nil {
		appLogger.Logf("error syncing analytics wallet role history during startup: %s", err)
	}

	// Read-only admin MCP endpoint, gated by the existing Privy JWT + admin check.
	// It reuses the running pools; the read-only guarantee comes from per-query
	// read-only transactions inside the mcp package.
	mcpService := mcp.New(pools.App, pools.Bot, pools.Ponder, a, activeChainID)

	return router.New(s, a, p, mcpService.RegisterRoutes), nil
}
