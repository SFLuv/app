export type ImproverStatus = "pending" | "approved" | "rejected"

export interface Improver {
  user_id: string
  first_name: string
  last_name: string
  email: string
  primary_rewards_account: string
  status: ImproverStatus
  created_at: string
  updated_at: string
}
