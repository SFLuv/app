export type OrgMemberRole = "member" | "admin" | "superadmin"
export type OrgRoleType = "affiliate" | "proposer" | "supervisor"
export type AllocationCycle = "daily" | "weekly" | "monthly" | "one_time"

export interface Organization {
  id: number
  name: string
  logo?: string | null
  created_at: string
  updated_at: string
}

export interface OrganizationMember {
  organization_id: number
  user_id: string
  role: OrgMemberRole
  contact_email?: string
  contact_name?: string
  created_at: string
}

export interface OrganizationRole {
  organization_id: number
  role_type: OrgRoleType
  status: "pending" | "approved" | "rejected"
  email?: string
  primary_rewards_account?: string
  requested_by?: string
  created_at: string
  approved_at?: string | null
}

export interface OrganizationAllocation {
  id: number
  organization_id: number
  cycle: AllocationCycle
  allocation: number
  balance: number
  last_reset_at?: string | null
}

export interface OrganizationInvite {
  id: number
  organization_id: number
  email: string
  invited_by: string
  expires_at: string
  accepted_at?: string | null
  created_at: string
}

export interface OrganizationView {
  organization: Organization
  my_role: OrgMemberRole
  members: OrganizationMember[]
  roles: OrganizationRole[]
  allocations: OrganizationAllocation[]
  invites?: OrganizationInvite[]
}
