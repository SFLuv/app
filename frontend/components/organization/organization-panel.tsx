"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { useRouter } from "next/navigation"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { ArrowUpRight, Award, Crown, LogOut, Mail, Pencil, Shield, Trash2, Upload, Users } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { useApp } from "@/context/AppProvider"
import { LogoCropModal } from "./logo-crop-modal"
import { formatCredentialLabel } from "@/lib/credential-labels"
import type { OrganizationView, OrgRoleType } from "@/types/organization"

const roleLabels: Record<OrgRoleType, string> = {
  affiliate: "Affiliate",
  proposer: "Proposer",
  supervisor: "Supervisor",
  issuer: "Issuer",
}

const cycleLabels: Record<string, string> = {
  daily: "Daily",
  weekly: "Weekly",
  monthly: "Monthly",
  one_time: "One-time",
}

// Roles whose "settings" are just a link to their working area.
const roleLinks: Partial<Record<OrgRoleType, { href: string; label: string }>> = {
  affiliate: { href: "/affiliates", label: "Open affiliate dashboard" },
  proposer: { href: "/proposer", label: "Open proposer panel" },
  issuer: { href: "/issuer", label: "Open issuer panel" },
}

export function OrganizationPanel() {
  const { authFetch, user } = useApp()
  const { toast } = useToast()
  const router = useRouter()
  const [view, setView] = useState<OrganizationView | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)

  const [editingName, setEditingName] = useState(false)
  const [nameDraft, setNameDraft] = useState("")
  const [inviteEmail, setInviteEmail] = useState("")
  const [requestRole, setRequestRole] = useState<OrgRoleType | "">("")
  const [requestEmail, setRequestEmail] = useState("")
  const [cropOpen, setCropOpen] = useState(false)
  const [cropSrc, setCropSrc] = useState<string | null>(null)
  const [leaveOpen, setLeaveOpen] = useState(false)
  const [transferTarget, setTransferTarget] = useState<{ user_id: string; name: string } | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const isAdmin = view?.my_role === "admin" || view?.my_role === "superadmin"
  const isSuperadmin = view?.my_role === "superadmin"

  // The loading placeholder is first-load-only; later loads (after any action,
  // or a background refresh) swap data in place so the panel never flashes or
  // resets scroll — hasLoaded gates the placeholder, not each fetch.
  const [hasLoaded, setHasLoaded] = useState(false)
  const load = useCallback(async () => {
    try {
      const res = await authFetch("/organizations/mine")
      // Only a definitive 404 means "no organization" — clear the view. A
      // transient error must NOT blank an already-loaded panel to the empty
      // state (that would flash "No organization yet" on a background reload
      // after any member action), so keep the prior view on failure.
      if (res.status === 404) {
        setView(null)
        return
      }
      if (!res.ok) throw new Error()
      setView((await res.json()) as OrganizationView)
    } catch {
      // keep prior view; transient failure shouldn't drop the org
    } finally {
      setHasLoaded(true)
      setLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  const act = async (fn: () => Promise<Response>, okMessage: string) => {
    setBusy(true)
    try {
      const res = await fn()
      if (!res.ok) {
        const text = await res.text()
        toast({ title: "Something went wrong", description: text || "Please try again.", variant: "destructive" })
        return false
      }
      toast({ title: okMessage })
      await load()
      return true
    } catch {
      toast({ title: "Something went wrong", description: "Please try again.", variant: "destructive" })
      return false
    } finally {
      setBusy(false)
    }
  }

  const saveName = () =>
    act(
      () => authFetch("/organizations/mine/name", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: nameDraft }) }),
      "Organization name updated",
    ).then((ok) => ok && setEditingName(false))

  const saveLogo = (dataUrl: string) =>
    act(
      () => authFetch("/organizations/mine/logo", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ logo: dataUrl }) }),
      "Organization logo updated",
    )

  const sendInvite = () =>
    act(
      () => authFetch("/organizations/mine/invites", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email: inviteEmail }) }),
      "Invitation sent",
    ).then((ok) => ok && setInviteEmail(""))

  const removeMember = (userId: string) =>
    act(() => authFetch(`/organizations/mine/members/${encodeURIComponent(userId)}`, { method: "DELETE" }), "Member removed")

  const setMemberRole = (userId: string, role: string) =>
    act(
      () => authFetch("/organizations/mine/members/role", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ user_id: userId, role }) }),
      "Member role updated",
    )

  const confirmTransferSuperadmin = () => {
    const userId = transferTarget?.user_id
    setTransferTarget(null)
    if (!userId) return
    void act(
      () => authFetch("/organizations/mine/transfer-superadmin", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ user_id: userId }) }),
      "Superadmin transferred",
    )
  }

  const confirmLeaveOrg = () => {
    setLeaveOpen(false)
    void act(() => authFetch("/organizations/mine/leave", { method: "POST" }), "You left the organization")
  }

  const submitRoleRequest = () => {
    if (!requestRole) return
    void act(
      () =>
        authFetch("/organizations/mine/roles/request", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ role_type: requestRole, email: requestEmail }),
        }),
      "Role requested",
    ).then((ok) => {
      if (ok) {
        setRequestRole("")
        setRequestEmail("")
      }
    })
  }

  const onLogoFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      setCropSrc(reader.result as string)
      setCropOpen(true)
    }
    reader.readAsDataURL(file)
  }

  if (loading && !hasLoaded) {
    return <p className="py-8 text-center text-sm text-muted-foreground">Loading organization…</p>
  }

  if (!view) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-black dark:text-white">No organization yet</CardTitle>
          <CardDescription>
            Request an affiliate, proposer, supervisor, or issuer role to create your organization — or accept an email invite
            from an existing one.
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  const org = view.organization
  const approvedRoles = view.roles.filter((r) => r.status === "approved")
  const pendingRoles = view.roles.filter((r) => r.status === "pending")
  const requestableRoles = (Object.keys(roleLabels) as OrgRoleType[]).filter(
    (rt) => !view.roles.some((r) => r.role_type === rt && r.status !== "rejected"),
  )

  return (
    <div className="space-y-6">
      {/* Org identity */}
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-4">
              <div className="relative">
                <Avatar className="h-16 w-16 rounded-lg">
                  <AvatarImage src={org.logo || undefined} alt={org.name} className="object-cover" />
                  <AvatarFallback className="rounded-lg bg-primary text-xl text-primary-foreground">
                    {org.name.split(" ").filter(Boolean).slice(0, 2).map((n) => n[0]).join("")}
                  </AvatarFallback>
                </Avatar>
                {isAdmin && (
                  <button
                    className="absolute -bottom-1 -right-1 rounded-full border bg-background p-1 shadow-sm hover:bg-muted"
                    onClick={() => fileInputRef.current?.click()}
                    title="Upload logo"
                  >
                    <Upload className="h-3.5 w-3.5" />
                  </button>
                )}
                <input ref={fileInputRef} type="file" accept="image/*" className="hidden" onChange={onLogoFile} />
              </div>
              <div>
                {editingName ? (
                  <div className="flex items-center gap-2">
                    <Input value={nameDraft} onChange={(e) => setNameDraft(e.target.value)} className="h-9 w-56" />
                    <Button size="sm" className="bg-primary hover:bg-primary/90" onClick={() => void saveName()} disabled={busy}>
                      Save
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => setEditingName(false)}>Cancel</Button>
                  </div>
                ) : (
                  <div className="flex items-center gap-2">
                    <CardTitle className="text-black dark:text-white">{org.name}</CardTitle>
                    {isAdmin && (
                      <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => { setNameDraft(org.name); setEditingName(true) }}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                    )}
                  </div>
                )}
                <div className="mt-1 flex flex-wrap items-center gap-2">
                  <Badge variant="outline" className="capitalize">{view.my_role}</Badge>
                  {approvedRoles.map((r) => (
                    <Badge key={r.role_type} className="bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-200">
                      {roleLabels[r.role_type]}
                    </Badge>
                  ))}
                  {pendingRoles.map((r) => (
                    <Badge key={r.role_type} variant="outline" className="text-yellow-700 dark:text-yellow-300">
                      {roleLabels[r.role_type]} pending
                    </Badge>
                  ))}
                </div>
              </div>
            </div>
            <Button variant="outline" className="text-red-600 hover:bg-red-50 dark:hover:bg-red-950/30" onClick={() => setLeaveOpen(true)} disabled={busy}>
              <LogOut className="mr-2 h-4 w-4" />
              Leave organization
            </Button>
          </div>
        </CardHeader>
      </Card>

      {/* Role tools & settings — shown inline per approved role instead of
          linking out to separate settings tabs. */}
      {approvedRoles.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base text-black dark:text-white">Role tools</CardTitle>
            <CardDescription>Settings and shortcuts for this organization's approved roles.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {approvedRoles.map((r) => {
              const link = roleLinks[r.role_type]
              return (
                <div key={r.role_type} className="space-y-3 rounded-lg border p-4">
                  <div className="flex items-center justify-between gap-2">
                    <p className="text-sm font-semibold">{roleLabels[r.role_type]}</p>
                    {link && (
                      <Button variant="outline" size="sm" onClick={() => router.push(link.href)}>
                        {link.label}
                        <ArrowUpRight className="ml-1 h-3.5 w-3.5" />
                      </Button>
                    )}
                  </div>
                  {r.role_type === "issuer" && (view.issuer_scopes?.length ?? 0) > 0 && (
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="flex items-center gap-1 text-xs text-muted-foreground">
                        <Award className="h-3.5 w-3.5" /> Issuable:
                      </span>
                      {view.issuer_scopes!.map((scope) => (
                        <Badge key={scope} variant="secondary">
                          {formatCredentialLabel(scope)}
                        </Badge>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </CardContent>
        </Card>
      )}

      {/* Allocations (visible to all members) */}
      {view.allocations.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base text-black dark:text-white">Allocations</CardTitle>
            <CardDescription>Shared organization spending budgets by cycle.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {view.allocations.map((al) => (
              <div key={al.id} className="rounded-lg border bg-muted/20 p-3">
                <p className="text-xs uppercase tracking-wide text-muted-foreground">{cycleLabels[al.cycle] ?? al.cycle}</p>
                <p className="mt-1 text-lg font-semibold">
                  {al.balance.toLocaleString()} <span className="text-xs font-normal text-muted-foreground">/ {al.allocation.toLocaleString()}</span>
                </p>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      {/* Role request */}
      {requestableRoles.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base text-black dark:text-white">Request a role</CardTitle>
            <CardDescription>Request additional platform roles for this organization.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap items-end gap-3">
            <div className="space-y-1">
              <Label className="text-xs">Role</Label>
              <Select value={requestRole} onValueChange={(v) => setRequestRole(v as OrgRoleType)}>
                <SelectTrigger className="w-44">
                  <SelectValue placeholder="Select role" />
                </SelectTrigger>
                <SelectContent>
                  {requestableRoles.map((rt) => (
                    <SelectItem key={rt} value={rt}>{roleLabels[rt]}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {(requestRole === "proposer" || requestRole === "supervisor" || requestRole === "issuer") && (
              <div className="space-y-1">
                <Label className="text-xs">Notification email</Label>
                <Input className="w-56" type="email" placeholder="you@example.org" value={requestEmail} onChange={(e) => setRequestEmail(e.target.value)} />
              </div>
            )}
            <Button
              className="bg-primary hover:bg-primary/90"
              disabled={busy || !requestRole || ((requestRole === "proposer" || requestRole === "supervisor" || requestRole === "issuer") && !requestEmail.trim())}
              onClick={submitRoleRequest}
            >
              Submit request
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Members */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base text-black dark:text-white">
            <Users className="h-4 w-4" /> Members
          </CardTitle>
          <CardDescription>
            Everyone in the organization shares its roles and tools.
            {isAdmin ? " Invite teammates by email." : ""}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {isAdmin && (
            <div className="flex flex-wrap items-end gap-2">
              <div className="flex-1 space-y-1">
                <Label className="text-xs">Invite by email</Label>
                <Input type="email" placeholder="teammate@example.org" value={inviteEmail} onChange={(e) => setInviteEmail(e.target.value)} />
              </div>
              <Button className="bg-primary hover:bg-primary/90" disabled={busy || !inviteEmail.trim()} onClick={() => void sendInvite()}>
                <Mail className="mr-2 h-4 w-4" /> Invite
              </Button>
            </div>
          )}

          <div className="divide-y rounded-lg border">
            {view.members.map((m) => (
              <div key={m.user_id} className="flex flex-wrap items-center justify-between gap-2 p-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm font-medium">
                    {m.contact_name || m.contact_email || m.user_id.slice(0, 18) + "…"}
                    {m.role === "superadmin" && <Crown className="h-3.5 w-3.5 text-amber-500" />}
                    {m.role === "admin" && <Shield className="h-3.5 w-3.5 text-primary" />}
                    {m.user_id === user?.id && <Badge variant="outline" className="text-[10px]">you</Badge>}
                  </div>
                  <p className="text-xs text-muted-foreground">{m.contact_email}</p>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="outline" className="capitalize">{m.role}</Badge>
                  {isSuperadmin && m.role !== "superadmin" && (
                    <>
                      <Button size="sm" variant="outline" disabled={busy} onClick={() => void setMemberRole(m.user_id, m.role === "admin" ? "member" : "admin")}>
                        {m.role === "admin" ? "Demote" : "Make admin"}
                      </Button>
                      <Button size="sm" variant="outline" disabled={busy} onClick={() => setTransferTarget({ user_id: m.user_id, name: m.contact_name || m.contact_email || m.user_id })}>
                        Make superadmin
                      </Button>
                    </>
                  )}
                  {isAdmin && m.role !== "superadmin" && m.user_id !== user?.id && (
                    <Button size="icon" variant="ghost" className="h-8 w-8 text-red-600" disabled={busy} onClick={() => void removeMember(m.user_id)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>

          {isAdmin && (view.invites?.length ?? 0) > 0 && (
            <div>
              <p className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Pending invites</p>
              <div className="divide-y rounded-lg border">
                {view.invites!.map((inv) => (
                  <div key={inv.id} className="flex items-center justify-between p-3 text-sm">
                    <span>{inv.email}</span>
                    <span className="text-xs text-muted-foreground">expires {new Date(inv.expires_at).toLocaleDateString()}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <LogoCropModal open={cropOpen} onOpenChange={setCropOpen} src={cropSrc} onCropped={(dataUrl) => void saveLogo(dataUrl)} />

      <Dialog open={leaveOpen} onOpenChange={setLeaveOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Leave {org.name}?</DialogTitle>
            <DialogDescription>
              You will lose access to this organization's roles and tools. An admin will need to re-invite you to rejoin.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-2">
            <Button variant="outline" onClick={() => setLeaveOpen(false)} disabled={busy}>
              Cancel
            </Button>
            <Button
              className="bg-red-600 text-white hover:bg-red-700"
              onClick={confirmLeaveOrg}
              disabled={busy}
            >
              <LogOut className="mr-2 h-4 w-4" />
              Leave organization
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={transferTarget != null} onOpenChange={(open) => !open && setTransferTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Transfer superadmin?</DialogTitle>
            <DialogDescription>
              {transferTarget?.name} will become the organization's superadmin, and you will be demoted to admin. This
              cannot be undone without the new superadmin transferring it back.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-2">
            <Button variant="outline" onClick={() => setTransferTarget(null)} disabled={busy}>
              Cancel
            </Button>
            <Button onClick={confirmTransferSuperadmin} disabled={busy}>
              <Crown className="mr-2 h-4 w-4" />
              Transfer superadmin
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
