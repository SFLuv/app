export type ProposerStatus = "pending" | "approved" | "rejected"

export interface Proposer {
  user_id: string
  organization: string
  email: string
  nickname?: string | null
  status: ProposerStatus
  created_at: string
  updated_at: string
}
