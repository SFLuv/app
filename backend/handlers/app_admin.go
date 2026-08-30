package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
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

// UpdateUserAccountType repairs an account type that the person it belongs to
// cannot change themselves. The signup answer is honoured once and never
// overwritten, so somebody who chose 'regular' by mistake — or who accepted the
// policies on a client that predates the question and got the 'regular' default
// — is locked out of merchant onboarding permanently. This is the only way back.
//
// Guarded by withAdmin in the router rather than by an in-handler IsAdmin check
// like its neighbour UpdateUserRole. withAdmin is what every other /admin/users
// route uses, it rejects before the body is read, and it honours X-Admin-Key,
// which is how support fixes this for a merchant who cannot get far enough into
// the app to be helped any other way.
func (a *AppService) UpdateUserAccountType(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update user account type body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.AdminUpdateUserAccountTypeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userId := strings.TrimSpace(req.UserId)
	if userId == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("user_id is required"))
		return
	}

	// A value the CHECK constraint would reject should come back as a 400
	// naming the field, not a 500 out of the database. An empty string is not
	// "unchanged" here the way it is on the signup path — this route exists to
	// state a type.
	accountType := strings.TrimSpace(req.AccountType)
	if !structs.IsValidAccountType(accountType) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("account_type must be 'regular' or 'merchant'"))
		return
	}

	result, err := a.db.SetUserAccountType(r.Context(), userId, accountType)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error updating account type for user %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// This changes what the person can do in the app — it is what puts a
	// merchant into onboarding, or takes them out of it — so who did it, to
	// whom, and from what to what has to survive the request. The onboarding
	// stamp goes in the same line because it, not the type alone, decides
	// whether they will actually be walked through setup.
	onboarding := "not completed"
	if result.MerchantOnboardingCompletedAt != nil {
		onboarding = result.MerchantOnboardingCompletedAt.UTC().Format(time.RFC3339)
	}
	// X-Admin-Key callers have no user identity to borrow when the database has
	// no admin, and the audit line still has to name somebody.
	actor := "x-admin-key"
	if userDid := utils.GetDid(r); userDid != nil {
		actor = *userDid
	}
	a.logger.Logf(
		"admin %s changed account type for user %s from %s to %s (merchant onboarding: %s)",
		actor, result.UserId, result.PreviousAccountType, result.AccountType, onboarding,
	)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
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

	// Approval is where a location gets its wallets. A merchant's first location
	// inherits the primary wallet they already hold; every one after it is minted
	// a till of its own, because two shops on one address cannot be told apart
	// afterwards. A tipping wallet is minted alongside whenever the merchant
	// answered yes to tips on the form, and never otherwise.
	//
	// The addresses are counterfactual — CREATE2 arithmetic off the account
	// factory against the merchant's own signer — so nothing is deployed and no
	// signature is needed from anyone to do this. The paymaster deploys each
	// account on its first outgoing transaction; until then tokens sit at the
	// address regardless, which is all a till needs.
	//
	// A location must not be published without a wallet, so a chain that cannot
	// be reached fails the approval and the admin retries, usually a minute
	// later. Creation is the opposite — see provisionNewLocationWallets, which is
	// the optional early path and treats a failed derivation as "not yet".
	provisioning, err := a.db.GetLocationProvisioningContext(r.Context(), u.Id)
	if err != nil {
		a.logger.Logf("error loading provisioning context for location %d: %s", u.Id, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var derived *db.DerivedLocationWallets
	if isApproving {
		derived, err = a.deriveLocationWallets(r.Context(), provisioning)
		if err != nil {
			a.logger.Logf("error deriving wallets for location %d: %s", u.Id, err.Error())
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "Could not reach the chain to create this location's wallets. Try approving again in a moment.",
			})
			return
		}
	}

	err = a.db.UpdateLocationApproval(r.Context(), u.Id, u.Approval, provisioning, derived)
	if errors.Is(err, db.ErrLocationWalletIndexMoved) {
		// Another of this merchant's locations took the address between the
		// derivation and the write. Nothing was committed; approving again derives
		// against the new state.
		a.logger.Logf("wallet index moved while approving location %d; asking the admin to retry", u.Id)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Another of this merchant's locations claimed that wallet. Try approving again.",
		})
		return
	}
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
