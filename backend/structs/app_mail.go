package structs

type EmailMask struct {
	ToEmail     string `json:"toEmail"`
	ToName      string `json:"toName"`
	Subject     string `json:"subject"`
	HtmlContent string `json:"htmlContent"`
	FromEmail   string `json:"fromEmail"`
	FromName    string `json:"fromName"`
}
