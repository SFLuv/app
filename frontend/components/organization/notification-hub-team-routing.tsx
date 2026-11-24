"use client"

import { useEffect, useMemo, useState } from "react"
import { Crown, Loader2, MessageSquare, Pencil, Plus, Search, Send, Shield, X } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { InfoTip } from "@/components/ui/info-tip"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { useToast } from "@/hooks/use-toast"
import {
  DEFAULT_TEAM_DESTINATIONS,
  TEAM_RULE_DEFAULT,
  type NotificationChatOption,
  type NotificationHubView,
  type NotificationMemberDestination,
  type NotificationTeam,
  type NotificationTeamDestinations,
  type NotificationTestResult,
} from "@/types/notifications"

import {
  CATCH_ALL,
  Warning,
  describeRule,
  memberLabel,
  plural,
  resolveTeamRule,
  type HubContext,
} from "./notification-hub-shared"

type RuleRow = {
  /** TEAM_RULE_DEFAULT, CATCH_ALL, or a topic id. */
  key: string
  label: string
  description?: string
  /** Topic rows can be tested; the default and catch-all rows are not a topic. */
  testable: boolean
}

/**
 * The Team routing tab: where each topic a team is given goes inside it. Its
 * lead and team admins see their own teams here, organization admins every
 * team. Only the lead (or an organization admin) chooses the team admins.
 */
