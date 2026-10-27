package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// Managing another organization's members.
//
// Until now every member operation was /organizations/mine/*, resolved from the
// caller's own membership, so nobody could act on an organization they did not
// belong to. That left the house organization able to see a partner's roster
// and change its superadmin, but not to invite, remove or re-role anybody —
// so the only way to fix a partner locked out of its own admin was to hand the
// whole organization to somebody else.
//
// These endpoints close that gap, and the same work is done by the same code:
// they call the identical database functions and send the identical invite as
// an organization's own admin does, with the organization supplied rather than
// inferred.
//
// Who may use them, in order:
//
//   - a platform admin, consistent with every other /admin route here;
//   - the house organization's superadmin, always; or
//   - a house organization member granted "Manage other organizations'
//     members" in the Administrative section.
//
// That third case is the point of the capability: the superadmin can delegate
// this without making somebody a superadmin, which would carry everything else
// with it.

// crossOrgMemberActor resolves who is asking and whether they may.
func (a *AppService) crossOrgMemberActor(w http.ResponseWriter, r *http.Request) (string, bool) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return "", false
	}
	actor := *userDid
	ctx := r.Context()

	if a.IsAdmin(ctx, actor) {
		return actor, true
	}

	org, role, err := a.db.GetOrganizationByUser(ctx, actor)
	if err != nil {
		a.logger.Logf("error loading organization for %s: %s", actor, err)
		w.WriteHeader(http.StatusInternalServerError)
		return "", false
	}
	// Checked explicitly rather than relying on the capability being
	// house-only. It is house-only because only the house superadmin can grant
	// it, which is a fact about another screen — and a gate that depends on a
	// fact about another screen is one rename away from being wrong.
	if org == nil || !org.IsHouse {
		w.WriteHeader(http.StatusForbidden)
		return "", false
	}
	if role == structs.OrgRoleSuperadmin {
		return actor, true
	}

	granted, err := a.db.HasOrgCapability(ctx, actor, structs.CapAdminOrgMembers)
	if err != nil {
		a.logger.Logf("error checking cross-org member access for %s: %s", actor, err)
		w.WriteHeader(http.StatusInternalServerError)
		return "", false
	}
	if !granted {
		w.WriteHeader(http.StatusForbidden)
		return "", false
	}
	return actor, true
}

// targetOrganization reads the {id} the caller is acting on.
//
// The house organization is refused: its own members are managed through
// /organizations/mine, where the superadmin rules already apply, and letting
// this path reach it would be a second way to change house membership with a
// different set of checks.
func (a *AppService) targetOrganization(w http.ResponseWriter, r *http.Request) (*structs.Organization, bool) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	orgId, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || orgId <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	org, err := a.db.GetOrganizationById(r.Context(), orgId)
	if err != nil {
		a.logger.Logf("error loading organization %d: %s", orgId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}
	if org == nil {
		w.WriteHeader(http.StatusNotFound)
		return nil, false
	}
	if org.IsHouse {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("manage the house organization from its own settings"))
		return nil, false
	}
	return org, true
}

// AdminInviteOrganizationMember invites somebody to a partner organization.
func (a *AppService) AdminInviteOrganizationMember(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.crossOrgMemberActor(w, r)
	if !ok {
		return
	}
	org, ok := a.targetOrganization(w, r)
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

	created, sent, err := a.createAndSendOrgInvite(r.Context(), org, email, actor, db.OrganizationInviteGrant{
		Role:         req.Role,
		Mode:         req.PermissionsMode,
		Capabilities: req.Capabilities,
	})
	if err != nil {
		a.logger.Logf("error creating organization invite for org %d: %s", org.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if created && !sent {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("invite created but the email could not be sent"))
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// AdminRemoveOrganizationMember removes somebody from a partner organization.
func (a *AppService) AdminRemoveOrganizationMember(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.crossOrgMemberActor(w, r); !ok {
		return
	}
	org, ok := a.targetOrganization(w, r)
	if !ok {
		return
	}

	target := strings.TrimSpace(chi.URLParam(r, "user_id"))
	if target == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.RemoveOrganizationMember(r.Context(), org.Id, target); err != nil {
		switch {
		case errors.Is(err, db.ErrOrgNotMember):
			w.WriteHeader(http.StatusNotFound)
		case errors.Is(err, db.ErrOrgSuperadminRequired):
			// Left as the organization's own rule rather than overridden here:
			// removing the last superadmin would leave an organization nobody
			// can administer, and the way to replace one is the superadmin
			// endpoint, which hands the role over in a single step.
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("the superadmin cannot be removed — transfer the role first"))
		default:
			a.logger.Logf("error removing member %s from org %d: %s", target, org.Id, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

// AdminSetOrganizationMemberRole changes a partner member's role.
func (a *AppService) AdminSetOrganizationMemberRole(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.crossOrgMemberActor(w, r); !ok {
		return
	}
	org, ok := a.targetOrganization(w, r)
	if !ok {
		return
	}

	var req structs.OrganizationMemberRoleUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	target := strings.TrimSpace(req.UserId)
	if target == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.SetOrganizationMemberRole(r.Context(), org.Id, target, req.Role); err != nil {
		switch {
		case errors.Is(err, db.ErrOrgNotMember):
			w.WriteHeader(http.StatusNotFound)
		case errors.Is(err, db.ErrOrgSuperadminRequired):
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("use the superadmin transfer to change who the superadmin is"))
		default:
			a.logger.Logf("error setting role for %s in org %d: %s", target, org.Id, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

// AdminOrganizationMemberAccess tells the panel whether to offer the controls.
//
// A separate endpoint rather than a field on the organization list, because
// that list returns a bare array and adding a wrapper to it would break every
// existing reader for one boolean.
func (a *AppService) AdminOrganizationMemberAccess(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	actor := *userDid
	ctx := r.Context()

	allowed := a.IsAdmin(ctx, actor)
	if !allowed {
		org, role, err := a.db.GetOrganizationByUser(ctx, actor)
		if err != nil {
			a.logger.Logf("error loading organization for %s: %s", actor, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if org != nil && org.IsHouse {
			if role == structs.OrgRoleSuperadmin {
				allowed = true
			} else if granted, err := a.db.HasOrgCapability(ctx, actor, structs.CapAdminOrgMembers); err == nil {
				allowed = granted
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"can_manage_members": allowed})
}
