export type OrgMemberRole = "member" | "admin" | "superadmin"
export type OrgRoleType = "affiliate" | "proposer" | "supervisor" | "issuer"

export interface Organization {
  id: number
  name: string
  logo?: string | null
  /** SFLuv's own organization rather than a partner. Gates the administrative section. */
  is_house: boolean
  created_at: string
  updated_at: string
}

export interface OrganizationMember {
  organization_id: number
  user_id: string
  role: OrgMemberRole
  /**
   * "inherit" means the member holds everything the organization's roles carry,
   * which is what membership meant before fine-grained permissions existed.
   */
  permissions_mode: "inherit" | "custom"
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
  issuer_scopes?: string[]
  invites?: OrganizationInvite[]
}

// ── Fine-grained permissions ─────────────────────────────────────────────────

export interface Capability {
  key: string
  label: string
  hint: string
  /** Part of the Read-only preset. */
  read: boolean
}

export interface CapabilityGroup {
  role_type: OrgRoleType | "administrative"
  label: string
  capabilities: Capability[]
}

export interface MemberPermissions {
  user_id: string
  mode: "inherit" | "custom"
  /** The member's effective capabilities, already resolved by the server. */
  granted: string[]
  /** SFLuv only; absent for partner organizations. */
  administrative?: string[]
}

export interface OrganizationPermissionsView {
  /** Only the roles this organization actually holds. */
  catalog: CapabilityGroup[]
  /** Present only inside SFLuv. */
  administrative?: CapabilityGroup
  members: MemberPermissions[]
  can_edit: boolean
  can_edit_administrative: boolean
}
