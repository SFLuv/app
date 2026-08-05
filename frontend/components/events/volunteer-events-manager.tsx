"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { AlertTriangle, CheckCircle2, Clock, Download, Leaf, Loader2, Megaphone, Pencil, QrCode, XCircle } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
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
  organizer: { type: string; name: string; logo_url?: string | null }
  cover_photos?: { id: string; url: string; position: number }[]
  description?: string
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
  /** Called when a card is opened, so the parent can offer an edit form. */
  onOpenEvent?: (event: ManagedVolunteerEvent) => void
  /** Shown in the detail panel when the caller supports editing. */
  onEditEvent?: (event: ManagedVolunteerEvent) => void
}

const REVIEW_FILTERS = [
  { value: "all", label: "All events" },
  { value: "pending", label: "Awaiting approval" },
  { value: "approved", label: "Approved" },
  { value: "rejected", label: "Rejected" },
  { value: "cancelled", label: "Cancelled" },
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
  onOpenEvent,
  onEditEvent,
}: VolunteerEventsManagerProps) {
  const { authFetch } = useApp()
  const { toast } = useToast()
  const [events, setEvents] = useState<ManagedVolunteerEvent[]>([])
  const [reviewFilter, setReviewFilter] = useState("all")
  const [search, setSearch] = useState("")
  const [busyId, setBusyId] = useState("")
  const [openEvent, setOpenEvent] = useState<ManagedVolunteerEvent | null>(null)
  const [error, setError] = useState("")
  // First-load-only spinner. The poll below must never blank a list the user is
  // reading, so the placeholder is gated on whether we have ever loaded rather
  // than on whether a request is in flight.
  const [initialLoading, setInitialLoading] = useState(true)
  const hasLoadedRef = useRef(false)
  // Signature of the last rendered payload. State is only replaced when this
  // changes, so a poll that finds nothing new causes no re-render at all — no
  // flash, no scroll jump, no dropdown closing under the user.
  const signatureRef = useRef("")

  const loadEvents = useCallback(async () => {
    setError("")
    try {
      const params = new URLSearchParams({ count: "50" })
      if (reviewFilter !== "all") params.set("review_status", reviewFilter)
      if (search.trim() !== "") params.set("search", search.trim())

      const res = await authFetch(`${basePath}?${params.toString()}`)
      if (!res.ok) throw new Error("Unable to load volunteer events.")
      const data = await res.json()
      const next: ManagedVolunteerEvent[] = data.events || []

      const signature = JSON.stringify(
        next.map((event) => [
          event.id,
          event.review_status,
          event.status,
          event.funding_status,
          event.signup_count,
          event.qr?.codes_generated,
          event.qr?.live,
        ]),
      )
      if (signature !== signatureRef.current) {
        signatureRef.current = signature
        setEvents(next)
        // Keep an open detail panel in step with the poll instead of showing a
        // snapshot from whenever it was opened.
        setOpenEvent((current) => (current ? next.find((event) => event.id === current.id) ?? null : null))
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load volunteer events.")
    } finally {
      hasLoadedRef.current = true
      setInitialLoading(false)
    }
  }, [authFetch, basePath, reviewFilter, search])

  useEffect(() => {
    // A filter or search change is a different query, so the cached signature
    // no longer describes what should be on screen.
    signatureRef.current = ""
    void loadEvents()
  }, [loadEvents])

  // Approval state advances server-side — the maintenance sweep generates
  // recurring occurrences, mints codes after a faucet top-up, and flips QR
  // codes live. Poll for those rather than making the admin reload, and rely on
  // the signature check to stay silent when nothing has moved.
  useEffect(() => {
    const timer = setInterval(() => {
      if (busyId === "") void loadEvents()
    }, 30_000)
    return () => clearInterval(timer)
  }, [loadEvents, busyId])

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
          {canReview && pendingCount > 0 && (
            <Badge className="bg-amber-500">{pendingCount} awaiting approval</Badge>
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

        {initialLoading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" /> Loading…
          </div>
        )}
        {error !== "" && <p className="text-sm text-destructive">{error}</p>}
        {!initialLoading && error === "" && events.length === 0 && (
          <p className="text-sm text-muted-foreground">
            {reviewFilter === "pending" ? "No events are awaiting approval." : "No events here yet."}
          </p>
        )}

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {events.map((event) => {
            const cover = [...(event.cover_photos || [])].sort((a, b) => a.position - b.position)[0]
            return (
              <button
                key={event.id}
                type="button"
                onClick={() => {
                  setOpenEvent(event)
                  onOpenEvent?.(event)
                }}
                // Uniform height regardless of title length or whether an image
                // exists, so a grid of cards never looks ragged.
                className="flex h-full flex-col overflow-hidden rounded-xl border text-left transition-shadow hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40"
              >
                <div className="relative h-32 w-full shrink-0 overflow-hidden bg-muted">
                  {cover ? (
                    // eslint-disable-next-line @next/next/no-img-element -- API host at arbitrary aspect ratios
                    <img src={cover.url} alt="" className="h-full w-full object-cover" />
                  ) : (
                    // Styled filler at exactly the image's size, so a card
                    // without a photo occupies the same space as one with.
                    <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-[#ff8a8a] via-[#eb6c6c] to-[#d55c5c]">
                      <Leaf className="h-8 w-8 text-white/70" />
                    </div>
                  )}
                  <div className="absolute left-2 top-2 flex flex-wrap gap-1">
                    <ReviewBadge event={event} />
                  </div>
                </div>

                <div className="flex min-h-0 flex-1 flex-col gap-2 p-4">
                  <div className="line-clamp-2 font-medium leading-snug">{event.title}</div>
                  <div className="text-xs text-muted-foreground">
                    {formatEventWhen(event)} · {event.organizer.name}
                  </div>
                  {event.recurrence?.summary && (
                    <div className="text-xs text-muted-foreground">{event.recurrence.summary}</div>
                  )}
                  <div className="mt-auto flex flex-wrap items-center gap-2 pt-2 text-xs text-muted-foreground">
                    <QrBadge event={event} />
                    <span>
                      {event.signup_count ?? 0}/{event.max_participants} · {event.reward_amount_sfluv} SFLUV each
                    </span>
                  </div>
                </div>
              </button>
            )
          })}
        </div>
      </CardContent>

      <Dialog open={openEvent !== null} onOpenChange={(next) => !next && setOpenEvent(null)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
          {openEvent && (
            <>
              <DialogHeader>
                <DialogTitle>{openEvent.title}</DialogTitle>
                <DialogDescription>
                  {formatEventWhen(openEvent)} · {openEvent.organizer.name}
                </DialogDescription>
              </DialogHeader>

              <div className="space-y-4">
                <div className="flex flex-wrap gap-2">
                  <ReviewBadge event={openEvent} />
                  <QrBadge event={openEvent} />
                </div>

                {openEvent.description && (
                  <p className="whitespace-pre-line text-sm text-muted-foreground">{openEvent.description}</p>
                )}

                <div className="grid grid-cols-2 gap-3 text-sm">
                  <div>
                    <div className="text-xs text-muted-foreground">Signed up</div>
                    <div>{openEvent.signup_count ?? 0} / {openEvent.max_participants}</div>
                  </div>
                  <div>
                    <div className="text-xs text-muted-foreground">Reward each</div>
                    <div>{openEvent.reward_amount_sfluv} SFLUV</div>
                  </div>
                  {openEvent.recurrence?.summary && (
                    <div className="col-span-2">
                      <div className="text-xs text-muted-foreground">Repeats</div>
                      <div>{openEvent.recurrence.summary}</div>
                    </div>
                  )}
                </div>

                <div className="flex flex-wrap gap-2 border-t pt-4">
                  {onEditEvent && openEvent.status !== "ended" && (
                    <Button variant="outline" size="sm" onClick={() => onEditEvent(openEvent)}>
                      <Pencil className="mr-2 h-3.5 w-3.5" />
                      Edit
                    </Button>
                  )}
                  {openEvent.qr?.codes_generated && (
                    <Button variant="outline" size="sm" disabled={busyId === openEvent.id} onClick={() => downloadCodes(openEvent)}>
                      <Download className="mr-2 h-3.5 w-3.5" />
                      QR codes
                    </Button>
                  )}
                  {openEvent.review_status === "approved" && (openEvent.signup_count ?? 0) > 0 && (
                    <Button variant="outline" size="sm" disabled={busyId === openEvent.id} onClick={() => sendBlast(openEvent)}>
                      <Megaphone className="mr-2 h-3.5 w-3.5" />
                      Message volunteers
                    </Button>
                  )}
                  {canReview && openEvent.review_status === "pending" && (
                    <>
                      <Button size="sm" disabled={busyId === openEvent.id} onClick={() => review(openEvent, "approve")}>
                        <CheckCircle2 className="mr-2 h-3.5 w-3.5" />
                        Approve
                      </Button>
                      <Button variant="outline" size="sm" disabled={busyId === openEvent.id} onClick={() => review(openEvent, "reject")}>
                        <XCircle className="mr-2 h-3.5 w-3.5" />
                        Reject
                      </Button>
                    </>
                  )}
                  {canReview && openEvent.review_status === "approved" && openEvent.status !== "cancelled" && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive"
                      disabled={busyId === openEvent.id}
                      onClick={() => review(openEvent, "cancel")}
                    >
                      Cancel event
                    </Button>
                  )}
                </div>
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
    </Card>
  )
}
