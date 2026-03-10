package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

func (a *AppService) appVerifyURL(token string) string {
	baseURL := strings.TrimSpace(os.Getenv("APP_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return fmt.Sprintf("%s/verify?token=%s", baseURL, url.QueryEscape(token))
}

func (a *AppService) sendUserEmailVerificationEmail(toEmail string, token string, expiresAt *time.Time) error {
	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		return fmt.Errorf("email sender is not configured")
	}

	verifyURL := a.appVerifyURL(token)
	expiryLabel := "30 minutes"
	if expiresAt != nil {
		expiryLabel = expiresAt.UTC().Format(time.RFC1123)
	}

	title := "Verify Your Email"
	htmlContent := utils.BuildStyledEmail(
		title,
		"Complete verification to use this email for SFLuv notifications.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:160px;">Email</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Verification Link</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px;">
      <a href="%s" style="color:#eb6c6c; text-decoration:none; font-weight:600;">Verify Email</a>
    </td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Expires</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">%s</td>
  </tr>
</table>
<p style="margin:14px 0 0; font-size:12px; color:#6b7280;">
  If you did not request this, you can ignore this email.
</p>`, toEmail, verifyURL, expiryLabel),
	)

	return emailSender.SendEmail(toEmail, "SFLuv User", title, htmlContent, utils.NotificationFromEmail(), "SFLuv")
}

func (a *AppService) GetUserVerifiedEmails(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	records, err := a.db.GetUserVerifiedEmails(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting verified emails for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(records)
}

func (a *AppService) RequestUserEmailVerification(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading verified email request body for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := structs.UserVerifiedEmailRequest{}
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	record, token, err := a.db.CreateOrRefreshUserEmailVerification(r.Context(), *userDid, req.Email)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "already verified") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error creating verified email request for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := a.sendUserEmailVerificationEmail(record.Email, token, record.VerificationTokenExpiresAt); err != nil {
		a.logger.Logf("error sending verification email to %s for user %s: %s", record.Email, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(record)
}

func (a *AppService) ResendUserEmailVerification(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	emailID := strings.TrimSpace(r.PathValue("email_id"))
	if emailID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	record, token, err := a.db.ResendUserEmailVerification(r.Context(), *userDid, emailID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not found") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "already verified") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error resending verification email for user %s email %s: %s", *userDid, emailID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := a.sendUserEmailVerificationEmail(record.Email, token, record.VerificationTokenExpiresAt); err != nil {
		a.logger.Logf("error sending verification email during resend to %s for user %s: %s", record.Email, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(record)
}

func (a *AppService) VerifyUserEmailToken(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(string(body)) != "" {
			req := structs.UserEmailVerificationTokenRequest{}
			if err := json.Unmarshal(body, &req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			token = strings.TrimSpace(req.Token)
		}
	}

	record, err := a.db.VerifyUserEmailToken(r.Context(), token)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "expired") {
			w.WriteHeader(http.StatusGone)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "already verified") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error verifying email token: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(record)
}
