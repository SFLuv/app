"use client"

import { useMemo, useRef, useState } from "react"
import { ChevronDown, Loader2, Send } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { InfoTip } from "@/components/ui/info-tip"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useToast } from "@/hooks/use-toast"
import { cn } from "@/lib/utils"
import type { NotificationHubView, NotificationTeam, NotificationTestResult } from "@/types/notifications"

import { CATCH_ALL, isEmailTopic, plural, reachOf, type HubContext, type Reach } from "./notification-hub-shared"

/**
 * The Who gets what tab, for organization admins: which teams get each topic,
 * with the catch-all for anything that has none, and a test send for each.
 */
export function NotificationHubRouting({
  view,
  hub,
  reachByTopic,
}: {
  view: NotificationHubView
  hub: HubContext
  reachByTopic: Map<string, Reach>
}) {
  const { toast } = useToast()
  const [savingTopics, setSavingTopics] = useState<Set<string>>(new Set())
  const [testing, setTesting] = useState<Set<string>>(new Set())
  const [routes, setRoutes] = useState<Record<string, number[]>>(view.routes ?? {})
  const [seen, setSeen] = useState(view.routes)

  // The view reloads after every change; take its routes as the truth then.
  if (seen !== view.routes) {
    setSeen(view.routes)
    setRoutes(view.routes ?? {})
  }

  // Each topic's route saves go out one at a time, the latest choice last, so
  // two quick clicks cannot arrive out of order and leave the first one saved.
  const routeQueue = useRef(new Map<string, number[]>())
  const routeSending = useRef(new Set<string>())

  const teams = view.teams ?? []
  const routeTeams = (topicId: string) => (routes[topicId] ?? []).filter((id) => hub.teamsById.has(id))

  const groups = useMemo(() => {
    const known = new Set(view.sets.map((set) => set.id))
    const out = view.sets
      .map((set) => ({ set, topics: view.topics.filter((topic) => topic.set === set.id) }))
      .filter((group) => group.topics.length > 0)
    const other = view.topics.filter((topic) => !known.has(topic.set))
    if (other.length > 0) out.push({ set: { id: "other", label: "Other", description: "" }, topics: other })
    return out
  }, [view])

  const flushRoute = (topicId: string) =>
    hub.change(async () => {
      routeSending.current.add(topicId)
      setSavingTopics((current) => new Set(current).add(topicId))
      try {
        let next = routeQueue.current.get(topicId)
        while (next !== undefined) {
          routeQueue.current.delete(topicId)
          const result = await hub.send("/organizations/mine/notification-routes", "PUT", { topic: topicId, team_ids: next })
          if (!result.ok) {
            routeQueue.current.delete(topicId)
            hub.fail("Could not save who gets this", result.error)
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
      }
    })

  const toggleRoute = (topicId: string, teamId: number, on: boolean) => {
    const current = routeTeams(topicId)
    const next = on ? [...current.filter((id) => id !== teamId), teamId] : current.filter((id) => id !== teamId)
    setRoutes((existing) => ({ ...existing, [topicId]: next }))
    routeQueue.current.set(topicId, next)
    if (!routeSending.current.has(topicId)) void flushRoute(topicId)
  }

  const sendTest = (topicId: string, label: string) =>
    hub.change(async () => {
      setTesting((current) => new Set(current).add(topicId))
      try {
        const result = await hub.send("/organizations/mine/notification-hub/test", "POST", { topic: topicId })
        if (!result.ok) {
          hub.fail(`Could not send a test of ${label}`, result.error)
          return
        }
        const sent = result.data as NotificationTestResult | null
        if (!sent || sent.reached_nobody) {
          toast({ title: `The test of ${label} reached nobody`, variant: "warning" })
          return
        }
        const where = [
          `${sent.people} ${plural(sent.people, "person", "people")}`,
          sent.emails > 0 ? `${sent.emails} ${plural(sent.emails, "email", "emails")}` : "",
          sent.chats > 0 ? `${sent.chats} ${plural(sent.chats, "chat", "chats")}` : "",
        ]
          .filter(Boolean)
          .join(", ")
        toast({
          title: `Test sent to ${sent.teams.map((team) => team.name).join(", ")}${sent.catch_all ? " (catch-all)" : ""}`,
          description: where,
        })
      } finally {
        setTesting((current) => {
          const copy = new Set(current)
          copy.delete(topicId)
          return copy
        })
      }
    })

  // Routes are local here while saves are in flight, so reach is worked out
  // from them rather than taken from the last load.
  const reachFor = (topicId: string) =>
    routes === view.routes
      ? (reachByTopic.get(topicId) ?? reachOf(topicId, routes, hub.teamsById, hub.orgMemberIds))
      : reachOf(topicId, routes, hub.teamsById, hub.orgMemberIds)

  return (
    <div className="space-y-section">
      <section className="space-y-3">
        <div className="flex items-center gap-2">
          <h2 className="text-lg font-semibold text-black dark:text-white">Catch-all</h2>
          <InfoTip label="About the catch-all">
            These teams get every kind of notification below that has no team of its own, or whose teams reach nobody.
          </InfoTip>
        </div>
        <div className="rounded-lg border border-primary/30 bg-primary/[0.04] px-4 py-3">
          <TopicRow
            label="Anything no team receives"
            teams={teams}
            selected={routeTeams(CATCH_ALL)}
            reach={reachOf(CATCH_ALL, routes, hub.teamsById, hub.orgMemberIds)}
            catchAllRow
            liveCount={0}
            saving={savingTopics.has(CATCH_ALL)}
            onToggle={(teamId, on) => toggleRoute(CATCH_ALL, teamId, on)}
          />
        </div>
      </section>

      {groups.map(({ set, topics }) => (
        <section key={set.id} className="space-y-3">
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold text-black dark:text-white">{set.label}</h2>
            {set.description ? <InfoTip label={`About ${set.label}`}>{set.description}</InfoTip> : null}
          </div>
          <div className="divide-y rounded-lg border">
            {topics.map((topic) => (
              <div key={topic.id} className="px-4 py-3">
                <TopicRow
                  label={topic.label}
                  description={topic.description}
                  teams={teams}
                  selected={routeTeams(topic.id)}
                  reach={reachFor(topic.id)}
                  liveCount={view.live_counts?.[topic.id] ?? 0}
                  emailOff={!view.inbound_email && isEmailTopic(topic.id)}
                  saving={savingTopics.has(topic.id)}
                  testing={testing.has(topic.id)}
                  onToggle={(teamId, on) => toggleRoute(topic.id, teamId, on)}
                  onTest={() => void sendTest(topic.id, topic.label)}
                />
              </div>
            ))}
          </div>
        </section>
      ))}
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
  testing = false,
  onToggle,
  onTest,
}: {
  label: string
  description?: string
  teams: NotificationTeam[]
  selected: number[]
  reach: Reach
  liveCount: number
  emailOff?: boolean
  catchAllRow?: boolean
  saving: boolean
  testing?: boolean
  onToggle: (teamId: number, on: boolean) => void
  onTest?: () => void
}) {
  const selectedTeams = teams.filter((team) => selected.includes(team.id))
  const summary = selectedTeams.map((team) => team.name).join(", ")
  const nobody = reach.kind === "nobody" && !(catchAllRow && selectedTeams.length === 0)

  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
        <span className="text-sm font-medium text-black dark:text-white">{label}</span>
        {description ? <InfoTip label={`About ${label}`}>{description}</InfoTip> : null}
        {liveCount > 0 ? <Badge variant="destructive">{liveCount} waiting</Badge> : null}
        {emailOff ? (
          <Badge variant="outline" className="font-normal text-muted-foreground">
            Email not set up
          </Badge>
        ) : null}
        {nobody ? (
          <span className="inline-flex items-center gap-1">
            <Badge variant="warning">Reaches nobody</Badge>
            <InfoTip label="Why it reaches nobody">{reach.kind === "nobody" ? reach.reason : ""}</InfoTip>
          </span>
        ) : null}
        {catchAllRow && selectedTeams.length === 0 ? (
          <span className="inline-flex items-center gap-1">
            <Badge variant="warning">No catch-all</Badge>
            <InfoTip label="About having no catch-all">Anything without a team reaches nobody.</InfoTip>
          </span>
        ) : null}
        {reach.kind === "catch-all" ? <Badge variant="secondary">Catch-all</Badge> : null}
      </div>

      <div className="flex items-center gap-2 sm:shrink-0">
        <Popover>
          <PopoverTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              className="w-full justify-between gap-2 font-normal sm:w-60"
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
              <div className="max-h-72 space-y-1 overflow-y-auto">
                {teams.map((team) => (
                  <label key={team.id} className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted/70">
                    <Checkbox
                      checked={selected.includes(team.id)}
                      onCheckedChange={(next) => onToggle(team.id, next === true)}
                      aria-label={team.name}
                    />
                    <span className="min-w-0 truncate">{team.name}</span>
                  </label>
                ))}
              </div>
            )}
          </PopoverContent>
        </Popover>
        {onTest ? (
          <Button variant="ghost" size="sm" onClick={onTest} disabled={testing} aria-label={`Send a test of ${label}`}>
            {testing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Send className="mr-2 h-4 w-4" />}
            Send test
          </Button>
        ) : null}
      </div>
    </div>
  )
}
