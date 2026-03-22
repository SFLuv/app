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

	pools, err := bootstrap.OpenDBPools(false)
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

	if err := bootstrap.RunInitializationSyncs(ctx, pools, appLogger); err != nil {
		log.Fatal(err)
	}

	fmt.Println("database initialization complete")
}
