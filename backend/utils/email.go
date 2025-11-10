package utils

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/mailgun/mailgun-go/v4"
)

type EmailSender struct {
	mg mailgun.Mailgun
}

func NewEmailSender() *EmailSender {
	domain := os.Getenv("MAILGUN_DOMAIN")
	apiKey := os.Getenv("MAILGUN_API_KEY")
	if domain == "" || apiKey == "" {
		fmt.Println("MAILGUN_DOMAIN or MAILGUN_API_KEY environment variable is not set")
		return nil
	}
	mg := mailgun.NewMailgun(domain, apiKey)
	return &EmailSender{mg: mg}
}

func (es *EmailSender) SendEmail(toEmail, toName, subject, htmlContent string, fromEmail, fromName string) error {
	// fromEmail = "no_reply@" + os.Getenv("MAILGUN_DOMAIN")
	// fromName = "SFLuv Admin"
	m := es.mg.NewMessage(
		fmt.Sprintf("%s <%s>", fromName, fromEmail),
		subject,
		htmlContent,
		toEmail,
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	_, _, err := es.mg.Send(ctx, m)
	return err
}

func (es *EmailSender) AddAuthorizedRecipient(toEmail string) error {
	req, _ := http.NewRequest("POST",
		"https://api.mailgun.net/v5/sandbox/auth_recipients?email="+toEmail, nil)
	req.SetBasicAuth("api", os.Getenv("MAILGUN_API_KEY"))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	fmt.Println("Status:", resp.Status)
	return nil
}
