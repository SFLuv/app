package test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/router"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/test/utils"
	"github.com/SFLuv/app/backend/utils/middleware"
	"github.com/go-chi/chi/v5"
)

var Spoofer *utils.ContextSpoofer

var t1e = "test1@test.com"
var t1p = "test1phone"
var t1n = "test1name"
var TEST_USER_1 = structs.User{
	Id:          "test1",
	Exists:      true,
	IsAdmin:     false,
	IsMerchant:  false,
	IsOrganizer: false,
	IsImprover:  false,
	Email:       &t1e,
	Phone:       &t1p,
	Name:        &t1n,
}

var t2e = "test2@test.com"
var t2p = "test2phone"
var t2n = "test2name"
var TEST_USER_2 = structs.User{
	Id:          "test2",
	Exists:      true,
	IsAdmin:     false,
	IsMerchant:  false,
	IsOrganizer: false,
	IsImprover:  false,
	Email:       &t2e,
	Phone:       &t2p,
	Name:        &t2n,
}

var TEST_USERS = []structs.User{TEST_USER_1, TEST_USER_2}

var TEST_WALLET_1 = structs.Wallet{
	Id:           nil,
	Owner:        TEST_USER_1.Id,
	Name:         "test_wallet",
	IsEoa:        true,
	EoaAddress:   "0x",
	SmartAddress: nil,
	SmartIndex:   nil,
}

var t2a = "0x"
var t2i = 0
var TEST_WALLET_2 = structs.Wallet{
	Id:           nil,
	Owner:        TEST_USER_1.Id,
	Name:         "test_smart_wallet",
	IsEoa:        false,
	EoaAddress:   "0x",
	SmartAddress: &t2a,
	SmartIndex:   &t2i,
}

var TEST_WALLETS = []structs.Wallet{TEST_WALLET_1, TEST_WALLET_2}

var TEST_LOCATION_1 = structs.Location{
	ID:          1,
	GoogleID:    "abc123",
	OwnerID:     TEST_USER_1.Id,
	Name:        "Bob's Burgers",
	Description: "A homestyle burger place",
	Type:        "Restaurant",
	Approval:    true,
	Street:      "123 Ocean Ave",
	City:        "Seymour's Bay",
	State:       "CA",
	ZIP:         "90210",
	Lat:         34.0522,
	Lng:         -118.2437,
	Phone:       "555-1234",
	Email:       "bob@example.com",
	Website:     "https://bobsburgers.com",
	ImageURL:    "https://images.example.com/bobs.jpg",
	Rating:      4.6,
	MapsPage:    "https://maps.google.com/?cid=abc123",
}

var TEST_LOCATION_2 = structs.Location{
	ID:          2,
	GoogleID:    "def345",
	OwnerID:     TEST_USER_2.Id,
	Name:        "Krusty Krab",
	Description: "Delicious Krabby Patties",
	Type:        "Fast Food",
	Approval:    false,
	Street:      "124 Bikini Bottom Blvd",
	City:        "Bikini Bottom",
	State:       "HI",
	ZIP:         "96815",
	Lat:         21.3069,
	Lng:         -157.8583,
	Phone:       "555-5678",
	Email:       "krabs@krustykrab.com",
	Website:     "https://krustykrab.com",
	ImageURL:    "https://images.example.com/krusty.jpg",
	Rating:      4.9,
	MapsPage:    "https://maps.google.com/?cid=def345",
}

var TEST_LOCATION_2A = structs.Location{
	ID:          2,
	GoogleID:    "def345",
	OwnerID:     TEST_USER_2.Id,
	Name:        "Krusty Krab - Updated",
	Description: "Updated the description for testing person",
	Type:        "Fast Food",
	Approval:    false,
	Street:      "124 Bikini Bottom Blvd",
	City:        "Bikini Bottom",
	State:       "HI",
	ZIP:         "96815",
	Lat:         21.3069,
	Lng:         -157.8583,
	Phone:       "555-5678",
	Email:       "krabs@krustykrab.com",
	Website:     "https://krustykrab.com",
	ImageURL:    "https://images.example.com/krusty.jpg",
	Rating:      4.9,
	MapsPage:    "https://maps.google.com/?cid=def345",
}

var TEST_LOCATIONS = []structs.Location{TEST_LOCATION_1, TEST_LOCATION_2A}

var AppDb *db.AppDB
var TestServer *httptest.Server

func TestApp(t *testing.T) {
	GroupControllers(t)
	GroupHandlers(t)
}

func GroupControllers(t *testing.T) {
	adb, err := db.PgxDB("test_app_controllers")
	if err != nil {
		t.Fatalf("error establishing db connection: %s\n", err)
	}
	defer adb.Close(context.Background())

	AppDb = db.App(adb)
	err = AppDb.CreateTables()
	if err != nil {
		t.Fatalf("error creating app db tables for controllers: %s\n", err)
	}

	usersControllers := t.Run("user controllers group", GroupUsersControllers)
	if !usersControllers {
		t.Fatal("users controllers group failed")
	}
	walletsControllers := t.Run("wallets controllers group", GroupWalletsControllers)
	if !walletsControllers {
		t.Error("wallets controllers group failed")
	}

	// locationControllers := t.Run("location controllers group", GroupLocationControllers)
	// if !locationControllers {
	// 	t.Fatal("location controllers group failed")
	// }
}

func GroupHandlers(t *testing.T) {
	adb, err := db.PgxDB("test_app_handlers")
	if err != nil {
		t.Fatalf("error establishing db connection: %s\n", err)
	}
	defer adb.Close(context.Background())

	appHandlersDb := db.App(adb)
	err = appHandlersDb.CreateTables()
	if err != nil {
		t.Fatalf("error creating app db tables for handlers: %s\n", err)
	}

	timeString := time.Now().Format(time.RFC3339)
	appLogger, err := logger.New(fmt.Sprintf("./logs/test/app/app_test_%s.log", timeString), "APP_TEST: ")
	if err != nil {
		t.Fatalf("error initializing app logger: %s\n", err)
	}
	defer appLogger.Close()

	testRouter := chi.NewRouter()
	appService := handlers.NewAppService(appHandlersDb, appLogger)
	Spoofer = utils.NewContextSpoofer("userDid", TEST_USER_1.Id)

	testRouter.Use(middleware.AuthMiddleware)
	testRouter.Use(Spoofer.Middleware)

	router.AddUserRoutes(testRouter, appService)
	router.AddWalletRoutes(testRouter, appService)
	router.AddLocationRoutes(testRouter, appService)

	TestServer = httptest.NewServer(testRouter)
	defer TestServer.Close()

	usersHandlers := t.Run("users handlers group", GroupUsersHandlers)
	if !usersHandlers {
		t.Fatal("users handlers group failed")
	}

	walletsHandlers := t.Run("wallets handlers group", GroupWalletsHandlers)
	if !walletsHandlers {
		t.Error("wallets handlers group failed")
	}

	// locationHandlers := t.Run("location handlers group", GroupLocationHandlers)
	// if !locationHandlers {
	// 	t.Fatal("location handlers group failed")
	// }
}
