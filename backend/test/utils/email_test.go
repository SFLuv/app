package utils_test

import (
	"testing"

	"github.com/SFLuv/app/backend/utils"
	"github.com/joho/godotenv"
)

func TestSendEmail(t *testing.T) {

	godotenv.Load("../../.env")

	to := "pete@timelight.com"
	subject := "Test Subject"
	body := "This is a test email."

	sender := utils.NewEmailSender()
	// Check if the sender is initialized
	if sender == nil {
		t.Fatal("Failed to create email sender")
	}
	err := sender.SendEmail(to, "Test User", subject, body, "pete@sfluv.org", "SFLuv")
	if err != nil {
		t.Errorf("SendEmail failed: %v", err)
	}
}

func TestAddAuthorizedRecipient(t *testing.T) {

	godotenv.Load("../../.env")

	to := "pete@timelight.com"
	sender := utils.NewEmailSender()
	// Check if the sender is initialized
	if sender == nil {
		t.Fatal("Failed to create email sender")
	}
	err := sender.AddAuthorizedRecipient(to)
	if err != nil {
		t.Errorf("AddAuthorizedRecipient failed: %v", err)
	}
}
