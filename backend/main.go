package main

import (
	"flag"
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
	updateFlag := flag.Bool("update", false, "set true to update tables instead of running server")
	flag.Parse()
	if *updateFlag {
		updateTables()
		return
	}

	bdb := db.InitDB("bot")
	adb := db.InitDB("account")

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

	s := handlers.NewBotService(botDb, bot)
	a := handlers.NewAccountService(accountDb)

	r := router.New(s, a)
	port := os.Getenv("PORT")

	fmt.Printf("now listening on port %s\n", port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", port), r)
	fmt.Println(err)
}

func updateTables() {
	bdb := db.InitDB("bot")
	botDb := db.Bot(bdb)
	err := botDb.UpdateTables()
	if err != nil {
		fmt.Println("error updating bot db:", err)
	}
}
