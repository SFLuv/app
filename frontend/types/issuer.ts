import { CredentialType } from "@/types/workflow"

export interface IssuerRecord {
  user_id: string
  organization: string
  email: string
  nickname?: string | null
  status: string
  created_at: string
  updated_at: string
}

export interface IssuerWithScopes {
  user_id: string
  is_issuer: boolean
  allowed_credentials: CredentialType[]
  organization: string
  nickname?: string | null
}

export interface CredentialIssueRequest {
  user_id: string
  credential_type: CredentialType
}

export interface CredentialRequest {
  id: string
  user_id: string
  credential_type: CredentialType
  status: "pending" | "approved" | "rejected"
  requested_at: string
  resolved_at?: string | null
  resolved_by?: string | null
  created_at: string
  updated_at: string
  requester_name: string
  requester_first_name: string
  requester_last_name: string
  requester_email: string
}

export interface CredentialRequestCreateRequest {
  credential_type: CredentialType
}

export interface CredentialRequestDecisionRequest {
  decision?: "approve" | "reject" | "pending"
  status?: "pending" | "approved" | "rejected"
}

export interface UserCredential {
  id: number
  user_id: string
  credential_type: CredentialType
  issued_by?: string | null
  issued_at: string
  is_revoked: boolean
  revoked_at?: string | null
}
