"use client"

import { useCallback, useEffect, useState } from "react"
import { AlertTriangle, CheckCircle2, Clock, Download, Loader2, Megaphone, QrCode, XCircle } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"

export interface ManagedVolunteerEvent {
  id: string
  title: string
  start_at: string
  end_at: string
  timezone: string
  max_participants: number
  signup_count: number | null
  reward_amount_sfluv: number
  status: string
  review_status?: string
  funding_status?: string
  organizer: { type: string; name: string }
  recurrence: { summary: string } | null
  qr?: { live: boolean; live_at: string | null; codes_generated: boolean }
}

interface VolunteerEventsManagerProps {
  /** "/admin/volunteer-events" or "/affiliates/volunteer-events". */
  basePath: string
  /** Admins get approve / reject / cancel; affiliates get a read-only view plus QR download. */
  canReview: boolean
  title?: string
  description?: string
}

const REVIEW_FILTERS = [
  { value: "pending", label: "Awaiting approval" },
  { value: "approved", label: "Approved" },
  { value: "rejected", label: "Rejected" },
  { value: "cancelled", label: "Cancelled" },
  { value: "all", label: "All" },
]

function formatEventWhen(event: ManagedVolunteerEvent): string {
  // Rendered in the viewer's local time, per PJ's Q-M4 ruling — the payload
  // carries an instant, and every surface shows it in the reader's own zone.
  const start = new Date(event.start_at)
  if (Number.isNaN(start.getTime())) return ""
  return start.toLocaleString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  })
}

function ReviewBadge({ event }: { event: ManagedVolunteerEvent }) {
  const review = event.review_status
  if (review === "pending") {
    return (
      <Badge variant="outline" className="border-amber-500 text-amber-600">
        <Clock className="mr-1 h-3 w-3" /> Awaiting approval
      </Badge>
    )
  }
  if (review === "rejected") return <Badge variant="destructive">Rejected</Badge>
  if (review === "cancelled" || event.status === "cancelled") return <Badge variant="destructive">Cancelled</Badge>
  if (event.funding_status === "awaiting_funding") {
    return (
      <Badge variant="outline" className="border-destructive text-destructive">
        <AlertTriangle className="mr-1 h-3 w-3" /> Needs faucet top-up
      </Badge>
    )
  }
  return (
    <Badge variant="outline" className="border-emerald-500 text-emerald-600">
      <CheckCircle2 className="mr-1 h-3 w-3" /> Approved
    </Badge>
  )
}

/** QR codes are downloadable immediately but only redeemable 24h before start. */
function QrBadge({ event }: { event: ManagedVolunteerEvent }) {
  if (!event.qr?.codes_generated) {
    return <Badge variant="outline" className="text-muted-foreground">QR codes not generated</Badge>
  }
  if (event.qr.live) {
    return (
      <Badge variant="outline" className="border-emerald-500 text-emerald-600">
        <QrCode className="mr-1 h-3 w-3" /> QR live
      </Badge>
    )
  }
  const liveAt = event.qr.live_at ? new Date(event.qr.live_at) : null
  const label = liveAt && !Number.isNaN(liveAt.getTime())
    ? `QR live ${liveAt.toLocaleDateString(undefined, { month: "short", day: "numeric" })}`
    : "QR not live yet"
  return <Badge variant="outline" className="text-muted-foreground"><QrCode className="mr-1 h-3 w-3" />{label}</Badge>
}

/**
 * Approval queue and QR management for volunteer events.
 *
 * Shared by the admin panel and the affiliate panel: affiliates can no longer
 * create events unilaterally, but they still need to see their requests' status
 * and download the QR codes for approved events to print.
 */
