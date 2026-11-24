"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { AlertTriangle, Loader2 } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { InfoTip } from "@/components/ui/info-tip"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import { cn } from "@/lib/utils"
import type { NotificationHubEntry, NotificationHubView } from "@/types/notifications"

import {
  CATCH_ALL,
  Warning,
  formatWhen,
  isEmailTopic,
  plural,
  reachOf,
  sentence,
  type HubContext,
  type Reach,
  type SendResult,
} from "./notification-hub-shared"
import { NotificationHubRouting } from "./notification-hub-routing"
import { NotificationHubTeamRouting } from "./notification-hub-team-routing"
import { NotificationHubTeams } from "./notification-hub-teams"
import { NotificationHubVisualizer } from "./notification-hub-visualizer"

/**
 * Who gets told when something needs a person.
 *
 * Every notification has a topic. The house organization's admins give topics
 * to teams of its members; each team's lead and admins decide where each topic
 * goes inside the team: everyone, particular people, a shared address, group
 * chats. Until they do, everyone on the team gets it in the app and on their
 * phone.
 *
 * The failure this page exists to make visible is a notification that is
 * posted, reaches nobody, and looks exactly like nothing having gone wrong, so
 * every place a topic would reach nobody says so.
 *
 * Receiving a notification is not permission to act on it. Approve and reject
 * still only work for administrators.
 */

/** How many recent notifications show before "Show all". */
const RECENT_PREVIEW = 15
/** How often the Visualizer tab reloads the hub while it is open and visible. */
const LIVE_INTERVAL_MS = 10_000

type Tab = "teams" | "routing" | "team-routing" | "recent" | "visualizer"

