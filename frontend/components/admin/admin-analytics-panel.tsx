"use client"

import { useEffect, useMemo, useState } from "react"
import {
  Activity,
  ArrowDownToLine,
  Clock3,
  Coins,
  Download,
  Loader2,
  Repeat2,
  Store,
  Users,
  type LucideIcon,
} from "lucide-react"

import { useApp } from "@/context/AppProvider"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { SFLUV_DECIMALS } from "@/lib/constants"
import type { AdminAnalyticsResponse } from "@/types/admin-analytics"

const tokenFormatter = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 })
const decimalFormatter = new Intl.NumberFormat("en-US", { maximumFractionDigits: 1 })
const countFormatter = new Intl.NumberFormat("en-US")

const weiToNumber = (value?: string): number => {
  try {
    const wei = BigInt(value || "0")
    const scale = BigInt(10) ** BigInt(SFLUV_DECIMALS)
    const whole = wei / scale
    const fractional = wei % scale
    return Number(whole) + Number(fractional) / Number(scale)
  } catch {
    return 0
  }
}

const formatToken = (value?: string): string => `${tokenFormatter.format(weiToNumber(value))} SFLuv`
const formatCount = (value?: number): string => countFormatter.format(value || 0)
const formatPercent = (value?: number): string => `${decimalFormatter.format(value || 0)}%`

const formatDuration = (seconds?: number): string => {
  const value = seconds || 0
  if (value <= 0) return "No usage yet"
  const days = value / 86400
  if (days >= 1) return `${decimalFormatter.format(days)} days`
  const hours = value / 3600
  if (hours >= 1) return `${decimalFormatter.format(hours)} hours`
  return `${Math.max(1, Math.round(value / 60))} min`
}

