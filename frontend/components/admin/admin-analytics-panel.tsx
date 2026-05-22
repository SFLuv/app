"use client"

import { useEffect, useMemo, useState } from "react"
import {
  Activity,
  BarChart3,
  Clipboard,
  Clock3,
  Coins,
  Download,
  Loader2,
  Repeat2,
  Store,
  Users,
  Wallet,
  type LucideIcon,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useApp } from "@/context/AppProvider"
import { SFLUV_DECIMALS } from "@/lib/constants"
import type {
  AdminAnalyticsDefinition,
  AdminAnalyticsMetricValue,
  AdminAnalyticsPeriod,
  AdminAnalyticsResponse,
  AdminAnalyticsTrendPoint,
} from "@/types/admin-analytics"

const tokenFormatter = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 })
const decimalFormatter = new Intl.NumberFormat("en-US", { maximumFractionDigits: 1 })
const countFormatter = new Intl.NumberFormat("en-US")

const metricIcons: Record<string, LucideIcon> = {
  active_users: Users,
  active_wallets: Wallet,
  transactions: Activity,
  transaction_volume: BarChart3,
  rewards: Coins,
  total_payments: Store,
  total_sfluv_distributed: Coins,
  usage_percentage: BarChart3,
  unique_volunteers: Users,
  volunteer_frequency: Repeat2,
  repeat_business: Store,
  value_weighted_average_time_to_spend: Clock3,
  event_frequency: Activity,
}

const orderedMetricKeys = [
  "active_users",
  "active_wallets",
  "transactions",
  "transaction_volume",
  "rewards",
  "total_payments",
  "total_sfluv_distributed",
  "usage_percentage",
  "unique_volunteers",
  "volunteer_frequency",
  "repeat_business",
  "value_weighted_average_time_to_spend",
  "event_frequency",
]

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
const formatDecimal = (value?: number): string => decimalFormatter.format(value || 0)

const formatDuration = (seconds?: number): string => {
  const value = seconds || 0
  if (value <= 0) return "No spend yet"
  const days = value / 86400
  if (days >= 1) return `${decimalFormatter.format(days)} days`
  const hours = value / 3600
  if (hours >= 1) return `${decimalFormatter.format(hours)} hours`
  return `${Math.max(1, Math.round(value / 60))} min`
}

const formatMetricValue = (metric?: AdminAnalyticsMetricValue): string => {
  if (!metric) return "0"
  if (metric.kind === "wei") return formatToken(metric.wei)
  if (metric.kind === "count") return formatCount(metric.count)
  if (metric.kind === "percent") return formatPercent(metric.percent)
  if (metric.kind === "seconds") return formatDuration(metric.seconds)
  return formatDecimal(metric.decimal)
}

