package main

import (
	"testing"

	"github.com/faucet-portal/backend/db"
)

func CleanUpMerchantTestDB() {
	// Clean up the test database
	mdb := db.MerchantDB()
	if mdb == nil {
		return
	}

	// Clean the merchant table
	mdb.GetDB().Exec("delete from merchants")
	// Clean the address table
	mdb.GetDB().Exec("delete from addresses")
}

func TestMerchantInsertPostgres(t *testing.T) {
	t.Setenv("DB", "postgres")
	TestMerchantInsert(t)
}

func TestMerchantInsertSQLite(t *testing.T) {
	t.Setenv("DB", "sqlite")
	t.Setenv("DB_FOLDER_PATH", "./test_data")
	// Ensure the test database is clean before running the test
	TestMerchantInsert(t)
}

func TestMerchantInsert(t *testing.T) {

	mdb := db.MerchantDB()

	if mdb == nil {
		t.Fatal("MerchantDB returned nil")
	}

	address := db.Address{
		Street:   "123 Test St",
		Street_2: "Apt 4B",
		City:     "Test City",
		State:    "Test State",
		Zip:      "12345",
	}

	contact := db.Person{
		Email:     "test@example.com",
		Phone:     "123-456-7890",
		FirstName: "Test",
		LastName:  "User",
	}

	category := db.Category{
		Name: "Test Category",
	}

	// Create a new merchant
	merchant := db.Merchant{
		Name:        "Test Merchant",
		Description: "This is a test merchant",
		Address:     address,
		Contact:     contact,
		Category:    category,
	}

	// Save the merchant to the database
	result := mdb.GetGormDB().Create(&merchant)
	if result.Error != nil {
		t.Fatalf("Failed to create merchant: %v", result.Error)
	}
	// Check if the merchant was created successfully
	if result.RowsAffected != 1 {
		t.Fatalf("Expected 1 row affected, got %d", result.RowsAffected)
	}
	// Check if the merchant ID is set
	if merchant.ID == 0 {
		t.Fatal("Expected merchant ID to be set, got 0")
	}
	// Check if the merchant name is correct
	if merchant.Name != "Test Merchant" {
		t.Fatalf("Expected merchant name to be 'Test Merchant', got '%s'", merchant.Name)
	}
	// Check if the merchant description is correct
	if merchant.Description != "This is a test merchant" {
		t.Fatalf("Expected merchant description to be 'This is a test merchant', got '%s'", merchant.Description)
	}
	// Check if the merchant email is correct
	if merchant.Email != "" {
		t.Fatalf("Expected merchant email to be empty, got '%s'", merchant.Email)
	}
	// Check if the merchant website is correct
	if merchant.Website != "" {
		t.Fatalf("Expected merchant website to be empty, got '%s'", merchant.Website)
	}
	// Check if the merchant phone is correct
	if merchant.Phone != "" {
		t.Fatalf("Expected merchant phone to be empty, got '%s'", merchant.Phone)
	}
	mdb.GetGormDB().Delete(&merchant)
	// Check if the merchant was deleted successfully
	if result.RowsAffected != 1 {
		t.Fatalf("Expected 1 row affected, got %d", result.RowsAffected)
	}

	result = mdb.GetGormDB().Take(&merchant)
	// Check if the merchant was deleted successfully
	if result.Error == nil {
		t.Fatal("Expected merchant to be deleted, got nil")
	}

	CleanUpMerchantTestDB()
}

func TestMerchanAddressInsert(t *testing.T) {
	t.Setenv("DB_FOLDER_PATH", "./test_data")
	mdb := db.MerchantDB()
	if mdb == nil {
		t.Fatal("MerchantDB returned nil")
	}
	// Create a new address
	address := db.Address{
		Street:   "123 Test St",
		Street_2: "Apt 4B",
		City:     "Test City",
		State:    "Test State",
		Zip:      "12345",
	}
	// Save the address to the database
	result := mdb.GetGormDB().Create(&address)
	if result.Error != nil {
		t.Fatalf("Failed to create address: %v", result.Error)
	}
	// Check if the address was created successfully
	if result.RowsAffected != 1 {
		t.Fatalf("Expected 1 row affected, got %d", result.RowsAffected)
	}

	merchant := db.Merchant{
		Name:        "Test Merchant",
		Description: "This is a test merchant",
		Address:     address,
	}
	// Save the merchant to the database
	result = mdb.GetGormDB().Create(&merchant)
	if result.Error != nil {
		t.Fatalf("Failed to create merchant: %v", result.Error)
	}
	// Check if the merchant was created successfully
	if result.RowsAffected != 1 {
		t.Fatalf("Expected 1 row affected, got %d", result.RowsAffected)
	}
	// check if merchant address is set
	if merchant.Address.ID == 0 {
		t.Fatal("Expected merchant address ID to be set, got 0")
	}
	// Check if the merchant address is correct
	if merchant.Address.Street != "123 Test St" {
		t.Fatalf("Expected merchant address street to be '123 Test St', got '%s'", merchant.Address.Street)
	}
	// delete merchant
	result = mdb.GetGormDB().Delete(&merchant)
	// Check if the merchant was deleted successfully
	if result.RowsAffected != 1 {
		t.Fatalf("Expected 1 row affected, got %d", result.RowsAffected)
	}
	// delete address
	result = mdb.GetGormDB().Delete(&address)
	// Check if the address was deleted successfully
	if result.RowsAffected != 1 {
		t.Fatalf("Expected 1 row affected, got %d", result.RowsAffected)
	}
	// Check if the address was deleted successfully
	result = mdb.GetGormDB().Take(&address)
	// Check if the address was deleted successfully
	if result.Error == nil {
		t.Fatal("Expected address to be deleted, got nil")
	}
	// Check if the merchant was deleted successfully
	result = mdb.GetGormDB().Take(&merchant)
	// Check if the merchant was deleted successfully
	if result.Error == nil {
		t.Fatal("Expected merchant to be deleted, got nil")
	}

	CleanUpMerchantTestDB()
}
