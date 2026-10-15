package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

// Fine-grained organization permissions: the resolver the route guards call,
// and the dashboard endpoints that configure them.
//
// Nothing here narrows anyone's access on the day it ships. Every member is
// "inherit" until an org admin says otherwise, and an inheriting member
// resolves true for every capability of every role their organization holds —
// which is exactly what membership meant before.

// ── Resolution ───────────────────────────────────────────────────────────────

// HasOrgCapability reports whether a user may do one specific thing.
//
// Global admins pass everything, as they always have: this is the same bypass
// every with* guard already had, kept in one place so adding a capability
// cannot accidentally lock an admin out.
func (a *AppService) HasOrgCapability(ctx context.Context, userId string, capability string) bool {
	if a.IsAdmin(ctx, userId) {
		return true
	}
	ok, err := a.db.HasOrgCapability(ctx, userId, capability)
	if err != nil {
		a.logger.Logf("error resolving capability %s for %s: %s", capability, userId, err)
		return false
	}
	return ok
}

// ── The dashboard payload ────────────────────────────────────────────────────

type organizationPermissionsResponse struct {
	// Catalog is only the groups this organization can actually delegate.
	Catalog []structs.CapabilityGroup `json:"catalog"`
	// Administrative is present only for the house organization, and only for
	// its superadmin.
	Administrative *structs.CapabilityGroup `json:"administrative,omitempty"`
	// Members carries each member's mode and effective grants.
	Members []structs.OrganizationMemberPermissions `json:"members"`
	// CanEdit is false for ordinary members, who may read this to see their
	// own access but may not change anyone's.
	CanEdit bool `json:"can_edit"`
	// CanEditAdministrative is true only for the house organization's superadmin.
	CanEditAdministrative bool `json:"can_edit_administrative"`
}

