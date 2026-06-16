package main

import (
	"context"
	"fmt"
	"log"

	"github.com/SFLuv/app/backend/bootstrap"
)

func main() {
	bootstrap.LoadEnv()
	ctx := context.Background()

	// Include the Ponder pool so migrations that backfill Ponder tables (e.g.
	// the chain-id backfill) have it available; without it those migrations
	// would be marked complete here without running.
	pools, err := bootstrap.OpenDBPools(true)
	if err != nil {
		log.Fatal(err)
	}
	defer pools.Close()

	appLogger, err := bootstrap.NewAppLogger()
	if err != nil {
		log.Fatal(fmt.Sprintf("error initializing app logger: %s", err))
	}
	defer appLogger.Close()

	if err := bootstrap.InitializeDatabases(ctx, pools, appLogger); err != nil {
		log.Fatal(err)
	}

	if err := bootstrap.RunPendingMigrations(ctx, pools, appLogger); err != nil {
		log.Fatal(err)
	}

	if err := bootstrap.RunInitializationSyncs(ctx, pools, appLogger); err != nil {
		log.Fatal(err)
	}

	fmt.Println("database initialization complete")
}
