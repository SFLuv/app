"use client"

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react"
import { AlertTriangle, CheckCircle2, Clock, Download, Leaf, Loader2, Megaphone, Pencil, QrCode, XCircle, ChevronRight } from "lucide-react"

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
import { EventBlastModal } from "@/components/events/event-blast-modal"
import { Pagination } from "@/components/opportunities/pagination"
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
  /** Management-only: who made the event. Email is present only when the
      short name is ambiguous within the organization. */
  creator?: { name: string; email?: string } | null
}

interface VolunteerEventsManagerProps {
  /** "/admin/volunteer-events" or "/affiliates/volunteer-events". */
  basePath: string
  /** Admins get approve / reject / cancel; affiliates get a read-only view plus QR download. */
  canReview: boolean
  title?: string
  /** Sits under the title. A node, so a caller can put live figures there. */
  description?: ReactNode
  /**
   * Total awaiting approval across every page.
   *
   * Counting the rows on screen would undercount the moment the list is
   * paginated or filtered, and this badge is a summary of the queue rather
   * than of the current view. Falls back to the visible rows when a caller has
   * no better figure.
   */
  pendingCount?: number
  /** Called when a card is opened, so the parent can offer an edit form. */
  onOpenEvent?: (event: ManagedVolunteerEvent) => void
  /** Shown in the detail panel when the caller supports editing. */
  onEditEvent?: (event: ManagedVolunteerEvent) => void
  /** Rendered on the title line, for the panel's primary action. */
  action?: ReactNode
}

/** Rows per page. Small enough that the panel never becomes the whole screen. */
const PAGE_SIZE = 8

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
  action,
  pendingCount: pendingCountOverride,
}: VolunteerEventsManagerProps) {
  const { authFetch } = useApp()
  const { toast } = useToast()
  const [events, setEvents] = useState<ManagedVolunteerEvent[]>([])
  const [page, setPage] = useState(0)
  const [totalEvents, setTotalEvents] = useState(0)
  const [reviewFilter, setReviewFilter] = useState("all")
  const [search, setSearch] = useState("")
  const [busyId, setBusyId] = useState("")
  const [openEvent, setOpenEvent] = useState<ManagedVolunteerEvent | null>(null)
  const [blastEvent, setBlastEvent] = useState<ManagedVolunteerEvent | null>(null)
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
      const params = new URLSearchParams({ count: String(PAGE_SIZE), page: String(page) })
      if (reviewFilter !== "all") params.set("review_status", reviewFilter)
      if (search.trim() !== "") params.set("search", search.trim())

      const res = await authFetch(`${basePath}?${params.toString()}`)
      if (!res.ok) throw new Error("Unable to load volunteer events.")
      const data = await res.json()
      const next: ManagedVolunteerEvent[] = data.events || []
      setTotalEvents(typeof data.total === "number" ? data.total : next.length)

      const signature = JSON.stringify(
        next.map((event) => [
          event.id,
          event.review_status,
          event.status,
          event.funding_status,
          event.signup_count,
          event.qr?.codes_generated,
          event.qr?.live,
          event.creator?.name,
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
  }, [authFetch, basePath, page, reviewFilter, search])

  // Filtering or searching is a different result set, so any page number from
  // the previous one is meaningless — and page 3 of a set that now has one page
  // renders as an empty panel.
  useEffect(() => {
    setPage(0)
  }, [reviewFilter, search])

  useEffect(() => {
    // A filter, search or page change is a different query, so the cached
    // signature no longer describes what should be on screen.
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

  const pendingCount =
    pendingCountOverride ?? events.filter((event) => event.review_status === "pending").length
  const totalPages = Math.max(1, Math.ceil(totalEvents / PAGE_SIZE))

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="flex flex-wrap items-center gap-2">
              {title}
              {canReview && pendingCount > 0 && (
                <Badge className="border-primary bg-primary text-white hover:bg-primary/90">
                  {pendingCount} awaiting approval
                </Badge>
              )}
            </CardTitle>
            {description ? <CardDescription className="mt-1.5">{description}</CardDescription> : null}
          </div>
          {/* The panel's primary action sits on the title line rather than in a
              card of its own further down the page. */}
          {action ? <div className="shrink-0">{action}</div> : null}
        </div>
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

        <div className="divide-y rounded-lg border">
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
                className="flex w-full items-center gap-4 p-3 text-left transition-colors first:rounded-t-lg last:rounded-b-lg hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/40"
              >
                {/* Fixed thumbnail box so every row is the same height whether
                    or not the event has a photo. */}
                <div className="h-14 w-20 shrink-0 overflow-hidden rounded-md bg-muted">
                  {cover ? (
                    // eslint-disable-next-line @next/next/no-img-element -- API host, arbitrary ratios
                    <img src={cover.url} alt="" className="h-full w-full object-cover" />
                  ) : (
                    <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-[#ff8a8a] via-[#eb6c6c] to-[#d55c5c]">
                      <Leaf className="h-5 w-5 text-white/70" />
                    </div>
                  )}
                </div>

                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="truncate font-medium">{event.title}</span>
                    <ReviewBadge event={event} />
                  </div>
                  <div className="mt-0.5 truncate text-sm text-muted-foreground">
                    {formatEventWhen(event)} · {event.organizer.name}
                    {event.recurrence?.summary ? ` · ${event.recurrence.summary}` : ""}
                  </div>
                </div>

                <div className="hidden shrink-0 flex-col items-end gap-1 sm:flex">
                  <QrBadge event={event} />
                  <span className="text-xs text-muted-foreground">
                    {event.signup_count ?? 0}/{event.max_participants} · {event.reward_amount_sfluv} SFLUV
                  </span>
                </div>

                <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
              </button>
            )
          })}
        </div>

        {/* Only when there is more than one page: a lone page of results does
            not need a control that cannot go anywhere. */}
        {totalPages > 1 && (
          <Pagination
            currentPage={page + 1}
            totalPages={totalPages}
            onPageChange={(next) => setPage(Math.max(0, next - 1))}
          />
        )}

        {totalEvents > 0 && (
          <p className="text-xs text-muted-foreground">
            Showing {events.length} of {totalEvents} event{totalEvents === 1 ? "" : "s"}
          </p>
        )}
      </CardContent>

      <Dialog open={openEvent !== null} onOpenChange={(next) => !next && setOpenEvent(null)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
          {openEvent && (
            <>
              <DialogHeader>
                <DialogTitle>{openEvent.title}</DialogTitle>
                <DialogDescription className="space-y-1">
                  <span className="block">
                    {formatEventWhen(openEvent)} · {openEvent.organizer.name}
                  </span>
                  {openEvent.creator && (
                    <span className="block text-xs">
                      Created by: {openEvent.creator.name}
                      {openEvent.creator.email && (
                        <span className="text-muted-foreground"> ({openEvent.creator.email})</span>
                      )}
                    </span>
                  )}
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
                    <Button variant="outline" size="sm" disabled={busyId === openEvent.id} onClick={() => setBlastEvent(openEvent)}>
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

      {blastEvent && (
        <EventBlastModal
          open
          onOpenChange={(next) => !next && setBlastEvent(null)}
          basePath={basePath}
          eventId={blastEvent.id}
          eventTitle={blastEvent.title}
          signupCount={blastEvent.signup_count ?? 0}
        />
      )}
    </Card>
  )
}
