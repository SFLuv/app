export interface AdminAnalyticsSummary {
  total_active_users: number
  daily_active_users: number
  monthly_active_users: number
  previous_monthly_active_users: number
  monthly_active_user_change_percent: number
  monthly_transaction_volume_wei: string
  total_distributed_wei: string
  distributed_spent_at_merchants_wei: string
  token_redeemed_wei: string
  token_unused_wei: string
  token_redeemed_percent: number
  average_community_project_cost_wei: string
  merchant_customer_wallets: number
  merchant_repeat_customer_wallets: number
  merchant_repeat_customer_percent: number
  average_seconds_to_use_distribution: number
  average_volunteer_events_per_week: number
  average_volunteer_events_per_month: number
  average_volunteer_events_per_year: number
  average_unique_earners_per_week: number
  average_unique_earners_per_month: number
  average_unique_earners_per_year: number
  volunteer_participation_count: number
  volunteer_unique_earners: number
  volunteer_repeat_earners: number
  volunteer_repeat_participation_rate: number
  volunteer_events: number
  event_code_redemption_percent: number
}

export interface AdminAnalyticsMonthlyPoint {
  key: string
  label: string
  active_users: number
  transaction_volume_wei: string
  distributed_wei: string
  merchant_spend_wei: string
  merchant_customer_wallets: number
  merchant_repeat_customer_wallets: number
  volunteer_events: number
  unique_earners: number
  average_project_cost_wei: string
}

export interface AdminAnalyticsDailyPoint {
  key: string
  label: string
  active_users: number
}

export interface AdminAnalyticsDefinition {
  key: string
  label: string
  definition: string
}

export interface AdminAnalyticsResponse {
  generated_at: number
  configured_paid_addresses: string[]
  summary: AdminAnalyticsSummary
  monthly: AdminAnalyticsMonthlyPoint[]
  daily: AdminAnalyticsDailyPoint[]
  metric_definitions: AdminAnalyticsDefinition[]
}
