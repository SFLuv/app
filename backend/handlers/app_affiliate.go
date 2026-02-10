package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

func (a *AppService) RequestAffiliateStatus(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading affiliate request body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.AffiliateRequest
	err = json.Unmarshal(body, &req)
	if err != nil || req.Organization == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	affiliate, err := a.db.UpsertAffiliateRequest(r.Context(), *userDid, req.Organization)
	if err != nil {
		if err.Error() == "affiliate already approved" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		a.logger.Logf("error upserting affiliate request for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	adminEmail := os.Getenv("AFFILIATE_ADMIN_EMAIL")
	emailSender := utils.NewEmailSender()
	if adminEmail != "" && emailSender != nil {
		fromDomain := os.Getenv("MAILGUN_DOMAIN")
		fromEmail := "no_reply@sfluv.org"
		if fromDomain != "" {
			fromEmail = "no_reply@" + fromDomain
		}

		subject := "New Affiliate Request"
		details := fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px;">User</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Organization</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">%s</td>
  </tr>
</table>`, affiliate.UserId, affiliate.Organization)

		htmlContent := utils.BuildStyledEmail(
			"New Affiliate Request",
			"A user has requested affiliate status.",
			details,
		)

		err = emailSender.SendEmail(adminEmail, "Admin", subject, htmlContent, fromEmail, "SFLuv Affiliates")
		if err != nil {
			a.logger.Logf("error sending affiliate request email: %s", err.Error())
		}
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(affiliate)
}

func (a *AppService) GetAffiliates(w http.ResponseWriter, r *http.Request) {
	affiliates, err := a.db.GetAffiliates(r.Context())
	if err != nil {
		a.logger.Logf("error getting affiliates: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(affiliates)
}

func (a *AppService) UpdateAffiliate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update affiliate body: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.AffiliateUpdateRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	affiliate, err := a.db.UpdateAffiliate(r.Context(), &req)
	if err != nil {
		a.logger.Logf("error updating affiliate %s: %s", req.UserId, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(affiliate)
}

func (a *AppService) IsAffiliate(ctx context.Context, id string) bool {
	isAffiliate, err := a.db.IsAffiliate(ctx, id)
	if err != nil {
		a.logger.Logf("error getting affiliate state for user %s: %s", id, err)
		return false
	}

	return isAffiliate
}

func (a *AppService) GetFirstAdminId(ctx context.Context) string {
	id, err := a.db.GetFirstAdminId(ctx)
	if err != nil && err != pgx.ErrNoRows {
		a.logger.Logf("error getting default admin id: %s", err)
	}
	return id
}
