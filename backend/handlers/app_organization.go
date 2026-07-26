package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

const orgInviteTTL = 7 * 24 * time.Hour

func orgInviteToken() (token string, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("organization invite: crypto/rand failed: " + err.Error())
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	return token, base64.RawURLEncoding.EncodeToString(sum[:])
}

func orgInviteHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// requireOrgRole loads the caller's org and enforces a minimum membership role.
// minRole may be "member", "admin", or "superadmin".
func (a *AppService) requireOrgRole(w http.ResponseWriter, r *http.Request, minRole string) (*structs.Organization, string, string, bool) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return nil, "", "", false
	}
	org, role, err := a.db.GetOrganizationByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error loading organization for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, "", "", false
	}
	if org == nil {
		w.WriteHeader(http.StatusNotFound)
		return nil, "", "", false
	}
	allowed := false
	switch minRole {
	case structs.OrgRoleMember:
		allowed = true
	case structs.OrgRoleAdmin:
		allowed = role == structs.OrgRoleAdmin || role == structs.OrgRoleSuperadmin
	case structs.OrgRoleSuperadmin:
		allowed = role == structs.OrgRoleSuperadmin
	}
	if !allowed {
		w.WriteHeader(http.StatusForbidden)
		return nil, "", "", false
	}
	return org, role, *userDid, true
}

