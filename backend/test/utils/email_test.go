package utils_test

import (
	"testing"

	"github.com/SFLuv/app/backend/utils"
	"github.com/joho/godotenv"
)

func TestSendEmail(t *testing.T) {

	godotenv.Load("../../.env")

	to := "pete@sfluv.com"
	subject := "Test Subject"
	body := "This is a test email."

	sender := utils.NewEmailSender()
	err := sender.SendEmail(to, "Test User", subject, body, "sender@example.com", "Sender Name")
	if err != nil {
		t.Errorf("SendEmail failed: %v", err)
	}
}
