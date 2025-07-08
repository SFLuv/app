package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/faucet-portal/backend/db"
	"github.com/joho/godotenv"
	"github.com/faucet-portal/backend/handlers"
    /*"github.com/faucet-portal/backend/handlers"
	"github.com/faucet-portal/backend/router"
	"github.com/faucet-portal/backend/structs"*/)

func TestMain(m *testing.M) {
    // setup code
    godotenv.Load("../test.env", "../.env")
    mdb := db.InitDB("merchants")
	merchantDb := db.Merchant(mdb)
	merchantDb.CreateTables()
    merchant_service := handlers.NewMerchantService(merchantDb)

    //run tests
    code := m.Run()
    // teardown code

    os.Exit(code)
}

func TestGeneric(t *testing.T) {
    fmt.Println("generic test run")
}

func TestGeneric2(t *testing.T) {
    fmt.Println("generic test 2 run")
}
