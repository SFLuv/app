package structs

import "time"

// Organization member roles.
const (
	OrgRoleMember     = "member"
	OrgRoleAdmin      = "admin"
	OrgRoleSuperadmin = "superadmin"
)

// Organization-level platform roles (what the org itself is approved for).
const (
	OrgRoleTypeAffiliate  = "affiliate"
	OrgRoleTypeProposer   = "proposer"
	OrgRoleTypeSupervisor = "supervisor"
	OrgRoleTypeIssuer     = "issuer"
)

// Allocation cycles.
const (
	// Allocation cycles. These no longer describe a standing organization
	// allowance — that model is gone — and survive because event_allocations
	// records which cycle an approved event's reward draws from.
	AllocationCycleDaily   = "daily"
	AllocationCycleWeekly  = "weekly"
	AllocationCycleMonthly = "monthly"
	AllocationCycleOneTime = "one_time"
)

type Organization struct {
	Id   int64   `json:"id"`
	Name string  `json:"name"`
	Logo *string `json:"logo,omitempty"`
	// IsHouse marks SFLuv's own organization rather than a partner. It gates
	// the administrative permission section, and replaces matching on the
	// normalized name, which would have let a rename move admin powers.
	IsHouse   bool      `json:"is_house"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OrganizationMember struct {
	OrganizationId int64  `json:"organization_id"`
	UserId         string `json:"user_id"`
	Role           string `json:"role"` // member | admin | superadmin
	// PermissionsMode is inherit | custom. "inherit" means the member holds
	// every capability of every role the organization is approved for, which
	// is what membership meant before fine-grained permissions existed.
	PermissionsMode string    `json:"permissions_mode"`
	ContactEmail    string    `json:"contact_email,omitempty"`
	ContactName     string    `json:"contact_name,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type OrganizationRole struct {
	OrganizationId int64  `json:"organization_id"`
	RoleType       string `json:"role_type"` // affiliate | proposer | supervisor
	Status         string `json:"status"`    // pending | approved | rejected
	// Role-specific extras collected at request time.
	Email                 string     `json:"email,omitempty"`
	PrimaryRewardsAccount string     `json:"primary_rewards_account,omitempty"`
	RequestedBy           string     `json:"requested_by,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	ApprovedAt            *time.Time `json:"approved_at,omitempty"`
}

type OrganizationInvite struct {
	Id             int64  `json:"id"`
	OrganizationId int64  `json:"organization_id"`
	Email          string `json:"email"`
	// Role the invitee will hold on accepting, so a pending row in the member
	// list can say what it is pending as rather than just "pending".
	Role       string     `json:"role"`
	InvitedBy  string     `json:"invited_by"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// OrganizationView is the full member-facing settings payload.
type OrganizationView struct {
	Organization Organization         `json:"organization"`
	MyRole       string               `json:"my_role"`
	Members      []OrganizationMember `json:"members"`
	Roles        []OrganizationRole   `json:"roles"`
	IssuerScopes []string             `json:"issuer_scopes"`
	Invites      []OrganizationInvite `json:"invites,omitempty"` // admins only
}

type OrganizationInviteRequest struct {
	Email string `json:"email"`
	// Role is "member" or "admin". Absent means member.
	Role string `json:"role,omitempty"`
	// PermissionsMode is "inherit" or "custom".
	//
	// Absent means custom, which is the opposite of what membership used to
	// mean and is the point: a new member starts with nothing and is given what
	// they need, rather than starting with everything the organization holds
	// and having it taken away. A caller that wants the old behaviour has to
	// ask for inherit by name.
	PermissionsMode string `json:"permissions_mode,omitempty"`
	// Capabilities are the keys to grant on acceptance, for a custom member.
	// Ignored for an admin, who is not limited by grants.
	Capabilities []string `json:"capabilities,omitempty"`
}

type OrganizationRoleRequest struct {
	RoleType              string `json:"role_type"`
	Email                 string `json:"email,omitempty"`
	PrimaryRewardsAccount string `json:"primary_rewards_account,omitempty"`
}

type OrganizationNameUpdateRequest struct {
	Name string `json:"name"`
}

type OrganizationLogoUpdateRequest struct {
	Logo string `json:"logo"`
}

type OrganizationMemberRoleUpdateRequest struct {
	UserId string `json:"user_id"`
	Role   string `json:"role"` // member | admin
}

type OrganizationTransferSuperadminRequest struct {
	UserId string `json:"user_id"`
}

type OrganizationJoinRequest struct {
	Token string `json:"token"`
}

type AdminOrganizationSuperadminRequest struct {
	OrganizationId int64  `json:"organization_id"`
	Email          string `json:"email"`
}

// AdminOrganizationIssuerScopesRequest replaces an organization's issuance
// settings: the set of credential types its members may issue while the org's
// issuer role is approved.
type AdminOrganizationIssuerScopesRequest struct {
	OrganizationId  int64    `json:"organization_id"`
	CredentialTypes []string `json:"credential_types"`
}
