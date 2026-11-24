"use client"

import type { ReactNode } from "react"
import { format } from "date-fns"
import { AlertTriangle } from "lucide-react"

import { cn } from "@/lib/utils"
import {
  DEFAULT_TEAM_DESTINATIONS,
  TEAM_RULE_DEFAULT,
  type NotificationHubMember,
  type NotificationTeam,
  type NotificationTeamDestinations,
} from "@/types/notifications"

/** The catch-all route: teams here get every topic that has no team. */
export const CATCH_ALL = "unrouted"

export type Reach =
  | { kind: "teams" }
  | { kind: "catch-all" }
  | { kind: "nobody"; reason: string }

export type SendResult = { ok: true; data: unknown } | { ok: false; error: string }

/** What the hub's tabs share: the view's lookups and the way to write. */
export type HubContext = {
  teamsById: Map<number, NotificationTeam>
  memberById: Map<string, NotificationHubMember>
  orgMemberIds: Set<string>
  send: (path: string, method: string, body?: unknown) => Promise<SendResult>
  /** Reports a failed write. */
  fail: (title: string, error: string) => void
  /** Wraps a write so the view reloads once it, and every write alongside it, is done. */
  change: <T>(work: () => Promise<T>) => Promise<T>
}

export const memberLabel = (member: NotificationHubMember | undefined) =>
  member?.name || member?.email || "Unnamed member"

export const plural = (count: number, one: string, many: string) => (count === 1 ? one : many)

/** Backend refusals arrive lowercase ("there is already a team called that"). */
export const sentence = (text: string) => (text ? text.charAt(0).toUpperCase() + text.slice(1) : text)

export const isEmailTopic = (topicId: string) => topicId.startsWith("email.")

export function formatWhen(iso: string) {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ""
  return format(date, date.getFullYear() === new Date().getFullYear() ? "MMM d, h:mm a" : "MMM d yyyy, h:mm a")
}

/**
 * The rule a team applies to a topic. Mirrors the backend's ResolveTeamRule:
 * the topic's own, then (for what came through the catch-all) the catch-all's,
 * then the team's default, then everyone on the team in the app and on their phone.
 */
export function resolveTeamRule(
  team: NotificationTeam,
  topicId: string,
  viaCatchAll: boolean,
): NotificationTeamDestinations {
  const rules = team.rules ?? {}
  if (rules[topicId]) return rules[topicId]
  if (viaCatchAll && rules[CATCH_ALL]) return rules[CATCH_ALL]
  if (rules[TEAM_RULE_DEFAULT]) return rules[TEAM_RULE_DEFAULT]
  return DEFAULT_TEAM_DESTINATIONS
}

/** Whether a team's rule for a topic reaches anybody, anywhere. */
export function teamReaches(
  team: NotificationTeam,
  topicId: string,
  viaCatchAll: boolean,
  orgMemberIds: Set<string>,
): boolean {
  const rule = resolveTeamRule(team, topicId, viaCatchAll)
  const members = team.member_ids.filter((id) => orgMemberIds.has(id))
  if (rule.everyone && members.length > 0) return true
  if (rule.members.some((member) => members.includes(member.user_id))) return true
  if (rule.email_address && team.email_address) return true
  return rule.chats.some((id) => team.chats.some((chat) => chat.id === id))
}

/**
 * Where one topic would go right now. Mirrors the backend's delivery plan: a
 * topic's own teams, falling back to the catch-all's when it has none or when
 * its own reach nobody under their rules, and nobody when neither reaches
 * anyone. `without` previews deleting a team.
 */
export function reachOf(
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

  const own = teamsFor(topicId)
  if (own.some((team) => teamReaches(team, topicId, false, orgMemberIds))) return { kind: "teams" }
  if (topicId === CATCH_ALL) {
    return { kind: "nobody", reason: own.length > 0 ? "Its teams reach nobody." : "No team is set." }
  }
  const catchAll = teamsFor(CATCH_ALL)
  if (catchAll.some((team) => teamReaches(team, topicId, true, orgMemberIds))) return { kind: "catch-all" }
  if (own.length > 0) {
    return { kind: "nobody", reason: (own.length === 1 ? "Its team reaches" : "Its teams reach") + " nobody, and no catch-all can take it." }
  }
  return {
    kind: "nobody",
    reason: catchAll.length === 0 ? "No team gets this, and there is no catch-all." : "No team gets this, and the catch-all reaches nobody.",
  }
}

/** A short account of where a rule sends things, for a row. */
export function describeRule(
  rule: NotificationTeamDestinations,
  team: NotificationTeam,
  memberById: Map<string, NotificationHubMember>,
): string {
  const parts: string[] = []
  const how = (channels: { push: boolean; email: boolean }) =>
    [channels.push ? "phone" : "", channels.email ? "email" : ""].filter(Boolean).join(" and ")
  if (rule.everyone) {
    const extra = how(rule.everyone)
    parts.push(extra ? `Everyone: app and ${extra}` : "Everyone: app")
  }
  for (const member of rule.members) {
    if (!team.member_ids.includes(member.user_id)) continue
    const extra = how(member)
    parts.push(`${memberLabel(memberById.get(member.user_id))}: ${extra ? `app and ${extra}` : "app"}`)
  }
  if (rule.email_address && team.email_address) parts.push(team.email_address)
  for (const id of rule.chats) {
    const chat = team.chats.find((entry) => entry.id === id)
    if (chat) parts.push(`“${chat.title || `Chat ${chat.id}`}”`)
  }
  return parts.length > 0 ? parts.join(" · ") : "Nobody"
}

export function Warning({ tone, children }: { tone: "warning" | "danger"; children: ReactNode }) {
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
