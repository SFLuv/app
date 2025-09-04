"use client"

import { useState } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { DollarSign, BarChart3, TrendingUp, ArrowDownToLine } from "lucide-react"
import type { TransactionAnalytics } from "@/types/transaction"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"

interface TransactionAnalyticsProps {
  analytics: TransactionAnalytics
}

export function TransactionAnalytics({ analytics }: TransactionAnalyticsProps) {
  const [timeframe, setTimeframe] = useState<"daily" | "weekly" | "monthly" | "yearly">("monthly")
  const [hoveredBar, setHoveredBar] = useState<number | null>(null)

  // Get data based on selected timeframe
  const getData = () => {
    switch (timeframe) {
      case "daily":
        return analytics.dailyData.slice(-7) // Only show last 7 days
      case "weekly":
        return analytics.weeklyData
      case "monthly":
        return analytics.monthlyData
      case "yearly":
        return analytics.yearlyData
      default:
        return analytics.monthlyData
    }
  }

  const data = getData()

  // Find max value for scaling
  const maxAmount = Math.max(...data.map((d) => d.amount))

  // Format currency
  const formatCurrency = (amount: number) => {
    return amount.toLocaleString("en-US", {
      style: "decimal",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    })
  }

  // Format date for tooltip
  const formatDate = (date: string) => {
    if (timeframe === "daily") {
      return new Date(date).toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric" })
    }
    return date
  }

  // Mock total unwrapped amount
  const totalUnwrapped = 3250

  return (
    <div className="space-y-6">
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium text-black dark:text-white">Total Income</CardTitle>
            <DollarSign className="h-4 w-4 text-green-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-black dark:text-white">
              {formatCurrency(analytics.totalIncome)} SFLuv
            </div>
            <p className="text-xs text-gray-500 dark:text-gray-400">Lifetime earnings</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium text-black dark:text-white">Total Transactions</CardTitle>
            <BarChart3 className="h-4 w-4 text-blue-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-black dark:text-white">{analytics.totalTransactions}</div>
            <p className="text-xs text-gray-500 dark:text-gray-400">All time</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium text-black dark:text-white">Average Transaction</CardTitle>
            <TrendingUp className="h-4 w-4 text-purple-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-black dark:text-white">
              {formatCurrency(analytics.averageTransaction)} SFLuv
            </div>
            <p className="text-xs text-gray-500 dark:text-gray-400">Per transaction</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium text-black dark:text-white">Total Unwrapped</CardTitle>
            <ArrowDownToLine className="h-4 w-4 text-orange-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-black dark:text-white">{formatCurrency(totalUnwrapped)} SFLuv</div>
            <p className="text-xs text-gray-500 dark:text-gray-400">Converted to USD</p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-black dark:text-white">Income Overview</CardTitle>
            <Select value={timeframe} onValueChange={(value: any) => setTimeframe(value)}>
              <SelectTrigger className="w-[180px] text-black dark:text-white bg-secondary">
                <SelectValue placeholder="Select timeframe" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="daily">Daily</SelectItem>
                <SelectItem value="weekly">Weekly</SelectItem>
                <SelectItem value="monthly">Monthly</SelectItem>
                <SelectItem value="yearly">Yearly</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <CardDescription>
            {timeframe === "daily"
              ? "Last 7 days"
              : timeframe === "weekly"
                ? "Last 12 weeks"
                : timeframe === "monthly"
                  ? "Last 12 months"
                  : "Last 5 years"}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <TooltipProvider>
            <div className="h-[300px] w-full overflow-hidden">
              <div className="flex h-full items-end gap-0 w-full">
                {data.map((item, index) => (
                  <Tooltip key={index}>
                    <TooltipTrigger asChild>
                      <div
                        className="relative flex h-full flex-1 flex-col justify-end px-1"
                        onMouseEnter={() => setHoveredBar(index)}
                        onMouseLeave={() => setHoveredBar(null)}
                      >
                        <div
                          className={`bg-[#eb6c6c] hover:bg-[#d55c5c] rounded-t w-full transition-all duration-300 ${
                            hoveredBar === index ? "bg-[#d55c5c]" : ""
                          }`}
                          style={{ height: `${(item.amount / maxAmount) * 100}%` }}
                        ></div>
                        <span className="mt-1 text-xs text-gray-500 dark:text-gray-400 truncate text-center w-full">
                          {item.date}
                        </span>
                      </div>
                    </TooltipTrigger>
                    <TooltipContent side="top" className="bg-secondary text-black dark:text-white">
                      <div className="space-y-1">
                        <p className="font-medium">{formatDate(item.date)}</p>
                        <p className="text-sm">{formatCurrency(item.amount)} SFLuv</p>
                        <p className="text-xs text-gray-500 dark:text-gray-400">{item.count} transactions</p>
                      </div>
                    </TooltipContent>
                  </Tooltip>
                ))}
              </div>
            </div>
          </TooltipProvider>
        </CardContent>
      </Card>
    </div>
  )
}
