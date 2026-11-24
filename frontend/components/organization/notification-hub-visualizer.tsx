"use client"

import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { formatDistanceToNowStrict } from "date-fns"
import { Loader2, RefreshCw } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import {
  DEFAULT_TEAM_DESTINATIONS,
  TEAM_RULE_DEFAULT,
  type NotificationHubMember,
  type NotificationHubView,
  type NotificationTeam,
  type NotificationTopicDefinition,
  type NotificationTouchpointDefinition,
} from "@/types/notifications"

import type { Reach } from "./notification-hub-shared"

/**
 * The hub, drawn live.
 *
 * Three columns, the same shape as the report's map: every way in on the left,
 * the hub's topics in the middle, and the teams that would hear about each one
 * on the right. A touchpoint is wired to each topic id it posts under, and a
 * topic to the teams that would get it right now — its own, the catch-all's,
 * or "Reaches nobody". The routing is the same rule the backend delivers by,
 * so what is drawn is what would happen if each touchpoint fired now.
 *
 * The parent reloads the hub while this tab is open, so live counts and
 * firings appear without a refresh. A topic that fired since the last reload
 * pulses briefly.
 */

const CATCH_ALL = "unrouted"
const NOBODY = "nobody"
const ISSUERS = "issuers"
/** How long a topic that just fired stays highlighted. */
const FRESH_MS = 12_000

type Selection =
  | { kind: "touchpoint"; id: string }
  | { kind: "topic"; id: string }
  | { kind: "team"; id: number }
  | { kind: "nobody" }
  | { kind: "issuers" }
  | null

type Wire = { key: string; from: string; to: string; d: string }

const plural = (count: number, one: string, many: string) => (count === 1 ? one : many)

function ago(iso?: string) {
  if (!iso) return ""
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ""
  return formatDistanceToNowStrict(date, { addSuffix: true })
}

