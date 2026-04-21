package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/router"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const deletedAccountPurgeRunTimeout = 30 * time.Minute

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

	var err error
	pools.Bot, err = db.PgxDB("bot")
	if err != nil {
		pools.Close()
		return nil, fmt.Errorf("error initializing bot db: %w", err)
	}

	pools.App, err = db.PgxDB("app")
	if err != nil {
		pools.Close()
		return nil, fmt.Errorf("error initializing app db: %w", err)
	}

	if includePonder {
		pools.Ponder, err = db.PgxDB("ponder")
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

	redeemer := handlers.NewRedeemerService(appDb, appLogger)
	if err := redeemer.SyncApprovedMerchants(ctx); err != nil {
		appLogger.Logf("error syncing redeemer roles during init: %s", err)
	}
	if err := redeemer.SyncAdmins(ctx); err != nil {
		appLogger.Logf("error syncing admin redeemer roles during init: %s", err)
	}

	minter := handlers.NewMinterService(appDb, appLogger)
	if err := minter.SyncWalletMinterStatuses(ctx); err != nil {
		appLogger.Logf("error syncing minter roles during init: %s", err)
	}

	appService := handlers.NewAppService(appDb, appLogger, nil)
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

	botClient, err := bot.Init()
	if err != nil {
		return nil, fmt.Errorf("error initializing bot service: %w", err)
	}

	w9 := handlers.NewW9Service(appDb, ponderDb, appLogger)
	affiliateScheduler := handlers.NewAffiliateScheduler(appDb, botDb, appLogger)
	affiliateScheduler.Start(ctx)

	redeemer := handlers.NewRedeemerService(appDb, appLogger)
	minter := handlers.NewMinterService(appDb, appLogger)

	s := handlers.NewBotService(botDb, appDb, botClient, w9, affiliateScheduler)
	a := handlers.NewAppService(appDb, appLogger, w9)
	a.SetBotService(s)
	a.SetRedeemerService(redeemer)
	a.SetMinterService(minter)
	StartDeletedAccountPurgeLoop(ctx, a, appLogger)

	p := handlers.NewPonderService(ponderDb, appDb, appLogger)
	return router.New(s, a, p), nil
}
