package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/faucet-portal/backend/bot"
	"github.com/faucet-portal/backend/db"
	"github.com/faucet-portal/backend/handlers"
	"github.com/faucet-portal/backend/router"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	bdb := db.InitDB("bot")
	adb := db.InitDB("account")
	mdb := db.MerchantDB()

	botDb := db.Bot(bdb)
	err := botDb.CreateTables()
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
