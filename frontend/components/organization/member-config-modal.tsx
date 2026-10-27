"use client"

import { useEffect, useMemo, useState } from "react"
import { Crown, Loader2, Shield, Trash2 } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import { CapabilityPicker } from "@/components/organization/capability-picker"
import type {
  CapabilityGroup,
  MemberPermissions,
  OrgMemberEndpoints,
  OrgMemberRole,
  OrganizationMember,
} from "@/types/organization"

/**
 * Everything about one member, in one place.
 *
 * This replaces a page where the roster and the permissions were two separate
 * lists of the same people: you read somebody's name at the top, scrolled past
 * everything else, and found their capabilities again at the bottom. Deciding
 * about a person meant holding two rows in your head. Now clicking the person
 * is the whole interaction.
 *
 * Changes are drafted and saved together rather than applied as you tick, so a
 * member is never briefly live with a half-configured set — which matters most
 * when narrowing somebody, because the intermediate state is the permissive
 * one.
 *
 * Role and permissions are saved as two requests because they are two
 * endpoints. The role goes first: promoting to admin makes the capability set
 * irrelevant (an admin is unrestricted), and the permissions endpoint refuses
 * to write a grant set for an admin at all, so sending them the other way
 * round would fail on a change that is actually valid.
 */
export function MemberConfigModal({
  open,
  onOpenChange,
  member,
  memberName,
  endpoints,
  catalog,
  permissions,
  canEditRole,
  canRemove,
  administrative,
  canEditAdministrative,
  administrativeEndpoint,
  onTransferSuperadmin,
  onSaved,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  member: OrganizationMember | null
  memberName: string
  endpoints: OrgMemberEndpoints
  catalog: CapabilityGroup[]
  permissions: MemberPermissions | null
  canEditRole: boolean
  canRemove: boolean
  /**
   * The SFLuv-only group, present only inside the house organization.
   *
   * Kept on this modal rather than left on a separate screen because it is the
   * only place these are granted — including the capability that lets somebody
   * manage other organizations' members. Writing to its own endpoint, which is
   * own-organization only: these are deliberately not reachable from the
   * platform admin panel against a partner, since granting another
   * organization's user power over our protocol accounts is a different
   * decision wearing the same checkbox.
   */
  administrative?: CapabilityGroup
  canEditAdministrative?: boolean
  administrativeEndpoint?: string
  /** Offered only where a superadmin is doing the configuring. */
  onTransferSuperadmin?: (member: OrganizationMember) => void
  onSaved: () => void
}) {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [role, setRole] = useState<OrgMemberRole>("member")
  const [mode, setMode] = useState<"inherit" | "custom">("custom")
  const [granted, setGranted] = useState<Set<string>>(new Set())
  const [adminGranted, setAdminGranted] = useState<Set<string>>(new Set())
  const [saving, setSaving] = useState(false)

  // Re-seeded whenever a different person is opened, so the draft never starts
  // from the last member's answer.
  useEffect(() => {
    if (!open || !member) return
    setRole(member.role)
    setMode(permissions?.mode === "inherit" ? "inherit" : "custom")
    setGranted(new Set(permissions?.granted ?? []))
    setAdminGranted(new Set(permissions?.administrative ?? []))
  }, [open, member, permissions])

  const isSuperadmin = member?.role === "superadmin"
  // An admin is unrestricted whatever the table says, so the tree is hidden
  // rather than shown ticked — showing it would invite somebody to untick a
  // box that has no effect.
  const showCapabilities = role === "member" && !isSuperadmin

  const dirty = useMemo(() => {
    if (!member || !permissions) return false
    if (role !== member.role) return true
    if (mode !== (permissions.mode === "inherit" ? "inherit" : "custom")) return true
    const before = new Set(permissions.granted)
    if (before.size !== granted.size) return true
    for (const key of granted) if (!before.has(key)) return true
    const adminBefore = new Set(permissions.administrative ?? [])
    if (adminBefore.size !== adminGranted.size) return true
    for (const key of adminGranted) if (!adminBefore.has(key)) return true
    return false
  }, [member, permissions, role, mode, granted, adminGranted])

  if (!member) return null

  const save = async () => {
    setSaving(true)
    try {
      if (role !== member.role) {
        const res = await authFetch(endpoints.role, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ user_id: member.user_id, role }),
        })
        if (!res.ok) {
          toast({ title: "Could not change the role", description: (await res.text()).trim(), variant: "destructive" })
          return
        }
      }

      // Only for a member. An admin is unrestricted and the endpoint refuses a
      // grant set for one, so sending it would turn a valid promotion into an
      // error.
      if (role === "member") {
        const res = await authFetch(endpoints.permissions, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            user_id: member.user_id,
            mode,
            granted: mode === "inherit" ? [] : Array.from(granted),
          }),
        })
        if (!res.ok) {
          toast({
            title: "Could not save permissions",
            description: (await res.text()).trim(),
            variant: "destructive",
          })
          return
        }
      }

      // Its own endpoint, and sent whatever the role is: administrative grants
      // are not inherited and an admin does not get them by being an admin, so
      // this is the one set that still means something for an org admin.
      if (administrativeEndpoint && canEditAdministrative) {
        const res = await authFetch(administrativeEndpoint, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ user_id: member.user_id, granted: Array.from(adminGranted) }),
        })
        if (!res.ok) {
          toast({
            title: "Could not save administrative access",
            description: (await res.text()).trim(),
            variant: "destructive",
          })
          return
        }
      }

      toast({ title: "Member updated" })
      onSaved()
      onOpenChange(false)
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    setSaving(true)
    try {
      const res = await authFetch(endpoints.removeMember(member.user_id), { method: "DELETE" })
      if (!res.ok) {
        toast({ title: "Could not remove them", description: (await res.text()).trim(), variant: "destructive" })
        return
      }
      toast({ title: "Member removed" })
      onSaved()
      onOpenChange(false)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {memberName}
            {member.role === "superadmin" && <Crown className="h-4 w-4 text-amber-500" />}
            {member.role === "admin" && <Shield className="h-4 w-4 text-primary" />}
          </DialogTitle>
          <DialogDescription>
            {member.contact_email || member.user_id}
          </DialogDescription>
        </DialogHeader>

        {isSuperadmin ? (
          <p className="rounded-lg border bg-muted/30 p-3 text-sm text-muted-foreground">
            The superadmin has full access and cannot be restricted or removed. Transfer the role to somebody else
            first if that needs to change.
          </p>
        ) : (
          <div className="space-y-5">
            <div className="space-y-2">
              <Label className="text-xs uppercase tracking-wide text-muted-foreground">Role</Label>
              <Select
                value={role}
                onValueChange={(next) => setRole(next as OrgMemberRole)}
                disabled={!canEditRole || saving}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="member">Member</SelectItem>
                  <SelectItem value="admin">Admin</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                {role === "admin"
                  ? "Admins have full access to everything the organization holds, and can invite and remove members."
                  : "Members get only the capabilities ticked below."}
              </p>
            </div>

            {showCapabilities && (
              <>
                <Separator />
                <CapabilityPicker
                  catalog={catalog}
                  value={granted}
                  onChange={setGranted}
                  mode={mode}
                  onModeChange={setMode}
                  disabled={saving}
                />
              </>
            )}

            {(canRemove || onTransferSuperadmin) && (
              <>
                <Separator />
                <div className="flex flex-wrap gap-2">
                  {onTransferSuperadmin && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={saving}
                      onClick={() => {
                        onOpenChange(false)
                        onTransferSuperadmin(member)
                      }}
                    >
                      <Crown className="mr-2 h-3.5 w-3.5" />
                      Make superadmin
                    </Button>
                  )}
                  {canRemove && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="text-red-600 hover:text-red-700"
                      disabled={saving}
                      onClick={() => void remove()}
                    >
                      <Trash2 className="mr-2 h-3.5 w-3.5" />
                      Remove from organization
                    </Button>
                  )}
                </div>
              </>
            )}
          </div>
        )}

          {/*
          Outside the role branch on purpose. Administrative grants are never
          inherited, so a superadmin does not hold them by being a superadmin —
          which means the superadmin's own row is exactly where somebody has to
          be able to grant them. Putting this inside the "members only" branch
          would have quietly removed the only way to give the house superadmin
          protocol access.
        */}
        {administrative && canEditAdministrative && (
            <>
              <Separator />
              <div className="space-y-2">
                <Label className="text-xs uppercase tracking-wide text-muted-foreground">
                  {administrative.label} — SFLuv only
                </Label>
                <p className="text-xs text-muted-foreground">
                  Granted one at a time and never inherited, so an admin does not hold these by being an admin.
                </p>
                <div className="space-y-2">
                  {administrative.capabilities.map((capability) => (
                    <label
                      key={capability.key}
                      className="flex cursor-pointer items-start gap-3 rounded-lg border p-3 hover:bg-muted/40"
                    >
                      <Checkbox
                        className="mt-0.5"
                        checked={adminGranted.has(capability.key)}
                        disabled={saving}
                        onCheckedChange={(checked) => {
                          const next = new Set(adminGranted)
                          if (checked === true) next.add(capability.key)
                          else next.delete(capability.key)
                          setAdminGranted(next)
                        }}
                      />
                      <span className="min-w-0">
                        <span className="block text-sm font-medium text-black dark:text-white">
                          {capability.label}
                        </span>
                        <span className="block text-xs text-muted-foreground">{capability.hint}</span>
                      </span>
                    </label>
                  ))}
                </div>
              </div>
            </>
          )}

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            Close
          </Button>
          {!isSuperadmin && (
            <Button onClick={() => void save()} disabled={saving || !dirty}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Save changes
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
