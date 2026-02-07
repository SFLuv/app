export interface W9Submission {
  id: number
  wallet_address: string
  year: number
  email: string
  submitted_at: string
  pending_approval: boolean
  approved_at?: string | null
  approved_by_user_id?: string | null
  rejected_at?: string | null
  rejected_by_user_id?: string | null
  rejection_reason?: string | null
  w9_url?: string | null
  user_contact_email?: string | null
}
