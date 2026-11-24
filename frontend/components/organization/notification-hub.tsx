"use client"

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { format } from "date-fns"
import {
  AlertTriangle,
  Bell,
  ChevronDown,
  Loader2,
  Mail,
  MessageSquare,
  Pencil,
  Plus,
  Smartphone,
  Trash2,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import { cn } from "@/lib/utils"
import type {
  NotificationHubEntry,
  NotificationHubMember,
  NotificationHubView,
  NotificationTeam,
  NotificationTeamInput,
} from "@/types/notifications"

import { NotificationHubVisualizer } from "./notification-hub-visualizer"

/**
 * Who gets told when something needs a person.
 *
 * Every notification has a topic, topics are given to teams, and a team is a
 * set of SFLuv organization members plus how they want to be told. The failure
 * this screen exists to make visible is a notification that is posted, reaches
 * nobody, and looks exactly like nothing having gone wrong — so every place a
 * topic would reach nobody says so, in the warnings at the top, on the topic
 * itself, and on each recent notification that did.
 *
 * Receiving a notification is not permission to act on it. Approve and reject
 * still only work for administrators.
 */

/** The catch-all route: teams here get every topic that has no team. */
const CATCH_ALL = "unrouted"
/** How many recent notifications show before "Show all". */
const RECENT_PREVIEW = 15
/** How often the Visualizer tab reloads the hub while it is open and visible. */
const LIVE_INTERVAL_MS = 10_000
const NAME_MAX = 80
const DESCRIPTION_MAX = 300

export type Reach =
  | { kind: "teams" }
  | { kind: "catch-all" }
  | { kind: "nobody"; reason: string }

const memberLabel = (member: NotificationHubMember | undefined) =>
  member?.name || member?.email || "Unnamed member"

const plural = (count: number, one: string, many: string) => (count === 1 ? one : many)

/** Backend refusals arrive lowercase ("there is already a team called that"). */
const sentence = (text: string) => (text ? text.charAt(0).toUpperCase() + text.slice(1) : text)

const isEmailTopic = (topicId: string) => topicId.startsWith("email.")

function formatWhen(iso: string) {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ""
  return format(date, date.getFullYear() === new Date().getFullYear() ? "MMM d, h:mm a" : "MMM d yyyy, h:mm a")
}

/**
 * Where one topic would go right now. Mirrors the backend's delivery plan: a
 * topic's own teams, falling back to the catch-all's when it has none or when
 * its own hold no member, address or chat, and nobody when neither reaches
 * anyone. `without` previews deleting a team.
 */
function reachOf(
  topicId: string,
  routes: Record<string, number[]>,
  teamsById: Map<number, NotificationTeam>,
  orgMemberIds: Set<string>,
  without?: number,
): Reach {
  const teamsFor = (id: string) =>
    (routes[id] ?? [])
      .filter((teamId) => teamId !== without)
      .map((teamId) => teamsById.get(teamId))
      .filter((team): team is NotificationTeam => team !== undefined)
  const reachesSomeone = (team: NotificationTeam) =>
    team.member_ids.some((id) => orgMemberIds.has(id)) || !!team.email_address || !!team.chat_conversation_id

  const own = teamsFor(topicId)
  if (own.some(reachesSomeone)) return { kind: "teams" }
  if (topicId === CATCH_ALL) {
    return { kind: "nobody", reason: own.length > 0 ? "Its teams have nobody on them." : "No team is set." }
  }
  const catchAll = teamsFor(CATCH_ALL)
  if (catchAll.some(reachesSomeone)) return { kind: "catch-all" }
  if (own.length > 0) {
    return { kind: "nobody", reason: (own.length === 1 ? "Its team has" : "Its teams have") + " nobody on it, and no catch-all can take it." }
  }
  return {
    kind: "nobody",
    reason: catchAll.length === 0 ? "No team gets this, and there is no catch-all." : "No team gets this, and the catch-all has nobody on it.",
  }
}

type SendResult = { ok: true; data: unknown } | { ok: false; error: string }

export function NotificationHub() {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [view, setView] = useState<NotificationHubView | null>(null)
  const [loading, setLoading] = useState(true)
  const [savingTopics, setSavingTopics] = useState<Set<string>>(new Set())
  const [teamDialog, setTeamDialog] = useState<{ team: NotificationTeam | null } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<NotificationTeam | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [recentFilter, setRecentFilter] = useState<"all" | "nobody">("all")
  const [showAllRecent, setShowAllRecent] = useState(false)
  const [tab, setTab] = useState<"settings" | "visualizer">("settings")
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  const recentRef = useRef<HTMLDivElement>(null)
  const routingRef = useRef<HTMLDivElement>(null)

  // Changes are optimistic, then the view is reloaded as the correction. A
  // load that was already running when a change began would put back what the
  // change replaced, so a load only lands when no change is in flight and none
  // started while it ran.
  const inflight = useRef(0)
  const generation = useRef(0)
  // Each topic's route saves go out one at a time, the latest choice last, so
  // two quick clicks cannot arrive out of order and leave the first one saved.
  const routeQueue = useRef(new Map<string, number[]>())
  const routeSending = useRef(new Set<string>())

  const load = useCallback(async () => {
    const started = generation.current
    try {
      const res = await authFetch("/organizations/mine/notification-hub")
      if (!res.ok) {
        // 403 is a partner organization: the hub belongs to SFLuv's own.
        if (res.status === 403 || res.status === 404) setView(null)
        return
      }
      const next = (await res.json()) as NotificationHubView
      if (inflight.current === 0 && generation.current === started) {
        setView(next)
        setUpdatedAt(new Date())
      }
    } catch {
      // Additive panel: a failure here leaves the rest of the page usable.
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

  const begin = () => {
    inflight.current += 1
    generation.current += 1
  }
  const end = () => {
    inflight.current -= 1
    if (inflight.current === 0) void load()
  }

  const send = async (path: string, method: string, body?: unknown): Promise<SendResult> => {
    try {
      const res = await authFetch(path, {
        method,
        headers: body === undefined ? undefined : { "Content-Type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      })
      if (res.ok) {
        const data: unknown = res.status === 201 ? await res.json().catch(() => null) : null
        return { ok: true, data }
      }
      const text = (await res.text()).trim()
      return {
        ok: false,
        error: sentence(text) || (res.status === 403 ? "Only organization admins can change this." : "Please try again."),
      }
    } catch {
      return { ok: false, error: "Could not reach the server. Please try again." }
    }
  }

  const fail = (title: string, error: string) => toast({ title, description: error, variant: "destructive" })

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

  /** Topics grouped under their set, in the backend's order. */
  const groups = useMemo(() => {
    if (!view) return []
    const known = new Set(view.sets.map((set) => set.id))
    const out = view.sets
      .map((set) => ({ set, topics: view.topics.filter((topic) => topic.set === set.id) }))
      .filter((group) => group.topics.length > 0)
    const other = view.topics.filter((topic) => !known.has(topic.set))
    if (other.length > 0) {
      out.push({ set: { id: "other", label: "Other", description: "" }, topics: other })
    }
    return out
  }, [view])

  const chatNames = useMemo(() => {
    const map = new Map<number, string>()
    for (const team of view?.teams ?? []) {
      if (team.chat_conversation_id && team.chat_name) map.set(team.chat_conversation_id, team.chat_name)
    }
    return map
  }, [view])

  // ── Changes ────────────────────────────────────────────────────────────────

  const routeTeams = (topicId: string) => (routes[topicId] ?? []).filter((id) => teamsById.has(id))

  const flushRoute = async (topicId: string) => {
    routeSending.current.add(topicId)
    setSavingTopics((current) => new Set(current).add(topicId))
    begin()
    try {
      let next = routeQueue.current.get(topicId)
      while (next !== undefined) {
        routeQueue.current.delete(topicId)
        const result = await send("/organizations/mine/notification-routes", "PUT", {
          topic: topicId,
          team_ids: next,
        })
        if (!result.ok) {
          routeQueue.current.delete(topicId)
          fail("Could not save who gets this", result.error)
          break
        }
        next = routeQueue.current.get(topicId)
      }
    } finally {
      routeSending.current.delete(topicId)
      setSavingTopics((current) => {
        const copy = new Set(current)
        copy.delete(topicId)
        return copy
      })
      end()
    }
  }

  const setRoute = (topicId: string, teamIds: number[]) => {
    setView((current) => (current ? { ...current, routes: { ...current.routes, [topicId]: teamIds } } : current))
    routeQueue.current.set(topicId, teamIds)
    if (!routeSending.current.has(topicId)) void flushRoute(topicId)
  }

  const toggleRoute = (topicId: string, teamId: number, on: boolean) => {
    const current = routeTeams(topicId)
    setRoute(topicId, on ? [...current.filter((id) => id !== teamId), teamId] : current.filter((id) => id !== teamId))
  }

  const saveTeam = async (
    team: NotificationTeam | null,
    input: NotificationTeamInput,
    memberIds: string[],
  ): Promise<boolean> => {
    // Somebody who has left the organization can still be named on a team row,
    // and the backend refuses a member list that includes them.
    const members = memberIds.filter((id) => orgMemberIds.has(id))
    begin()
    try {
      if (!team) {
        const created = await send("/organizations/mine/notification-teams", "POST", input)
        if (!created.ok) {
          fail("Could not create the team", created.error)
          return false
        }
        const id = (created.data as { id?: number } | null)?.id
        if (members.length > 0 && id) {
          const saved = await send(`/organizations/mine/notification-teams/${id}/members`, "PUT", { user_ids: members })
          if (!saved.ok) {
            fail("The team was created, but its members were not saved", saved.error)
            return true
          }
        }
        toast({ title: `Created ${input.name}` })
        return true
      }

      const detailsChanged =
        input.name !== team.name ||
        input.description !== team.description ||
        input.push_members !== team.push_members ||
        input.email_members !== team.email_members ||
        input.email_address !== team.email_address ||
        input.chat_conversation_id !== (team.chat_conversation_id ?? null)
      if (detailsChanged) {
        const saved = await send(`/organizations/mine/notification-teams/${team.id}`, "PUT", input)
        if (!saved.ok) {
          fail("Could not save the team", saved.error)
          return false
        }
      }
      const before = team.member_ids.filter((id) => orgMemberIds.has(id))
      const membersChanged =
        before.length !== members.length ||
        before.some((id) => !members.includes(id)) ||
        team.member_ids.length !== before.length
      if (membersChanged) {
        const saved = await send(`/organizations/mine/notification-teams/${team.id}/members`, "PUT", {
          user_ids: members,
        })
        if (!saved.ok) {
          fail("Could not save who is on the team", saved.error)
          return false
        }
      }
      toast({ title: `Saved ${input.name}` })
      return true
    } finally {
      end()
    }
  }

  const confirmDelete = async () => {
    const team = deleteTarget
    if (!team) return
    setDeleting(true)
    begin()
    try {
      const result = await send(`/organizations/mine/notification-teams/${team.id}`, "DELETE")
      if (!result.ok) {
        fail("Could not delete the team", result.error)
        return
      }
      setView((current) =>
        current
          ? {
              ...current,
              teams: current.teams.filter((entry) => entry.id !== team.id),
              routes: Object.fromEntries(
                Object.entries(current.routes ?? {}).map(([topic, ids]) => [topic, ids.filter((id) => id !== team.id)]),
              ),
            }
          : current,
      )
      toast({ title: `Deleted ${team.name}` })
      setDeleteTarget(null)
    } finally {
      setDeleting(false)
      end()
    }
  }

  const showReachedNobody = () => {
    setRecentFilter("nobody")
    setShowAllRecent(true)
    requestAnimationFrame(() => recentRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }))
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  if (loading || !view) return null

  const editable = view.can_edit
  const teams = view.teams ?? []
  const members = view.members ?? []
  const recent = view.recent ?? []
  const catchAllReach = reachOf(CATCH_ALL, routes, teamsById, orgMemberIds)
  const emailTopics = view.topics.filter((topic) => isEmailTopic(topic.id))

  const recentNobody = recent.filter((entry) => entry.delivery?.reached_nobody)
  const recentShown = recentFilter === "nobody" ? recentNobody : recent
  const recentVisible = showAllRecent ? recentShown : recentShown.slice(0, RECENT_PREVIEW)

  const deletePreview = deleteTarget
    ? view.topics.filter(
        (topic) =>
          reachByTopic.get(topic.id)?.kind !== "nobody" &&
          reachOf(topic.id, routes, teamsById, orgMemberIds, deleteTarget.id).kind === "nobody",
      )
    : []

  return (
    <Tabs value={tab} onValueChange={(value) => setTab(value === "visualizer" ? "visualizer" : "settings")} className="space-y-4">
      <TabsList>
        <TabsTrigger value="settings">Settings</TabsTrigger>
        <TabsTrigger value="visualizer">Visualizer</TabsTrigger>
      </TabsList>

      <TabsContent value="visualizer" className="mt-0">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base text-black dark:text-white">
              <Bell className="h-4 w-4" />
              Notification map
            </CardTitle>
            <CardDescription>
              Every way in, the topic each one posts under, and who would hear about it right now. It updates by
              itself while this tab is open.
            </CardDescription>
          </CardHeader>
          <CardContent>
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
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="settings" className="mt-0">
        <div className="space-y-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base text-black dark:text-white">
                <Bell className="h-4 w-4" />
                Notifications
              </CardTitle>
              <CardDescription>
                Everything that needs a person lands here under a topic. Each topic goes to one or more teams, and each
                team chooses how it is told. Getting a notification is not permission to act on it: approve and reject
                only work for administrators.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              {view.reached_nobody > 0 || !view.inbound_email || nobodyTopics.length > 0 ? (
                <div className="space-y-2">
                  {view.reached_nobody > 0 ? (
                    <Warning tone="danger">
                      <span className="font-medium text-black dark:text-white">
                        {view.reached_nobody} {plural(view.reached_nobody, "notification", "notifications")} in the last 30
                        days reached nobody.
                      </span>{" "}
                      <span className="text-muted-foreground">
                        They were raised, but no person, address or chat was told.
                      </span>{" "}
                      <button type="button" className="font-medium text-primary underline-offset-4 hover:underline" onClick={showReachedNobody}>
                        Show them
                      </button>
                    </Warning>
                  ) : null}
                  {!view.inbound_email ? (
                    <Warning tone="warning">
                      <span className="font-medium text-black dark:text-white">Email can&apos;t come in yet.</span>{" "}
                      <span className="text-muted-foreground">
                        Until inbound email is set up on the server,{" "}
                        {emailTopics.length > 0
                          ? emailTopics.map((topic) => topic.label).join(", ")
                          : "the email topics"}{" "}
                        will never receive anything, whoever they go to.
                      </span>
                    </Warning>
                  ) : null}
                  {nobodyTopics.length > 0 ? (
                    <Warning tone="warning">
                      <span className="font-medium text-black dark:text-white">
                        {nobodyTopics.length === view.topics.length
                          ? "Every kind of notification would reach nobody right now."
                          : `${nobodyTopics.length} ${plural(nobodyTopics.length, "kind", "kinds")} of notification would reach nobody right now.`}
                      </span>{" "}
                      <span className="text-muted-foreground">
                        {nobodyTopics.length <= 8 && nobodyTopics.length < view.topics.length
                          ? `${nobodyTopics.map((topic) => topic.label).join(", ")}. `
                          : ""}
                        Give them a team, or add a team to the catch-all.
                      </span>{" "}
                      <button
                        type="button"
                        className="font-medium text-primary underline-offset-4 hover:underline"
                        onClick={() => routingRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })}
                      >
                        Who gets what
                      </button>
                    </Warning>
                  ) : null}
                </div>
              ) : null}

              <div className="space-y-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h3 className="text-sm font-semibold text-black dark:text-white">Teams</h3>
                  {editable ? (
                    <Button size="sm" onClick={() => setTeamDialog({ team: null })}>
                      <Plus className="mr-1 h-4 w-4" />
                      New team
                    </Button>
                  ) : null}
                </div>
                <p className="text-sm text-muted-foreground">
                  A team is a group of people from the SFLuv organization, and a person can be on several. Everyone on a
                  team sees its notifications in the app. A team can also buzz their phones, email them, email a shared
                  address, or post in a group chat.
                </p>

                {teams.length === 0 ? (
                  <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
                    No teams yet. Until there is one, every notification reaches nobody.
                    {editable ? " Create one to start." : ""}
                  </div>
                ) : (
                  <div className="space-y-3">
                    {teams.map((team) => (
                      <TeamCard
                        key={team.id}
                        team={team}
                        memberById={memberById}
                        topicCount={view.topics.filter((topic) => routeTeams(topic.id).includes(team.id)).length}
                        isCatchAll={routeTeams(CATCH_ALL).includes(team.id)}
                        editable={editable}
                        onEdit={() => setTeamDialog({ team })}
                        onDelete={() => setDeleteTarget(team)}
                      />
                    ))}
                  </div>
                )}
              </div>
            </CardContent>
          </Card>

          <Card ref={routingRef} className="scroll-mt-4">
            <CardHeader className="pb-3">
              <CardTitle className="text-base text-black dark:text-white">Who gets what</CardTitle>
              <CardDescription>
                Choose which teams get each kind of notification. Anything without a team of its own goes to the
                catch-all, if the catch-all has a team.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="rounded-lg border border-primary/30 bg-primary/[0.04] p-3">
                <TopicRow
                  label="Anything no team receives"
                  description="These teams get every kind of notification below that has no team of its own."
                  teams={teams}
                  selected={routeTeams(CATCH_ALL)}
                  reach={catchAllReach}
                  catchAllRow
                  liveCount={0}
                  saving={savingTopics.has(CATCH_ALL)}
                  editable={editable}
                  onToggle={(teamId, on) => toggleRoute(CATCH_ALL, teamId, on)}
                />
              </div>

              {groups.map(({ set, topics }) => (
                <section key={set.id} className="space-y-2">
                  <div>
                    <h3 className="text-sm font-semibold text-black dark:text-white">{set.label}</h3>
                    {set.description ? <p className="text-xs text-muted-foreground">{set.description}</p> : null}
                  </div>
                  <div className="divide-y rounded-lg border">
                    {topics.map((topic) => (
                      <div key={topic.id} className="p-3">
                        <TopicRow
                          label={topic.label}
                          description={topic.description}
                          teams={teams}
                          selected={routeTeams(topic.id)}
                          reach={reachByTopic.get(topic.id) ?? { kind: "teams" }}
                          liveCount={view.live_counts?.[topic.id] ?? 0}
                          emailOff={!view.inbound_email && isEmailTopic(topic.id)}
                          saving={savingTopics.has(topic.id)}
                          editable={editable}
                          onToggle={(teamId, on) => toggleRoute(topic.id, teamId, on)}
                        />
                      </div>
                    ))}
                  </div>
                </section>
              ))}
            </CardContent>
          </Card>

          <Card ref={recentRef} className="scroll-mt-4">
            <CardHeader className="pb-3">
              <CardTitle className="text-base text-black dark:text-white">Recent notifications</CardTitle>
              <CardDescription>The latest ones, newest first, and where each one went.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
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
            </CardContent>
          </Card>

          {teamDialog ? (
            <TeamDialog
              team={teamDialog.team}
              members={members}
              onClose={() => setTeamDialog(null)}
              onSave={async (input, memberIds) => {
                const ok = await saveTeam(teamDialog.team, input, memberIds)
                if (ok) setTeamDialog(null)
              }}
            />
          ) : null}

          <Dialog open={deleteTarget !== null} onOpenChange={(open) => !open && !deleting && setDeleteTarget(null)}>
            <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-md">
              <DialogHeader>
                <DialogTitle>Delete {deleteTarget?.name}?</DialogTitle>
                <DialogDescription>
                  Its members stop getting its notifications. This can&apos;t be undone.
                </DialogDescription>
              </DialogHeader>
              {deletePreview.length > 0 ? (
                <Warning tone="danger">
                  <span className="font-medium text-black dark:text-white">
                    {deletePreview.length} {plural(deletePreview.length, "kind", "kinds")} of notification would then reach
                    nobody:
                  </span>{" "}
                  <span className="text-muted-foreground">{deletePreview.map((topic) => topic.label).join(", ")}.</span>
                </Warning>
              ) : null}
              <DialogFooter className="gap-2 sm:gap-2">
                <Button variant="outline" onClick={() => setDeleteTarget(null)} disabled={deleting}>
                  Cancel
                </Button>
                <Button variant="destructive" onClick={() => void confirmDelete()} disabled={deleting}>
                  {deleting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
                  Delete team
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </TabsContent>
    </Tabs>
  )
}

// ── Pieces ─────────────────────────────────────────────────────────────────

function Warning({ tone, children }: { tone: "warning" | "danger"; children: ReactNode }) {
  return (
    <div
      className={cn(
        "flex gap-2 rounded-lg border p-3 text-sm",
        tone === "danger" ? "border-destructive/50 bg-destructive/5" : "border-amber-500/50 bg-amber-500/5",
      )}
    >
      <AlertTriangle
        className={cn("mt-0.5 h-4 w-4 shrink-0", tone === "danger" ? "text-destructive" : "text-amber-600")}
      />
      <p className="min-w-0">{children}</p>
    </div>
  )
}

function TeamCard({
  team,
  memberById,
  topicCount,
  isCatchAll,
  editable,
  onEdit,
  onDelete,
}: {
  team: NotificationTeam
  memberById: Map<string, NotificationHubMember>
  topicCount: number
  isCatchAll: boolean
  editable: boolean
  onEdit: () => void
  onDelete: () => void
}) {
  const current = team.member_ids.filter((id) => memberById.has(id))
  const departed = team.member_ids.length - current.length
  const noPhone = team.push_members ? current.filter((id) => (memberById.get(id)?.devices ?? 0) === 0) : []
  const noEmail = team.email_members ? current.filter((id) => !memberById.get(id)?.email) : []
  const reachesNobody = current.length === 0 && !team.email_address && !team.chat_conversation_id

  return (
    <div className="space-y-3 rounded-lg border p-3 sm:p-4">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="break-words font-medium text-black dark:text-white">{team.name}</p>
          {team.description ? (
            <p className="break-words text-sm text-muted-foreground">{team.description}</p>
          ) : null}
        </div>
        {editable ? (
          <div className="flex shrink-0 gap-1">
            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={onEdit} aria-label={`Edit ${team.name}`}>
              <Pencil className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 text-destructive"
              onClick={onDelete}
              aria-label={`Delete ${team.name}`}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ) : null}
      </div>

      <div className="flex flex-wrap gap-1.5">
        <Badge variant="secondary">
          <Bell className="mr-1 h-3 w-3" />
          In the app
        </Badge>
        {team.push_members ? (
          <Badge variant="secondary">
            <Smartphone className="mr-1 h-3 w-3" />
            Push
          </Badge>
        ) : null}
        {team.email_members ? (
          <Badge variant="secondary">
            <Mail className="mr-1 h-3 w-3" />
            Emails members
          </Badge>
        ) : null}
        {team.email_address ? (
          <Badge variant="outline" className="max-w-full">
            <Mail className="mr-1 h-3 w-3 shrink-0" />
            <span className="truncate">{team.email_address}</span>
          </Badge>
        ) : null}
        {team.chat_conversation_id ? (
          <Badge variant="outline" className="max-w-full">
            <MessageSquare className="mr-1 h-3 w-3 shrink-0" />
            <span className="truncate">{team.chat_name || `Chat ${team.chat_conversation_id}`}</span>
          </Badge>
        ) : null}
      </div>

      <div className="flex flex-wrap gap-1.5">
        {current.map((id) => (
          <Badge key={id} variant="outline" className="max-w-full font-normal">
            <span className="truncate">{memberLabel(memberById.get(id))}</span>
          </Badge>
        ))}
        {departed > 0 ? (
          <Badge variant="outline" className="font-normal text-muted-foreground">
            {departed} no longer in SFLuv
          </Badge>
        ) : null}
        {current.length === 0 ? (
          <span className={cn("text-sm", reachesNobody ? "text-amber-700 dark:text-amber-300" : "text-muted-foreground")}>
            Nobody on this team{reachesNobody ? ", so it reaches nobody." : "."}
          </span>
        ) : null}
      </div>

      <p className="text-xs text-muted-foreground">
        {topicCount === 0 && !isCatchAll
          ? "Gets nothing yet. Choose its topics under Who gets what."
          : [
              topicCount > 0 ? `Gets ${topicCount} ${plural(topicCount, "kind", "kinds")} of notification` : "",
              isCatchAll ? (topicCount > 0 ? "and the catch-all" : "Gets the catch-all") : "",
            ]
              .filter(Boolean)
              .join(" ") + "."}
      </p>

      {noPhone.length > 0 ? (
        <p className="text-xs text-amber-700 dark:text-amber-300">
          {noPhone.map((id) => memberLabel(memberById.get(id))).join(", ")} won&apos;t be buzzed: no phone with the
          app.
        </p>
      ) : null}
      {noEmail.length > 0 ? (
        <p className="text-xs text-amber-700 dark:text-amber-300">
          {noEmail.map((id) => memberLabel(memberById.get(id))).join(", ")} won&apos;t be emailed: no email address.
        </p>
      ) : null}
    </div>
  )
}

function TopicRow({
  label,
  description,
  teams,
  selected,
  reach,
  liveCount,
  emailOff = false,
  catchAllRow = false,
  saving,
  editable,
  onToggle,
}: {
  label: string
  description: string
  teams: NotificationTeam[]
  selected: number[]
  reach: Reach
  liveCount: number
  emailOff?: boolean
  catchAllRow?: boolean
  saving: boolean
  editable: boolean
  onToggle: (teamId: number, on: boolean) => void
}) {
  const selectedTeams = teams.filter((team) => selected.includes(team.id))
  const summary = selectedTeams.map((team) => team.name).join(", ")

  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
      <div className="min-w-0 flex-1 space-y-0.5">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-sm font-medium text-black dark:text-white">{label}</span>
          {liveCount > 0 ? <Badge variant="destructive">{liveCount} waiting</Badge> : null}
          {emailOff ? (
            <Badge variant="outline" className="font-normal text-muted-foreground">
              Email not set up
            </Badge>
          ) : null}
        </div>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>

      <div className="flex flex-col gap-1 sm:w-60 sm:shrink-0 sm:items-end">
        {editable ? (
          <Popover>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                className="w-full justify-between gap-2 font-normal"
                aria-label={`Teams for ${label}`}
              >
                <span className={cn("truncate", !summary && "text-muted-foreground")}>
                  {summary || (catchAllRow ? "No catch-all" : "No team")}
                </span>
                {saving ? (
                  <Loader2 className="h-4 w-4 shrink-0 animate-spin text-muted-foreground" />
                ) : (
                  <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
                )}
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64 p-2">
              {teams.length === 0 ? (
                <p className="p-2 text-sm text-muted-foreground">Create a team first.</p>
              ) : (
                <div className="max-h-72 space-y-0.5 overflow-y-auto">
                  {teams.map((team) => {
                    const checked = selected.includes(team.id)
                    return (
                      <label
                        key={team.id}
                        className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted/70"
                      >
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(next) => onToggle(team.id, next === true)}
                          aria-label={team.name}
                        />
                        <span className="min-w-0 truncate">{team.name}</span>
                      </label>
                    )
                  })}
                </div>
              )}
            </PopoverContent>
          </Popover>
        ) : (
          <p className={cn("text-sm sm:text-right", !summary && "text-muted-foreground")}>
            {summary || (catchAllRow ? "No catch-all" : "No team")}
          </p>
        )}

        {reach.kind === "nobody" && !(catchAllRow && selectedTeams.length === 0) ? (
          <div className="sm:text-right">
            <Badge variant="warning">Reaches nobody</Badge>
            <p className="mt-0.5 text-xs text-muted-foreground">{reach.reason}</p>
          </div>
        ) : null}
        {catchAllRow && selectedTeams.length === 0 ? (
          <p className="text-xs text-muted-foreground sm:text-right">
            Anything without a team reaches nobody.
          </p>
        ) : null}
        {reach.kind === "catch-all" ? <p className="text-xs text-muted-foreground sm:text-right">Catch-all</p> : null}
      </div>
    </div>
  )
}

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
    <div
      className={cn(
        "space-y-1.5 rounded-lg border p-3",
        nobody && "border-destructive/50 bg-destructive/5",
      )}
    >
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
                ? ` (sent to ${deliveredTeams.map((team) => team.name).join(", ")}, which had nobody to tell)`
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
              {chats.length > 0
                ? ` · Posted in ${chats.map((id) => chatNames.get(id) || `chat ${id}`).join(", ")}`
                : ""}
            </p>
          )}
          {failures.length > 0 ? (
            <ul className="space-y-0.5 text-destructive">
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

function TeamDialog({
  team,
  members,
  onClose,
  onSave,
}: {
  /** Null to create a team. */
  team: NotificationTeam | null
  members: NotificationHubMember[]
  onClose: () => void
  onSave: (input: NotificationTeamInput, memberIds: string[]) => Promise<void>
}) {
  const [name, setName] = useState(team?.name ?? "")
  const [description, setDescription] = useState(team?.description ?? "")
  const [push, setPush] = useState(team?.push_members ?? true)
  const [emailMembers, setEmailMembers] = useState(team?.email_members ?? false)
  const [emailAddress, setEmailAddress] = useState(team?.email_address ?? "")
  const [chatId, setChatId] = useState(team?.chat_conversation_id ? String(team.chat_conversation_id) : "")
  const [memberIds, setMemberIds] = useState<string[]>(team?.member_ids ?? [])
  const [filter, setFilter] = useState("")
  const [saving, setSaving] = useState(false)

  const trimmedChat = chatId.trim()
  const chatNumber = trimmedChat === "" ? null : Number(trimmedChat)
  const chatInvalid = chatNumber !== null && (!Number.isInteger(chatNumber) || chatNumber <= 0)
  const chatUnchanged = chatNumber !== null && chatNumber === (team?.chat_conversation_id ?? null)
  const canSave = name.trim() !== "" && !chatInvalid && !saving

  const query = filter.trim().toLowerCase()
  const shownMembers = query
    ? members.filter((member) =>
        [member.name, member.email, member.role].some((value) => value?.toLowerCase().includes(query)),
      )
    : members
  const chosen = memberIds.filter((id) => members.some((member) => member.user_id === id)).length

  const submit = async () => {
    if (!canSave) return
    setSaving(true)
    try {
      await onSave(
        {
          name: name.trim(),
          description: description.trim(),
          push_members: push,
          email_members: emailMembers,
          email_address: emailAddress.trim(),
          chat_conversation_id: chatNumber,
        },
        memberIds,
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !saving && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{team ? `Edit ${team.name}` : "New team"}</DialogTitle>
          <DialogDescription>
            Everyone on the team sees its notifications in the app. That part is always on.
          </DialogDescription>
        </DialogHeader>

        <form
          className="space-y-5"
          onSubmit={(event) => {
            event.preventDefault()
            void submit()
          }}
        >
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="team-name">Name</Label>
              <Input
                id="team-name"
                value={name}
                maxLength={NAME_MAX}
                onChange={(event) => setName(event.target.value)}
                placeholder="Support"
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="team-description">Description</Label>
              <Textarea
                id="team-description"
                value={description}
                maxLength={DESCRIPTION_MAX}
                rows={2}
                onChange={(event) => setDescription(event.target.value)}
                placeholder="Optional. What this team looks after."
              />
              {description.length > DESCRIPTION_MAX - 50 ? (
                <p className="text-xs text-muted-foreground">
                  {description.length} of {DESCRIPTION_MAX}
                </p>
              ) : null}
            </div>
          </div>

          <div className="space-y-3">
            <p className="text-sm font-medium">How they are told</p>
            <SwitchRow
              id="team-push"
              label="Buzz their phones"
              hint="A push notification to everyone on the team who has the app."
              checked={push}
              onChange={setPush}
            />
            <SwitchRow
              id="team-email-members"
              label="Email each member"
              hint="Sent to the email address on each person's account."
              checked={emailMembers}
              onChange={setEmailMembers}
            />
            <div className="space-y-1.5">
              <Label htmlFor="team-email-address">Shared email address</Label>
              <Input
                id="team-email-address"
                type="email"
                value={emailAddress}
                onChange={(event) => setEmailAddress(event.target.value)}
                placeholder="Optional, like support@sfluv.org"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="team-chat">Group chat id</Label>
              <Input
                id="team-chat"
                type="number"
                inputMode="numeric"
                min={1}
                step={1}
                value={chatId}
                onChange={(event) => setChatId(event.target.value)}
                placeholder="Optional"
              />
              <p className={cn("text-xs", chatInvalid ? "text-destructive" : "text-muted-foreground")}>
                {chatInvalid
                  ? "A chat id is a whole number."
                  : chatUnchanged && team?.chat_name
                    ? `Posting in ${team.chat_name}.`
                    : chatNumber !== null
                      ? "The chat's name shows here once saved."
                      : "Notifications are also posted into this group chat in the app."}
              </p>
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-baseline justify-between gap-2">
              <p className="text-sm font-medium">People</p>
              <span className="text-xs text-muted-foreground">{chosen} chosen</span>
            </div>
            {members.length > 8 ? (
              <Input
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                placeholder="Find a person"
                aria-label="Find a person"
                className="h-9"
              />
            ) : null}
            {members.length === 0 ? (
              <p className="text-sm text-muted-foreground">The SFLuv organization has no members to add.</p>
            ) : (
              <div className="max-h-64 divide-y overflow-y-auto rounded-lg border">
                {shownMembers.map((member) => {
                  const checked = memberIds.includes(member.user_id)
                  return (
                    <label
                      key={member.user_id}
                      className="flex cursor-pointer items-start gap-3 px-3 py-2 hover:bg-muted/50"
                    >
                      <Checkbox
                        className="mt-0.5"
                        checked={checked}
                        onCheckedChange={(next) =>
                          setMemberIds((current) =>
                            next === true
                              ? [...current.filter((id) => id !== member.user_id), member.user_id]
                              : current.filter((id) => id !== member.user_id),
                          )
                        }
                        aria-label={memberLabel(member)}
                      />
                      <span className="min-w-0 flex-1">
                        <span className="flex flex-wrap items-center gap-1.5">
                          <span className="truncate text-sm font-medium">{memberLabel(member)}</span>
                          <Badge variant="outline" className="px-1.5 py-0.5 font-normal capitalize">
                            {member.role}
                          </Badge>
                        </span>
                        <span className="flex flex-wrap gap-x-2 text-xs text-muted-foreground">
                          {member.email && member.name ? <span className="truncate">{member.email}</span> : null}
                          <span
                            className={
                              push && checked && member.devices === 0 ? "text-amber-700 dark:text-amber-300" : undefined
                            }
                          >
                            {member.devices === 0
                              ? "No phone"
                              : `${member.devices} ${plural(member.devices, "phone", "phones")}`}
                          </span>
                        </span>
                      </span>
                    </label>
                  )
                })}
                {shownMembers.length === 0 ? (
                  <p className="px-3 py-2 text-sm text-muted-foreground">Nobody matches.</p>
                ) : null}
              </div>
            )}
          </div>

          <DialogFooter className="gap-2 sm:gap-2">
            <Button type="button" variant="outline" onClick={onClose} disabled={saving}>
              Cancel
            </Button>
            <Button type="submit" disabled={!canSave}>
              {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              {team ? "Save" : "Create team"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function SwitchRow({
  id,
  label,
  hint,
  checked,
  onChange,
}: {
  id: string
  label: string
  hint: string
  checked: boolean
  onChange: (next: boolean) => void
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0">
        <Label htmlFor={id}>{label}</Label>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onChange} />
    </div>
  )
}
