package test

import (
	"context"
	"testing"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
)

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

var AppDb *db.AppDB

func TestApp(t *testing.T) {
	adb, err := db.PgxDB("test_app")
	if err != nil {
		t.Fatalf("error establishing db connection: %s", err)
	}
	defer adb.Close(context.Background())

	AppDb = db.App(adb)
	err = AppDb.CreateTables()
	if err != nil {
		t.Fatalf("error creating app db tables: %s", err)
	}

	usersControllers := t.Run("user controllers group", GroupUsersControllers)
	if !usersControllers {
		t.Fatalf("users controllers group failed")
	}
	walletsControllers := t.Run("wallets controllers group", GroupWalletsControllers)
	if !walletsControllers {
		t.Fatalf("wallets controllers group failed")
	}
}
