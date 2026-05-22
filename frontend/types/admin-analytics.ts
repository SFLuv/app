export interface AdminAnalyticsSummary {
  current_circulating_sfluv_wei: string
}

export interface AdminAnalyticsMetricValue {
  metric_key: string
  label: string
  kind: "wei" | "count" | "seconds" | "percent" | "decimal"
  wei?: string
  count?: number
  seconds?: number
  percent?: number
  decimal?: number
}

export interface AdminAnalyticsPeriod {
  key: string
  label: string
  unit: string
  start_at: number
  end_at: number
  metrics: AdminAnalyticsMetricValue[]
}

export interface AdminAnalyticsTrendPoint {
  key: string
  label: string
  start_at: number
  end_at: number
  metrics: AdminAnalyticsMetricValue[]
}

export interface AdminAnalyticsDefinition {
  key: string
  label: string
  definition: string
}

export interface AdminAnalyticsResponse {
  generated_at: number
  chain_id: number
  summary: AdminAnalyticsSummary
  periods: AdminAnalyticsPeriod[]
  monthly_trend: AdminAnalyticsTrendPoint[]
  metric_definitions: AdminAnalyticsDefinition[]
  glossary: AdminAnalyticsDefinition[]
}