export function NotificationHubVisualizer({
  view,
  reachByTopic,
  teamsById,
  orgMemberIds,
  memberById,
  updatedAt,
  refreshing,
  onRefresh,
}: {
  view: NotificationHubView
  reachByTopic: Map<string, Reach>
  teamsById: Map<number, NotificationTeam>
  orgMemberIds: Set<string>
  memberById: Map<string, NotificationHubMember>
  updatedAt: Date | null
  refreshing: boolean
  onRefresh: () => void
}) {
  const [selection, setSelection] = useState<Selection>(null)
  const [now, setNow] = useState(() => Date.now())
  const [fresh, setFresh] = useState<Map<string, number>>(new Map())
  const [wires, setWires] = useState<Wire[]>([])

  const flowRef = useRef<HTMLDivElement>(null)
  const nodeRefs = useRef(new Map<string, HTMLElement>())
  const previousActivity = useRef<Record<string, string | undefined> | null>(null)

  const touchpoints = useMemo(() => view.touchpoints ?? [], [view])
  const topicsById = useMemo(() => new Map(view.topics.map((topic) => [topic.id, topic])), [view])
  const routes = useMemo(() => view.routes ?? {}, [view])
  const activity = useMemo(() => view.topic_activity ?? {}, [view])
  const liveCounts = useMemo(() => view.live_counts ?? {}, [view])

  // ── The graph ──────────────────────────────────────────────────────────────

  /** Where a topic goes right now, as node keys: teams, or nobody. */
  const targetsOf = useMemo(() => {
    const map = new Map<string, string[]>()
    for (const topic of view.topics) {
      const reach = reachByTopic.get(topic.id)
      let ids: number[] = []
      if (reach?.kind === "teams") ids = routes[topic.id] ?? []
      if (reach?.kind === "catch-all") ids = routes[CATCH_ALL] ?? []
      const keys = ids.filter((id) => teamsById.has(id)).map((id) => `team:${id}`)
      if (reach?.kind === "nobody" || keys.length === 0) keys.push(NOBODY)
      // Issuers are emailed directly, beside whatever the hub does.
      if (topic.id === "credential.request") keys.push(ISSUERS)
      map.set(topic.id, keys)
    }
    return map
  }, [view, reachByTopic, routes, teamsById])

  const feedsOf = useMemo(() => {
    const map = new Map<string, NotificationTouchpointDefinition[]>()
    for (const touchpoint of touchpoints) {
      for (const topic of touchpoint.topics) map.set(topic, [...(map.get(topic) ?? []), touchpoint])
    }
    return map
  }, [touchpoints])

  const touchpointGroups = useMemo(() => {
    const order: string[] = []
    const byGroup = new Map<string, NotificationTouchpointDefinition[]>()
    for (const touchpoint of touchpoints) {
      if (!byGroup.has(touchpoint.group)) {
        order.push(touchpoint.group)
        byGroup.set(touchpoint.group, [])
      }
      byGroup.get(touchpoint.group)!.push(touchpoint)
    }
    return order.map((group) => ({ group, touchpoints: byGroup.get(group)! }))
  }, [touchpoints])

  const topicGroups = useMemo(
    () =>
      view.sets
        .map((set) => ({ set, topics: view.topics.filter((topic) => topic.set === set.id) }))
        .filter((group) => group.topics.length > 0),
    [view],
  )

  const catchAllTeams = useMemo(() => new Set(routes[CATCH_ALL] ?? []), [routes])
  const nobodyTopics = view.topics.filter((topic) => (targetsOf.get(topic.id) ?? []).includes(NOBODY))

  // ── Live: what fired since the last reload ─────────────────────────────────

  useEffect(() => {
    const current: Record<string, string | undefined> = {}
    for (const [topic, entry] of Object.entries(activity)) current[topic] = entry.last_at
    const previous = previousActivity.current
    previousActivity.current = current
    if (!previous) return
    const fired = Object.keys(current).filter((topic) => current[topic] && current[topic] !== previous[topic])
    if (fired.length === 0) return
    setFresh((existing) => {
      const next = new Map(existing)
      for (const topic of fired) next.set(topic, Date.now())
      return next
    })
  }, [activity])

  useEffect(() => {
    const timer = window.setInterval(() => {
      setNow(Date.now())
      setFresh((existing) => {
        if (existing.size === 0) return existing
        const next = new Map([...existing].filter(([, at]) => Date.now() - at < FRESH_MS))
        return next.size === existing.size ? existing : next
      })
    }, 1000)
    return () => window.clearInterval(timer)
  }, [])

  // ── Wires ──────────────────────────────────────────────────────────────────

  const register = (key: string) => (el: HTMLElement | null) => {
    if (el) nodeRefs.current.set(key, el)
    else nodeRefs.current.delete(key)
  }

  useLayoutEffect(() => {
    const flow = flowRef.current
    if (!flow) return
    const wide = window.matchMedia("(min-width: 1024px)")

    const draw = () => {
      if (!wide.matches) {
        setWires([])
        return
      }
      const base = flow.getBoundingClientRect()
      const rectOf = (key: string) => nodeRefs.current.get(key)?.getBoundingClientRect()
      // Topic rows sit inside the hub card, so their wires meet the card's edge
      // level with the row instead of vanishing behind the card.
      const hub = rectOf("hub")
      const curve = (from: DOMRect, to: DOMRect, fromTopic = false, toTopic = false) => {
        const x1 = (fromTopic && hub ? hub.right : from.right) - base.left
        const y1 = from.top + from.height / 2 - base.top
        const x2 = (toTopic && hub ? hub.left : to.left) - base.left
        const y2 = to.top + to.height / 2 - base.top
        const dx = Math.max(24, (x2 - x1) * 0.5)
        return `M${x1.toFixed(1)},${y1.toFixed(1)} C${(x1 + dx).toFixed(1)},${y1.toFixed(1)} ${(x2 - dx).toFixed(1)},${y2.toFixed(1)} ${x2.toFixed(1)},${y2.toFixed(1)}`
      }
      const next: Wire[] = []
      for (const touchpoint of touchpoints) {
        const from = rectOf(`tp:${touchpoint.id}`)
        if (!from) continue
        for (const topic of touchpoint.topics) {
          const to = rectOf(`topic:${topic}`)
          if (to)
            next.push({
              key: `tp:${touchpoint.id}>topic:${topic}`,
              from: `tp:${touchpoint.id}`,
              to: `topic:${topic}`,
              d: curve(from, to, false, true),
            })
        }
      }
      for (const topic of view.topics) {
        const from = rectOf(`topic:${topic.id}`)
        if (!from) continue
        for (const target of targetsOf.get(topic.id) ?? []) {
          const to = rectOf(target)
          if (to)
            next.push({ key: `topic:${topic.id}>${target}`, from: `topic:${topic.id}`, to: target, d: curve(from, to, true, false) })
        }
      }
      setWires(next)
    }

    let frame = requestAnimationFrame(draw)
    const redraw = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(draw)
    }
    const observer = new ResizeObserver(redraw)
    observer.observe(flow)
    wide.addEventListener("change", redraw)
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
      wide.removeEventListener("change", redraw)
    }
  }, [touchpoints, view, targetsOf])

  // ── Selection ──────────────────────────────────────────────────────────────

  const hot = useMemo(() => {
    const nodes = new Set<string>()
    const edges = new Set<string>()
    if (!selection) return { nodes, edges }
    const lightTopic = (topic: string, withFeeds: boolean) => {
      nodes.add(`topic:${topic}`)
      for (const target of targetsOf.get(topic) ?? []) {
        nodes.add(target)
        edges.add(`topic:${topic}>${target}`)
      }
      if (withFeeds) {
        for (const touchpoint of feedsOf.get(topic) ?? []) {
          nodes.add(`tp:${touchpoint.id}`)
          edges.add(`tp:${touchpoint.id}>topic:${topic}`)
        }
      }
    }
    if (selection.kind === "touchpoint") {
      const touchpoint = touchpoints.find((entry) => entry.id === selection.id)
      nodes.add(`tp:${selection.id}`)
      for (const topic of touchpoint?.topics ?? []) {
        edges.add(`tp:${selection.id}>topic:${topic}`)
        lightTopic(topic, false)
      }
    } else if (selection.kind === "topic") {
      lightTopic(selection.id, true)
    } else {
      // A team, Reaches nobody or the issuers: light every topic that ends
      // there and every touchpoint behind those topics.
      const key = selection.kind === "team" ? `team:${selection.id}` : selection.kind === "issuers" ? ISSUERS : NOBODY
      nodes.add(key)
      for (const topic of view.topics) {
        if ((targetsOf.get(topic.id) ?? []).includes(key)) {
          nodes.add(`topic:${topic.id}`)
          edges.add(`topic:${topic.id}>${key}`)
          for (const touchpoint of feedsOf.get(topic.id) ?? []) {
            nodes.add(`tp:${touchpoint.id}`)
            edges.add(`tp:${touchpoint.id}>topic:${topic.id}`)
          }
        }
      }
    }
    return { nodes, edges }
  }, [selection, targetsOf, feedsOf, touchpoints, view])

  const toggle = (next: Selection) =>
    setSelection((current) => (JSON.stringify(current) === JSON.stringify(next) ? null : next))
  const dimmed = (key: string) => selection !== null && !hot.nodes.has(key)
  const selected = (key: string) =>
    (selection?.kind === "touchpoint" && key === `tp:${selection.id}`) ||
    (selection?.kind === "topic" && key === `topic:${selection.id}`) ||
    (selection?.kind === "team" && key === `team:${selection.id}`) ||
    (selection?.kind === "nobody" && key === NOBODY) ||
    (selection?.kind === "issuers" && key === ISSUERS)

  // ── Words ──────────────────────────────────────────────────────────────────

  const teamName = (key: string) =>
    key === NOBODY ? "nobody" : key === ISSUERS ? "partner issuers" : teamsById.get(Number(key.slice(5)))?.name ?? key
  const whereTopicGoes = (topic: string) => {
    const reach = reachByTopic.get(topic)
    const names = (targetsOf.get(topic) ?? []).filter((key) => key !== NOBODY && key !== ISSUERS).map(teamName)
    const issuers = topic === "credential.request" ? " Issuers who can grant it are emailed too." : ""
    if (reach?.kind === "teams") return `${names.join(", ")}.${issuers}`
    if (reach?.kind === "catch-all") return `${names.join(", ")}, through the catch-all.${issuers}`
    return `Nobody. ${reach?.kind === "nobody" ? reach.reason : ""}${issuers}`
  }
  // Every way the team's rules can tell somebody, across its topics. A team
  // with no default rule still buzzes everyone for topics without their own.
  const channelsOf = (team: NotificationTeam) => {
    const rules = Object.values(team.rules ?? {})
    if (!team.rules?.[TEAM_RULE_DEFAULT]) rules.push(DEFAULT_TEAM_DESTINATIONS)
    const out = ["in the app"]
    if (rules.some((rule) => rule.everyone?.push || rule.members.some((member) => member.push))) out.push("push")
    if (rules.some((rule) => rule.everyone?.email || rule.members.some((member) => member.email))) out.push("email to members")
    if (team.email_address && rules.some((rule) => rule.email_address)) out.push(team.email_address)
    for (const chat of team.chats ?? []) {
      if (rules.some((rule) => rule.chats.includes(chat.id))) out.push(`chat “${chat.title || chat.id}”`)
    }
    return out
  }
  const activeMembers = (team: NotificationTeam) => team.member_ids.filter((id) => orgMemberIds.has(id))
  const emailOff = (topic: string) => topic.startsWith("email.") && !view.inbound_email

  const liveTotal = Object.values(liveCounts).reduce((sum, count) => sum + count, 0)
  const dayTotal = Object.values(activity).reduce((sum, entry) => sum + (entry.last_24h ?? 0), 0)
  const outside = touchpoints.filter((touchpoint) => touchpoint.outside).length

  let detail: { title: ReactNode; rows: Array<[string, ReactNode]> }
  if (selection?.kind === "touchpoint") {
    const touchpoint = touchpoints.find((entry) => entry.id === selection.id)
    detail = {
      title: touchpoint?.label ?? selection.id,
      rows: [
        ["Where", touchpoint?.group ?? ""],
        ["How", touchpoint?.description ?? ""],
        [
          plural(touchpoint?.topics.length ?? 0, "Topic", "Topics"),
          touchpoint?.topics.length ? (
            <ul className="space-y-1">
              {touchpoint.topics.map((topic) => (
                <li key={topic}>
                  <code className="text-xs">{topic}</code>
                  <span className="text-muted-foreground"> → {whereTopicGoes(topic)}</span>
                </li>
              ))}
            </ul>
          ) : (
            "None: this is outside our systems and cannot post to the hub."
          ),
        ],
      ],
    }
  } else if (selection?.kind === "topic") {
    const topic = topicsById.get(selection.id)
    const entry = activity[selection.id]
    detail = {
      title: (
        <>
          {topic?.label ?? selection.id} <code className="ml-1 text-xs font-normal text-muted-foreground">{selection.id}</code>
        </>
      ),
      rows: [
        ["What", topic?.description ?? ""],
        ["Goes to", whereTopicGoes(selection.id)],
        ["Waiting now", String(liveCounts[selection.id] ?? 0)],
        ["Last day", `${entry?.last_24h ?? 0} raised`],
        ["Last fired", entry?.last_at ? ago(entry.last_at) : "Never"],
        ["Reached nobody", `${entry?.reached_nobody_30d ?? 0} in the last 30 days`],
        ["Posted by", (feedsOf.get(selection.id) ?? []).map((touchpoint) => touchpoint.label).join(", ") || "Nothing"],
        ...(emailOff(selection.id) ? ([["Note", "Email can't be received until inbound email is set up."]] as Array<[string, ReactNode]>) : []),
      ],
    }
  } else if (selection?.kind === "team") {
    const team = teamsById.get(selection.id)
    const members = team ? activeMembers(team) : []
    const topics = view.topics.filter((topic) => (targetsOf.get(topic.id) ?? []).includes(`team:${selection.id}`))
    detail = {
      title: team?.name ?? "Team",
      rows: [
        ["About", team?.description ?? ""],
        [
          "Members",
          members.length
            ? members.map((id) => memberById.get(id)?.name || memberById.get(id)?.email || id).join(", ")
            : "Nobody yet",
        ],
        ["Told by", team ? channelsOf(team).join(" · ") : ""],
        ["Catch-all", catchAllTeams.has(selection.id) ? "Yes: it also gets every topic no other team reaches" : "No"],
        [`Gets ${topics.length} ${plural(topics.length, "topic", "topics")}`, topics.map((topic) => topic.id).join(", ") || "None"],
      ],
    }
  } else if (selection?.kind === "issuers") {
    detail = {
      title: "Partner issuers",
      rows: [
        ["How", "Credential requests are emailed straight to the issuers who can grant that credential, beside the hub."],
        ["Posted by", (feedsOf.get("credential.request") ?? []).map((touchpoint) => touchpoint.label).join(", ")],
      ],
    }
  } else if (selection?.kind === "nobody") {
    detail = {
      title: "Reaches nobody",
      rows: [
        [
          `${nobodyTopics.length} ${plural(nobodyTopics.length, "topic", "topics")}`,
          nobodyTopics.length ? (
            <ul className="space-y-1">
              {nobodyTopics.map((topic) => {
                const reach = reachByTopic.get(topic.id)
                return (
                  <li key={topic.id}>
                    <code className="text-xs">{topic.id}</code>
                    <span className="text-muted-foreground"> {reach?.kind === "nobody" ? reach.reason : ""}</span>
                  </li>
                )
              })}
            </ul>
          ) : (
            "None. Every topic reaches somebody."
          ),
        ],
        ["Kept", "A notification that reaches nobody is still stored, logged and listed in red on the Settings tab."],
      ],
    }
  } else {
    detail = {
      title: "Every touchpoint, one hub",
      rows: [
        ["Touchpoints", `${touchpoints.length}, of which ${touchpoints.length - outside} post to the hub and ${outside} are outside our systems`],
        ["Waiting now", `${liveTotal} ${plural(liveTotal, "notification", "notifications")}`],
        ["Last day", `${dayTotal} raised`],
        [
          "Reaches nobody",
          `${nobodyTopics.length} ${plural(nobodyTopics.length, "topic", "topics")} right now; ${view.reached_nobody} ${plural(view.reached_nobody, "notification", "notifications")} in the last 30 days`,
        ],
        ["Try it", "Click a touchpoint, a topic or a team to follow its path."],
      ],
    }
  }

  const updatedLabel = updatedAt ? `${Math.max(0, Math.round((now - updatedAt.getTime()) / 1000))}s ago` : "not yet"

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <span className="relative flex h-2.5 w-2.5">
            <span className="absolute inline-flex h-full w-full rounded-full bg-emerald-500 opacity-60 motion-safe:animate-ping" />
            <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-500" />
          </span>
          Live · updated {updatedLabel}
        </div>
        <Button variant="outline" size="sm" onClick={onRefresh} disabled={refreshing}>
          {refreshing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
          Refresh
        </Button>
      </div>

      <div className="min-h-[132px] rounded-xl border bg-card p-4" aria-live="polite">
        <div className="mb-2 text-base font-semibold text-black dark:text-white">{detail.title}</div>
        <dl className="grid gap-x-4 gap-y-1.5 text-sm sm:grid-cols-[max-content_minmax(0,1fr)]">
          {detail.rows
            .filter(([, value]) => value !== "" && value !== null)
            .map(([label, value]) => (
              <div key={label} className="contents">
                <dt className="text-muted-foreground">{label}</dt>
                <dd className="min-w-0 break-words">{value}</dd>
              </div>
            ))}
        </dl>
      </div>

      <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <Legend className="bg-emerald-500">goes to its own teams</Legend>
        <Legend className="bg-amber-500">goes to the catch-all</Legend>
        <Legend className="bg-red-500">reaches nobody</Legend>
        <Legend className="bg-primary">fired since you opened this</Legend>
      </div>

      <div
        ref={flowRef}
        className="relative grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1.05fr)_minmax(0,1fr)_minmax(0,0.85fr)] lg:gap-14"
      >
        <svg className="pointer-events-none absolute inset-0 hidden h-full w-full overflow-visible lg:block" aria-hidden="true">
          {wires.map((wire) => {
            const isHot = hot.edges.has(wire.key)
            const toNobody = wire.to === NOBODY
            return (
              <path
                key={wire.key}
                d={wire.d}
                fill="none"
                strokeWidth={isHot ? 2.2 : 1.2}
                className={cn(
                  "transition-opacity",
                  isHot ? (toNobody ? "stroke-red-500" : "stroke-primary") : toNobody ? "stroke-red-300 dark:stroke-red-900" : "stroke-border",
                  selection && !isHot && "opacity-20",
                )}
              />
            )
          })}
        </svg>

        {/* Where people reach us */}
        <div className="relative z-[1] min-w-0 space-y-4">
          <ColumnHead>Where people reach us</ColumnHead>
          {touchpointGroups.map(({ group, touchpoints: items }) => (
            <div key={group} className="space-y-1.5">
              <div className="text-xs font-medium text-muted-foreground">{group}</div>
              {items.map((touchpoint) => {
                const key = `tp:${touchpoint.id}`
                const day = touchpoint.topics.reduce((sum, topic) => sum + (activity[topic]?.last_24h ?? 0), 0)
                const firing = touchpoint.topics.some((topic) => fresh.has(topic))
                return (
                  <button
                    key={touchpoint.id}
                    ref={register(key)}
                    type="button"
                    onClick={() => toggle({ kind: "touchpoint", id: touchpoint.id })}
                    className={cn(
                      "flex w-full flex-wrap items-center gap-2 rounded-lg border bg-card px-2.5 py-1.5 text-left text-sm transition",
                      "hover:border-foreground/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                      selected(key) && "border-primary bg-primary/10",
                      dimmed(key) && "opacity-35",
                      firing && "ring-2 ring-primary/60",
                    )}
                  >
                    <span className="min-w-0 flex-1">{touchpoint.label}</span>
                    {touchpoint.outside ? (
                      <Badge variant="outline" className="border-amber-500/50 text-amber-700 dark:text-amber-300">
                        outside
                      </Badge>
                    ) : day > 0 ? (
                      <span className="text-xs tabular-nums text-muted-foreground">{day} today</span>
                    ) : null}
                    <span className="w-full break-words font-mono text-[11px] text-muted-foreground lg:hidden">
                      {touchpoint.topics.length ? `→ ${touchpoint.topics.join(", ")}` : "→ not in the hub"}
                    </span>
                  </button>
                )
              })}
            </div>
          ))}
        </div>

        {/* The hub */}
        <div className="relative z-[1] min-w-0 space-y-3">
          <ColumnHead>The hub</ColumnHead>
          <div ref={register("hub")} className="space-y-4 rounded-2xl border-2 border-primary bg-card p-3">
            <div className="flex items-baseline justify-between gap-2">
              <span className="font-semibold text-black dark:text-white">Notification hub</span>
              <span className="text-xs text-muted-foreground">
                {view.topics.length} topics · {liveTotal} waiting
              </span>
            </div>
            {topicGroups.map(({ set, topics }) => (
              <div key={set.id} className="space-y-1">
                <div className="text-xs font-medium text-muted-foreground">{set.label}</div>
                {topics.map((topic) => (
                  <TopicRow
                    key={topic.id}
                    topic={topic}
                    reach={reachByTopic.get(topic.id)}
                    live={liveCounts[topic.id] ?? 0}
                    fresh={fresh.has(topic.id)}
                    off={emailOff(topic.id)}
                    selected={selected(`topic:${topic.id}`)}
                    dimmed={dimmed(`topic:${topic.id}`)}
                    goesTo={(targetsOf.get(topic.id) ?? []).map(teamName).join(", ")}
                    refFn={register(`topic:${topic.id}`)}
                    onClick={() => toggle({ kind: "topic", id: topic.id })}
                  />
                ))}
              </div>
            ))}
          </div>
        </div>

        {/* Who hears */}
        <div className="relative z-[1] min-w-0 space-y-3">
          <ColumnHead>Who hears, and how</ColumnHead>
          {view.teams.length === 0 ? (
            <div className="rounded-xl border border-dashed p-3 text-sm text-muted-foreground">
              No teams yet. Make one on the Settings tab and route topics to it.
            </div>
          ) : null}
          {view.teams.map((team) => {
            const key = `team:${team.id}`
            const members = activeMembers(team)
            return (
              <button
                key={team.id}
                ref={register(key)}
                type="button"
                onClick={() => toggle({ kind: "team", id: team.id })}
                className={cn(
                  "w-full space-y-1 rounded-lg border bg-card px-3 py-2 text-left transition",
                  "hover:border-foreground/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                  selected(key) && "border-primary bg-primary/10",
                  dimmed(key) && "opacity-35",
                )}
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="min-w-0 flex-1 font-medium">{team.name}</span>
                  {catchAllTeams.has(team.id) ? <Badge variant="secondary">catch-all</Badge> : null}
                </div>
                <div className={cn("text-xs", members.length ? "text-muted-foreground" : "text-amber-700 dark:text-amber-300")}>
                  {members.length} {plural(members.length, "member", "members")} · {channelsOf(team).join(" · ")}
                </div>
              </button>
            )
          })}
          <button
            ref={register(NOBODY)}
            type="button"
            onClick={() => toggle({ kind: "nobody" })}
            className={cn(
              "flex w-full items-center gap-2 rounded-lg border border-red-500/60 bg-red-500/5 px-3 py-2 text-left transition",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500",
              selected(NOBODY) && "bg-red-500/15",
              dimmed(NOBODY) && "opacity-35",
            )}
          >
            <span className="flex-1 font-medium text-red-700 dark:text-red-300">Reaches nobody</span>
            <span className="text-xs tabular-nums text-red-700 dark:text-red-300">{nobodyTopics.length}</span>
          </button>
          <button
            ref={register(ISSUERS)}
            type="button"
            onClick={() => toggle({ kind: "issuers" })}
            className={cn(
              "w-full rounded-lg border border-dashed px-3 py-2 text-left text-sm text-muted-foreground transition",
              "hover:border-foreground/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
              selected(ISSUERS) && "border-primary bg-primary/10",
              dimmed(ISSUERS) && "opacity-35",
            )}
          >
            Partner issuers, emailed directly for credential requests
          </button>
        </div>
      </div>
    </div>
  )
}

