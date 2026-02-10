export interface Affiliate {
  user_id: string
  organization: string
  nickname?: string | null
  status: "pending" | "approved" | "rejected"
  affiliate_logo?: string | null
  weekly_allocation?: number
  weekly_balance: number
  one_time_balance: number
}

export interface AffiliateBalance {
  available: number
  weekly_allocation: number
  weekly_balance: number
  one_time_balance: number
  reserved: number
}
