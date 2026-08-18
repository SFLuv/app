package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"github.com/SFLuv/app/backend/db"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

func (a *AppService) GetUsers(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if !a.IsAdmin(r.Context(), *userDid) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	params := r.URL.Query()
	page, err := strconv.Atoi(params.Get("page"))
	if err != nil || page < 0 {
		page = 0
	}
	count, err := strconv.Atoi(params.Get("count"))
	if err != nil || count <= 0 || count > 500 {
		count = 100
	}
	search := strings.TrimSpace(params.Get("search"))
	versionFilters := append([]string{}, params["version"]...)
	versionFilters = append(versionFilters, params["versions"]...)

	users, err := a.db.GetUsers(r.Context(), page, count, search, versionFilters)
	if err != nil {
		a.logger.Logf("error getting users: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := a.db.AttachClientVersionDevices(r.Context(), users); err != nil {
		a.logger.Logf("error attaching client version devices to users: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	total, err := a.db.CountUsers(r.Context(), search, versionFilters)
	if err != nil {
		a.logger.Logf("error counting users: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	versionOptions, err := a.db.GetClientVersionFilterOptions(r.Context())
	if err != nil {
		a.logger.Logf("error getting client version filter options: %s", err)
		versionOptions = []string{}
	}

	versionCounts, err := a.db.GetClientVersionUserCounts(r.Context())
	if err != nil {
		a.logger.Logf("error getting client version user counts: %s", err)
		versionCounts = []*structs.ClientVersionUserCount{}
	}

	response := struct {
		Users                []*structs.User                   `json:"users"`
		Total                int                               `json:"total"`
		Page                 int                               `json:"page"`
		Count                int                               `json:"count"`
		ClientVersionOptions []string                          `json:"client_version_options"`
		ClientVersionCounts  []*structs.ClientVersionUserCount `json:"client_version_counts"`
	}{
		Users:                users,
		Total:                total,
		Page:                 page,
		Count:                count,
		ClientVersionOptions: versionOptions,
		ClientVersionCounts:  versionCounts,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (a *AppService) ExportUserEmailList(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if !a.IsAdmin(r.Context(), *userDid) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	emails, err := a.db.GetMailingListEmails(r.Context())
	if err != nil {
		a.logger.Logf("error exporting mailing list emails: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"email"}); err != nil {
		a.logger.Logf("error writing mailing list csv header: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for _, email := range emails {
		if err := writer.Write([]string{email}); err != nil {
			a.logger.Logf("error writing mailing list csv row: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		a.logger.Logf("error flushing mailing list csv: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	filename := "sfluv-email-list-" + time.Now().UTC().Format("2006-01-02") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func (a *AppService) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if !a.IsAdmin(r.Context(), *userDid) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update user role body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req struct {
		UserId string `json:"user_id"`
		Role   string `json:"role"`
		Value  bool   `json:"value"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.UserId == "" || req.Role == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.UpdateUserRole(r.Context(), req.UserId, req.Role, req.Value); err != nil {
		a.logger.Logf("error updating user role: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *AppService) UpdateLocationApproval(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update location approval body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var u structs.UpdateLocationApprovalRequest
	err = json.Unmarshal(body, &u)
	if err != nil {
		a.logger.Logf("error unmarshalling update location approval body: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ownerID, wasApproved, err := a.db.GetLocationOwnerAndApproval(r.Context(), u.Id)
	if err != nil {
		a.logger.Logf("error loading location %d owner/approval state: %s", u.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	isApproving := u.Approval != nil && *u.Approval
	hadOtherApprovedLocations := false
	if isApproving {
		hadOtherApprovedLocations, err = a.db.OwnerHasApprovedLocationExcluding(r.Context(), ownerID, u.Id)
		if err != nil {
			a.logger.Logf("error checking existing approved locations for owner %s: %s", ownerID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	// Work out what this location needs, and derive any new addresses, before the
	// approval transaction opens. A second location must never be published
	// sharing another shop's wallet, so if the chain cannot be reached to derive
	// one the approval fails and the admin retries — usually a minute later.
	provisioning, err := a.db.GetLocationProvisioningContext(r.Context(), u.Id)
	if err != nil {
		a.logger.Logf("error loading provisioning context for location %d: %s", u.Id, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var derived *db.DerivedLocationWallets
	if u.Approval != nil && *u.Approval && provisioning.NeedsDerivedWallets() {
		paymentIndex := provisioning.NextSmartIndex
		tippingIndex := paymentIndex + 1

		paymentAddress, deriveErr := a.deriveSmartAccountAddress(r.Context(), provisioning.OwnerEOA, paymentIndex)
		if deriveErr != nil {
			a.logger.Logf("error deriving payment wallet for location %d: %s", u.Id, deriveErr.Error())
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "Could not reach the chain to create this location's wallets. Try approving again in a moment.",
			})
			return
		}
		tippingAddress, deriveErr := a.deriveSmartAccountAddress(r.Context(), provisioning.OwnerEOA, tippingIndex)
		if deriveErr != nil {
			a.logger.Logf("error deriving tipping wallet for location %d: %s", u.Id, deriveErr.Error())
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "Could not reach the chain to create this location's wallets. Try approving again in a moment.",
			})
			return
		}

		derived = &db.DerivedLocationWallets{
			PaymentAddress: paymentAddress,
			PaymentIndex:   paymentIndex,
			TippingAddress: tippingAddress,
			TippingIndex:   tippingIndex,
			Street:         provisioning.Street,
		}
	}

	err = a.db.UpdateLocationApproval(r.Context(), u.Id, u.Approval, provisioning, derived)
	if err != nil {
		status := "pending"
		if u.Approval != nil {
			if *u.Approval {
				status = "approved"
			} else {
				status = "rejected"
			}
		}
		a.logger.Logf("error updating location approval for location %d to %s", u.Id, status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if a.redeemer != nil && a.redeemer.IsEnabled() && isApproving && !wasApproved && !hadOtherApprovedLocations {
		if err := a.redeemer.EnsureMerchantHasRedeemerWallet(r.Context(), ownerID); err != nil {
			a.logger.Logf("error auto-granting redeemer role for user %s after location %d approval: %s", ownerID, u.Id, err)
		}
	}

	// Only on the transition into approved, so re-saving an already-approved
	// location does not re-notify the merchant.
	if isApproving && !wasApproved {
		a.sendLocationApprovedEmail(r.Context(), u.Id)
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *AppService) IsAdmin(ctx context.Context, id string) bool {
	isAdmin, err := a.db.IsAdmin(ctx, id)
	if err != nil {
		a.logger.Logf("error getting admin state for user %s: %s", id, err)
		return false
	}

	return isAdmin
}
