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

	b := db.InitDB("bot")
	botDb := db.Bot(b)
	err := botDb.CreateTables()
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

	r := router.New(s)
	port := os.Getenv("PORT")

	fmt.Printf("now listening on port %s\n", port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", port), r)
	fmt.Println(err)
}
