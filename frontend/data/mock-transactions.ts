import type { Transaction, TransactionAnalytics } from "@/types/transaction"

// Helper function to generate a random blockchain address
const generateAddress = () => {
  return "0x" + Array.from({ length: 40 }, () => Math.floor(Math.random() * 16).toString(16)).join("")
}

// Helper function to generate a random transaction ID
const generateTransactionId = () => {
  return "0x" + Array.from({ length: 64 }, () => Math.floor(Math.random() * 16).toString(16)).join("")
}

// Generate mock transactions for a merchant
export const generateMerchantTransactions = (count: number): Transaction[] => {
  const transactions: Transaction[] = []
  const now = new Date()

  for (let i = 0; i < count; i++) {
    const type =
      Math.random() > 0.7 ? (Math.random() > 0.5 ? "currency_unwrap" : "currency_transfer") : "customer_purchase"

    const amount =
      type === "customer_purchase"
        ? Math.floor(Math.random() * 100) + 5
        : type === "currency_unwrap"
          ? -(Math.floor(Math.random() * 500) + 100)
          : (Math.random() > 0.5 ? 1 : -1) * (Math.floor(Math.random() * 200) + 20)

    const daysAgo = Math.floor(Math.random() * 90)
    const date = new Date(now)
    date.setDate(date.getDate() - daysAgo)

    const fromName = type === "customer_purchase" ? "Customer" : "Your Business"
    const toName =
      type === "customer_purchase" ? "Your Business" : type === "currency_unwrap" ? "Bank Account" : "Another Merchant"

    transactions.push({
      id: `tx-${i}`,
      type: type as any,
      amount,
      timestamp: date.toISOString(),
      status: Math.random() > 0.05 ? "completed" : Math.random() > 0.5 ? "pending" : "failed",
      fromName,
      fromAddress: generateAddress(),
      toName,
      toAddress: generateAddress(),
      description:
        type === "customer_purchase"
          ? `Purchase of goods or services`
          : type === "currency_unwrap"
            ? "Conversion to USD"
            : "Transfer to another merchant",
      transactionId: generateTransactionId(),
      category:
        type === "customer_purchase"
          ? ["Food", "Retail", "Services", "Other"][Math.floor(Math.random() * 4)]
          : undefined,
    })
  }

  // Sort by timestamp (newest first)
  return transactions.sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
}

// Generate mock transactions for a user
export const generateUserTransactions = (count: number): Transaction[] => {
  const transactions: Transaction[] = []
  const now = new Date()

  for (let i = 0; i < count; i++) {
    const type = Math.random() > 0.6 ? "volunteer_reward" : "currency_transfer"

    const amount =
      type === "volunteer_reward"
        ? Math.floor(Math.random() * 100) + 20
        : (Math.random() > 0.7 ? 1 : -1) * (Math.floor(Math.random() * 50) + 5)

    const daysAgo = Math.floor(Math.random() * 90)
    const date = new Date(now)
    date.setDate(date.getDate() - daysAgo)

    const fromName =
      type === "volunteer_reward"
        ? ["SF Food Bank", "Golden Gate Park Conservancy", "Habitat for Humanity SF", "SPCA San Francisco"][
            Math.floor(Math.random() * 4)
          ]
        : amount > 0
          ? "Another User"
          : "You"

    const toName =
      type === "volunteer_reward"
        ? "You"
        : amount > 0
          ? "You"
          : ["Green Earth Café", "Mission Threads", "Noe Valley Grocery", "Another User"][Math.floor(Math.random() * 4)]

    transactions.push({
      id: `tx-${i}`,
      type: type as any,
      amount,
      timestamp: date.toISOString(),
      status: Math.random() > 0.05 ? "completed" : Math.random() > 0.5 ? "pending" : "failed",
      fromName,
      fromAddress: generateAddress(),
      toName,
      toAddress: generateAddress(),
      description:
        type === "volunteer_reward"
          ? `Reward for volunteering at ${fromName}`
          : `${amount > 0 ? "Received from" : "Sent to"} ${amount > 0 ? fromName : toName}`,
      transactionId: generateTransactionId(),
      category: type === "volunteer_reward" ? "Volunteer" : amount > 0 ? "Received" : "Sent",
    })
  }

  // Sort by timestamp (newest first)
  return transactions.sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
}

// Generate mock analytics data for merchants
export const generateMerchantAnalytics = (): TransactionAnalytics => {
  const totalIncome = Math.floor(Math.random() * 10000) + 2000
  const totalTransactions = Math.floor(Math.random() * 500) + 100

  // Generate daily data for the last 30 days
  const dailyData = Array.from({ length: 30 }, (_, i) => {
    const date = new Date()
    date.setDate(date.getDate() - i)
    return {
      date: date.toISOString().split("T")[0],
      amount: Math.floor(Math.random() * 200) + 20,
      count: Math.floor(Math.random() * 10) + 1,
    }
  }).reverse()

  // Generate weekly data for the last 12 weeks
  const weeklyData = Array.from({ length: 12 }, (_, i) => {
    const date = new Date()
    date.setDate(date.getDate() - i * 7)
    return {
      date: `Week ${12 - i}`,
      amount: Math.floor(Math.random() * 1000) + 200,
      count: Math.floor(Math.random() * 50) + 5,
    }
  }).reverse()

  // Generate monthly data for the last 12 months
  const monthlyData = Array.from({ length: 12 }, (_, i) => {
    const date = new Date()
    date.setMonth(date.getMonth() - i)
    return {
      date: date.toLocaleString("default", { month: "short", year: "numeric" }),
      amount: Math.floor(Math.random() * 4000) + 800,
      count: Math.floor(Math.random() * 200) + 20,
    }
  }).reverse()

  // Generate yearly data for the last 5 years
  const yearlyData = Array.from({ length: 5 }, (_, i) => {
    const date = new Date()
    date.setFullYear(date.getFullYear() - i)
    return {
      date: date.getFullYear().toString(),
      amount: Math.floor(Math.random() * 40000) + 8000,
      count: Math.floor(Math.random() * 2000) + 200,
    }
  }).reverse()

  // Generate category breakdown
  const categories = ["Food", "Retail", "Services", "Other"]
  const categoryBreakdown = categories.map((category) => {
    const amount = Math.floor(Math.random() * 5000) + 500
    return {
      category,
      amount,
      percentage: 0, // Will be calculated below
    }
  })

  const totalCategoryAmount = categoryBreakdown.reduce((sum, item) => sum + item.amount, 0)
  categoryBreakdown.forEach((item) => {
    item.percentage = Math.round((item.amount / totalCategoryAmount) * 100)
  })

  return {
    totalIncome,
    totalTransactions,
    averageTransaction: Math.round(totalIncome / totalTransactions),
    dailyData,
    weeklyData,
    monthlyData,
    yearlyData,
    categoryBreakdown,
  }
}

// Create mock data instances
export const mockMerchantTransactions = generateMerchantTransactions(50)
export const mockUserTransactions = generateUserTransactions(30)
export const mockMerchantAnalytics = generateMerchantAnalytics()
