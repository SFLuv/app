package main

import (
	"testing"

	"github.com/faucet-portal/backend/db"
)

func TestInsert(t *testing.T) {

	t.Setenv("DB_FOLDER_PATH", "./test_data")

	mdb := db.MerchantDB()

	if mdb == nil {
		t.Fatal("MerchantDB returned nil")
	}
	// Create a new merchant
	merchant := db.Merchant{
		Name:        "Test Merchant",
		Description: "This is a test merchant",
	}
	// Save the merchant to the database
	result := mdb.Create(&merchant)
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
}
