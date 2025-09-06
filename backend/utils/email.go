package utils

import (
	"context"
	"fmt"
	"os"

	brevo "github.com/getbrevo/brevo-go/lib"
)

type EmailSender struct {
	client *brevo.APIClient
}

func NewEmailSender() *EmailSender {
	apiKey := os.Getenv("BREVO_API_KEY")
	if apiKey == "" {
		panic("BREVO_API_KEY environment variable is not set")
	}
	cfg := brevo.NewConfiguration()
	cfg.AddDefaultHeader("api-key", apiKey)
	client := brevo.NewAPIClient(cfg)
	result, resp, err := client.AccountApi.GetAccount(context.Background())
	if err != nil {
		fmt.Println("Error when calling AccountApi->get_account: ", err.Error())
		return nil
	}
	fmt.Println("GetAccount Object:", result, " GetAccount Response: ", resp)
	return &EmailSender{client: client}
}

func (es *EmailSender) SendEmail(toEmail, toName, subject, htmlContent string, fromEmail, fromName string) error {
	sendSmtpEmail := brevo.SendSmtpEmail{
		To: []brevo.SendSmtpEmailTo{
			{
				Email: toEmail,
				Name:  toName,
			},
		},
		Subject:     subject,
		HtmlContent: htmlContent,
		Sender: &brevo.SendSmtpEmailSender{
			Email: fromEmail,
			Name:  fromName,
		},
	}

	_, _, err := es.client.TransactionalEmailsApi.SendTransacEmail(context.Background(), sendSmtpEmail)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}

// Example usage:
// func main() {
// 	apiKey := os.Getenv("BREVO_API_KEY")
// 	sender := NewEmailSender(apiKey)
// 	err := sender.SendEmail(
// 		"recipient@example.com",
// 		"Recipient Name",
// 		"Test Subject",
// 		"<h1>Hello from Brevo!</h1>",
// 		"sender@example.com",
// 		"Sender Name",
// 	)
// 	if err != nil {
// 		fmt.Println("Error:", err)
// 	} else {
// 		fmt.Println("Email sent successfully!")
// 	}
// }
