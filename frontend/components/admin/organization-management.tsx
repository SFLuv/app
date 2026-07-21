"use client"

import { useCallback, useEffect, useState } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Crown, RefreshCw } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { useApp } from "@/context/AppProvider"
import type { AllocationCycle, OrganizationView, OrgRoleType } from "@/types/organization"

const cycles: AllocationCycle[] = ["daily", "weekly", "monthly", "one_time"]
const roleTypes: OrgRoleType[] = ["affiliate", "proposer", "supervisor", "issuer"]

// Platform-admin organization management: superadmin reassignment by email,
// per-cycle allocations, and org role approval.
export function OrganizationManagement() {
  const { authFetch } = useApp()
  const { toast } = useToast()
  const [orgs, setOrgs] = useState<OrganizationView[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [superadminDrafts, setSuperadminDrafts] = useState<Record<number, string>>({})
  const [allocDrafts, setAllocDrafts] = useState<Record<string, string>>({})

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await authFetch("/admin/organizations")
      if (!res.ok) throw new Error()
      setOrgs(((await res.json()) as OrganizationView[]) || [])
    } catch {
      toast({ title: "Unable to load organizations", variant: "destructive" })
    } finally {
      setLoading(false)
    }
  }, [authFetch, toast])

  useEffect(() => {
    void load()
  }, [load])

  const act = async (fn: () => Promise<Response>, okMessage: string) => {
    setBusy(true)
    try {
      const res = await fn()
      if (!res.ok) {
        toast({ title: "Something went wrong", description: (await res.text()) || undefined, variant: "destructive" })
        return
      }
      toast({ title: okMessage })
      await load()
    } catch {
      toast({ title: "Something went wrong", variant: "destructive" })
    } finally {
      setBusy(false)
    }
  }

  const setSuperadmin = (orgId: number) =>
    act(
      () =>
        authFetch("/admin/organizations/superadmin", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ organization_id: orgId, email: superadminDrafts[orgId] || "" }),
        }),
      "Superadmin updated",
    )

  const setAllocation = (orgId: number, cycle: AllocationCycle) => {
    const key = `${orgId}:${cycle}`
    const amount = Number.parseInt(allocDrafts[key] || "", 10)
    if (!Number.isFinite(amount) || amount < 0) {
      toast({ title: "Enter a valid allocation amount", variant: "destructive" })
      return
    }
    void act(
      () =>
        authFetch("/admin/organizations/allocations", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ organization_id: orgId, cycle, allocation: amount }),
        }),
      "Allocation updated",
    )
  }

  const setRoleStatus = (orgId: number, roleType: OrgRoleType, status: string) =>
    act(
      () =>
        authFetch("/admin/organizations/roles", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ organization_id: orgId, role_type: roleType, status }),
        }),
      `Role ${status}`,
    )

  if (loading) {
    return <p className="py-8 text-center text-sm text-muted-foreground">Loading organizations…</p>
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold">Organizations ({orgs.length})</h3>
        <Button variant="outline" size="sm" onClick={() => void load()} disabled={busy}>
          <RefreshCw className="mr-2 h-4 w-4" /> Refresh
        </Button>
      </div>

      {orgs.map((view) => {
        const org = view.organization
        const superadmin = view.members.find((m) => m.role === "superadmin")
        return (
          <Card key={org.id}>
            <CardHeader className="pb-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-3">
                  <Avatar className="h-10 w-10 rounded-lg">
                    <AvatarImage src={org.logo || undefined} alt={org.name} className="object-cover" />
                    <AvatarFallback className="rounded-lg">
                      {org.name.split(" ").filter(Boolean).slice(0, 2).map((n) => n[0]).join("")}
                    </AvatarFallback>
                  </Avatar>
                  <div>
                    <CardTitle className="text-base">{org.name}</CardTitle>
                    <CardDescription className="flex items-center gap-1 text-xs">
                      <Crown className="h-3 w-3 text-amber-500" />
                      {superadmin ? superadmin.contact_email || superadmin.user_id : "no superadmin"}
                      <span className="ml-2">{view.members.length} member{view.members.length === 1 ? "" : "s"}</span>
                    </CardDescription>
                  </div>
                </div>
                <div className="flex flex-wrap gap-1">
                  {view.roles.map((r) => (
                    <Badge
                      key={r.role_type}
                      variant={r.status === "approved" ? "default" : "outline"}
                      className={r.status === "approved" ? "bg-green-600" : r.status === "pending" ? "text-yellow-700 dark:text-yellow-300" : "text-red-600"}
                    >
                      {r.role_type}: {r.status}
                    </Badge>
                  ))}
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Role approvals */}
              <div className="flex flex-wrap items-center gap-2">
                {roleTypes.map((rt) => {
                  const existing = view.roles.find((r) => r.role_type === rt)
                  if (!existing) return null
                  return (
                    <div key={rt} className="flex items-center gap-1 rounded-lg border p-2">
                      <span className="text-xs font-medium capitalize">{rt}</span>
                      <Select value={existing.status} onValueChange={(v) => void setRoleStatus(org.id, rt, v)} disabled={busy}>
                        <SelectTrigger className="h-8 w-32 text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="pending">pending</SelectItem>
                          <SelectItem value="approved">approved</SelectItem>
                          <SelectItem value="rejected">rejected</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  )
                })}
              </div>

              {/* Superadmin reassignment */}
              <div className="flex flex-wrap items-end gap-2">
                <div className="space-y-1">
                  <Label className="text-xs">Superadmin email (member)</Label>
                  <Input
                    className="h-9 w-64"
                    type="email"
                    placeholder={superadmin?.contact_email || "member@example.org"}
                    value={superadminDrafts[org.id] ?? ""}
                    onChange={(e) => setSuperadminDrafts((d) => ({ ...d, [org.id]: e.target.value }))}
                  />
                </div>
                <Button size="sm" variant="outline" disabled={busy || !(superadminDrafts[org.id] || "").trim()} onClick={() => void setSuperadmin(org.id)}>
                  Set superadmin
                </Button>
              </div>

              {/* Allocations by cycle */}
              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                {cycles.map((cycle) => {
                  const existing = view.allocations.find((al) => al.cycle === cycle)
                  const key = `${org.id}:${cycle}`
                  return (
                    <div key={cycle} className="space-y-1 rounded-lg border p-2">
                      <p className="text-xs font-medium capitalize">{cycle.replace("_", "-")}</p>
                      <p className="text-xs text-muted-foreground">
                        {existing ? `${existing.balance.toLocaleString()} / ${existing.allocation.toLocaleString()}` : "not set"}
                      </p>
                      <div className="flex gap-1">
                        <Input
                          className="h-8 text-xs"
                          type="number"
                          min="0"
                          placeholder={existing ? String(existing.allocation) : "0"}
                          value={allocDrafts[key] ?? ""}
                          onChange={(e) => setAllocDrafts((d) => ({ ...d, [key]: e.target.value }))}
                        />
                        <Button size="sm" variant="outline" className="h-8 px-2 text-xs" disabled={busy || !(allocDrafts[key] || "").trim()} onClick={() => setAllocation(org.id, cycle)}>
                          Set
                        </Button>
                      </div>
                    </div>
                  )
                })}
              </div>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
