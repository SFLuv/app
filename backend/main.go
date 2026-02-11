package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/router"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	ctx := context.Background()

	bdb, err := db.PgxDB("bot")
	if err != nil {
		log.Fatal(fmt.Sprintf("error initializing bot db: %s\n", err))
	}
	adb, err := db.PgxDB("app")
	if err != nil {
		log.Fatal(fmt.Sprintf("error initializing app db: %s\n", err))
	}
	pdb, err := db.PgxDB("ponder")
	if err != nil {
		log.Fatal(fmt.Sprintf("error initializing ponder db: %s\n", err))
	}

	appLogger, err := logger.New("./logs/prod/app.log", "APP: ")
	if err != nil {
		fmt.Printf("error initializing app logger: %s\n", err)
		return
	}
	defer appLogger.Close()

	appDb := db.App(adb, appLogger)
	err = appDb.CreateTables()
	if err != nil {
		fmt.Println(err)
		return
	}

	defaultAdminId, err := appDb.GetFirstAdminId(ctx)
	if err != nil {
		fmt.Printf("error getting default admin id: %s\n", err)
	}

	botDb := db.Bot(bdb)
	err = botDb.CreateTables(defaultAdminId)
	if err != nil {
		fmt.Println(err)
		return
	}

	// TODO: Enable service flag to disable ponder for lighter-weight instances?
	ponderDb := db.Ponder(pdb, appLogger)
	err = ponderDb.Ping()
	if err != nil {
		fmt.Println(err)
		return
	}

	bot, err := bot.Init()
	if err != nil {
		fmt.Printf("error initializing bot service: %s\n", err)
		return
	}

	affiliateScheduler := handlers.NewAffiliateScheduler(appDb, botDb, appLogger)
	affiliateScheduler.Start(ctx)

	s := handlers.NewBotService(botDb, appDb, bot, affiliateScheduler)
	a := handlers.NewAppService(appDb, appLogger)
	p := handlers.NewPonderService(ponderDb, appLogger)

	r := router.New(s, a, p)
	port := os.Getenv("PORT")

	fmt.Printf("now listening on port %s\n", port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", port), r)
	fmt.Println(err)
}