export function VolunteerEventsManager({
  basePath,
  canReview,
  title = "Volunteer Events",
  description = "Approve requests, download QR codes, and manage published events.",
}: VolunteerEventsManagerProps) {
  const { authFetch } = useApp()
  const { toast } = useToast()
  const [events, setEvents] = useState<ManagedVolunteerEvent[]>([])
  const [reviewFilter, setReviewFilter] = useState(canReview ? "pending" : "all")
  const [search, setSearch] = useState("")
  const [loading, setLoading] = useState(false)
  const [busyId, setBusyId] = useState("")
  const [error, setError] = useState("")

  const loadEvents = useCallback(async () => {
    setLoading(true)
    setError("")
    try {
      const params = new URLSearchParams({ count: "50" })
      if (reviewFilter !== "all") params.set("review_status", reviewFilter)
      if (search.trim() !== "") params.set("search", search.trim())

      const res = await authFetch(`${basePath}?${params.toString()}`)
      if (!res.ok) throw new Error("Unable to load volunteer events.")
      const data = await res.json()
      setEvents(data.events || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load volunteer events.")
    } finally {
      setLoading(false)
    }
  }, [authFetch, basePath, reviewFilter, search])

  useEffect(() => {
    void loadEvents()
  }, [loadEvents])

  const review = async (event: ManagedVolunteerEvent, action: "approve" | "reject" | "cancel") => {
    let reason = ""
    if (action === "reject") {
      reason = window.prompt(`Why is "${event.title}" being rejected? (optional)`) ?? ""
    }
    if (action === "cancel" && !window.confirm(`Cancel "${event.title}"? Everyone signed up will be emailed.`)) {
      return
    }

    setBusyId(event.id)
    try {
      const res = await authFetch(`${basePath}/${event.id}/${action}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(action === "reject" ? { reason } : {}),
      })
      if (!res.ok) {
        // The backend returns a specific reason (e.g. insufficient faucet
        // balance) — surfacing it is the difference between a fixable message
        // and a button that appears to do nothing.
        throw new Error((await res.text()).trim() || `Unable to ${action} the event.`)
      }
      toast({
        title:
          action === "approve" ? "Event approved — QR codes generated"
          : action === "reject" ? "Event rejected"
          : "Event cancelled",
      })
      await loadEvents()
    } catch (err) {
      toast({
        title: `Could not ${action} event`,
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
    } finally {
      setBusyId("")
    }
  }

  // The CSV endpoint is auth-guarded, so a plain link would 403. Fetch it with
  // credentials and hand the browser a blob instead.
  const downloadCodes = async (event: ManagedVolunteerEvent) => {
    setBusyId(event.id)
    try {
      const res = await authFetch(`${basePath}/${event.id}/codes.csv`)
      if (!res.ok) throw new Error("Unable to download QR codes.")
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const link = document.createElement("a")
      link.href = url
      link.download = `${event.title.replace(/[^a-z0-9]+/gi, "-").toLowerCase()}-qr-codes.csv`
      document.body.appendChild(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
    } catch (err) {
      toast({
        title: "Could not download QR codes",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
    } finally {
      setBusyId("")
    }
  }

  // Blast: organizers message everyone holding a confirmed spot. Delivered as a
  // push to volunteers with the app, email to everyone else.
  const sendBlast = async (event: ManagedVolunteerEvent) => {
    // Only confirmed signups receive a blast, so the recipient count can be
    // lower than the signup count — say so up front rather than letting the
    // result look like a bug.
    const subject = window.prompt(
      `Message volunteers at "${event.title}".\n\nOnly volunteers who have confirmed their email will receive it, so this may reach fewer people than the signup count.\n\nSubject:`,
    )
    if (subject === null || subject.trim() === "") return
    const message = window.prompt("Message:")
    if (message === null || message.trim() === "") return

    setBusyId(event.id)
    try {
      const res = await authFetch(`${basePath}/${event.id}/blast`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ subject: subject.trim(), message: message.trim() }),
      })
      if (!res.ok) throw new Error((await res.text()).trim() || "Unable to send the message.")
      const result = await res.json()
      toast({
        title: "Message sent",
        description: `${result.pushed} push notification(s), ${result.emailed} email(s).`,
      })
    } catch (err) {
      toast({
        title: "Could not send message",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
    } finally {
      setBusyId("")
    }
  }

  const pendingCount = events.filter((event) => event.review_status === "pending").length

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {title}
          {canReview && pendingCount > 0 && reviewFilter === "pending" && (
            <Badge className="bg-amber-500">{pendingCount} awaiting</Badge>
          )}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-col gap-2 sm:flex-row">
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search events…"
            className="sm:max-w-xs"
          />
          <Select value={reviewFilter} onValueChange={setReviewFilter}>
            <SelectTrigger className="sm:w-[200px]"><SelectValue /></SelectTrigger>
            <SelectContent>
              {REVIEW_FILTERS.map((option) => (
                <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {loading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" /> Loading…
          </div>
        )}
        {error !== "" && <p className="text-sm text-destructive">{error}</p>}
        {!loading && error === "" && events.length === 0 && (
          <p className="text-sm text-muted-foreground">
            {reviewFilter === "pending" ? "No events are awaiting approval." : "No events here yet."}
          </p>
        )}

        <div className="space-y-3">
          {events.map((event) => (
            <div key={event.id} className="rounded-lg border p-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{event.title}</span>
                    <ReviewBadge event={event} />
                    <QrBadge event={event} />
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {formatEventWhen(event)} · {event.organizer.name}
                    {event.recurrence?.summary ? ` · ${event.recurrence.summary}` : ""}
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {event.signup_count ?? 0}/{event.max_participants} signed up ·{" "}
                    {event.reward_amount_sfluv} SFLUV each ·{" "}
                    reserves {event.reward_amount_sfluv * event.max_participants} SFLUV
                  </div>
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  {event.qr?.codes_generated && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busyId === event.id}
                      onClick={() => downloadCodes(event)}
                    >
                      <Download className="mr-2 h-3.5 w-3.5" />
                      QR codes
                    </Button>
                  )}

                  {event.review_status === "approved" && (event.signup_count ?? 0) > 0 && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busyId === event.id}
                      onClick={() => sendBlast(event)}
                    >
                      <Megaphone className="mr-2 h-3.5 w-3.5" />
                      Message volunteers
                    </Button>
                  )}

                  {canReview && event.review_status === "pending" && (
                    <>
                      <Button size="sm" disabled={busyId === event.id} onClick={() => review(event, "approve")}>
                        <CheckCircle2 className="mr-2 h-3.5 w-3.5" />
                        Approve
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busyId === event.id}
                        onClick={() => review(event, "reject")}
                      >
                        <XCircle className="mr-2 h-3.5 w-3.5" />
                        Reject
                      </Button>
                    </>
                  )}

                  {canReview && event.review_status === "approved" && event.status !== "cancelled" && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive"
                      disabled={busyId === event.id}
                      onClick={() => review(event, "cancel")}
                    >
                      Cancel event
                    </Button>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
