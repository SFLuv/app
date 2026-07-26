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
	AllocationCycleDaily   = "daily"
	AllocationCycleWeekly  = "weekly"
	AllocationCycleMonthly = "monthly"
	AllocationCycleOneTime = "one_time"
)

type Organization struct {
	Id        int64     `json:"id"`
	Name      string    `json:"name"`
	Logo      *string   `json:"logo,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OrganizationMember struct {
	OrganizationId int64     `json:"organization_id"`
	UserId         string    `json:"user_id"`
	Role           string    `json:"role"` // member | admin | superadmin
	ContactEmail   string    `json:"contact_email,omitempty"`
	ContactName    string    `json:"contact_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type OrganizationRole struct {
	OrganizationId int64      `json:"organization_id"`
	RoleType       string     `json:"role_type"` // affiliate | proposer | supervisor
	Status         string     `json:"status"`    // pending | approved | rejected
	// Role-specific extras collected at request time.
	Email                 string     `json:"email,omitempty"`
	PrimaryRewardsAccount string     `json:"primary_rewards_account,omitempty"`
	RequestedBy           string     `json:"requested_by,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	ApprovedAt            *time.Time `json:"approved_at,omitempty"`
}

type OrganizationAllocation struct {
	Id             int64      `json:"id"`
	OrganizationId int64      `json:"organization_id"`
	Cycle          string     `json:"cycle"` // daily | weekly | monthly | one_time
	Allocation     uint64     `json:"allocation"`
	Balance        uint64     `json:"balance"`
	LastResetAt    *time.Time `json:"last_reset_at,omitempty"`
}

type OrganizationInvite struct {
	Id             int64      `json:"id"`
	OrganizationId int64      `json:"organization_id"`
	Email          string     `json:"email"`
	InvitedBy      string     `json:"invited_by"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// OrganizationView is the full member-facing settings payload.
type OrganizationView struct {
	Organization Organization             `json:"organization"`
	MyRole       string                   `json:"my_role"`
	Members      []OrganizationMember     `json:"members"`
	Roles        []OrganizationRole       `json:"roles"`
	Allocations  []OrganizationAllocation `json:"allocations"`
	Invites      []OrganizationInvite     `json:"invites,omitempty"` // admins only
}

type OrganizationInviteRequest struct {
	Email string `json:"email"`
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

type AdminOrganizationAllocationItem struct {
	Cycle      string `json:"cycle"`
	Allocation uint64 `json:"allocation"`
}

// AdminOrganizationAllocationsRequest replaces an organization's full
// allocation list: cycles present are upserted, cycles absent are removed.
type AdminOrganizationAllocationsRequest struct {
	OrganizationId int64                             `json:"organization_id"`
	Allocations    []AdminOrganizationAllocationItem `json:"allocations"`
}
