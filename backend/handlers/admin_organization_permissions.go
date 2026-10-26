package handlers

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

// Configuring a partner organization's member permissions.
//
// The own-organization pair is /organizations/mine/permissions, resolved from
// the caller's own membership, so until now a partner's fine-grained
// permissions could only be set by that partner's own admins. The house
// organization could change somebody's role — member to admin — but not narrow
// what a member may do, which is the half that actually needs doing when a
// partner asks for help.
//
// Guarded by crossOrgMemberActor, the same check the member endpoints use: the
// house superadmin, or a house member granted "Manage other organizations'
// members".
//
// ── What is deliberately NOT exposed ─────────────────────────────────────────
//
// The Administrative group. It is SFLuv-only, it is how the house superadmin
// delegates things like protocol key rotation, and it is granted on a live
// house member by that superadmin alone. Offering it on a partner's member
// list would let somebody grant another organization's user powers over our
// own protocol accounts, which is a different decision wearing the same
// checkbox. So CanEditAdministrative is false here and the group is absent,
// whatever the target organization is.

// AdminGetOrganizationPermissions is the permissions view for one organization.
func (a *AppService) AdminGetOrganizationPermissions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.crossOrgMemberActor(w, r); !ok {
		return
	}
	org, ok := a.targetOrganization(w, r)
	if !ok {
		return
	}

	approved, err := a.db.OrganizationApprovedRoleTypes(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("error loading approved roles for org %d: %s", org.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

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

	out := organizationPermissionsResponse{
		Catalog: catalog,
		Members: make([]structs.OrganizationMemberPermissions, 0, len(members)),
		CanEdit: true,
		// See the note above: never from here.
		CanEditAdministrative: false,
	}

	for _, member := range members {
		entry := structs.OrganizationMemberPermissions{
			UserId: member.UserId,
			Mode:   member.PermissionsMode,
		}
		// Same rule the own-org view uses: show what the person actually
		// resolves to, so the screen never claims somebody has less than they
		// do. An admin is unrestricted and an inheriting member holds
		// everything the organization holds.
		unrestricted := member.Role == structs.OrgRoleAdmin ||
			member.Role == structs.OrgRoleSuperadmin ||
			member.PermissionsMode != structs.OrgPermissionsCustom
		if unrestricted {
			for _, group := range catalog {
				entry.Granted = append(entry.Granted, structs.CapabilitiesForRole(group.RoleType, false)...)
			}
		} else {
			for _, capability := range grants[member.UserId] {
				if roleType, known := structs.CapabilityRoleType(capability); known &&
					roleType != structs.OrgRoleTypeAdministrative {
					entry.Granted = append(entry.Granted, capability)
				}
			}
		}
		if entry.Granted == nil {
			entry.Granted = []string{}
		}
		out.Members = append(out.Members, entry)
	}

	writeJSON(w, http.StatusOK, out)
}

// AdminSetOrganizationMemberPermissions configures one member of a partner
// organization.
func (a *AppService) AdminSetOrganizationMemberPermissions(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.crossOrgMemberActor(w, r)
	if !ok {
		return
	}
	org, ok := a.targetOrganization(w, r)
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

	// Kept from the own-org path rather than relaxed for an admin: an org admin
	// is unrestricted by definition, so writing a narrow grant set for one
	// would record a restriction that is not in force and read as though it
	// were. Change the role first, which this panel can also do.
	targetRole, err := a.db.GetOrganizationMemberRole(r.Context(), org.Id, req.UserId)
	if err != nil {
		a.logger.Logf("error loading member role in org %d: %s", org.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if targetRole == "" {
		http.Error(w, "that user is not a member of this organization", http.StatusNotFound)
		return
	}
	if targetRole == structs.OrgRoleAdmin || targetRole == structs.OrgRoleSuperadmin {
		http.Error(w, "organization admins always have full access; change their role first", http.StatusBadRequest)
		return
	}

	approved, err := a.db.OrganizationApprovedRoleTypes(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("error loading approved roles for org %d: %s", org.Id, err)
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
		a.logger.Logf("error setting member permissions in org %d: %s", org.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