// GetMyOrganization returns the caller's organization view (404 when none).
func (a *AppService) GetMyOrganization(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	view, err := a.db.GetOrganizationView(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error loading organization view for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if view == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *AppService) UpdateMyOrganizationName(w http.ResponseWriter, r *http.Request) {
	org, _, userDid, ok := a.requireOrgRole(w, r, structs.OrgRoleAdmin)
	if !ok {
		return
	}
	var req structs.OrganizationNameUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := a.db.UpdateOrganizationName(r.Context(), org.Id, req.Name); err != nil {
		if errors.Is(err, db.ErrOrgNameTaken) {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("organization name is already taken"))
			return
		}
		a.logger.Logf("error updating organization name for user %s: %s", userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *AppService) UpdateMyOrganizationLogo(w http.ResponseWriter, r *http.Request) {
	org, _, userDid, ok := a.requireOrgRole(w, r, structs.OrgRoleAdmin)
	if !ok {
		return
	}
	var req structs.OrganizationLogoUpdateRequest
	// Logos are square-cropped data URLs; allow a few MB.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Logo) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := a.db.UpdateOrganizationLogo(r.Context(), org.Id, req.Logo); err != nil {
		a.logger.Logf("error updating organization logo for user %s: %s", userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// InviteOrganizationMember creates a single-use invite and emails the recipient
// an SFLuv-branded join link.
func (a *AppService) InviteOrganizationMember(w http.ResponseWriter, r *http.Request) {
	org, _, userDid, ok := a.requireOrgRole(w, r, structs.OrgRoleAdmin)
	if !ok {
		return
	}
	var req structs.OrganizationInviteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("a valid email address is required"))
		return
	}

	token, hash := orgInviteToken()
	if err := a.db.CreateOrganizationInvite(r.Context(), org.Id, email, hash, userDid, orgInviteTTL); err != nil {
		a.logger.Logf("error creating organization invite for org %d: %s", org.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	appBase := strings.TrimRight(os.Getenv("APP_BASE_URL"), "/")
	if appBase == "" {
		appBase = "https://app.sfluv.org"
	}
	joinLink := fmt.Sprintf("%s/organization/join?token=%s", appBase, token)

	emailSender := utils.NewEmailSender()
	if emailSender != nil {
		details := fmt.Sprintf(`
<p style="font-size:14px; color:#111827;">You have been invited to join <strong>%s</strong> on SFLuv.</p>
<p style="font-size:14px; color:#111827;">As a member you'll share the organization's roles and tools — events, workflows, and reporting — with the rest of the team.</p>
<table role="presentation" cellpadding="0" cellspacing="0" style="margin:24px auto;">
  <tr>
    <td style="border-radius:8px; background:#eb6c6c;">
      <a href="%s" style="display:inline-block; padding:12px 28px; font-size:14px; font-weight:600; color:#ffffff; text-decoration:none;">Join %s</a>
    </td>
  </tr>
</table>
<p style="font-size:12px; color:#6b7280;">This invitation expires in 7 days and can only be used with the email address it was sent to. If you weren't expecting it, you can ignore this email.</p>`,
			utils.EscapeEmailHTML(org.Name), joinLink, utils.EscapeEmailHTML(org.Name))

		htmlContent := utils.BuildStyledEmail(
			"You're invited to "+org.Name,
			"Join your team on SFLuv",
			details,
		)
		if err := emailSender.SendEmail(email, "", "You're invited to join "+org.Name+" on SFLuv", htmlContent, utils.NotificationFromEmail(), "SFLuv"); err != nil {
			a.logger.Logf("error sending organization invite email to %s: %s", email, err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("invite created but the email could not be sent"))
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
}

// JoinOrganization accepts an invite token for the logged-in user.
func (a *AppService) JoinOrganization(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var req structs.OrganizationJoinRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	orgId, err := a.db.AcceptOrganizationInvite(r.Context(), orgInviteHash(strings.TrimSpace(req.Token)), *userDid)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrOrgInviteInvalid):
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("This invite is invalid or has expired."))
		case errors.Is(err, db.ErrOrgInviteEmail):
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("This invite was sent to a different email address. Add or verify that email on your account first."))
		case errors.Is(err, db.ErrOrgAlreadyMember):
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("You already belong to an organization. Leave it before joining another."))
		default:
			a.logger.Logf("error accepting organization invite for user %s: %s", *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"organization_id": orgId})
}

func (a *AppService) RemoveOrganizationMember(w http.ResponseWriter, r *http.Request) {
	org, _, userDid, ok := a.requireOrgRole(w, r, structs.OrgRoleAdmin)
	if !ok {
		return
	}
	target := strings.TrimSpace(chi.URLParam(r, "user_id"))
	if target == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if target == userDid {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("use leave organization to remove yourself"))
		return
	}
	if err := a.db.RemoveOrganizationMember(r.Context(), org.Id, target); err != nil {
		switch {
		case errors.Is(err, db.ErrOrgNotMember):
			w.WriteHeader(http.StatusNotFound)
		case errors.Is(err, db.ErrOrgSuperadminRequired):
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("the superadmin cannot be removed"))
		default:
			a.logger.Logf("error removing organization member %s: %s", target, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SetOrganizationMemberRole promotes/demotes members (superadmin only).
func (a *AppService) SetOrganizationMemberRole(w http.ResponseWriter, r *http.Request) {
	org, _, _, ok := a.requireOrgRole(w, r, structs.OrgRoleSuperadmin)
	if !ok {
		return
	}
	var req structs.OrganizationMemberRoleUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := a.db.SetOrganizationMemberRole(r.Context(), org.Id, strings.TrimSpace(req.UserId), req.Role); err != nil {
		if errors.Is(err, db.ErrOrgNotMember) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error setting organization member role: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *AppService) TransferOrganizationSuperadmin(w http.ResponseWriter, r *http.Request) {
	org, _, userDid, ok := a.requireOrgRole(w, r, structs.OrgRoleSuperadmin)
	if !ok {
		return
	}
	var req structs.OrganizationTransferSuperadminRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	target := strings.TrimSpace(req.UserId)
	if target == "" || target == userDid {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := a.db.TransferOrganizationSuperadmin(r.Context(), org.Id, userDid, target); err != nil {
		if errors.Is(err, db.ErrOrgNotMember) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error transferring organization superadmin: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *AppService) LeaveOrganization(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if err := a.db.LeaveOrganization(r.Context(), *userDid); err != nil {
		switch {
		case errors.Is(err, db.ErrOrgNotMember):
			w.WriteHeader(http.StatusNotFound)
		case errors.Is(err, db.ErrOrgSuperadminRequired):
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("transfer the superadmin role before leaving"))
		default:
			a.logger.Logf("error leaving organization for user %s: %s", *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

// RequestOrganizationRole records a role request (affiliate/proposer/supervisor)
// for the caller's organization. Any member may request; admins approve via the
// platform admin panel.
func (a *AppService) RequestOrganizationRole(w http.ResponseWriter, r *http.Request) {
	org, _, userDid, ok := a.requireOrgRole(w, r, structs.OrgRoleMember)
	if !ok {
		return
	}
	var req structs.OrganizationRoleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := a.db.UpsertOrganizationRoleRequest(r.Context(), org.Id, &req, userDid); err != nil {
		a.logger.Logf("error requesting organization role for org %d: %s", org.Id, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// ensureOrgForRoleRequest backs the legacy role-request endpoints: if the user
// already belongs to an organization the request attaches to it (the submitted
// name is ignored); otherwise the submitted organization is created with the
// user as superadmin. Returns a client-safe error message on conflict.
func (a *AppService) ensureOrgForRoleRequest(r *http.Request, userDid string, orgName string, roleReq *structs.OrganizationRoleRequest) (int64, string) {
	ctx := r.Context()
	org, _, err := a.db.GetOrganizationByUser(ctx, userDid)
	if err != nil {
		a.logger.Logf("error resolving organization for role request by %s: %s", userDid, err)
		return 0, "internal error"
	}
	if org == nil {
		created, err := a.db.CreateOrganizationWithSuperadmin(ctx, orgName, userDid)
		if err != nil {
			if errors.Is(err, db.ErrOrgNameTaken) {
				return 0, "an organization with that name already exists; ask one of its admins for an invite"
			}
			a.logger.Logf("error creating organization %q for %s: %s", orgName, userDid, err)
			return 0, "internal error"
		}
		org = created
	}
	if err := a.db.UpsertOrganizationRoleRequest(ctx, org.Id, roleReq, userDid); err != nil {
		a.logger.Logf("error recording organization role request for org %d: %s", org.Id, err)
		return 0, "internal error"
	}
	return org.Id, ""
}

// --- Platform admin ----------------------------------------------------------

func (a *AppService) AdminListOrganizations(w http.ResponseWriter, r *http.Request) {
	views, err := a.db.ListOrganizations(r.Context())
	if err != nil {
		a.logger.Logf("error listing organizations: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

// AdminSetOrganizationSuperadmin reassigns an org's superadmin by email — for
// when a superadmin leaves without transferring control.
func (a *AppService) AdminSetOrganizationSuperadmin(w http.ResponseWriter, r *http.Request) {
	var req structs.AdminOrganizationSuperadminRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil || req.OrganizationId == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := a.db.AdminSetOrganizationSuperadminByEmail(r.Context(), req.OrganizationId, req.Email); err != nil {
		if errors.Is(err, db.ErrOrgNotMember) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("no organization member with that email"))
			return
		}
		a.logger.Logf("error setting organization superadmin: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// AdminSetOrganizationAllocations replaces an organization's full allocation
// list: cycles present in the request are upserted, absent cycles are removed.
func (a *AppService) AdminSetOrganizationAllocations(w http.ResponseWriter, r *http.Request) {
	var req structs.AdminOrganizationAllocationsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil || req.OrganizationId == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// A missing allocations field must fail loudly, not delete-all: a caller
	// using the old single-cycle body shape (or a typoed field) would otherwise
	// decode cleanly to nil and silently wipe the org's entire allocation set.
	// An explicit empty array remains the intentional way to remove everything.
	if req.Allocations == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := a.db.ReplaceOrganizationAllocations(r.Context(), req.OrganizationId, req.Allocations); err != nil {
		a.logger.Logf("error replacing organization allocations: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// AdminSetOrganizationRoleStatus approves/rejects an organization role.
func (a *AppService) AdminSetOrganizationRoleStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrganizationId int64  `json:"organization_id"`
		RoleType       string `json:"role_type"`
		Status         string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil || req.OrganizationId == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Status != "approved" && req.Status != "rejected" && req.Status != "pending" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := a.db.SetOrganizationRoleStatus(r.Context(), req.OrganizationId, req.RoleType, req.Status); err != nil {
		if errors.Is(err, db.ErrOrgNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error setting organization role status: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
