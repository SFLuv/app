export type TransactionType =  "currency_unwrap" | "currency_transfer" | "volunteer_reward"

export type TransactionStatus = "completed" | "pending" | "failed"

export interface Transaction {
  id: string
  type: TransactionType
  amount: number
  timestamp: string
  status: TransactionStatus
  fromName?: string
  fromAddress: string
  toName?: string
  toAddress: string
  description?: string
  transactionId: string
  category?: string
  metadata?: Record<string, any>
}

export const transactionTypeLabels: Record<TransactionType, string> = {
  currency_unwrap: "Currency Unwrap",
  currency_transfer: "Currency Transfer",
  volunteer_reward: "Volunteer Reward",
}

export interface TransactionAnalytics {
  totalIncome: number
  totalTransactions: number
  averageTransaction: number
  dailyData: AnalyticsDataPoint[]
  weeklyData: AnalyticsDataPoint[]
  monthlyData: AnalyticsDataPoint[]
  yearlyData: AnalyticsDataPoint[]
  categoryBreakdown: CategoryBreakdown[]
}

export interface AnalyticsDataPoint {
  date: string
  amount: number
  count: number
}

export interface CategoryBreakdown {
  category: string
  amount: number
  percentage: number
}
