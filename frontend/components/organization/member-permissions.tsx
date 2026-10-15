"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { Loader2, ShieldCheck } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import type {
  CapabilityGroup,
  MemberPermissions,
  OrganizationMember,
  OrganizationPermissionsView,
} from "@/types/organization"

/**
 * Who in the organization can do what.
 *
 * The important thing this screen has to get across is that nobody is being
 * restricted by default. A member is "Full access" until someone changes them,
 * which is exactly what membership meant before this existed — so the empty
 * state of this panel is not "no permissions", it is "everything", and the copy
 * says so rather than leaving an admin to infer it from unticked boxes.
 *
 * Admins and superadmins are shown but not editable: their access is a
 * consequence of their role, and taking it away here would let an admin lock
 * the organization out of its own settings.
 */
export function MemberPermissions({
  members,
  canManage,
}: {
  members: OrganizationMember[]
  canManage: boolean
}) {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [view, setView] = useState<OrganizationPermissionsView | null>(null)
  const [loading, setLoading] = useState(true)
  const [savingUser, setSavingUser] = useState<string>("")

  const load = useCallback(async () => {
    try {
      const res = await authFetch("/organizations/mine/permissions")
      if (!res.ok) return
      setView((await res.json()) as OrganizationPermissionsView)
    } catch {
      // The panel is additive; a failure here leaves the rest of the page usable.
    } finally {
      setLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  const byUser = useMemo(() => {
    const map = new Map<string, MemberPermissions>()
    for (const entry of view?.members ?? []) map.set(entry.user_id, entry)
    return map
  }, [view])

  const save = async (
    userId: string,
    body: Record<string, unknown>,
    path = "/organizations/mine/permissions",
  ) => {
    setSavingUser(userId)
    try {
      const res = await authFetch(path, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user_id: userId, ...body }),
      })
      if (!res.ok) {
        const text = await res.text()
        toast({ title: "Could not save permissions", description: text || "Please try again.", variant: "destructive" })
        return
      }
      await load()
    } catch {
      toast({ title: "Could not save permissions", description: "Please try again.", variant: "destructive" })
    } finally {
      setSavingUser("")
    }
  }

  if (loading || !view) return null
  if (view.catalog.length === 0 && !view.administrative) return null

  const editable = canManage && view.can_edit

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base text-black dark:text-white">Member permissions</CardTitle>
        <CardDescription>
          Everyone has full access to this organization&apos;s tools unless you narrow it here. Admins always
          keep full access.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {members.map((member) => {
          const permissions = byUser.get(member.user_id)
          if (!permissions) return null
          const isOrgAdmin = member.role === "admin" || member.role === "superadmin"
          const name = member.contact_name || member.contact_email || member.user_id

          return (
            <MemberRow
              key={member.user_id}
              name={name}
              member={member}
              permissions={permissions}
              catalog={view.catalog}
              administrative={view.administrative}
              canEdit={editable && !isOrgAdmin}
              canEditAdministrative={view.can_edit_administrative && !isOrgAdmin}
              isOrgAdmin={isOrgAdmin}
              saving={savingUser === member.user_id}
              onSave={save}
            />
          )
        })}
      </CardContent>
    </Card>
  )
}

