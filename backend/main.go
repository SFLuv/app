package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/router"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	bdb, err := db.PgxDB("bot")
	if err != nil {
		log.Fatal(fmt.Sprintf("error initializing bot db: %s\n", err))
	}
	adb, err := db.PgxDB("account")
	if err != nil {
		log.Fatal(fmt.Sprintf("error initializing account db: %s\n", err))
	}
	mdb, err := db.PgxDB("merchant")
	if err != nil {
		log.Fatal(fmt.Sprintf("error initializing merchant db: %s\n", err))
	}

	botDb := db.Bot(bdb)
	err = botDb.CreateTables()
	if err != nil {
		fmt.Println(err)
		return
	}

	accountDb := db.Account(adb)
	err = accountDb.CreateTables()
	if err != nil {
		fmt.Println(err)
		return
	}

	bot, err := bot.Init()
	if err != nil {
		fmt.Printf("error initializing bot service: %s\n", err)
		return
	}

	if mdb == nil {
		fmt.Println("mdb is nil")
		return
	}

	s := handlers.NewBotService(botDb, bot)
	a := handlers.NewAccountService(accountDb)

	r := router.New(s, a)
	port := os.Getenv("PORT")

	fmt.Printf("now listening on port %s\n", port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", port), r)
	fmt.Println(err)
}
