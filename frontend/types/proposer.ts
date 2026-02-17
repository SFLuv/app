export type ProposerStatus = "pending" | "approved" | "rejected"

export interface Proposer {
  user_id: string
  organization: string
  nickname?: string | null
  status: ProposerStatus
  weekly_allocation: number
  weekly_balance: number
  one_time_balance: number
  created_at: string
  updated_at: string
}

export interface ProposerBalance {
  available: number
  weekly_allocation: number
  weekly_balance: number
  one_time_balance: number
  reserved: number
}
