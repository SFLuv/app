package structs

// Fine-grained organization permissions.
//
// Until this existed, membership was all-or-nothing: every member of an
// organization inherited every platform role the organization had been
// approved for, in full. A volunteer coordinator who only needed to read the
// workflow list held the same power as the person who runs the organization.
//
// The model here narrows that without changing anything that works today:
//
//   - A member whose permissions_mode is "inherit" — the default, and what
//     every member carries after the migration — gets exactly what they got
//     before: the full set of every approved org role. No grants are read.
//   - A member set to "custom" gets only the capabilities granted to them.
//   - Organization admins and superadmins are always unrestricted, whatever
//     their grants say, because they are the people who hand the grants out.
//
// The catalog below is code, not data, deliberately: a capability is only
// meaningful if a route checks it, so adding one is a code change either way.
// Adding to it needs no migration — the grant table stores the key as text.

// Permission modes on organization_members.
const (
	// OrgPermissionsInherit is the default: the member has every capability of
	// every role the organization holds. This is the pre-existing behaviour.
	OrgPermissionsInherit = "inherit"
	// OrgPermissionsCustom means the member's grants are authoritative.
	OrgPermissionsCustom = "custom"
)

// OrgRoleTypeAdministrative is not an organization_roles role_type; it is the
// SFLuv-only administrative section, configurable only by SFLuv's superadmin.
// It is stored in the same grants table under this role_type.
const OrgRoleTypeAdministrative = "administrative"

// ── Capability keys ──────────────────────────────────────────────────────────

// Supervisor.
const (
	// One capability over one set of rows. The export is the same workflows the
	// view already lists, in a file, so splitting them offered a distinction
	// that does not exist — withholding the download while allowing the page
	// protects nothing. Merged onto this key in migration 1.71.
	CapSupervisorWorkflowsView = "supervisor.workflows.view"
)

// Affiliate.
const (
	CapAffiliateEventsView    = "affiliate.events.view"
	CapAffiliateEventsManage  = "affiliate.events.manage"
	CapAffiliateCodesDownload = "affiliate.codes.download"
	CapAffiliateVolunteersMsg = "affiliate.volunteers.message"
	CapAffiliateProfileManage = "affiliate.profile.manage"
)

// Proposer.
const (
	CapProposerWorkflowsView    = "proposer.workflows.view"
	CapProposerWorkflowsCreate  = "proposer.workflows.create"
	CapProposerWorkflowsPropose = "proposer.workflows.propose"
	CapProposerTemplatesManage  = "proposer.templates.manage"
)

// Issuer.
const (
	CapIssuerCredentialsView   = "issuer.credentials.view"
	CapIssuerCredentialsIssue  = "issuer.credentials.issue"
	CapIssuerCredentialsRevoke = "issuer.credentials.revoke"
)

// Administrative (SFLuv only).
const (
	CapAdminEventsApprove    = "admin.events.approve"
	CapAdminEventsAutoCreate = "admin.events.autocreate"
	CapAdminMerchantsApprove = "admin.merchants.approve"
	CapAdminSiteBanner       = "admin.site.banner"
	CapAdminSiteFinancials   = "admin.site.financials"
	CapAdminSiteForms        = "admin.site.forms"
	CapAdminSitePastEvents   = "admin.site.past_events"
	// Protocol signing keys. Split in two deliberately: seeing which Safe pays
	// for what is useful to anyone operating the platform, while minting a key
	// and handing a Safe over to it is the most consequential button in the
	// admin panel.
	CapAdminOrgMembers     = "admin.orgs.members"
	CapAdminProtocolView   = "admin.protocol.view"
	CapAdminProtocolRotate = "admin.protocol.rotate"
)

// ── The catalog ──────────────────────────────────────────────────────────────

// Capability is one togglable permission within a role.
type Capability struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Hint is the one-line explanation shown under the toggle.
	Hint string `json:"hint"`
	// Read marks a capability that only reads. The "Read-only" preset is
	// exactly the capabilities with this set, so the preset cannot drift from
	// what the capabilities actually do.
	Read bool `json:"read"`
}

// CapabilityGroup is a role's capabilities, as the dashboard renders them.
type CapabilityGroup struct {
	RoleType string       `json:"role_type"`
	Label    string       `json:"label"`
	Caps     []Capability `json:"capabilities"`
}

// OrgCapabilityCatalog is every capability an organization can delegate,
// grouped by the platform role that carries it. The dashboard shows a group
// only when the organization actually holds that role.
func OrgCapabilityCatalog() []CapabilityGroup {
	return []CapabilityGroup{
		{
			RoleType: OrgRoleTypeSupervisor,
			Label:    "Supervisor",
			Caps: []Capability{
				{Key: CapSupervisorWorkflowsView, Label: "View and export workflow data", Hint: "See the workflows assigned to this organization, and download them.", Read: true},
			},
		},
		{
			RoleType: OrgRoleTypeAffiliate,
			Label:    "Affiliate",
			Caps: []Capability{
				{Key: CapAffiliateEventsView, Label: "View events", Hint: "See the organization's volunteer events and their status.", Read: true},
				{Key: CapAffiliateCodesDownload, Label: "Download QR codes", Hint: "Download the printable codes for approved events.", Read: true},
				{Key: CapAffiliateEventsManage, Label: "Request and edit events", Hint: "Submit event requests and change the ones already submitted."},
				{Key: CapAffiliateVolunteersMsg, Label: "Message volunteers", Hint: "Send event blasts to the volunteer list."},
				{Key: CapAffiliateProfileManage, Label: "Edit the organization's logo", Hint: "Change the logo shown on cards and the public site."},
			},
		},
		{
			RoleType: OrgRoleTypeProposer,
			Label:    "Proposer",
			Caps: []Capability{
				{Key: CapProposerWorkflowsView, Label: "View workflows", Hint: "See the organization's workflows and proposals.", Read: true},
				{Key: CapProposerWorkflowsCreate, Label: "Create workflows", Hint: "Create new workflows from scratch or a template."},
				{Key: CapProposerWorkflowsPropose, Label: "Propose edits and deletions", Hint: "Raise edit and deletion proposals on existing workflows."},
				{Key: CapProposerTemplatesManage, Label: "Manage templates", Hint: "Create and delete the organization's workflow templates."},
			},
		},
		{
			RoleType: OrgRoleTypeIssuer,
			Label:    "Issuer",
			Caps: []Capability{
				{Key: CapIssuerCredentialsView, Label: "View credentials and requests", Hint: "See issuing scopes, pending requests and a user's credentials.", Read: true},
				{Key: CapIssuerCredentialsIssue, Label: "Grant credentials", Hint: "Issue credentials and decide pending requests."},
				{Key: CapIssuerCredentialsRevoke, Label: "Revoke credentials", Hint: "Take a credential back."},
			},
		},
	}
}

