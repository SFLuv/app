"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { Bell, Loader2 } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import type { OrganizationMember } from "@/types/organization"
import type { NotificationSetsView } from "@/types/notifications"

/**
 * Who gets told when something needs attention.
 *
 * This is a subscription list, not a permission list, and keeping the two
 * apart is the thing the copy here has to do. Ticking somebody into "Admin
 * alerts" means they will be told that an affiliate is waiting on a decision.
 * It does not give them the power to make that decision — the approve button
 * appears for administrators and nobody else, and a recipient who is not one
 * sees the request and cannot act on it.
 *
 * The other difference from permissions: nothing here is on by default. An
 * unticked box means no alerts rather than all of them, which is the opposite
 * of how the permissions panel beside it reads, so the empty state says it
 * loudly — alerts going to nobody looks exactly like a system with nothing
 * wrong with it.
 */
export function NotificationSets({
  members,
  canManage,
}: {
  members: OrganizationMember[]
  canManage: boolean
}) {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [view, setView] = useState<NotificationSetsView | null>(null)
  const [loading, setLoading] = useState(true)
  const [savingUser, setSavingUser] = useState("")

  const load = useCallback(async () => {
    try {
      const res = await authFetch("/organizations/mine/notification-sets")
      if (!res.ok) return
      setView((await res.json()) as NotificationSetsView)
    } catch {
      // Additive panel: a failure here leaves the rest of the page usable.
    } finally {
      setLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  const byUser = useMemo(() => {
    const map = new Map<string, string[]>()
    for (const entry of view?.recipients ?? []) map.set(entry.user_id, entry.sets)
    return map
  }, [view])

  const save = async (userId: string, sets: string[]) => {
    setSavingUser(userId)
    // Optimistic, because a checkbox that waits for a round trip before moving
    // feels broken, and the reload below is the correction if it did not take.
    setView((current) =>
      current
        ? {
            ...current,
            recipients: current.recipients.map((entry) =>
              entry.user_id === userId ? { ...entry, sets } : entry,
            ),
          }
        : current,
    )
    try {
      const res = await authFetch("/organizations/mine/notification-sets", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user_id: userId, sets }),
      })
      if (!res.ok) {
        const text = await res.text()
        toast({
          title: "Could not save alerts",
          description: text || "Please try again.",
          variant: "destructive",
        })
      }
    } catch {
      toast({ title: "Could not save alerts", description: "Please try again.", variant: "destructive" })
    } finally {
      setSavingUser("")
      await load()
    }
  }

  if (loading || !view) return null
  // No catalog means this is not the house organization. Partner organizations
  // will have their own sets later; until then the panel simply is not there.
  if (view.catalog.length === 0) return null

  const editable = canManage && view.can_edit
  const unreceived = view.catalog.filter(
    (set) => !view.recipients.some((recipient) => recipient.sets.includes(set.id)),
  )

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base text-black dark:text-white">
          <Bell className="h-4 w-4" />
          Internal alerts
        </CardTitle>
        <CardDescription>
          Operational alerts are delivered to the SFLuv mobile app. Tick who should receive each kind.
          Receiving an alert is not permission to act on it — approve and reject only appear for
          administrators.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {/* The failure this panel exists to prevent. An alert nobody receives
            is posted, delivered to no one, and indistinguishable from nothing
            having gone wrong. */}
        {unreceived.length > 0 ? (
          <div className="rounded-lg border border-amber-500/50 bg-amber-500/5 p-3 text-sm">
            <span className="font-medium text-black dark:text-white">
              {unreceived.length === 1 ? "One kind of alert goes" : `${unreceived.length} kinds of alert go`} to
              nobody:
            </span>{" "}
            <span className="text-muted-foreground">{unreceived.map((set) => set.label).join(", ")}.</span>{" "}
            <span className="text-muted-foreground">
              These are still raised — they just reach no one, which looks the same as nothing being wrong.
            </span>
          </div>
        ) : null}

        <div className="space-y-2">
          {view.catalog.map((set) => (
            <div key={set.id} className="flex flex-wrap items-baseline gap-2 text-sm">
              <span className="font-medium text-black dark:text-white">{set.label}</span>
              {view.live_counts[set.id] ? (
                <Badge variant="destructive">{view.live_counts[set.id]} waiting</Badge>
              ) : null}
              <span className="text-muted-foreground">{set.description}</span>
            </div>
          ))}
        </div>

        <div className="space-y-3 border-t pt-4">
          {members.map((member) => {
            const sets = byUser.get(member.user_id)
            if (!sets) return null
            const name = member.contact_name || member.contact_email || member.user_id

            return (
              <div key={member.user_id} className="flex flex-wrap items-center gap-x-6 gap-y-2">
                <div className="min-w-[12rem] flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-black dark:text-white">{name}</span>
                    {savingUser === member.user_id ? (
                      <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                    ) : null}
                  </div>
                  <span className="text-xs text-muted-foreground">
                    {sets.length === 0
                      ? "No alerts"
                      : `${sets.length} of ${view.catalog.length} kinds`}
                  </span>
                </div>
                <div className="flex flex-wrap gap-x-5 gap-y-2">
                  {view.catalog.map((set) => {
                    const checked = sets.includes(set.id)
                    return (
                      <label
                        key={set.id}
                        className="flex items-center gap-2 text-sm"
                        // Not a title attribute: the description is already on
                        // screen above, and a tooltip repeating it would be the
                        // only way a keyboard user could reach it.
                      >
                        <Checkbox
                          checked={checked}
                          disabled={!editable || savingUser === member.user_id}
                          onCheckedChange={(next) =>
                            void save(
                              member.user_id,
                              next ? [...sets, set.id] : sets.filter((id) => id !== set.id),
                            )
                          }
                          aria-label={`${name}: ${set.label}`}
                        />
                        <span className={checked ? "text-black dark:text-white" : "text-muted-foreground"}>
                          {set.label}
                        </span>
                      </label>
                    )
                  })}
                </div>
              </div>
            )
          })}
        </div>
      </CardContent>
    </Card>
  )
}