function MemberRow({
  name,
  member,
  permissions,
  catalog,
  administrative,
  canEdit,
  canEditAdministrative,
  isOrgAdmin,
  saving,
  onSave,
}: {
  name: string
  member: OrganizationMember
  permissions: MemberPermissions
  catalog: CapabilityGroup[]
  administrative?: CapabilityGroup
  canEdit: boolean
  canEditAdministrative: boolean
  isOrgAdmin: boolean
  saving: boolean
  onSave: (userId: string, body: Record<string, unknown>, path?: string) => Promise<void>
}) {
  const granted = new Set(permissions.granted)
  const adminGranted = new Set(permissions.administrative ?? [])

  const allKeys = catalog.flatMap((group) => group.capabilities.map((c) => c.key))
  const readKeys = catalog.flatMap((group) => group.capabilities.filter((c) => c.read).map((c) => c.key))

  const preset =
    permissions.mode === "inherit"
      ? "full"
      : allKeys.every((k) => granted.has(k))
        ? "full"
        : readKeys.length > 0 && readKeys.every((k) => granted.has(k)) && granted.size === readKeys.length
          ? "read"
          : "custom"

  const applyPreset = (next: string) => {
    if (next === "full") return onSave(member.user_id, { mode: "inherit", granted: [] })
    if (next === "read") return onSave(member.user_id, { mode: "custom", granted: readKeys })
    if (next === "none") return onSave(member.user_id, { mode: "custom", granted: [] })
    // "custom" keeps whatever is already effective, but pins it so it stops
    // tracking the organization's roles.
    return onSave(member.user_id, { mode: "custom", granted: Array.from(granted) })
  }

  const toggle = (key: string, on: boolean) => {
    const next = new Set(granted)
    if (on) next.add(key)
    else next.delete(key)
    return onSave(member.user_id, { mode: "custom", granted: Array.from(next) })
  }

  const toggleAdministrative = (key: string, on: boolean) => {
    const next = new Set(adminGranted)
    if (on) next.add(key)
    else next.delete(key)
    return onSave(
      member.user_id,
      { granted: Array.from(next) },
      "/organizations/mine/permissions/administrative",
    )
  }

  return (
    <div className="rounded-lg border p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-black dark:text-white">{name}</span>
        {isOrgAdmin && (
          <Badge variant="outline" className="border-emerald-500 text-emerald-600">
            {member.role === "superadmin" ? "Superadmin" : "Admin"} — full access
          </Badge>
        )}
        {saving && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
        {!isOrgAdmin && (
          <div className="ml-auto w-44">
            <Select value={preset} onValueChange={applyPreset} disabled={!canEdit || saving}>
              <SelectTrigger className="h-8">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="full">Full access</SelectItem>
                <SelectItem value="read">Read-only</SelectItem>
                <SelectItem value="none">No access</SelectItem>
                <SelectItem value="custom">Custom</SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}
      </div>

      {isOrgAdmin ? (
        <p className="mt-2 text-sm text-muted-foreground">
          Change this person&apos;s organization role to set specific permissions.
        </p>
      ) : (
        <div className="mt-4 space-y-4">
          {catalog.map((group) => (
            <div key={group.role_type}>
              <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {group.label}
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                {group.capabilities.map((capability) => (
                  <label
                    key={capability.key}
                    className="flex items-start gap-2 text-sm"
                    title={capability.hint}
                  >
                    <Checkbox
                      checked={granted.has(capability.key)}
                      disabled={!canEdit || saving}
                      onCheckedChange={(checked) => void toggle(capability.key, checked === true)}
                      className="mt-0.5"
                    />
                    <span>
                      <span className="text-black dark:text-white">{capability.label}</span>
                      <span className="block text-xs text-muted-foreground">{capability.hint}</span>
                    </span>
                  </label>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {/*
        SFLuv only, and only for its superadmin. These are not organization
        tools — they are the platform's own admin powers — so they sit behind
        their own heading and are never inherited: an org admin who was not
        given one does not have it.
      */}
      {administrative && (
        <div className="mt-5 rounded-md border border-amber-500/40 bg-amber-500/5 p-3">
          <div className="mb-2 flex items-center gap-2">
            <ShieldCheck className="h-4 w-4 text-amber-600" />
            <Label className="text-xs font-semibold uppercase tracking-wide text-amber-700 dark:text-amber-500">
              {administrative.label}
            </Label>
          </div>
          {canEditAdministrative ? (
            <div className="grid gap-2 sm:grid-cols-2">
              {administrative.capabilities.map((capability) => (
                <label key={capability.key} className="flex items-start gap-2 text-sm" title={capability.hint}>
                  <Checkbox
                    checked={adminGranted.has(capability.key)}
                    disabled={saving}
                    onCheckedChange={(checked) => void toggleAdministrative(capability.key, checked === true)}
                    className="mt-0.5"
                  />
                  <span>
                    <span className="text-black dark:text-white">{capability.label}</span>
                    <span className="block text-xs text-muted-foreground">{capability.hint}</span>
                  </span>
                </label>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              {adminGranted.size === 0
                ? "No administrative access."
                : `${adminGranted.size} administrative permission${adminGranted.size === 1 ? "" : "s"} — only the SFLuv superadmin can change these.`}
            </p>
          )}
        </div>
      )}
    </div>
  )
}