// AdministrativeCapabilityGroup is the SFLuv-only section. It is kept out of
// OrgCapabilityCatalog so it can never be rendered or granted for another
// organization by accident.
func AdministrativeCapabilityGroup() CapabilityGroup {
	return CapabilityGroup{
		RoleType: OrgRoleTypeAdministrative,
		Label:    "Administrative",
		Caps: []Capability{
			{Key: CapAdminEventsApprove, Label: "Approve volunteer events", Hint: "Review affiliate event requests; approving one reserves its rewards and mints its QR codes."},
			{Key: CapAdminEventsAutoCreate, Label: "Auto-create events", Hint: "Create events directly, without a request to approve."},
			{Key: CapAdminMerchantsApprove, Label: "Approve merchant status", Hint: "Approve merchants and their locations."},
			{Key: CapAdminSiteBanner, Label: "Edit the homepage banner", Hint: "The Spotlight carousel on sfluv.org."},
			{Key: CapAdminSiteFinancials, Label: "Edit financial documents", Hint: "Statements and impact reports on sfluv.org."},
			{Key: CapAdminSiteForms, Label: "Edit forms and waivers", Hint: "Authoring, opening and closing public forms."},
			{Key: CapAdminSitePastEvents, Label: "Edit past events", Hint: "The volunteer archive and its photo galleries."},
			{Key: CapAdminOrgMembers, Label: "Manage other organizations' members", Hint: "Invite, remove and re-role members of any partner organization, which that organization's own admins would otherwise be the only ones able to do."},
			{Key: CapAdminProtocolView, Label: "View protocol wallets", Hint: "The Safes that pay rewards and grant redeemer access, and the signing key behind each.", Read: true},
			{Key: CapAdminProtocolRotate, Label: "Rotate protocol signing keys", Hint: "Mint a new key for a protocol Safe and remove the old one. The Safe's address never changes."},
		},
	}
}

// AdminSiteCapabilityCredentials maps the administrative website capabilities
// onto the credentials that actually gate site editing.
//
// Site editing is enforced by credentials (SiteCapabilitiesFor), and that path
// is left exactly as it is. The administrative section is a nicer place to set
// them from: granting here grants the credential, revoking here revokes it. A
// credential granted the old way, through the issuer panel, keeps working and
// shows up here as granted.
func AdminSiteCapabilityCredentials() map[string]string {
	return map[string]string{
		CapAdminSiteBanner:     SiteCapabilityBanner,
		CapAdminSiteFinancials: SiteCapabilityFinancials,
		CapAdminSiteForms:      SiteCapabilityForms,
		CapAdminSitePastEvents: SiteCapabilityPastEvents,
	}
}

// ── Lookups ──────────────────────────────────────────────────────────────────

// CapabilityRoleType returns the role a capability belongs to, and whether the
// key is one the catalog knows at all. Unknown keys are rejected on write so a
// typo cannot sit in the table looking like a grant.
func CapabilityRoleType(key string) (string, bool) {
	for _, group := range append(OrgCapabilityCatalog(), AdministrativeCapabilityGroup()) {
		for _, capability := range group.Caps {
			if capability.Key == key {
				return group.RoleType, true
			}
		}
	}
	return "", false
}

// CapabilitiesForRole returns a role's capability keys, optionally only the
// read-only ones. Used for the presets and for resolving an inheriting member.
func CapabilitiesForRole(roleType string, readOnly bool) []string {
	groups := append(OrgCapabilityCatalog(), AdministrativeCapabilityGroup())
	keys := []string{}
	for _, group := range groups {
		if group.RoleType != roleType {
			continue
		}
		for _, capability := range group.Caps {
			if readOnly && !capability.Read {
				continue
			}
			keys = append(keys, capability.Key)
		}
	}
	return keys
}

// OrganizationMemberPermissions is a member's configuration as the dashboard
// reads and writes it.
type OrganizationMemberPermissions struct {
	UserId string `json:"user_id"`
	// Mode is "inherit" or "custom".
	Mode string `json:"mode"`
	// Granted is every capability key the member holds. For an inheriting
	// member this is filled in with everything their organization's roles
	// carry, so the dashboard can show what they actually have today.
	Granted []string `json:"granted"`
	// Administrative is the SFLuv-only grant set, kept separate so a normal
	// organization's payload never carries it.
	Administrative []string `json:"administrative,omitempty"`
}
