package main

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"

	"bytes"
	"net/http"
	"net/http/httptest"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/faucet-portal/backend/db"
	"github.com/faucet-portal/backend/handlers"
	"github.com/faucet-portal/backend/structs"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/mock"
)

// TODO: factor this code with MakeBotService
func SetupBotTestDB(t *testing.T) *db.BotDB {

	t.Setenv("DB_FOLDER_PATH", "./test_data")

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
	// Clean up the test database TODO: make sure we don't drop the wrong database!
	bdb := db.InitDB("bot")
	if bdb == nil {
		return
	}
	// Clean the codes table
	bdb.GetDB().Exec("delete from codes")

	// Clean the events table
	bdb.GetDB().Exec("delete from events")
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

func GenerateKeyPair() (string, string) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyString := hexutil.Encode(privateKeyBytes)[2:]

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("cannot assert type: publicKey is not of type *ecdsa.PublicKey")
	}

	// publicKeyBytes := crypto.FromECDSAPub(publicKeyECDSA)
	// fmt.Println("Public Key:", hexutil.Encode(publicKeyBytes))

	address := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	return privateKeyString, address
}

type BotTest struct {
	mock.Mock
}

func (b *BotTest) Key() string {
	return "test-key"
}
func (b *BotTest) Send(amount uint64, address string) error {
	// Mock sending tokens
	if address == "" {
		return fmt.Errorf("invalid address")
	}
	if amount <= 0 {
		return fmt.Errorf("invalid amount")
	}
	// Simulate successful sending
	fmt.Printf("Sent %d tokens to %s\n", amount, address)
	return nil
}

func TestRedeem(t *testing.T) {
	LoadEnv(t)

	botDb := SetupBotTestDB(t)
	if botDb == nil {
		t.Fatalf("Failed to set up bot test database")
	}
	defer CleanUpBotTestDB()
	// Create a mock event
	event := &structs.Event{
		Id:          "test-event", // NOTE: NewEvent stomps on this Id
		Title:       "Test Event",
		Description: "This is a test event",
		Expiration:  0,
		Amount:      1,
	}
	// Insert the event into the database
	eventId, err := botDb.NewEvent(event)
	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}
	// create a code
	code := &structs.Code{
		Id:       "test-code", // NOTE: NewCode stomps on this Id
		Event:    eventId,
		Redeemed: false,
	}
	// Insert the code into the database
	codeId, err := botDb.NewCode(code)
	if err != nil {
		t.Fatalf("Failed to create code: %v", err)
	}

	bot := new(BotTest)

	bot_service := handlers.NewBotService(botDb, bot)

	t.Setenv("ADMIN_KEY", "0123456789")

	_, address := GenerateKeyPair()

	post_body := map[string]interface{}{
		"Code":    codeId,
		"Address": address,
	}
	body, _ := json.Marshal(post_body)
	req := httptest.NewRequest(http.MethodPost, "/redeem", bytes.NewReader(body))
	req.Header.Set("X-API-KEY", "0123456789")
	w := httptest.NewRecorder()
	bot_service.Redeem(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d", res.StatusCode)
	}

	fmt.Printf("SFLuv minted to address: %s\n", address)

	CleanUpBotTestDB()
}
