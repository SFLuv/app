"use client"

import { useState } from "react"
import { Crown, Loader2, Pencil, Plus, Shield, Trash2 } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { InfoTip } from "@/components/ui/info-tip"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { useToast } from "@/hooks/use-toast"
import type { NotificationHubMember, NotificationHubView, NotificationTeam } from "@/types/notifications"

import { CATCH_ALL, Warning, memberLabel, plural, reachOf, type HubContext, type Reach } from "./notification-hub-shared"

const NAME_MAX = 80
const DESCRIPTION_MAX = 300

/**
 * The Teams tab, for organization admins: who is on each team and who leads
 * it. Where a team's notifications go inside it is the team's own business, on
 * the Team routing tab.
 */
export function NotificationHubTeams({
  view,
  hub,
  reachByTopic,
}: {
  view: NotificationHubView
  hub: HubContext
  reachByTopic: Map<string, Reach>
}) {
  const { toast } = useToast()
  const [teamDialog, setTeamDialog] = useState<{ team: NotificationTeam | null } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<NotificationTeam | null>(null)
  const [deleting, setDeleting] = useState(false)

  const teams = view.teams ?? []
  const members = view.members ?? []
  const routes = view.routes ?? {}
  const routeTeams = (topicId: string) => (routes[topicId] ?? []).filter((id) => hub.teamsById.has(id))

  const saveTeam = (team: NotificationTeam | null, name: string, description: string, memberIds: string[], lead: string) =>
    hub.change(async () => {
      // Somebody who has left the organization can still be named on a team
      // row, and the backend refuses a member list that includes them.
      const people = memberIds.filter((id) => hub.orgMemberIds.has(id))
      let id = team?.id
      if (!team) {
        const created = await hub.send("/organizations/mine/notification-teams", "POST", { name, description })
        if (!created.ok) {
          hub.fail("Could not create the team", created.error)
          return false
        }
        id = (created.data as { id?: number } | null)?.id
      } else if (name !== team.name || description !== team.description) {
        const saved = await hub.send(`/organizations/mine/notification-teams/${team.id}`, "PUT", { name, description })
        if (!saved.ok) {
          hub.fail("Could not save the team", saved.error)
          return false
        }
      }
      if (!id) return false

      const before = team ? team.member_ids.filter((memberId) => hub.orgMemberIds.has(memberId)) : []
      const membersChanged =
        before.length !== people.length ||
        before.some((memberId, index) => people[index] !== memberId) ||
        (team !== null && team.member_ids.length !== before.length)
      if (membersChanged) {
        const saved = await hub.send(`/organizations/mine/notification-teams/${id}/members`, "PUT", { user_ids: people })
        if (!saved.ok) {
          hub.fail(team ? "Could not save who is on the team" : "The team was created, but its people were not saved", saved.error)
          return !team
        }
      }
      // The backend makes the first person added the lead; only a different
      // choice needs saying.
      const defaultLead = team && people.includes(team.lead_user_id) ? team.lead_user_id : people[0] ?? ""
      if (lead && people.includes(lead) && lead !== defaultLead) {
        const saved = await hub.send(`/organizations/mine/notification-teams/${id}/lead`, "PUT", { user_id: lead })
        if (!saved.ok) {
          hub.fail("Could not change the lead", saved.error)
          return false
        }
      }
      toast({ title: team ? `Saved ${name}` : `Created ${name}` })
      return true
    })

  const confirmDelete = async () => {
    const team = deleteTarget
    if (!team) return
    setDeleting(true)
    try {
      await hub.change(async () => {
        const result = await hub.send(`/organizations/mine/notification-teams/${team.id}`, "DELETE")
        if (!result.ok) {
          hub.fail("Could not delete the team", result.error)
          return
        }
        toast({ title: `Deleted ${team.name}` })
        setDeleteTarget(null)
      })
    } finally {
      setDeleting(false)
    }
  }

  const deletePreview = deleteTarget
    ? view.topics.filter(
        (topic) =>
          reachByTopic.get(topic.id)?.kind !== "nobody" &&
          reachOf(topic.id, routes, hub.teamsById, hub.orgMemberIds, deleteTarget.id).kind === "nobody",
      )
    : []

  return (
    <section className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <h2 className="text-lg font-semibold text-black dark:text-white">Teams</h2>
          <InfoTip label="About teams">
            A team is a group of people from the SFLuv organization, and a person can be on several. Its lead, the
            first person added unless you choose someone else, decides where the team&apos;s notifications go, and can
            make other members team admins to help.
          </InfoTip>
        </div>
        <Button size="sm" onClick={() => setTeamDialog({ team: null })}>
          <Plus className="mr-2 h-4 w-4" />
          New team
        </Button>
      </div>

      {teams.length === 0 ? (
        <div className="flex flex-wrap items-center gap-3 rounded-lg border border-dashed px-4 py-3 text-sm text-muted-foreground">
          No teams yet, so every notification reaches nobody.
        </div>
      ) : (
        <div className="divide-y rounded-lg border">
          {teams.map((team) => (
            <TeamRow
              key={team.id}
              team={team}
              memberById={hub.memberById}
              topicCount={view.topics.filter((topic) => routeTeams(topic.id).includes(team.id)).length}
              isCatchAll={routeTeams(CATCH_ALL).includes(team.id)}
              onEdit={() => setTeamDialog({ team })}
              onDelete={() => setDeleteTarget(team)}
            />
          ))}
        </div>
      )}

      {teamDialog ? (
        <TeamDialog
          team={teamDialog.team}
          members={members}
          onClose={() => setTeamDialog(null)}
          onSave={async (name, description, memberIds, lead) => {
            const ok = await saveTeam(teamDialog.team, name, description, memberIds, lead)
            if (ok) setTeamDialog(null)
          }}
        />
      ) : null}

      <Dialog open={deleteTarget !== null} onOpenChange={(open) => !open && !deleting && setDeleteTarget(null)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete {deleteTarget?.name}?</DialogTitle>
            <DialogDescription>Its members stop getting its notifications. This can&apos;t be undone.</DialogDescription>
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
          <DialogFooter>
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
    </section>
  )
}

function TeamRow({
  team,
  memberById,
  topicCount,
  isCatchAll,
  onEdit,
  onDelete,
}: {
  team: NotificationTeam
  memberById: Map<string, NotificationHubMember>
  topicCount: number
  isCatchAll: boolean
  onEdit: () => void
  onDelete: () => void
}) {
  const current = team.member_ids.filter((id) => memberById.has(id))
  const departed = team.member_ids.length - current.length
  const admins = new Set(team.admin_ids)
  const gets =
    topicCount === 0 && !isCatchAll
      ? "Gets nothing yet"
      : [
          topicCount > 0 ? `${topicCount} ${plural(topicCount, "topic", "topics")}` : "",
          isCatchAll ? "the catch-all" : "",
        ]
          .filter(Boolean)
          .join(" and ")

  return (
    <div className="space-y-3 px-4 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="break-words font-medium text-black dark:text-white">{team.name}</span>
          {team.description ? <InfoTip label={`About ${team.name}`}>{team.description}</InfoTip> : null}
          <Badge variant="outline" className="font-normal text-muted-foreground">
            {gets}
          </Badge>
        </div>
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
      </div>

      <div className="flex flex-wrap gap-1.5">
        {current.map((id) => (
          <Badge key={id} variant={id === team.lead_user_id ? "secondary" : "outline"} className="max-w-full gap-1 font-normal">
            {id === team.lead_user_id ? <Crown className="h-3 w-3 shrink-0" aria-label="Lead" /> : null}
            {admins.has(id) ? <Shield className="h-3 w-3 shrink-0" aria-label="Team admin" /> : null}
            <span className="truncate">{memberLabel(memberById.get(id))}</span>
          </Badge>
        ))}
        {departed > 0 ? (
          <Badge variant="outline" className="font-normal text-muted-foreground">
            {departed} no longer in SFLuv
          </Badge>
        ) : null}
        {current.length === 0 ? (
          <span className="text-sm text-amber-700 dark:text-amber-300">Nobody on this team yet.</span>
        ) : null}
      </div>
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
  onSave: (name: string, description: string, memberIds: string[], lead: string) => Promise<void>
}) {
  const inOrg = new Set(members.map((member) => member.user_id))
  const [name, setName] = useState(team?.name ?? "")
  const [description, setDescription] = useState(team?.description ?? "")
  // Kept in the order people were added: the first is the lead by default.
  const [memberIds, setMemberIds] = useState<string[]>((team?.member_ids ?? []).filter((id) => inOrg.has(id)))
  const [lead, setLead] = useState(team?.lead_user_id ?? "")
  const [filter, setFilter] = useState("")
  const [saving, setSaving] = useState(false)

  const effectiveLead = memberIds.includes(lead) ? lead : memberIds[0] ?? ""
  const canSave = name.trim() !== "" && !saving

  const query = filter.trim().toLowerCase()
  const shownMembers = query
    ? members.filter((member) => [member.name, member.email, member.role].some((value) => value?.toLowerCase().includes(query)))
    : members
  const byId = new Map(members.map((member) => [member.user_id, member]))

  const submit = async () => {
    if (!canSave) return
    setSaving(true)
    try {
      await onSave(name.trim(), description.trim(), memberIds, effectiveLead)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !saving && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{team ? `Edit ${team.name}` : "New team"}</DialogTitle>
        </DialogHeader>

        <form
          className="space-y-6"
          onSubmit={(event) => {
            event.preventDefault()
            void submit()
          }}
        >
          <div className="space-y-4">
            <div className="space-y-2">
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
            <div className="space-y-2">
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

          <div className="space-y-2">
            <div className="flex items-center justify-between gap-2">
              <span className="text-sm font-medium">People</span>
              <span className="text-xs text-muted-foreground">{memberIds.length} chosen</span>
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
                    <label key={member.user_id} className="flex cursor-pointer items-center gap-3 px-4 py-3 hover:bg-muted/50">
                      <Checkbox
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
                      <span className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                        <span className="truncate text-sm font-medium">{memberLabel(member)}</span>
                        {member.email && member.name ? (
                          <span className="truncate text-xs text-muted-foreground">{member.email}</span>
                        ) : null}
                      </span>
                      <span className={member.devices === 0 ? "text-xs text-amber-700 dark:text-amber-300" : "text-xs text-muted-foreground"}>
                        {member.devices === 0 ? "No phone" : `${member.devices} ${plural(member.devices, "phone", "phones")}`}
                      </span>
                    </label>
                  )
                })}
                {shownMembers.length === 0 ? <p className="px-4 py-3 text-sm text-muted-foreground">Nobody matches.</p> : null}
              </div>
            )}
          </div>

          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <Label htmlFor="team-lead">Lead</Label>
              <InfoTip label="About the lead">
                The lead chooses where the team&apos;s notifications go and who its team admins are. It is the first person
                added unless you choose someone else.
              </InfoTip>
            </div>
            <Select value={effectiveLead} onValueChange={setLead} disabled={memberIds.length === 0}>
              <SelectTrigger id="team-lead">
                <SelectValue placeholder="Add people first" />
              </SelectTrigger>
              <SelectContent>
                {memberIds.map((id) => (
                  <SelectItem key={id} value={id}>
                    {memberLabel(byId.get(id))}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <DialogFooter>
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
