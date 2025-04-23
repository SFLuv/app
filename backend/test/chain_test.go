package main

import (
	"os"
	"testing"

	"github.com/faucet-portal/backend/db"
	"github.com/faucet-portal/backend/structs"
	"github.com/joho/godotenv"
)

func SetupBotTestDB() *db.BotDB {
	// Set up the test database
	bdb := db.InitDB("bot")
	if bdb == nil {
		panic("Failed to initialize test database")
	}

	// Create a new bot database
	botDb := db.Bot(bdb)
	err := botDb.CreateTables()
	if err != nil {
		panic("Failed to create bot tables")
	}
	return botDb
}

func CleanUpBotTestDB() {
	// Clean up the test database
	bdb := db.InitDB("bot")
	if bdb == nil {
		return
	}
	// Drop the codes table
	bdb.Exec("DROP TABLE IF EXISTS codes")

	// Drop the events table
	bdb.Exec("DROP TABLE IF EXISTS events")
}

func LoadEnv(t *testing.T) {
	reader, err := os.Open("../.env")
	if err != nil {
		t.Fatalf("Failed to open .env file: %v", err)
	}
	defer reader.Close()
	myEnv, err := godotenv.Parse(reader)
	if err != nil {
		t.Fatalf("Failed to load environment variables: %v", err)
	}
	for key, value := range myEnv {
		t.Setenv(key, value)
	}
}

func TestRedeem(t *testing.T) {
	LoadEnv(t)

	botDb := SetupBotTestDB()
	if botDb == nil {
		t.Fatalf("Failed to set up bot test database")
	}
	defer CleanUpBotTestDB()
	// Create a mock event
	event := &structs.Event{
		Id:          "test-event",
		Title:       "Test Event",
		Description: "This is a test event",
		Expiration:  0,
		Amount:      100,
	}
	// Insert the event into the database
	_, err := botDb.NewEvent(event)
	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}
	// create a code
	code := &structs.Code{
		Id:       "test-code",
		Event:    event.Id,
		Redeemed: false,
	}
	// Insert the code into the database
	_, err = botDb.NewCode(code)
	if err != nil {
		t.Fatalf("Failed to create code: %v", err)
	}

	// mock blockchain calls

	CleanUpBotTestDB()
}
