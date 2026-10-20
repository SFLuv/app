"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { Archive, Inbox, Loader2, Mail, RefreshCw, Undo2 } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"

/**
 * Support requests from the website's Get in touch form.
 *
 * These used to be a mailto: link, so there was no record of them: whether one
 * was answered depended on whose inbox it reached, and the sender had nothing
 * to quote if they followed up. Every request now has a reference, which the
 * sender is emailed and which appears in the Telegram alert, so a reply can be
 * matched to a request by anyone.
 *
 * Archiving is the only state change. Nothing is deleted, because the common
 * question about a support request is what was said three months ago.
 */

type SupportRequest = {
  id: number
  reference: string
  email: string
  name: string
  subject: string
  message: string
  source: string
  status: "new" | "archived"
  created_at: string
  archived_at?: string | null
  archived_by?: string
}

type Filter = "new" | "archived" | "all"

const when = (iso: string) => {
  const date = new Date(iso)
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`
}

export function SupportPanel() {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [requests, setRequests] = useState<SupportRequest[]>([])
  const [filter, setFilter] = useState<Filter>("new")
  const [loading, setLoading] = useState(true)
  const [busyId, setBusyId] = useState<number | null>(null)
  const [unavailable, setUnavailable] = useState("")

  const load = useCallback(async () => {
    setLoading(true)
    try {
      // Always fetch everything and filter here: the counts on the filter
      // buttons are the useful part, and they need both sets anyway.
      const res = await authFetch("/admin/support-requests")
      if (!res.ok) {
        setUnavailable((await res.text()).trim() || "Support requests are unavailable.")
        return
      }
      setUnavailable("")
      setRequests(((await res.json()) as SupportRequest[]) ?? [])
    } catch {
      setUnavailable("Could not reach the server.")
    } finally {
      setLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  const setArchived = async (request: SupportRequest, archived: boolean) => {
    setBusyId(request.id)
    try {
      const res = await authFetch(`/admin/support-requests/${request.id}/archive`, {
        method: "POST",
        body: JSON.stringify({ archived }),
      })
      if (!res.ok) {
        toast({ title: archived ? "Could not archive" : "Could not reopen", variant: "destructive" })
        return
      }
      const updated = (await res.json()) as SupportRequest
      setRequests((current) => current.map((r) => (r.id === updated.id ? updated : r)))
      toast({ title: archived ? `Archived ${updated.reference}` : `Reopened ${updated.reference}` })
    } finally {
      setBusyId(null)
    }
  }

  const counts = useMemo(
    () => ({
      new: requests.filter((r) => r.status === "new").length,
      archived: requests.filter((r) => r.status === "archived").length,
      all: requests.length,
    }),
    [requests]
  )

  const shown = useMemo(() => {
    if (filter === "all") return requests
    return requests.filter((r) => r.status === filter)
  }, [requests, filter])

  if (loading && requests.length === 0) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading support requests…
      </div>
    )
  }

  if (unavailable) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base text-black dark:text-white">Support</CardTitle>
          <CardDescription>{unavailable}</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-black dark:text-white">Support</h2>
          <p className="text-sm text-muted-foreground">
            {counts.new === 0
              ? "Nothing outstanding."
              : `${counts.new} request${counts.new === 1 ? "" : "s"} waiting for a reply.`}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {(["new", "archived", "all"] as Filter[]).map((option) => (
            <Button
              key={option}
              size="sm"
              variant={filter === option ? "default" : "outline"}
              onClick={() => setFilter(option)}
            >
              {option === "new" ? "Open" : option === "archived" ? "Archived" : "All"}
              <span className="ml-2 tabular-nums opacity-70">{counts[option]}</span>
            </Button>
          ))}
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            <RefreshCw className={`mr-2 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </Button>
        </div>
      </div>

      {shown.length === 0 ? (
        <Card>
          <CardContent className="flex items-center gap-3 py-10 text-sm text-muted-foreground">
            <Inbox className="h-5 w-5" />
            {filter === "new" ? "No open requests." : "Nothing here."}
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {shown.map((request) => {
            const subject = request.subject || "(no subject)"
            // The reference goes in the reply's subject so a follow-up can be
            // matched back without anyone searching for it.
            const mailto = `mailto:${encodeURIComponent(request.email)}?subject=${encodeURIComponent(
              `Re: ${subject} [${request.reference}]`
            )}`
            return (
              <Card key={request.id} className={request.status === "archived" ? "opacity-70" : undefined}>
                <CardHeader className="pb-3">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0">
                      <CardTitle className="flex flex-wrap items-center gap-2 text-base text-black dark:text-white">
                        {subject}
                        {request.status === "archived" ? (
                          <Badge variant="outline" className="text-muted-foreground">
                            Archived
                          </Badge>
                        ) : (
                          <Badge variant="secondary">Open</Badge>
                        )}
                      </CardTitle>
                      <CardDescription className="mt-1">
                        <span className="font-mono text-xs">{request.reference}</span>
                        {" · "}
                        {request.name ? `${request.name} · ` : ""}
                        {request.email}
                        {" · "}
                        {when(request.created_at)}
                      </CardDescription>
                    </div>
                    <div className="flex gap-2">
                      <Button asChild variant="outline" size="sm">
                        <a href={mailto}>
                          <Mail className="mr-2 h-3.5 w-3.5" />
                          Respond
                        </a>
                      </Button>
                      {request.status === "new" ? (
                        <Button size="sm" onClick={() => void setArchived(request, true)} disabled={busyId === request.id}>
                          {busyId === request.id ? (
                            <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <Archive className="mr-2 h-3.5 w-3.5" />
                          )}
                          Archive
                        </Button>
                      ) : (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => void setArchived(request, false)}
                          disabled={busyId === request.id}
                        >
                          <Undo2 className="mr-2 h-3.5 w-3.5" />
                          Reopen
                        </Button>
                      )}
                    </div>
                  </div>
                </CardHeader>
                <CardContent>
                  {/* pre-wrap, not a paragraph: people write in with line breaks
                      and lists, and collapsing them loses the shape of what
                      they said. */}
                  <p className="whitespace-pre-wrap text-sm text-black dark:text-white">{request.message}</p>
                  {request.status === "archived" && request.archived_by ? (
                    <p className="mt-3 text-xs text-muted-foreground">
                      Archived by {request.archived_by}
                      {request.archived_at ? ` · ${when(request.archived_at)}` : ""}
                    </p>
                  ) : null}
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}
