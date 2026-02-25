export type SupervisorStatus = "pending" | "approved" | "rejected"

export interface Supervisor {
  user_id: string
  organization: string
  email: string
  nickname?: string | null
  status: SupervisorStatus
  created_at: string
  updated_at: string
}

