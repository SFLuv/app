import { CredentialType } from "@/types/workflow"

export interface IssuerWithScopes {
  user_id: string
  is_issuer: boolean
  allowed_credentials: CredentialType[]
}

export interface CredentialIssueRequest {
  user_id: string
  credential_type: CredentialType
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
