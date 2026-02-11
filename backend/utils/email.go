package utils

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
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
	m := mailgun.NewMessage(
		fmt.Sprintf("%s <%s>", fromName, fromEmail),
		subject,
		"",
		toEmail,
	)
	m.SetHTML(htmlContent)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	_, _, err := es.mg.Send(ctx, m)
	return err
}

func BuildStyledEmail(title, subtitle, contentHTML string) string {
	template := `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>{{TITLE}}</title>
  </head>
  <body style="margin:0; padding:0; background-color:#f6f7fb; font-family: 'Helvetica Neue', Arial, sans-serif; color:#111827;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#f6f7fb; padding:24px 0;">
      <tr>
        <td align="center">
          <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px; background-color:#ffffff; border-radius:16px; overflow:hidden; box-shadow:0 10px 30px rgba(15, 23, 42, 0.08);">
            <tr>
              <td style="background: linear-gradient(120deg, #ff8a8a 0%, #eb6c6c 55%, #d55c5c 100%); padding:20px 28px; color:#ffffff;">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                  <tr>
                    <td style="width:48px; padding-right:12px;">
                      <img
                        src="https://app.sfluv.org/icon.png"
                        alt="SFLuv"
                        width="40"
                        height="40"
                        style="display:block; border-radius:10px; background:#ffffff; padding:4px;"
                      />
                    </td>
                    <td>
                      <h1 style="margin:0 0 6px; font-size:20px; letter-spacing:0.4px;">{{TITLE}}</h1>
                      <p style="margin:0; font-size:14px; opacity:0.9;">{{SUBTITLE}}</p>
                    </td>
                  </tr>
                </table>
              </td>
            </tr>
            <tr>
              <td style="padding:24px 28px 24px;">
                {{CONTENT}}
              </td>
            </tr>
          </table>
          <p style="margin:16px 0 0; font-size:11px; color:#9ca3af;">SFLuv · Notifications</p>
        </td>
      </tr>
    </table>
  </body>
</html>`

	replacer := strings.NewReplacer(
		"{{TITLE}}", title,
		"{{SUBTITLE}}", subtitle,
		"{{CONTENT}}", contentHTML,
	)
	return replacer.Replace(template)
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