export function NotificationHub() {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [view, setView] = useState<NotificationHubView | null>(null)
  const [loading, setLoading] = useState(true)
  const [forbidden, setForbidden] = useState(false)
  const [failed, setFailed] = useState(false)
  const [tab, setTab] = useState<Tab | null>(null)
  const [recentFilter, setRecentFilter] = useState<"all" | "nobody">("all")
  const [showAllRecent, setShowAllRecent] = useState(false)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  // Changes are optimistic, then the view is reloaded as the correction. A
  // load that was already running when a change began would put back what the
  // change replaced, so a load only lands when no change is in flight and none
  // started while it ran.
  const inflight = useRef(0)
  const generation = useRef(0)

  const load = useCallback(async () => {
    const started = generation.current
    try {
      const res = await authFetch("/organizations/mine/notification-hub")
      if (!res.ok) {
        if (res.status === 403 || res.status === 404) setForbidden(true)
        else setFailed(true)
        return
      }
      const next = (await res.json()) as NotificationHubView
      setFailed(false)
      setForbidden(false)
      if (inflight.current === 0 && generation.current === started) {
        setView(next)
        setUpdatedAt(new Date())
      }
    } catch {
      setFailed(true)
    } finally {
      setLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  // The Visualizer is a live view: while it is open, and only while the page is
  // being looked at, the hub is reloaded on a short timer.
  useEffect(() => {
    if (tab !== "visualizer") return
    const tick = () => {
      if (document.visibilityState === "visible") void load()
    }
    tick()
    const timer = window.setInterval(tick, LIVE_INTERVAL_MS)
    document.addEventListener("visibilitychange", tick)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener("visibilitychange", tick)
    }
  }, [tab, load])

  const refreshNow = async () => {
    setRefreshing(true)
    try {
      await load()
    } finally {
      setRefreshing(false)
    }
  }

  const change = useCallback(
    async <T,>(work: () => Promise<T>): Promise<T> => {
      inflight.current += 1
      generation.current += 1
      try {
        return await work()
      } finally {
        inflight.current -= 1
        if (inflight.current === 0) void load()
      }
    },
    [load],
  )

  const send = useCallback(
    async (path: string, method: string, body?: unknown): Promise<SendResult> => {
      try {
        const res = await authFetch(path, {
          method,
          headers: body === undefined ? undefined : { "Content-Type": "application/json" },
          body: body === undefined ? undefined : JSON.stringify(body),
        })
        if (res.ok) {
          const text = await res.text()
          let data: unknown = null
          try {
            data = text ? JSON.parse(text) : null
          } catch {
            data = null
          }
          return { ok: true, data }
        }
        const text = (await res.text()).trim()
        return {
          ok: false,
          error: sentence(text) || (res.status === 403 ? "You can't change this." : "Please try again."),
        }
      } catch {
        return { ok: false, error: "Could not reach the server. Please try again." }
      }
    },
    [authFetch],
  )

  const fail = useCallback(
    (title: string, error: string) => toast({ title, description: error, variant: "destructive" }),
    [toast],
  )

  // ── Derived ────────────────────────────────────────────────────────────────

  const teamsById = useMemo(() => new Map((view?.teams ?? []).map((team) => [team.id, team])), [view])
  const memberById = useMemo(
    () => new Map((view?.members ?? []).map((member) => [member.user_id, member])),
    [view],
  )
  const orgMemberIds = useMemo(() => new Set(memberById.keys()), [memberById])
  const routes = useMemo(() => view?.routes ?? {}, [view])

  const reachByTopic = useMemo(() => {
    const map = new Map<string, Reach>()
    for (const topic of view?.topics ?? []) {
      map.set(topic.id, reachOf(topic.id, routes, teamsById, orgMemberIds))
    }
    return map
  }, [view, routes, teamsById, orgMemberIds])

  const nobodyTopics = useMemo(
    () => (view?.topics ?? []).filter((topic) => reachByTopic.get(topic.id)?.kind === "nobody"),
    [view, reachByTopic],
  )

  const chatNames = useMemo(() => {
    const map = new Map<number, string>()
    for (const team of view?.teams ?? []) {
      for (const chat of team.chats ?? []) map.set(chat.id, chat.title)
    }
    return map
  }, [view])

  const hub: HubContext = useMemo(
    () => ({ teamsById, memberById, orgMemberIds, send, fail, change }),
    [teamsById, memberById, orgMemberIds, send, fail, change],
  )

  // ── Render ─────────────────────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="flex min-h-[40vh] items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (forbidden || (!view && !failed)) {
    return (
      <div className="space-y-3">
        <h1 className="text-2xl font-semibold text-black dark:text-white">Notification Hub</h1>
        <p className="text-sm text-muted-foreground">
          The Notification Hub is for SFLuv&apos;s organization admins and team leads.
        </p>
      </div>
    )
  }

  if (!view) {
    return (
      <div className="space-y-3">
        <h1 className="text-2xl font-semibold text-black dark:text-white">Notification Hub</h1>
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-sm text-muted-foreground">The Notification Hub could not be loaded.</p>
          <Button size="sm" variant="outline" onClick={() => void refreshNow()} disabled={refreshing}>
            {refreshing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
            Retry
          </Button>
        </div>
      </div>
    )
  }

  const admin = view.can_edit
  const tabs: Array<{ id: Tab; label: string }> = admin
    ? [
        { id: "teams", label: "Teams" },
        { id: "routing", label: "Who gets what" },
        { id: "team-routing", label: "Team routing" },
        { id: "recent", label: "Recent" },
        { id: "visualizer", label: "Visualizer" },
      ]
    : [
        { id: "team-routing", label: "Team routing" },
        { id: "recent", label: "Recent" },
      ]
  const activeTab: Tab = tab && tabs.some((entry) => entry.id === tab) ? tab : tabs[0].id

  const recent = view.recent ?? []
  const recentNobody = recent.filter((entry) => entry.delivery?.reached_nobody)
  const recentShown = recentFilter === "nobody" ? recentNobody : recent
  const recentVisible = showAllRecent ? recentShown : recentShown.slice(0, RECENT_PREVIEW)
  const emailTopics = view.topics.filter((topic) => isEmailTopic(topic.id))

  const showReachedNobody = () => {
    setRecentFilter("nobody")
    setShowAllRecent(true)
    setTab("recent")
  }

  const warnings = [
    admin && view.reached_nobody > 0 ? (
      <Warning key="nobody" tone="danger">
        <span className="font-medium text-black dark:text-white">
          {view.reached_nobody} {plural(view.reached_nobody, "notification", "notifications")} in the last 30 days
          reached nobody.
        </span>{" "}
        <button type="button" className="font-medium text-primary underline-offset-4 hover:underline" onClick={showReachedNobody}>
          Show them
        </button>
      </Warning>
    ) : null,
    !view.inbound_email ? (
      <Warning key="email" tone="warning">
        <span className="font-medium text-black dark:text-white">Email can&apos;t come in yet.</span>{" "}
        <span className="text-muted-foreground">
          Until inbound email is set up on the server,{" "}
          {emailTopics.length > 0 ? emailTopics.map((topic) => topic.label).join(", ") : "the email topics"} will never
          receive anything, whoever they go to.
        </span>
      </Warning>
    ) : null,
    admin && nobodyTopics.length > 0 ? (
      <Warning key="topics" tone="warning">
        <span className="font-medium text-black dark:text-white">
          {nobodyTopics.length === view.topics.length
            ? "Every kind of notification would reach nobody right now."
            : `${nobodyTopics.length} ${plural(nobodyTopics.length, "kind", "kinds")} of notification would reach nobody right now.`}
        </span>{" "}
        <button
          type="button"
          className="font-medium text-primary underline-offset-4 hover:underline"
          onClick={() => setTab("routing")}
        >
          Who gets what
        </button>
      </Warning>
    ) : null,
  ].filter(Boolean)

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-2xl font-semibold text-black dark:text-white">Notification Hub</h1>
        <InfoTip label="About the Notification Hub">
          Everything that needs a person lands here under a topic. Organization admins give each topic to one or more
          teams; each team&apos;s lead and admins choose where it goes inside the team. Until they do, everyone on the
          team gets it in the app and on their phone. Getting a notification is not permission to act on it: approve
          and reject only work for administrators.
        </InfoTip>
      </div>

      {warnings.length > 0 ? <div className="space-y-2">{warnings}</div> : null}

      <Tabs value={activeTab} onValueChange={(value) => setTab(value as Tab)} className="space-y-6">
        <TabsList className="h-auto flex-wrap justify-start">
          {tabs.map((entry) => (
            <TabsTrigger key={entry.id} value={entry.id}>
              {entry.label}
            </TabsTrigger>
          ))}
        </TabsList>

        {admin ? (
          <>
            <TabsContent value="teams" className="mt-0">
              <NotificationHubTeams view={view} hub={hub} reachByTopic={reachByTopic} />
            </TabsContent>
            <TabsContent value="routing" className="mt-0">
              <NotificationHubRouting view={view} hub={hub} reachByTopic={reachByTopic} />
            </TabsContent>
            <TabsContent value="visualizer" className="mt-0">
              <NotificationHubVisualizer
                view={view}
                reachByTopic={reachByTopic}
                teamsById={teamsById}
                orgMemberIds={orgMemberIds}
                memberById={memberById}
                updatedAt={updatedAt}
                refreshing={refreshing}
                onRefresh={() => void refreshNow()}
              />
            </TabsContent>
          </>
        ) : null}

        <TabsContent value="team-routing" className="mt-0">
          <NotificationHubTeamRouting view={view} hub={hub} />
        </TabsContent>

        <TabsContent value="recent" className="mt-0">
          <section className="space-y-4">
            {recent.length === 0 ? (
              <p className="text-sm text-muted-foreground">Nothing yet.</p>
            ) : (
              <>
                <div className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    variant={recentFilter === "all" ? "secondary" : "ghost"}
                    onClick={() => setRecentFilter("all")}
                    aria-pressed={recentFilter === "all"}
                  >
                    All {recent.length}
                  </Button>
                  <Button
                    size="sm"
                    variant={recentFilter === "nobody" ? "secondary" : "ghost"}
                    onClick={() => setRecentFilter("nobody")}
                    aria-pressed={recentFilter === "nobody"}
                    className={recentNobody.length > 0 ? "text-destructive" : undefined}
                  >
                    Reached nobody {recentNobody.length}
                  </Button>
                </div>

                {recentShown.length === 0 ? (
                  <p className="text-sm text-muted-foreground">None of the recent notifications reached nobody.</p>
                ) : (
                  <div className="space-y-2">
                    {recentVisible.map((entry) => (
                      <RecentRow key={entry.id} entry={entry} chatNames={chatNames} />
                    ))}
                  </div>
                )}

                {recentShown.length > RECENT_PREVIEW ? (
                  <Button variant="outline" size="sm" onClick={() => setShowAllRecent((current) => !current)}>
                    {showAllRecent ? "Show fewer" : `Show all ${recentShown.length}`}
                  </Button>
                ) : null}
              </>
            )}
          </section>
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ── Recent ───────────────────────────────────────────────────────────────────

function statusOf(entry: NotificationHubEntry): { label: string; variant: "warning" | "success" | "secondary"; note: string } {
  if (entry.resolution === "info") return { label: "Info", variant: "secondary", note: "" }
  if (entry.resolved_at) {
    const fallback =
      entry.resolution === "cleared"
        ? "Cleared on its own"
        : entry.resolution === "acked"
          ? "Marked as seen"
          : entry.resolution === "acted"
            ? "Handled"
            : ""
    return { label: "Resolved", variant: "success", note: entry.outcome || fallback }
  }
  return { label: "Live", variant: "warning", note: "" }
}

function RecentRow({ entry, chatNames }: { entry: NotificationHubEntry; chatNames: Map<number, string> }) {
  const status = statusOf(entry)
  const delivery = entry.delivery
  const nobody = delivery?.reached_nobody === true
  const deliveredTeams = delivery?.teams ?? []
  const chats = delivery?.chats ?? []
  const emailed = delivery?.emailed ?? []
  const failures = delivery?.failures ?? []

  return (
    <div className={cn("space-y-2 rounded-lg border px-4 py-3", nobody && "border-destructive/50 bg-destructive/5")}>
      <div className="flex flex-wrap items-center gap-1.5 text-xs">
        <span className="text-muted-foreground">{formatWhen(entry.created_at)}</span>
        <Badge variant="outline" className="max-w-full font-normal">
          <span className="truncate">{entry.topic_label || entry.topic || "No topic"}</span>
        </Badge>
        <Badge variant={status.variant}>{status.label}</Badge>
        {status.note ? <span className="text-muted-foreground">{status.note}</span> : null}
      </div>

      <div className="min-w-0">
        <p className="break-words text-sm font-medium text-black dark:text-white">{entry.title}</p>
        {entry.body ? <p className="line-clamp-2 break-words text-xs text-muted-foreground">{entry.body}</p> : null}
      </div>

      {!delivery ? (
        <p className="text-xs text-muted-foreground">Where it went was not recorded.</p>
      ) : (
        <div className="space-y-1 text-xs">
          {nobody ? (
            <p className="flex items-center gap-1 font-medium text-destructive">
              <AlertTriangle className="h-3.5 w-3.5" />
              Reached nobody
              {deliveredTeams.length > 0
                ? ` (sent to ${deliveredTeams.map((team) => team.name).join(", ")}, which reached nobody)`
                : ""}
            </p>
          ) : (
            <p className="text-muted-foreground">
              <span className="text-foreground">
                {deliveredTeams.length > 0 ? deliveredTeams.map((team) => team.name).join(", ") : "No team"}
              </span>
              {delivery.catch_all ? " via catch-all" : ""}
              {" · "}
              {delivery.recipients} {plural(delivery.recipients, "person", "people")}, {delivery.pushed} pushed
              {emailed.length > 0 ? ` · Emailed ${emailed.join(", ")}` : ""}
              {chats.length > 0 ? ` · Posted in ${chats.map((id) => chatNames.get(id) || `chat ${id}`).join(", ")}` : ""}
            </p>
          )}
          {failures.length > 0 ? (
            <ul className="space-y-1 text-destructive">
              {failures.map((failure, index) => (
                <li key={index} className="break-words">
                  {failure}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      )}
    </div>
  )
}

export { CATCH_ALL }