const periodRange = (period: AdminAnalyticsPeriod | AdminAnalyticsTrendPoint): string => {
  if (period.key === "all_time") return "All indexed time"
  const start = new Date(period.start_at * 1000)
  const end = new Date(period.end_at * 1000)
  return `${start.toLocaleDateString()} - ${end.toLocaleDateString()}`
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

const metricCsvValue = (metric: AdminAnalyticsMetricValue): string | number => {
  if (metric.kind === "wei") return weiToNumber(metric.wei)
  if (metric.kind === "count") return metric.count || 0
  if (metric.kind === "percent") return metric.percent || 0
  if (metric.kind === "seconds") return metric.seconds || 0
  return metric.decimal || 0
}

const buildAnalyticsCsv = (analytics: AdminAnalyticsResponse): string => {
  const rows: unknown[][] = [
    ["section", "period_key", "period_label", "metric_key", "metric_label", "value", "unit", "notes"],
    ["summary", "", "Current", "current_circulating_sfluv", "Current Circulating SFLUV", weiToNumber(analytics.summary.current_circulating_sfluv_wei), "SFLuv", "All-time rewards minus redemptions"],
    ["summary", "", "Generated", "generated_at", "Generated At", new Date(analytics.generated_at * 1000).toISOString(), "", ""],
    ["summary", "", "Chain", "chain_id", "Chain ID", analytics.chain_id, "", ""],
    ...analytics.periods.flatMap((period) =>
      period.metrics.map((metric) => [
        "period",
        period.key,
        period.label,
        metric.metric_key,
        metric.label,
        metricCsvValue(metric),
        metric.kind === "wei" ? "SFLuv" : metric.kind,
        periodRange(period),
      ]),
    ),
    ...analytics.monthly_trend.flatMap((period) =>
      period.metrics.map((metric) => [
        "monthly_trend",
        period.key,
        period.label,
        metric.metric_key,
        metric.label,
        metricCsvValue(metric),
        metric.kind === "wei" ? "SFLuv" : metric.kind,
        periodRange(period),
      ]),
    ),
    [],
    ["definition", "", "", "key", "label", "definition", "", ""],
    ...analytics.metric_definitions.map((definition) => ["definition", "", "", definition.key, definition.label, definition.definition, "", ""]),
    [],
    ["glossary", "", "", "key", "label", "definition", "", ""],
    ...analytics.glossary.map((definition) => ["glossary", "", "", definition.key, definition.label, definition.definition, "", ""]),
  ]

  return rows.map((row) => row.map(csvEscape).join(",")).join("\n")
}

const escapeSvg = (value: string): string =>
  value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;")

const copyMetricImage = async (title: string, value: string, detail: string) => {
  const width = 880
  const height = 420
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
      <rect width="100%" height="100%" rx="24" fill="#fffafa"/>
      <rect x="24" y="24" width="${width - 48}" height="${height - 48}" rx="20" fill="#ffffff" stroke="#f1d9d6" stroke-width="3"/>
      <text x="72" y="112" font-family="Inter, Arial, sans-serif" font-size="38" font-weight="700" fill="#252936">${escapeSvg(title)}</text>
      <text x="72" y="230" font-family="Inter, Arial, sans-serif" font-size="72" font-weight="800" fill="#252936">${escapeSvg(value)}</text>
      <text x="72" y="304" font-family="Inter, Arial, sans-serif" font-size="30" fill="#667085">${escapeSvg(detail)}</text>
      <text x="72" y="358" font-family="Inter, Arial, sans-serif" font-size="22" fill="#eb6c6c">SFLuv analytics</text>
    </svg>`
  const image = new Image()
  const blob = new Blob([svg], { type: "image/svg+xml;charset=utf-8" })
  const url = URL.createObjectURL(blob)
  image.src = url
  await new Promise<void>((resolve, reject) => {
    image.onload = () => resolve()
    image.onerror = () => reject(new Error("Unable to render metric image."))
  })
  const canvas = document.createElement("canvas")
  canvas.width = width
  canvas.height = height
  const ctx = canvas.getContext("2d")
  if (!ctx) throw new Error("Unable to render metric image.")
  ctx.drawImage(image, 0, 0)
  URL.revokeObjectURL(url)
  const png = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/png"))
  if (!navigator.clipboard) {
    return
  }
  if (!png || !("ClipboardItem" in window)) {
    await navigator.clipboard.writeText(`${title}: ${value} (${detail})`)
    return
  }
  await navigator.clipboard.write([new ClipboardItem({ "image/png": png })])
}

const definitionFor = (definitions: AdminAnalyticsDefinition[], key: string): string =>
  definitions.find((definition) => definition.key === key)?.definition || ""

function MetricTile({
  metric,
  definition,
  periodLabel,
}: {
  metric: AdminAnalyticsMetricValue
  definition: string
  periodLabel: string
}) {
  const Icon = metricIcons[metric.metric_key] || Activity
  const [copied, setCopied] = useState(false)
  const value = formatMetricValue(metric)
  const copy = async () => {
    try {
      await copyMetricImage(metric.label, value, periodLabel)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3 space-y-0 pb-3">
        <div className="min-w-0">
          <CardTitle className="text-sm font-medium">{metric.label}</CardTitle>
          <CardDescription className="mt-1 line-clamp-2 text-xs">{definition}</CardDescription>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Icon className="h-4 w-4 text-[#eb6c6c]" />
          <Button type="button" size="icon" variant="ghost" className="h-8 w-8" onClick={copy} title="Copy metric image">
            <Clipboard className="h-4 w-4" />
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold leading-tight">{value}</div>
        <p className="mt-2 min-h-5 text-xs text-muted-foreground">{copied ? "Copied image to clipboard" : periodLabel}</p>
      </CardContent>
    </Card>
  )
}

export function AdminAnalyticsPanel() {
  const { authFetch, status, user } = useApp()
  const [analytics, setAnalytics] = useState<AdminAnalyticsResponse | null>(null)
  const [selectedPeriodKey, setSelectedPeriodKey] = useState("current_month")
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

  const selectedPeriod = useMemo(() => {
    if (!analytics) return null
    return analytics.periods.find((period) => period.key === selectedPeriodKey) || analytics.periods[0] || null
  }, [analytics, selectedPeriodKey])

  const maxMonthlyVolume = useMemo(() => {
    return Math.max(...(analytics?.monthly_trend || []).map((point) => weiToNumber(point.metrics.find((metric) => metric.metric_key === "transaction_volume")?.wei)), 1)
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

  if (!analytics || !selectedPeriod) return null

  const sortedMetrics = orderedMetricKeys
    .map((key) => selectedPeriod.metrics.find((metric) => metric.metric_key === key))
    .filter((metric): metric is AdminAnalyticsMetricValue => Boolean(metric))

  return (
    <div className="space-y-6">
      <Card className="border-[#eb6c6c]/20 bg-[#fff7f7]">
        <CardHeader className="gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5 text-[#eb6c6c]" />
              Analytics
            </CardTitle>
            <CardDescription>Glossary-based internal reporting for SFLuv circulation, rewards, payments, redemptions, and volunteering.</CardDescription>
          </div>
          <div className="flex flex-col gap-2 sm:items-end">
            <Button onClick={exportCsv}>
              <Download className="mr-2 h-4 w-4" />
              Export CSV
            </Button>
            <Badge variant="secondary">Chain {analytics.chain_id}</Badge>
          </div>
        </CardHeader>
      </Card>

      <Card>
        <CardHeader className="gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <CardTitle>Current Circulating SFLUV</CardTitle>
            <CardDescription>All-time total SFLUV distributed through rewards minus all-time redemptions.</CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={() => copyMetricImage("Current Circulating SFLUV", formatToken(analytics.summary.current_circulating_sfluv_wei), "All-time rewards minus redemptions")}
          >
            <Clipboard className="mr-2 h-4 w-4" />
            Copy Image
          </Button>
        </CardHeader>
        <CardContent>
          <div className="text-4xl font-semibold tracking-normal">{formatToken(analytics.summary.current_circulating_sfluv_wei)}</div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Period Metrics</CardTitle>
          <CardDescription>Weeks run Sunday through Saturday; months and years use calendar boundaries.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="flex flex-wrap gap-2">
            {analytics.periods.map((period) => (
              <Button
                key={period.key}
                type="button"
                size="sm"
                variant={period.key === selectedPeriod.key ? "default" : "outline"}
                onClick={() => setSelectedPeriodKey(period.key)}
              >
                {period.label}
              </Button>
            ))}
          </div>
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {sortedMetrics.map((metric) => (
              <MetricTile
                key={metric.metric_key}
                metric={metric}
                definition={definitionFor(analytics.metric_definitions, metric.metric_key)}
                periodLabel={periodRange(selectedPeriod)}
              />
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Monthly Trend</CardTitle>
          <CardDescription>Last 12 calendar months using the same glossary calculations.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {analytics.monthly_trend.map((point) => {
            const transactionVolumeMetric = point.metrics.find((metric) => metric.metric_key === "transaction_volume")
            const volume = weiToNumber(transactionVolumeMetric?.wei)
            const rewards = formatMetricValue(point.metrics.find((metric) => metric.metric_key === "rewards"))
            const payments = formatMetricValue(point.metrics.find((metric) => metric.metric_key === "total_payments"))
            const volunteers = formatMetricValue(point.metrics.find((metric) => metric.metric_key === "unique_volunteers"))
            const width = Math.max(3, Math.round((volume / maxMonthlyVolume) * 100))
            return (
              <div key={point.key} className="grid gap-2 md:grid-cols-[92px_minmax(0,1fr)_280px] md:items-center">
                <div className="text-sm font-medium">{point.label}</div>
                <div className="h-3 overflow-hidden rounded-full bg-secondary">
                  <div className="h-full rounded-full bg-[#eb6c6c]" style={{ width: `${width}%` }} />
                </div>
                <div className="grid grid-cols-2 gap-x-3 gap-y-1 text-xs text-muted-foreground">
                  <span>{formatMetricValue(transactionVolumeMetric)}</span>
                  <span>{volunteers} volunteers</span>
                  <span>{rewards} rewards</span>
                  <span>{payments} payments</span>
                </div>
              </div>
            )
          })}
        </CardContent>
      </Card>

      <div className="grid gap-6 xl:grid-cols-2">
        <DefinitionList title="Metric Definitions" items={analytics.metric_definitions} />
        <DefinitionList title="Glossary" items={analytics.glossary} />
      </div>
    </div>
  )
}

function DefinitionList({ title, items }: { title: string; items: AdminAnalyticsDefinition[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {items.map((definition) => (
          <div key={definition.key} className="rounded-md border p-3">
            <div className="font-medium">{definition.label}</div>
            <p className="mt-1 text-sm text-muted-foreground">{definition.definition}</p>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