const csvEscape = (value: unknown): string => {
  const text = String(value ?? "")
  if (/[",\n]/.test(text)) return `"${text.replaceAll('"', '""')}"`
  return text
}

const downloadTextFile = (filename: string, content: string) => {
  const blob = new Blob([content], { type: "text/csv;charset=utf-8;" })
  const url = URL.createObjectURL(blob)
  const link = document.createElement("a")
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

const buildAnalyticsCsv = (analytics: AdminAnalyticsResponse): string => {
  const rows: unknown[][] = [
    ["section", "key", "label", "value", "notes"],
    ["summary", "generated_at", "Generated At", new Date(analytics.generated_at * 1000).toISOString(), ""],
    ["summary", "configured_paid_addresses", "Configured Paid Addresses", analytics.configured_paid_addresses.join(" "), ""],
    ["summary", "total_active_users", "Total Active Users", analytics.summary.total_active_users, ""],
    ["summary", "daily_active_users", "Daily Active Users", analytics.summary.daily_active_users, ""],
    ["summary", "monthly_active_users", "Monthly Active Users", analytics.summary.monthly_active_users, ""],
    ["summary", "monthly_transaction_volume_sfluv", "Monthly Transaction Volume", weiToNumber(analytics.summary.monthly_transaction_volume_wei), "SFLuv"],
    ["summary", "total_distributed_sfluv", "Total Distributed", weiToNumber(analytics.summary.total_distributed_wei), "SFLuv"],
    ["summary", "distributed_spent_at_merchants_sfluv", "Distributed Spent At Merchants", weiToNumber(analytics.summary.distributed_spent_at_merchants_wei), "SFLuv"],
    ["summary", "token_redeemed_percent", "Redeemed Token Percent", analytics.summary.token_redeemed_percent, "%"],
    ["summary", "average_project_cost_sfluv", "Average Community Project Cost", weiToNumber(analytics.summary.average_community_project_cost_wei), "SFLuv"],
    ["summary", "merchant_repeat_customer_percent", "Merchant Repeat Customer Percent", analytics.summary.merchant_repeat_customer_percent, "%"],
    ["summary", "average_seconds_to_use_distribution", "Average Seconds To Use Distribution", analytics.summary.average_seconds_to_use_distribution, ""],
    ["summary", "volunteer_events", "Volunteer Events", analytics.summary.volunteer_events, ""],
    ["summary", "volunteer_unique_earners", "Volunteer Unique Earners", analytics.summary.volunteer_unique_earners, ""],
    ["summary", "volunteer_repeat_participation_rate", "Volunteer Repeat Participation Rate", analytics.summary.volunteer_repeat_participation_rate, "%"],
    [],
    ["monthly", "key", "label", "active_users", "transaction_volume_sfluv", "distributed_sfluv", "merchant_spend_sfluv", "merchant_customer_wallets", "merchant_repeat_customer_wallets", "volunteer_events", "unique_earners", "average_project_cost_sfluv"],
    ...analytics.monthly.map((point) => [
      "monthly",
      point.key,
      point.label,
      point.active_users,
      weiToNumber(point.transaction_volume_wei),
      weiToNumber(point.distributed_wei),
      weiToNumber(point.merchant_spend_wei),
      point.merchant_customer_wallets,
      point.merchant_repeat_customer_wallets,
      point.volunteer_events,
      point.unique_earners,
      weiToNumber(point.average_project_cost_wei),
    ]),
    [],
    ["daily", "key", "label", "active_users"],
    ...analytics.daily.map((point) => ["daily", point.key, point.label, point.active_users]),
    [],
    ["definition", "key", "label", "definition"],
    ...analytics.metric_definitions.map((definition) => ["definition", definition.key, definition.label, definition.definition]),
  ]

  return rows.map((row) => row.map(csvEscape).join(",")).join("\n")
}

function MetricCard({
  title,
  value,
  detail,
  icon: Icon,
}: {
  title: string
  value: string
  detail: string
  icon: LucideIcon
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <Icon className="h-4 w-4 text-[#eb6c6c]" />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold">{value}</div>
        <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
      </CardContent>
    </Card>
  )
}

export function AdminAnalyticsPanel() {
  const { authFetch, status, user } = useApp()
  const [analytics, setAnalytics] = useState<AdminAnalyticsResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    if (status !== "authenticated" || !user?.isAdmin) return

    let cancelled = false
    const loadAnalytics = async () => {
      setLoading(true)
      setError("")
      try {
        const res = await authFetch("/admin/analytics/dashboard")
        if (!res.ok) throw new Error("Unable to load admin analytics.")
        const data = (await res.json()) as AdminAnalyticsResponse
        if (!cancelled) setAnalytics(data)
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : "Unable to load admin analytics.")
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void loadAnalytics()
    return () => {
      cancelled = true
    }
  }, [authFetch, status, user?.isAdmin])

  const maxMonthlyVolume = useMemo(() => {
    return Math.max(...(analytics?.monthly || []).map((point) => weiToNumber(point.transaction_volume_wei)), 1)
  }, [analytics])

  const maxDailyActive = useMemo(() => {
    return Math.max(...(analytics?.daily || []).map((point) => point.active_users), 1)
  }, [analytics])

  const exportCsv = () => {
    if (!analytics) return
    downloadTextFile(`sfluv-admin-analytics-${new Date().toISOString().slice(0, 10)}.csv`, buildAnalyticsCsv(analytics))
  }

  if (loading && !analytics) {
    return (
      <Card>
        <CardContent className="flex min-h-[280px] items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-[#eb6c6c]" />
        </CardContent>
      </Card>
    )
  }

  if (error) {
    return (
      <Card className="border-red-200 bg-red-50">
        <CardHeader>
          <CardTitle className="text-red-700">Analytics unavailable</CardTitle>
          <CardDescription className="text-red-600">{error}</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  if (!analytics) return null

  const summary = analytics.summary
  const paidAddressStatus = analytics.configured_paid_addresses.length > 0
    ? `${analytics.configured_paid_addresses.length} paid source address${analytics.configured_paid_addresses.length === 1 ? "" : "es"} configured`
    : "No paid source addresses configured"

  return (
    <div className="space-y-6">
      <Card className="border-[#eb6c6c]/20 bg-[#fff7f7]">
        <CardHeader className="gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5 text-[#eb6c6c]" />
              Analytics
            </CardTitle>
            <CardDescription>
              Internal reporting for financials, grant narratives, merchant activity, and volunteer participation.
            </CardDescription>
          </div>
          <div className="flex flex-col gap-2 sm:items-end">
            <Button onClick={exportCsv}>
              <Download className="mr-2 h-4 w-4" />
              Export CSV
            </Button>
            <Badge variant={analytics.configured_paid_addresses.length > 0 ? "secondary" : "destructive"}>
              {paidAddressStatus}
            </Badge>
          </div>
        </CardHeader>
      </Card>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          title="Active Users"
          value={formatCount(summary.monthly_active_users)}
          detail={`${formatCount(summary.daily_active_users)} today, ${formatPercent(summary.monthly_active_user_change_percent)} vs prior month`}
          icon={Users}
        />
        <MetricCard
          title="Monthly Volume"
          value={formatToken(summary.monthly_transaction_volume_wei)}
          detail="Gross indexed SFLuv transfer volume this month"
          icon={Coins}
        />
        <MetricCard
          title="Redeemed Tokens"
          value={formatPercent(summary.token_redeemed_percent)}
          detail={`${formatToken(summary.token_redeemed_wei)} used, ${formatToken(summary.token_unused_wei)} still held`}
          icon={ArrowDownToLine}
        />
        <MetricCard
          title="Avg Project Cost"
          value={formatToken(summary.average_community_project_cost_wei)}
          detail="Completed and paid-out workflow bounty average"
          icon={Activity}
        />
        <MetricCard
          title="Merchant Customers"
          value={formatCount(summary.merchant_customer_wallets)}
          detail={`${formatCount(summary.merchant_repeat_customer_wallets)} repeat wallets (${formatPercent(summary.merchant_repeat_customer_percent)})`}
          icon={Store}
        />
        <MetricCard
          title="Total Distributed"
          value={formatToken(summary.total_distributed_wei)}
          detail={`${formatToken(summary.distributed_spent_at_merchants_wei)} spent at merchants`}
          icon={Coins}
        />
        <MetricCard
          title="Time To Use"
          value={formatDuration(summary.average_seconds_to_use_distribution)}
          detail="Average first outgoing transfer after distribution"
          icon={Clock3}
        />
        <MetricCard
          title="Volunteer Frequency"
          value={formatCount(summary.volunteer_participation_count)}
          detail={`${formatCount(summary.volunteer_repeat_earners)} repeat earners (${formatPercent(summary.volunteer_repeat_participation_rate)})`}
          icon={Repeat2}
        />
      </div>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(320px,0.65fr)]">
        <Card>
          <CardHeader>
            <CardTitle>Monthly Reporting Trend</CardTitle>
            <CardDescription>Volume, merchant spend, active users, volunteer events, and unique earners.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {analytics.monthly.map((point) => {
              const volume = weiToNumber(point.transaction_volume_wei)
              const width = Math.max(3, Math.round((volume / maxMonthlyVolume) * 100))
              return (
                <div key={point.key} className="grid gap-2 md:grid-cols-[92px_minmax(0,1fr)_220px] md:items-center">
                  <div className="text-sm font-medium">{point.label}</div>
                  <div className="h-3 overflow-hidden rounded-full bg-secondary">
                    <div className="h-full rounded-full bg-[#eb6c6c]" style={{ width: `${width}%` }} />
                  </div>
                  <div className="grid grid-cols-2 gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    <span>{formatToken(point.transaction_volume_wei)}</span>
                    <span>{formatCount(point.active_users)} active</span>
                    <span>{formatToken(point.merchant_spend_wei)} merchants</span>
                    <span>{formatCount(point.unique_earners)} earners</span>
                  </div>
                </div>
              )
            })}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Daily Active Users</CardTitle>
            <CardDescription>Last 30 days from indexed transfer activity.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex h-56 items-end gap-1">
              {analytics.daily.map((point) => {
                const height = Math.max(4, Math.round((point.active_users / maxDailyActive) * 100))
                return (
                  <div key={point.key} className="flex min-w-0 flex-1 flex-col items-center gap-2">
                    <div className="w-full rounded-t bg-[#eb6c6c]" style={{ height: `${height}%` }} title={`${point.label}: ${point.active_users}`} />
                    <span className="hidden text-[10px] text-muted-foreground sm:block">{point.label.split(" ")[1]}</span>
                  </div>
                )
              })}
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Volunteer And Earner Averages</CardTitle>
          <CardDescription>Event averages use bot event timing; unique earners use redemption addresses.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-3">
          <div className="rounded-md border p-4">
            <div className="text-sm text-muted-foreground">Events</div>
            <div className="mt-2 text-xl font-semibold">{decimalFormatter.format(summary.average_volunteer_events_per_week)} / week</div>
            <div className="text-sm text-muted-foreground">{decimalFormatter.format(summary.average_volunteer_events_per_month)} / month</div>
            <div className="text-sm text-muted-foreground">{decimalFormatter.format(summary.average_volunteer_events_per_year)} / year</div>
          </div>
          <div className="rounded-md border p-4">
            <div className="text-sm text-muted-foreground">Unique Earners</div>
            <div className="mt-2 text-xl font-semibold">{decimalFormatter.format(summary.average_unique_earners_per_week)} / week</div>
            <div className="text-sm text-muted-foreground">{decimalFormatter.format(summary.average_unique_earners_per_month)} / month</div>
            <div className="text-sm text-muted-foreground">{decimalFormatter.format(summary.average_unique_earners_per_year)} / year</div>
          </div>
          <div className="rounded-md border p-4">
            <div className="text-sm text-muted-foreground">Code Redemption</div>
            <div className="mt-2 text-xl font-semibold">{formatPercent(summary.event_code_redemption_percent)}</div>
            <div className="text-sm text-muted-foreground">{formatCount(summary.volunteer_events)} total events</div>
            <div className="text-sm text-muted-foreground">{formatCount(summary.volunteer_unique_earners)} total unique earners</div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Metric Definitions</CardTitle>
          <CardDescription>Export includes these definitions so finance and grant work keeps the same assumptions.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {analytics.metric_definitions.map((definition) => (
            <div key={definition.key} className="rounded-md border p-3">
              <div className="font-medium">{definition.label}</div>
              <p className="mt-1 text-sm text-muted-foreground">{definition.definition}</p>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}