export function NotificationHubTeamRouting({ view, hub }: { view: NotificationHubView; hub: HubContext }) {
  const managed = view.managed_teams ?? {}
  const teams = useMemo(
    () => (view.teams ?? []).filter((team) => view.can_edit || managed[String(team.id)]),
    [view, managed],
  )
  const [teamId, setTeamId] = useState<number | null>(null)
  const team = teams.find((entry) => entry.id === teamId) ?? teams[0]

  if (!team) {
    return (
      <p className="text-sm text-muted-foreground">
        {view.can_edit ? "No teams yet. Create one on the Teams tab." : "You don't lead any teams."}
      </p>
    )
  }

  const role = managed[String(team.id)]
  const canChooseAdmins = view.can_edit || role === "lead"

  return (
    <div className="space-y-section">
      {teams.length > 1 ? (
        <div className="max-w-xs space-y-2">
          <Label htmlFor="routing-team">Team</Label>
          <Select value={String(team.id)} onValueChange={(value) => setTeamId(Number(value))}>
            <SelectTrigger id="routing-team">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {teams.map((entry) => (
                <SelectItem key={entry.id} value={String(entry.id)}>
                  {entry.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : (
        <h2 className="text-lg font-semibold text-black dark:text-white">{team.name}</h2>
      )}

      <TeamPeople key={`people-${team.id}`} team={team} hub={hub} canChooseAdmins={canChooseAdmins} />
      <TeamResources key={`resources-${team.id}`} team={team} hub={hub} />
      <TeamRules key={`rules-${team.id}`} team={team} view={view} hub={hub} />
    </div>
  )
}

// ── People: lead and team admins ─────────────────────────────────────────────

function TeamPeople({ team, hub, canChooseAdmins }: { team: NotificationTeam; hub: HubContext; canChooseAdmins: boolean }) {
  const { toast } = useToast()
  const [saving, setSaving] = useState(false)
  const members = team.member_ids.filter((id) => hub.orgMemberIds.has(id))
  const admins = new Set(team.admin_ids)
  const candidates = members.filter((id) => id !== team.lead_user_id)

  const setAdmin = (userId: string, on: boolean) =>
    hub.change(async () => {
      setSaving(true)
      try {
        const next = on ? [...team.admin_ids.filter((id) => id !== userId), userId] : team.admin_ids.filter((id) => id !== userId)
        const result = await hub.send(`/organizations/mine/notification-teams/${team.id}/admins`, "PUT", { user_ids: next })
        if (!result.ok) {
          hub.fail("Could not save the team admins", result.error)
          return
        }
        toast({ title: on ? `${memberLabel(hub.memberById.get(userId))} is a team admin` : "Team admin removed" })
      } finally {
        setSaving(false)
      }
    })

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <h3 className="text-base font-semibold text-black dark:text-white">People</h3>
          <InfoTip label="About team leads and admins">
            The lead and team admins choose where this team&apos;s notifications go. Only the lead chooses the team
            admins. Organization admins choose who is on the team and who leads it, on the Teams tab.
          </InfoTip>
        </div>
        {canChooseAdmins && candidates.length > 0 ? (
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="outline" size="sm" disabled={saving}>
                {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Shield className="mr-2 h-4 w-4" />}
                Team admins
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64 p-2">
              <div className="max-h-72 space-y-1 overflow-y-auto">
                {candidates.map((id) => (
                  <label key={id} className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted/70">
                    <Checkbox
                      checked={admins.has(id)}
                      onCheckedChange={(next) => void setAdmin(id, next === true)}
                      aria-label={memberLabel(hub.memberById.get(id))}
                    />
                    <span className="min-w-0 truncate">{memberLabel(hub.memberById.get(id))}</span>
                  </label>
                ))}
              </div>
            </PopoverContent>
          </Popover>
        ) : null}
      </div>

      {members.length === 0 ? (
        <p className="text-sm text-amber-700 dark:text-amber-300">Nobody is on this team yet.</p>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {members.map((id) => (
            <Badge key={id} variant={id === team.lead_user_id ? "secondary" : "outline"} className="max-w-full gap-1 font-normal">
              {id === team.lead_user_id ? <Crown className="h-3 w-3 shrink-0" aria-label="Lead" /> : null}
              {admins.has(id) ? <Shield className="h-3 w-3 shrink-0" aria-label="Team admin" /> : null}
              <span className="truncate">{memberLabel(hub.memberById.get(id))}</span>
              {hub.memberById.get(id)?.devices === 0 ? <span className="text-amber-700 dark:text-amber-300">· no phone</span> : null}
            </Badge>
          ))}
        </div>
      )}
    </section>
  )
}

// ── Where it can go: shared address and group chats ─────────────────────────

function TeamResources({ team, hub }: { team: NotificationTeam; hub: HubContext }) {
  const { toast } = useToast()
  const [address, setAddress] = useState(team.email_address)
  const [saving, setSaving] = useState(false)
  const [query, setQuery] = useState("")
  const [results, setResults] = useState<NotificationChatOption[] | null>(null)
  const [searching, setSearching] = useState(false)

  useEffect(() => setAddress(team.email_address), [team.email_address])

  // Searches as they type, a moment after they stop.
  useEffect(() => {
    const text = query.trim()
    if (text === "") {
      setResults(null)
      return
    }
    let cancelled = false
    setSearching(true)
    const timer = window.setTimeout(async () => {
      const result = await hub.send(`/organizations/mine/notification-hub/chats?q=${encodeURIComponent(text)}`, "GET")
      if (cancelled) return
      setSearching(false)
      setResults(result.ok ? ((result.data as NotificationChatOption[] | null) ?? []) : [])
    }, 250)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [query, hub])

  const save = (nextAddress: string, chatIds: number[], done: string) =>
    hub.change(async () => {
      setSaving(true)
      try {
        const result = await hub.send(`/organizations/mine/notification-teams/${team.id}/resources`, "PUT", {
          email_address: nextAddress,
          chat_ids: chatIds,
        })
        if (!result.ok) {
          hub.fail("Could not save that", result.error)
          return false
        }
        toast({ title: done })
        return true
      } finally {
        setSaving(false)
      }
    })

  const chatIds = team.chats.map((chat) => chat.id)
  const addressChanged = address.trim() !== team.email_address

  return (
    <section className="space-y-4">
      <div className="flex items-center gap-2">
        <h3 className="text-base font-semibold text-black dark:text-white">Where it can go</h3>
        <InfoTip label="About the team's places">
          Besides its members, a team can send to a shared email address and post in group chats, as the support bot.
          Adding them here doesn&apos;t send anything there yet: choose them for a topic under Routing.
        </InfoTip>
      </div>

      <form
        className="max-w-form space-y-2"
        onSubmit={(event) => {
          event.preventDefault()
          if (addressChanged) void save(address.trim(), chatIds, "Shared address saved")
        }}
      >
        <Label htmlFor={`team-address-${team.id}`}>Shared email address</Label>
        <div className="flex gap-2">
          <Input
            id={`team-address-${team.id}`}
            type="email"
            value={address}
            onChange={(event) => setAddress(event.target.value)}
            placeholder="Optional, like support@sfluv.org"
          />
          <Button type="submit" variant="outline" disabled={!addressChanged || saving}>
            Save
          </Button>
        </div>
      </form>

      <div className="space-y-2">
        <span className="text-sm font-medium">Group chats</span>
        {team.chats.length > 0 ? (
          <div className="divide-y rounded-lg border">
            {team.chats.map((chat) => (
              <div key={chat.id} className="flex items-center justify-between gap-3 px-4 py-2">
                <span className="flex min-w-0 items-center gap-2 text-sm">
                  <MessageSquare className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="truncate">{chat.title || `Chat ${chat.id}`}</span>
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  disabled={saving}
                  aria-label={`Remove ${chat.title || `chat ${chat.id}`}`}
                  onClick={() =>
                    void save(
                      team.email_address,
                      chatIds.filter((id) => id !== chat.id),
                      `Removed ${chat.title || "the chat"}`,
                    )
                  }
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        ) : null}

        <div className="relative max-w-form">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Find a group chat by name"
            aria-label="Find a group chat by name"
            className="pl-9"
          />
          {searching ? (
            <Loader2 className="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 animate-spin text-muted-foreground" />
          ) : null}
        </div>
        {results !== null ? (
          results.length === 0 ? (
            <p className="text-sm text-muted-foreground">No group chat with someone from SFLuv in it matches.</p>
          ) : (
            <div className="max-w-form divide-y rounded-lg border">
              {results.map((chat) => {
                const added = chatIds.includes(chat.id)
                return (
                  <div key={chat.id} className="flex items-center justify-between gap-3 px-4 py-2">
                    <span className="min-w-0 truncate text-sm">
                      {chat.title || `Chat ${chat.id}`}{" "}
                      <span className="text-xs text-muted-foreground">
                        {chat.members} {plural(chat.members, "person", "people")}
                      </span>
                    </span>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={added || saving}
                      onClick={async () => {
                        const ok = await save(team.email_address, [...chatIds, chat.id], `Added ${chat.title || "the chat"}`)
                        if (ok) {
                          setQuery("")
                          setResults(null)
                        }
                      }}
                    >
                      {added ? "Added" : (
                        <>
                          <Plus className="mr-2 h-4 w-4" />
                          Add
                        </>
                      )}
                    </Button>
                  </div>
                )
              })}
            </div>
          )
        ) : null}
      </div>
    </section>
  )
}

// ── Routing: where each topic goes inside the team ──────────────────────────

function TeamRules({ team, view, hub }: { team: NotificationTeam; view: NotificationHubView; hub: HubContext }) {
  const { toast } = useToast()
  const [editing, setEditing] = useState<RuleRow | null>(null)
  const [testing, setTesting] = useState<Set<string>>(new Set())

  const topicById = new Map(view.topics.map((topic) => [topic.id, topic]))
  const rows: RuleRow[] = [
    { key: TEAM_RULE_DEFAULT, label: "Anything without its own rule", testable: false },
    ...(team.topics.includes(CATCH_ALL)
      ? [{ key: CATCH_ALL, label: "Anything from the catch-all", testable: false }]
      : []),
    ...team.topics
      .filter((id) => id !== CATCH_ALL && topicById.has(id))
      .map((id) => ({
        key: id,
        label: topicById.get(id)!.label,
        description: topicById.get(id)!.description,
        testable: true,
      })),
  ]

  const ruleFor = (key: string) =>
    key === TEAM_RULE_DEFAULT
      ? (team.rules[TEAM_RULE_DEFAULT] ?? DEFAULT_TEAM_DESTINATIONS)
      : resolveTeamRule(team, key, key === CATCH_ALL)

  const sourceOf = (key: string) => {
    if (team.rules[key]) return "Own rule"
    if (key === TEAM_RULE_DEFAULT) return "Everyone's phones until set"
    if (team.rules[TEAM_RULE_DEFAULT]) return "Team default"
    return "Everyone's phones"
  }

  const sendTest = (row: RuleRow) =>
    hub.change(async () => {
      setTesting((current) => new Set(current).add(row.key))
      try {
        const result = await hub.send("/organizations/mine/notification-hub/test", "POST", { topic: row.key, team_id: team.id })
        if (!result.ok) {
          hub.fail(`Could not send a test of ${row.label}`, result.error)
          return
        }
        const sent = result.data as NotificationTestResult | null
        if (!sent || sent.reached_nobody) {
          toast({ title: `The test of ${row.label} reached nobody on ${team.name}`, variant: "warning" })
          return
        }
        toast({
          title: `Test sent to ${team.name}`,
          description: [
            `${sent.people} ${plural(sent.people, "person", "people")}`,
            sent.emails > 0 ? `${sent.emails} ${plural(sent.emails, "email", "emails")}` : "",
            sent.chats > 0 ? `${sent.chats} ${plural(sent.chats, "chat", "chats")}` : "",
          ]
            .filter(Boolean)
            .join(", "),
        })
      } finally {
        setTesting((current) => {
          const copy = new Set(current)
          copy.delete(row.key)
          return copy
        })
      }
    })

  return (
    <section className="space-y-3">
      <div className="flex items-center gap-2">
        <h3 className="text-base font-semibold text-black dark:text-white">Routing</h3>
        <InfoTip label="About team routing">
          Where each kind of notification this team is given goes inside it. A topic without its own rule follows
          &ldquo;Anything without its own rule&rdquo;; until that is set, everyone on the team gets it in the app and on
          their phone. Organization admins choose which topics the team is given.
        </InfoTip>
      </div>

      <div className="divide-y rounded-lg border">
        {rows.map((row) => (
          <div key={row.key} className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
            <div className="min-w-0 flex-1 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-black dark:text-white">{row.label}</span>
                {row.description ? <InfoTip label={`About ${row.label}`}>{row.description}</InfoTip> : null}
                <Badge variant={team.rules[row.key] ? "secondary" : "outline"} className="font-normal">
                  {sourceOf(row.key)}
                </Badge>
              </div>
              <p className="break-words text-sm text-muted-foreground">{describeRule(ruleFor(row.key), team, hub.memberById)}</p>
            </div>
            <div className="flex items-center gap-2 sm:shrink-0">
              <Button variant="outline" size="sm" onClick={() => setEditing(row)}>
                <Pencil className="mr-2 h-4 w-4" />
                Edit
              </Button>
              {row.testable ? (
                <Button variant="ghost" size="sm" onClick={() => void sendTest(row)} disabled={testing.has(row.key)}>
                  {testing.has(row.key) ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Send className="mr-2 h-4 w-4" />}
                  Send test
                </Button>
              ) : null}
            </div>
          </div>
        ))}
      </div>
      {rows.length === 1 ? (
        <p className="text-sm text-muted-foreground">This team isn&apos;t given any topics yet.</p>
      ) : null}

      {editing ? (
        <RuleDialog
          team={team}
          hub={hub}
          row={editing}
          initial={ruleFor(editing.key)}
          hasOwnRule={Boolean(team.rules[editing.key])}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </section>
  )
}

function RuleDialog({
  team,
  hub,
  row,
  initial,
  hasOwnRule,
  onClose,
}: {
  team: NotificationTeam
  hub: HubContext
  row: RuleRow
  initial: NotificationTeamDestinations
  hasOwnRule: boolean
  onClose: () => void
}) {
  const { toast } = useToast()
  const [everyone, setEveryone] = useState(initial.everyone !== null)
  const [everyonePush, setEveryonePush] = useState(initial.everyone?.push ?? true)
  const [everyoneEmail, setEveryoneEmail] = useState(initial.everyone?.email ?? false)
  const [people, setPeople] = useState<Map<string, NotificationMemberDestination>>(
    () => new Map(initial.members.map((member) => [member.user_id, member])),
  )
  const [address, setAddress] = useState(initial.email_address)
  const [chats, setChats] = useState<Set<number>>(() => new Set(initial.chats))
  const [saving, setSaving] = useState(false)

  const members = team.member_ids.filter((id) => hub.orgMemberIds.has(id))

  const draft: NotificationTeamDestinations = {
    everyone: everyone ? { push: everyonePush, email: everyoneEmail } : null,
    members: [...people.values()].filter((member) => members.includes(member.user_id)),
    email_address: address && team.email_address !== "",
    chats: [...chats].filter((id) => team.chats.some((chat) => chat.id === id)),
  }
  const reachesNobody =
    !(draft.everyone && members.length > 0) && draft.members.length === 0 && !draft.email_address && draft.chats.length === 0

  const setPerson = (userId: string, change: Partial<NotificationMemberDestination> | null) =>
    setPeople((current) => {
      const next = new Map(current)
      if (change === null) next.delete(userId)
      else next.set(userId, { user_id: userId, push: true, email: false, ...current.get(userId), ...change })
      return next
    })

  const save = async (destinations: NotificationTeamDestinations | null) => {
    setSaving(true)
    try {
      await hub.change(async () => {
        const result = await hub.send(`/organizations/mine/notification-teams/${team.id}/rules`, "PUT", {
          topic: row.key,
          destinations,
        })
        if (!result.ok) {
          hub.fail("Could not save that", result.error)
          return
        }
        toast({ title: destinations ? `Saved where ${row.label} goes` : `${row.label} is back to the default` })
        onClose()
      })
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !saving && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <span>{row.label}</span>
            <InfoTip label="About these choices">
              Everyone chosen sees it in the app on {team.name}. Their phone and email are on top of that.
            </InfoTip>
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-6">
          <div className="space-y-3">
            <div className="flex items-center justify-between gap-3">
              <Label htmlFor="rule-everyone">Everyone on the team</Label>
              <Switch id="rule-everyone" checked={everyone} onCheckedChange={setEveryone} />
            </div>
            {everyone ? (
              <div className="flex flex-wrap gap-4 pl-4">
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox checked={everyonePush} onCheckedChange={(next) => setEveryonePush(next === true)} />
                  Phone
                </label>
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox checked={everyoneEmail} onCheckedChange={(next) => setEveryoneEmail(next === true)} />
                  Email
                </label>
              </div>
            ) : null}
          </div>

          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium">{everyone ? "Also, for particular people" : "Particular people"}</span>
            </div>
            {members.length === 0 ? (
              <p className="text-sm text-muted-foreground">Nobody is on this team yet.</p>
            ) : (
              <div className="divide-y rounded-lg border">
                {members.map((id) => {
                  const chosen = people.get(id)
                  return (
                    <div key={id} className="flex flex-wrap items-center gap-3 px-4 py-2">
                      <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-3">
                        <Checkbox
                          checked={chosen !== undefined}
                          onCheckedChange={(next) => setPerson(id, next === true ? {} : null)}
                          aria-label={memberLabel(hub.memberById.get(id))}
                        />
                        <span className="truncate text-sm">{memberLabel(hub.memberById.get(id))}</span>
                      </label>
                      {chosen ? (
                        <div className="flex gap-4">
                          <label className="flex items-center gap-2 text-sm">
                            <Checkbox checked={chosen.push} onCheckedChange={(next) => setPerson(id, { push: next === true })} />
                            Phone
                          </label>
                          <label className="flex items-center gap-2 text-sm">
                            <Checkbox checked={chosen.email} onCheckedChange={(next) => setPerson(id, { email: next === true })} />
                            Email
                          </label>
                        </div>
                      ) : null}
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          <div className="space-y-2">
            <span className="text-sm font-medium">Places</span>
            <div className="space-y-2">
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={address && team.email_address !== ""}
                  disabled={team.email_address === ""}
                  onCheckedChange={(next) => setAddress(next === true)}
                />
                {team.email_address ? `Email ${team.email_address}` : "Shared email address (none set)"}
              </label>
              {team.chats.map((chat) => (
                <label key={chat.id} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={chats.has(chat.id)}
                    onCheckedChange={(next) =>
                      setChats((current) => {
                        const copy = new Set(current)
                        if (next === true) copy.add(chat.id)
                        else copy.delete(chat.id)
                        return copy
                      })
                    }
                  />
                  <MessageSquare className="h-4 w-4 text-muted-foreground" />
                  {chat.title || `Chat ${chat.id}`}
                </label>
              ))}
              {team.chats.length === 0 ? (
                <p className="text-sm text-muted-foreground">No group chats yet. Add them under Where it can go.</p>
              ) : null}
            </div>
          </div>

          {reachesNobody ? (
            <Warning tone="warning">
              <span className="font-medium text-black dark:text-white">This sends it nowhere on {team.name}.</span>{" "}
              <span className="text-muted-foreground">
                If no other team takes it, it goes to the catch-all instead.
              </span>
            </Warning>
          ) : null}
        </div>

        <DialogFooter className="sm:justify-between">
          {hasOwnRule ? (
            <Button variant="ghost" onClick={() => void save(null)} disabled={saving}>
              {row.key === TEAM_RULE_DEFAULT ? "Back to everyone's phones" : "Use the team default"}
            </Button>
          ) : (
            <span />
          )}
          <div className="flex flex-col-reverse gap-2 sm:flex-row">
            <Button variant="outline" onClick={onClose} disabled={saving}>
              Cancel
            </Button>
            <Button onClick={() => void save(draft)} disabled={saving}>
              {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Save
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
