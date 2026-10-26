"use client"

import { useEffect, useRef, useState } from "react"
import { CheckCircle2, FileText, Loader2, Search, Send, XCircle } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { useApp } from "@/context/AppProvider"
import type { W9RequestResult, W9UserStatus } from "@/types/w9"

type FoundUser = {
  id: string
  contact_name?: string | null
  contact_email?: string | null
}

const FILING_LABELS: Record<string, string> = {
  not_started: "Not started",
  requested: "Requested, not filled in",
  in_progress: "Started, not finished",
  completed: "On file",
  legacy_approved: "On file (approved before TaxBandits)",
  manually_cleared: "Cleared by an admin",
  invalid: "Rejected, needs a new form",
}

/**
 * Ask anyone for a W-9 by hand.
 *
 * The thresholds ask on their own as money is earned; this is for everything
 * else — while enforcement is off, ahead of a large payout, or for someone the
 * automatic path missed. Outcomes are shown inline rather than as toasts, which
 * this app does not render.
 */
export function W9RequestPanel() {
  const { authFetch } = useApp()
  const [query, setQuery] = useState("")
  const [results, setResults] = useState<FoundUser[]>([])
  const [searching, setSearching] = useState(false)
  const [selected, setSelected] = useState<FoundUser | null>(null)
  const [status, setStatus] = useState<W9UserStatus | null>(null)
  const [statusLoading, setStatusLoading] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [sending, setSending] = useState(false)
  const [result, setResult] = useState<W9RequestResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const searchSeq = useRef(0)

  useEffect(() => {
    const term = query.trim()
    if (term.length < 2) {
      setResults([])
      return
    }
    const seq = ++searchSeq.current
    const timer = setTimeout(async () => {
      setSearching(true)
      try {
        const params = new URLSearchParams({ page: "0", count: "8", search: term })
        const res = await authFetch(`/admin/users?${params}`)
        if (!res.ok) throw new Error()
        const data = await res.json()
        if (seq === searchSeq.current) setResults((Array.isArray(data) ? data : data.users) || [])
      } catch {
        if (seq === searchSeq.current) setResults([])
      } finally {
        if (seq === searchSeq.current) setSearching(false)
      }
    }, 300)
    return () => clearTimeout(timer)
  }, [query, authFetch])

  const loadStatus = async (user: FoundUser) => {
    setStatusLoading(true)
    setStatus(null)
    try {
      const res = await authFetch(`/admin/w9/${encodeURIComponent(user.id)}/status`)
      if (!res.ok) throw new Error()
      setStatus(await res.json())
    } catch {
      setError("Couldn't load this person's W-9 status.")
    } finally {
      setStatusLoading(false)
    }
  }

  const choose = (user: FoundUser) => {
    setSelected(user)
    setResults([])
    setQuery("")
    setConfirming(false)
    setResult(null)
    setError(null)
    void loadStatus(user)
  }

  const send = async () => {
    if (!selected) return
    setSending(true)
    setError(null)
    try {
      const res = await authFetch(`/admin/w9/${encodeURIComponent(selected.id)}/request`, { method: "POST" })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) {
        setError(body?.error || "The request couldn't be sent.")
        return
      }
      setResult(body as W9RequestResult)
      void loadStatus(selected)
    } catch {
      setError("The request couldn't be sent.")
    } finally {
      setSending(false)
      setConfirming(false)
    }
  }

  const displayName = (user: FoundUser) =>
    user.contact_name?.trim() || user.contact_email?.trim() || user.id

  return (
    <Card>
      <CardHeader className="pb-4">
        <CardTitle className="flex items-center gap-2 text-xl">
          <FileText className="h-6 w-6" />
          Request a W-9
        </CardTitle>
        <CardDescription className="text-base mt-2">
          Search for someone and send them the W-9 form. They get an email with the link, a push notification,
          and a prompt in the app.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search by name or email"
            className="pl-9"
          />
          {searching && <Loader2 className="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 animate-spin" />}
        </div>

        {results.length > 0 && (
          <div className="rounded-lg border divide-y">
            {results.map((user) => (
              <button
                key={user.id}
                type="button"
                onClick={() => choose(user)}
                className="w-full text-left px-4 py-2 hover:bg-muted/60"
              >
                <div className="font-medium">{displayName(user)}</div>
                {user.contact_email && user.contact_name && (
                  <div className="text-sm text-muted-foreground">{user.contact_email}</div>
                )}
              </button>
            ))}
          </div>
        )}

        {selected && (
          <div className="rounded-lg border p-4 space-y-3">
            <div>
              <div className="font-medium">{displayName(selected)}</div>
              <div className="text-sm text-muted-foreground">{selected.contact_email || "No email address on file"}</div>
            </div>

            {statusLoading ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : status ? (
              <div className="flex flex-wrap gap-2 text-sm">
                <Badge variant={status.cleared ? "default" : "secondary"}>
                  {status.tax_year}: {FILING_LABELS[status.filing_status] || status.filing_status}
                </Badge>
                <Badge variant="outline">
                  Earned {status.earned_sfluv} of {status.threshold_sfluv} SFLUV this year
                </Badge>
              </div>
            ) : null}

            {status?.cleared ? (
              <p className="text-sm text-muted-foreground">They already have a W-9 on file for {status.tax_year}.</p>
            ) : confirming ? (
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm">Send the W-9 request to {displayName(selected)}?</span>
                <Button size="sm" onClick={send} disabled={sending}>
                  {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Send it"}
                </Button>
                <Button size="sm" variant="outline" onClick={() => setConfirming(false)} disabled={sending}>
                  Cancel
                </Button>
              </div>
            ) : (
              <Button size="sm" onClick={() => setConfirming(true)} disabled={statusLoading || !status}>
                <Send className="h-4 w-4 mr-2" />
                {status && status.filing_status !== "not_started" ? "Send the W-9 request again" : "Send W-9 request"}
              </Button>
            )}

            {result && (
              <div className="rounded-md bg-muted/50 p-3 text-sm space-y-1">
                <div className="font-medium">Request sent</div>
                <Outcome ok={result.email_sent}>
                  {result.email_sent ? `Email sent to ${result.email}` : `No email: ${result.email_error || "not sent"}`}
                </Outcome>
                <Outcome ok={result.push_delivered}>
                  {result.push_delivered ? "Push notification sent" : "No push: they have no app with notifications on"}
                </Outcome>
                <Outcome ok={!!result.tier}>
                  {result.tier ? "In-app prompt and bell entry waiting for them" : "In-app prompt couldn't be set up"}
                </Outcome>
              </div>
            )}
          </div>
        )}

        {error && <p className="text-sm text-destructive">{error}</p>}
      </CardContent>
    </Card>
  )
}

function Outcome({ ok, children }: { ok: boolean; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2">
      {ok ? (
        <CheckCircle2 className="h-4 w-4 text-green-600" />
      ) : (
        <XCircle className="h-4 w-4 text-muted-foreground" />
      )}
      <span>{children}</span>
    </div>
  )
}
