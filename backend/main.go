package main

import (
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

	bdb, err := db.PgxDB("bot")
	if err != nil {
		log.Fatalf("error initializing bot db: %s\n", err)
		os.Exit(1)
	}
	pdb, err := db.PgxDB("app")
	if err != nil {
		log.Fatalf("error initializing app db: %s\n", err)
		os.Exit(1)
	}

	botDb := db.Bot(bdb)
	err = botDb.CreateTables()
	if err != nil {
		fmt.Println(err)
		return
	}

	appLogFile := "./logs/prod/app.log"

	// create folders if they don't exist
	if _, err := os.Stat("./logs/prod"); os.IsNotExist(err) {
		err = os.MkdirAll("./logs/prod", os.ModePerm)
		if err != nil {
			fmt.Printf("error creating log directories: %s\n", err)
			return
		}
	}

	appLogger, err := logger.New(appLogFile, "APP: ")
	if err != nil {
		fmt.Printf("error initializing app logger: %s\n", err)
		return
	}
	defer appLogger.Close()

	appDb := db.App(pdb, appLogger)
	err = appDb.CreateTables()
	if err != nil {
		fmt.Println(err)
		return
	}

	bot, err := bot.Init()
	if err != nil {
		fmt.Printf("error initializing bot service: %s\n", err)
		return
	}

	s := handlers.NewBotService(botDb, bot)
	p := handlers.NewAppService(appDb, appLogger)

	r := router.New(s, p)
	port := os.Getenv("PORT")

	fmt.Printf("now listening on port %s\n", port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", port), r)
	fmt.Println(err)
}