function TopicRow({
  topic,
  reach,
  live,
  fresh,
  off,
  selected,
  dimmed,
  goesTo,
  refFn,
  onClick,
}: {
  topic: NotificationTopicDefinition
  reach: Reach | undefined
  live: number
  fresh: boolean
  off: boolean
  selected: boolean
  dimmed: boolean
  goesTo: string
  refFn: (el: HTMLElement | null) => void
  onClick: () => void
}) {
  const dot =
    reach?.kind === "teams" ? "bg-emerald-500" : reach?.kind === "catch-all" ? "bg-amber-500" : "bg-red-500"
  return (
    <button
      ref={refFn}
      type="button"
      onClick={onClick}
      title={`${topic.label}: ${topic.description}`}
      className={cn(
        "flex w-full flex-wrap items-center gap-2 rounded-md border border-transparent px-2 py-1 text-left transition",
        "hover:border-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
        selected && "border-primary bg-primary/10",
        dimmed && "opacity-35",
        fresh && "bg-primary/15",
        off && "opacity-60",
      )}
    >
      <span className="relative flex h-2 w-2 shrink-0">
        {fresh ? <span className="absolute inline-flex h-full w-full rounded-full bg-primary motion-safe:animate-ping" /> : null}
        <span className={cn("relative inline-flex h-2 w-2 rounded-full", fresh ? "bg-primary" : dot)} />
      </span>
      <code className="min-w-0 flex-1 break-all text-[12px]">{topic.id}</code>
      {live > 0 ? (
        <span className="rounded bg-muted px-1.5 text-[11px] tabular-nums text-foreground">{live}</span>
      ) : null}
      <span className="w-full pl-4 text-[11px] text-muted-foreground lg:hidden">→ {goesTo}</span>
    </button>
  )
}

function ColumnHead({ children }: { children: ReactNode }) {
  return <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{children}</div>
}

function Legend({ className, children }: { className: string; children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={cn("inline-block h-2 w-2 rounded-full", className)} />
      {children}
    </span>
  )
}