// GetOrganizationPermissions returns the catalog and every member's effective
// permissions, for the organization dashboard.
func (a *AppService) GetOrganizationPermissions(w http.ResponseWriter, r *http.Request) {
	org, role, _, ok := a.requireOrgRole(w, r, structs.OrgRoleMember)
	if !ok {
		return
	}

	approved, err := a.db.OrganizationApprovedRoleTypes(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("error loading approved roles for org %d: %s", org.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Only offer what the organization holds: a group for a role it was never
	// approved for would be configurable and then resolve false anyway.
	catalog := []structs.CapabilityGroup{}
	for _, group := range structs.OrgCapabilityCatalog() {
		if slices.Contains(approved, group.RoleType) {
			catalog = append(catalog, group)
		}
	}

	members, err := a.db.GetOrganizationMembersWithPermissions(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("error loading members for org %d: %s", org.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	grants, err := a.db.GetOrganizationGrants(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("error loading grants for org %d: %s", org.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	isHouse := org.IsHouse
	canEditAdministrative := isHouse && role == structs.OrgRoleSuperadmin

	out := organizationPermissionsResponse{
		Catalog:               catalog,
		Members:               make([]structs.OrganizationMemberPermissions, 0, len(members)),
		CanEdit:               role == structs.OrgRoleAdmin || role == structs.OrgRoleSuperadmin,
		CanEditAdministrative: canEditAdministrative,
	}
	if isHouse {
		group := structs.AdministrativeCapabilityGroup()
		out.Administrative = &group
	}

	for _, member := range members {
		entry := structs.OrganizationMemberPermissions{
			UserId: member.UserId,
			Mode:   member.PermissionsMode,
		}
		held := grants[member.UserId]

		// Org admins are unrestricted whatever the table says, and an
		// inheriting member holds everything the organization holds. Showing
		// the resolved set rather than the stored rows means the dashboard
		// never claims someone has less than they really do.
		unrestricted := member.Role == structs.OrgRoleAdmin ||
			member.Role == structs.OrgRoleSuperadmin ||
			member.PermissionsMode != structs.OrgPermissionsCustom
		if unrestricted {
			for _, group := range catalog {
				entry.Granted = append(entry.Granted, structs.CapabilitiesForRole(group.RoleType, false)...)
			}
		} else {
			for _, capability := range held {
				if roleType, known := structs.CapabilityRoleType(capability); known &&
					roleType != structs.OrgRoleTypeAdministrative {
					entry.Granted = append(entry.Granted, capability)
				}
			}
		}
		if entry.Granted == nil {
			entry.Granted = []string{}
		}

		// Administrative is never inherited and is only disclosed inside the
		// house organization.
		if isHouse {
			entry.Administrative = []string{}
			for _, capability := range held {
				if roleType, known := structs.CapabilityRoleType(capability); known &&
					roleType == structs.OrgRoleTypeAdministrative {
					entry.Administrative = append(entry.Administrative, capability)
				}
			}
		}
		out.Members = append(out.Members, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		a.logger.Logf("error encoding organization permissions: %s", err)
	}
}

// ── Writing ──────────────────────────────────────────────────────────────────

type setMemberPermissionsRequest struct {
	UserId string `json:"user_id"`
	// Mode is "inherit" or "custom". Empty leaves it alone.
	Mode string `json:"mode"`
	// Granted is the complete set for the platform roles, replacing what is there.
	Granted []string `json:"granted"`
}

// SetOrganizationMemberPermissions configures one member's platform-role
// permissions. Org admins and superadmins may call it.
func (a *AppService) SetOrganizationMemberPermissions(w http.ResponseWriter, r *http.Request) {
	org, _, actor, ok := a.requireOrgRole(w, r, structs.OrgRoleAdmin)
	if !ok {
		return
	}
	var req setMemberPermissionsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.UserId = strings.TrimSpace(req.UserId)
	if req.UserId == "" {
		http.Error(w, "a user is required", http.StatusBadRequest)
		return
	}

	// An admin's own access is not theirs to restrict, and a superadmin's is
	// not an admin's to touch: otherwise an admin could lock the organization
	// out of its own settings, or demote the person above them.
	targetRole, err := a.db.GetOrganizationMemberRole(r.Context(), org.Id, req.UserId)
	if err != nil {
		a.logger.Logf("error loading member role: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if targetRole == structs.OrgRoleAdmin || targetRole == structs.OrgRoleSuperadmin {
		http.Error(w, "organization admins always have full access; change their role first", http.StatusBadRequest)
		return
	}

	// Only the platform roles the organization actually holds, so this call can
	// never write an administrative grant.
	approved, err := a.db.OrganizationApprovedRoleTypes(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("error loading approved roles: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for _, capability := range req.Granted {
		roleType, known := structs.CapabilityRoleType(capability)
		if !known || roleType == structs.OrgRoleTypeAdministrative || !slices.Contains(approved, roleType) {
			http.Error(w, "unknown or unavailable capability: "+capability, http.StatusBadRequest)
			return
		}
	}

	if err := a.db.SetMemberPermissions(
		r.Context(), org.Id, req.UserId, req.Mode, approved, req.Granted, actor,
	); err != nil {
		a.logger.Logf("error setting member permissions: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type setAdministrativeRequest struct {
	UserId string `json:"user_id"`
	// Granted is the complete administrative set, replacing what is there.
	Granted []string `json:"granted"`
}

// SetOrganizationMemberAdministrative configures the SFLuv-only administrative
// grants. Only the house organization's superadmin may call it.
//
// The website capabilities are mirrored onto the credentials that actually gate
// site editing, so this is a nicer place to set them from without changing how
// they are enforced.
func (a *AppService) SetOrganizationMemberAdministrative(w http.ResponseWriter, r *http.Request) {
	org, _, actor, ok := a.requireOrgRole(w, r, structs.OrgRoleSuperadmin)
	if !ok {
		return
	}
	if !org.IsHouse {
		http.Error(w, "administrative permissions exist only for the SFLuv organization", http.StatusForbidden)
		return
	}
	var req setAdministrativeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.UserId = strings.TrimSpace(req.UserId)
	if req.UserId == "" {
		http.Error(w, "a user is required", http.StatusBadRequest)
		return
	}
	for _, capability := range req.Granted {
		if roleType, known := structs.CapabilityRoleType(capability); !known ||
			roleType != structs.OrgRoleTypeAdministrative {
			http.Error(w, "not an administrative capability: "+capability, http.StatusBadRequest)
			return
		}
	}

	if err := a.db.SetMemberPermissions(
		r.Context(), org.Id, req.UserId, "",
		[]string{structs.OrgRoleTypeAdministrative}, req.Granted, actor,
	); err != nil {
		a.logger.Logf("error setting administrative permissions: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Mirror the website capabilities onto the credentials that enforce them.
	// Failing here does not fail the request: the grants above are saved, and
	// the credential is recoverable from them, so reporting a 500 would be
	// misleading about what happened.
	a.syncSiteCredentials(r.Context(), req.UserId, req.Granted, actor)

	w.WriteHeader(http.StatusOK)
}

// syncSiteCredentials brings a user's website credentials in line with their
// administrative grants, granting what was ticked and revoking what was not.
func (a *AppService) syncSiteCredentials(ctx context.Context, userId string, granted []string, actor string) {
	for capability, credential := range structs.AdminSiteCapabilityCredentials() {
		want := slices.Contains(granted, capability)
		if err := a.db.SetUserSiteCredential(ctx, userId, credential, want, actor); err != nil {
			a.logger.Logf("error syncing credential %s for %s: %s", credential, userId, err)
		}
	}
}
