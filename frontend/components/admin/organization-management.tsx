"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Building2,
  CalendarDays,
  ChevronLeft,
  ChevronRight,
  Crown,
  Plus,
  RefreshCw,
  Search,
  Users,
  X,
} from "lucide-react"
import { EventModal } from "@/components/events/event-modal"
import { useToast } from "@/hooks/use-toast"
import { useApp } from "@/context/AppProvider"
import type { Event } from "@/types/event"
import type { AllocationCycle, OrganizationView, OrgRoleType } from "@/types/organization"

const roleTypes: OrgRoleType[] = ["affiliate", "proposer", "supervisor", "issuer"]
const cycleOrder: AllocationCycle[] = ["one_time", "daily", "weekly", "monthly"]
const cycleLabels: Record<AllocationCycle, string> = {
  one_time: "One time",
  daily: "Daily",
  weekly: "Weekly",
  monthly: "Monthly",
}

const PAGE_SIZE = 8
const EVENTS_PAGE_SIZE = 8

type SortKey = "name" | "members" | "newest"
type FilterKey = "all" | "affiliate" | "pending"

interface AllocationDraftRow {
  key: number
  cycle: AllocationCycle | ""
  amount: string
}

const orgInitials = (name: string) =>
  name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((n) => n[0])
    .join("")
    .toUpperCase()

const memberLabel = (m: { contact_name?: string; contact_email?: string; user_id: string }) =>
  m.contact_name || m.contact_email || m.user_id

// Platform-admin organization management: a paginated, searchable, sortable,
// filterable org list; clicking an org opens a details modal with tabs for
// approvals, members, and (for approved affiliates) allocations and events.
export function OrganizationManagement() {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [orgs, setOrgs] = useState<OrganizationView[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [busy, setBusy] = useState(false)

  const [search, setSearch] = useState("")
  const [filter, setFilter] = useState<FilterKey>("all")
  const [sort, setSort] = useState<SortKey>("name")
  const [page, setPage] = useState(0)

  const [selectedOrgId, setSelectedOrgId] = useState<number | null>(null)
  const [detailTab, setDetailTab] = useState("approvals")
  const [superadminDraft, setSuperadminDraft] = useState("")

  const [allocDrafts, setAllocDrafts] = useState<AllocationDraftRow[]>([])
  const [allocDraftSeq, setAllocDraftSeq] = useState(0)

  const [orgEvents, setOrgEvents] = useState<Event[]>([])
  const [orgEventsLoading, setOrgEventsLoading] = useState(false)
  const [orgEventsLoaded, setOrgEventsLoaded] = useState(false)
  const [orgEventsPage, setOrgEventsPage] = useState(0)
  const [orgEventsSearch, setOrgEventsSearch] = useState("")
  const [orgEventsActiveOnly, setOrgEventsActiveOnly] = useState(true)
  const [orgEventsRefresh, setOrgEventsRefresh] = useState(0)
  const [orgEventDetail, setOrgEventDetail] = useState<Event | null>(null)
  const [deleteOrgEventError, setDeleteOrgEventError] = useState<string | null>(null)

  // Background-refresh friendly: the "Loading…" placeholder only ever shows
  // before the FIRST load. Every later load() (refresh button, post-action
  // reloads) swaps data in place with no visual reset — the open modal included,
  // since it derives its org from `orgs` rather than holding a snapshot.
  const load = useCallback(async () => {
    setRefreshing(true)
    try {
      const res = await authFetch("/admin/organizations")
      if (!res.ok) throw new Error()
      setOrgs(((await res.json()) as OrganizationView[]) || [])
    } catch {
      toast({ title: "Unable to load organizations", variant: "destructive" })
    } finally {
      setLoading(false)
      setRefreshing(false)
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
        return false
      }
      toast({ title: okMessage })
      await load()
      return true
    } catch {
      toast({ title: "Something went wrong", variant: "destructive" })
      return false
    } finally {
      setBusy(false)
    }
  }

  const superadminOf = (view: OrganizationView) => view.members.find((m) => m.role === "superadmin")
  const isAffiliateApproved = (view: OrganizationView) =>
    view.roles.some((r) => r.role_type === "affiliate" && r.status === "approved")
  const hasPendingRole = (view: OrganizationView) => view.roles.some((r) => r.status === "pending")

  // ── List: search / filter / sort / paginate (client-side over the full list) ─
  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase()
    let list = orgs.filter((view) => {
      if (filter === "affiliate" && !isAffiliateApproved(view)) return false
      if (filter === "pending" && !hasPendingRole(view)) return false
      if (!term) return true
      const superadmin = superadminOf(view)
      return (
        view.organization.name.toLowerCase().includes(term) ||
        (superadmin ? memberLabel(superadmin).toLowerCase().includes(term) : false) ||
        (superadmin?.contact_email || "").toLowerCase().includes(term)
      )
    })
    list = [...list].sort((a, b) => {
      if (sort === "members") return b.members.length - a.members.length
      if (sort === "newest")
        return new Date(b.organization.created_at).getTime() - new Date(a.organization.created_at).getTime()
      return a.organization.name.localeCompare(b.organization.name)
    })
    return list
  }, [orgs, search, filter, sort])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const currentPage = Math.min(page, totalPages - 1)
  const pageItems = filtered.slice(currentPage * PAGE_SIZE, currentPage * PAGE_SIZE + PAGE_SIZE)

  useEffect(() => {
    setPage(0)
  }, [search, filter, sort])

  // ── Details modal ────────────────────────────────────────────────────────────
  const selected = useMemo(
    () => (selectedOrgId == null ? null : orgs.find((v) => v.organization.id === selectedOrgId) || null),
    [orgs, selectedOrgId],
  )

  const resetAllocDrafts = useCallback((view: OrganizationView | null) => {
    const rows = (view?.allocations || []).map((al, i) => ({
      key: i,
      cycle: al.cycle,
      amount: String(al.allocation),
    }))
    setAllocDrafts(rows)
    setAllocDraftSeq(rows.length)
  }, [])

  const openOrg = (view: OrganizationView) => {
    setSelectedOrgId(view.organization.id)
    setDetailTab("approvals")
    setSuperadminDraft("")
    resetAllocDrafts(view)
    setOrgEvents([])
    setOrgEventsLoaded(false)
    setOrgEventsPage(0)
    setOrgEventsSearch("")
    setOrgEventsActiveOnly(true)
    setOrgEventDetail(null)
    setDeleteOrgEventError(null)
  }

  const closeOrg = () => setSelectedOrgId(null)

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

  const setSuperadmin = async (orgId: number) => {
    const ok = await act(
      () =>
        authFetch("/admin/organizations/superadmin", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ organization_id: orgId, email: superadminDraft }),
        }),
      "Superadmin updated",
    )
    if (ok) setSuperadminDraft("")
  }

  // ── Allocations draft list (add / remove / save-replaces-all) ────────────────
  const usedCycles = allocDrafts.map((r) => r.cycle).filter(Boolean) as AllocationCycle[]
  const nextFreeCycle = cycleOrder.find((c) => !usedCycles.includes(c))

  const addAllocRow = () => {
    if (!nextFreeCycle) return
    setAllocDrafts((rows) => [...rows, { key: allocDraftSeq, cycle: nextFreeCycle, amount: "" }])
    setAllocDraftSeq((n) => n + 1)
  }

  const removeAllocRow = (key: number) => setAllocDrafts((rows) => rows.filter((r) => r.key !== key))

  const updateAllocRow = (key: number, patch: Partial<AllocationDraftRow>) =>
    setAllocDrafts((rows) => rows.map((r) => (r.key === key ? { ...r, ...patch } : r)))

  // Whole non-negative integers only — parseInt would silently truncate
  // "12.7" to 12 or "1e3" to 1, so reject anything but plain digits.
  const isValidAmount = (value: string) => /^\d+$/.test(value.trim())
  const allocRowsValid = allocDrafts.every((r) => r.cycle && isValidAmount(r.amount))

  const allocDirty = useMemo(() => {
    if (!selected) return false
    const server = new Map(selected.allocations.map((al) => [al.cycle, al.allocation]))
    if (allocDrafts.length !== server.size) return true
    return allocDrafts.some(
      (r) => !r.cycle || !isValidAmount(r.amount) || server.get(r.cycle) !== Number.parseInt(r.amount.trim(), 10),
    )
  }, [selected, allocDrafts])

  const saveAllocations = async () => {
    if (!selected || !allocRowsValid) return
    const ok = await act(
      () =>
        authFetch("/admin/organizations/allocations", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            organization_id: selected.organization.id,
            allocations: allocDrafts.map((r) => ({
              cycle: r.cycle,
              allocation: Number.parseInt(r.amount.trim(), 10),
            })),
          }),
        }),
      "Allocations saved",
    )
    if (!ok) return
  }

  const handleDeleteOrgEvent = async (id: string) => {
    try {
      const res = await authFetch(`/events/${id}`, { method: "DELETE" })
      if (res.status !== 200) throw new Error()
      setOrgEventDetail(null)
      setOrgEventsRefresh((n) => n + 1)
      toast({ title: "Event deleted" })
    } catch {
      setDeleteOrgEventError("Unable to delete this event. Please try again later.")
    }
  }

  // Rebase the allocation drafts on fresh server data after a save.
  useEffect(() => {
    if (selected && !allocDirty) resetAllocDrafts(selected)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orgs])

  // ── Org events (lazy-loaded when the Events tab is open) ─────────────────────
  useEffect(() => {
    if (!selected || detailTab !== "events" || !isAffiliateApproved(selected)) return
    const orgId = selected.organization.id
    // Latest-wins: a stale (slower) response must never overwrite a newer one,
    // and nothing should land after the modal closes or the params change.
    let cancelled = false
    const timer = setTimeout(() => {
      void (async () => {
        setOrgEventsLoading(true)
        try {
          const params = new URLSearchParams({ page: String(orgEventsPage), count: String(EVENTS_PAGE_SIZE) })
          if (orgEventsSearch.trim()) params.set("search", orgEventsSearch.trim())
          // Backend semantics: expired=true HIDES expired events (active-only);
          // omitting it returns everything.
          if (orgEventsActiveOnly) params.set("expired", "true")
          const res = await authFetch(`/admin/organizations/${orgId}/events?${params.toString()}`)
          if (!res.ok) throw new Error()
          const data = ((await res.json()) as Event[]) || []
          if (!cancelled) setOrgEvents(data)
        } catch {
          if (!cancelled) toast({ title: "Unable to load organization events", variant: "destructive" })
        } finally {
          if (!cancelled) {
            setOrgEventsLoading(false)
            setOrgEventsLoaded(true)
          }
        }
      })()
    }, 250)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedOrgId, detailTab, orgEventsPage, orgEventsSearch, orgEventsActiveOnly, orgEventsRefresh])

  // If a background refresh revokes affiliate approval while the admin is on a
  // now-hidden tab, snap back to Approvals instead of showing a blank body.
  useEffect(() => {
    if (!selected) return
    if ((detailTab === "allocations" || detailTab === "events") && !isAffiliateApproved(selected)) {
      setDetailTab("approvals")
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orgs, detailTab, selectedOrgId])

  // ── Render ───────────────────────────────────────────────────────────────────
  if (loading) {
    return <p className="py-8 text-center text-sm text-muted-foreground">Loading organizations…</p>
  }

  const selectedSuperadmin = selected ? superadminOf(selected) : undefined
  const selectedAffiliateApproved = selected ? isAffiliateApproved(selected) : false
  const detailTabCount = selectedAffiliateApproved ? 4 : 2

  return (
    <Card>
      <CardHeader className="pb-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2 text-xl">
              <Building2 className="h-6 w-6" />
              Organizations
            </CardTitle>
            <CardDescription className="mt-2">
              {filtered.length} organization{filtered.length === 1 ? "" : "s"}
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={busy || refreshing}>
            <RefreshCw className={`mr-2 h-4 w-4 ${refreshing ? "animate-spin" : ""}`} /> Refresh
          </Button>
        </div>
        <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="pl-9"
              placeholder="Search by organization or superadmin…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <Select value={filter} onValueChange={(v) => setFilter(v as FilterKey)}>
            <SelectTrigger className="w-full sm:w-[180px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All organizations</SelectItem>
              <SelectItem value="affiliate">Approved affiliates</SelectItem>
              <SelectItem value="pending">Pending approvals</SelectItem>
            </SelectContent>
          </Select>
          <Select value={sort} onValueChange={(v) => setSort(v as SortKey)}>
            <SelectTrigger className="w-full sm:w-[160px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="name">Name A–Z</SelectItem>
              <SelectItem value="members">Most members</SelectItem>
              <SelectItem value="newest">Newest</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </CardHeader>

      <CardContent className="space-y-3">
        {pageItems.length === 0 && (
          <p className="py-10 text-center text-sm text-muted-foreground">No organizations match.</p>
        )}

        {pageItems.map((view) => {
          const org = view.organization
          const superadmin = superadminOf(view)
          return (
            <Card
              key={org.id}
              className="overflow-hidden transition-shadow hover:shadow-md cursor-pointer"
              onClick={() => openOrg(view)}
              onKeyDown={(e) => {
                if (e.target !== e.currentTarget) return
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault()
                  openOrg(view)
                }
              }}
              role="button"
              tabIndex={0}
            >
              <CardContent className="p-4">
                <div className="flex items-center justify-between gap-3">
                  <div className="flex min-w-0 items-center gap-3">
                    <Avatar className="h-10 w-10 rounded-lg">
                      <AvatarImage src={org.logo || undefined} alt={org.name} className="object-cover" />
                      <AvatarFallback className="rounded-lg">{orgInitials(org.name)}</AvatarFallback>
                    </Avatar>
                    <div className="min-w-0">
                      <p className="truncate font-medium capitalize">{org.name}</p>
                      <p className="flex items-center gap-1 truncate text-xs text-muted-foreground">
                        <Crown className="h-3 w-3 shrink-0 text-amber-500" />
                        {superadmin ? memberLabel(superadmin) : "No superadmin"}
                      </p>
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-3 text-sm text-muted-foreground">
                    <span className="flex items-center gap-1">
                      <Users className="h-4 w-4" />
                      {view.members.length}
                    </span>
                    <ChevronRight className="h-4 w-4" />
                  </div>
                </div>
              </CardContent>
            </Card>
          )
        })}

        {filtered.length > PAGE_SIZE && (
          <div className="flex items-center justify-center gap-3 pt-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage(Math.max(0, currentPage - 1))}
              disabled={currentPage === 0}
            >
              <ChevronLeft className="h-4 w-4" />
              Previous
            </Button>
            <span className="text-sm text-muted-foreground">
              Page {currentPage + 1} of {totalPages}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage(Math.min(totalPages - 1, currentPage + 1))}
              disabled={currentPage >= totalPages - 1}
            >
              Next
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        )}
      </CardContent>

      {/* ── Organization details modal ─────────────────────────────────────── */}
      <Dialog open={selected != null} onOpenChange={(open) => !open && closeOrg()}>
        <DialogContent className="flex h-[min(44rem,85vh)] flex-col overflow-hidden sm:max-w-3xl">
          {selected && (
            <>
              <DialogHeader>
                <div className="flex items-center gap-3">
                  <Avatar className="h-12 w-12 rounded-lg">
                    <AvatarImage
                      src={selected.organization.logo || undefined}
                      alt={selected.organization.name}
                      className="object-cover"
                    />
                    <AvatarFallback className="rounded-lg">{orgInitials(selected.organization.name)}</AvatarFallback>
                  </Avatar>
                  <div className="min-w-0">
                    <DialogTitle className="truncate capitalize">{selected.organization.name}</DialogTitle>
                    <DialogDescription className="flex items-center gap-1">
                      <Users className="h-3.5 w-3.5" />
                      {selected.members.length} member{selected.members.length === 1 ? "" : "s"}
                    </DialogDescription>
                  </div>
                </div>
                {selected.roles.length > 0 && (
                  <div className="flex flex-wrap gap-1.5 pt-1">
                    {selected.roles.map((r) => (
                      <Badge
                        key={r.role_type}
                        className="border-transparent bg-primary text-primary-foreground capitalize hover:bg-primary/90"
                      >
                        {r.role_type}: {r.status}
                      </Badge>
                    ))}
                  </div>
                )}
              </DialogHeader>

              <Tabs value={detailTab} onValueChange={setDetailTab} className="flex min-h-0 flex-1 flex-col">
                <TabsList className={`grid h-auto w-full shrink-0 grid-cols-2 ${detailTabCount === 4 ? "sm:grid-cols-4" : ""}`}>
                  <TabsTrigger value="approvals">Approvals</TabsTrigger>
                  <TabsTrigger value="members">Members</TabsTrigger>
                  {selectedAffiliateApproved && <TabsTrigger value="allocations">Allocations</TabsTrigger>}
                  {selectedAffiliateApproved && <TabsTrigger value="events">Events</TabsTrigger>}
                </TabsList>

                {/* Approvals */}
                <TabsContent value="approvals" className="min-h-0 flex-1 space-y-2 overflow-y-auto pt-3">
                  {roleTypes.map((rt) => {
                    const existing = selected.roles.find((r) => r.role_type === rt)
                    if (!existing) return null
                    return (
                      <div key={rt} className="flex items-center justify-between gap-3 rounded-lg border p-3">
                        <div className="min-w-0">
                          <p className="text-sm font-medium capitalize">{rt}</p>
                          {existing.email && (
                            <p className="truncate text-xs text-muted-foreground">{existing.email}</p>
                          )}
                        </div>
                        <Select
                          value={existing.status}
                          onValueChange={(v) => void setRoleStatus(selected.organization.id, rt, v)}
                          disabled={busy}
                        >
                          <SelectTrigger className="h-9 w-36 capitalize">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="pending">Pending</SelectItem>
                            <SelectItem value="approved">Approved</SelectItem>
                            <SelectItem value="rejected">Rejected</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    )
                  })}
                  {selected.roles.length === 0 && (
                    <p className="py-6 text-center text-sm text-muted-foreground">No role requests yet.</p>
                  )}
                </TabsContent>

                {/* Members */}
                <TabsContent value="members" className="min-h-0 flex-1 space-y-3 overflow-y-auto pt-3">
                  <div className="space-y-2">
                    {selected.members.map((m) => (
                      <div key={m.user_id} className="flex items-center justify-between gap-3 rounded-lg border p-3">
                        <div className="flex min-w-0 items-center gap-2">
                          <Avatar className="h-8 w-8">
                            <AvatarFallback className="text-xs">
                              {orgInitials(memberLabel(m)) || "?"}
                            </AvatarFallback>
                          </Avatar>
                          <div className="min-w-0">
                            <p className="flex items-center gap-1 truncate text-sm font-medium">
                              {m.role === "superadmin" && <Crown className="h-3.5 w-3.5 shrink-0 text-amber-500" />}
                              {memberLabel(m)}
                            </p>
                            {m.contact_email && m.contact_name && (
                              <p className="truncate text-xs text-muted-foreground">{m.contact_email}</p>
                            )}
                          </div>
                        </div>
                        <Badge variant="secondary" className="shrink-0 capitalize">
                          {m.role}
                        </Badge>
                      </div>
                    ))}
                  </div>
                  <div className="space-y-2 rounded-lg border bg-secondary/40 p-3">
                    <Label className="text-xs">Set superadmin by member email</Label>
                    <div className="flex gap-2">
                      <Input
                        className="h-9"
                        type="email"
                        placeholder={selectedSuperadmin?.contact_email || "member@example.org"}
                        value={superadminDraft}
                        onChange={(e) => setSuperadminDraft(e.target.value)}
                      />
                      <Button
                        size="sm"
                        variant="outline"
                        className="h-9 shrink-0"
                        disabled={busy || !superadminDraft.trim()}
                        onClick={() => void setSuperadmin(selected.organization.id)}
                      >
                        Set superadmin
                      </Button>
                    </div>
                  </div>
                </TabsContent>

                {/* Allocations (approved affiliates only) */}
                {selectedAffiliateApproved && (
                  <TabsContent value="allocations" className="min-h-0 flex-1 space-y-3 overflow-y-auto pt-3">
                    <div className="flex items-center justify-between rounded-lg border bg-secondary/40 px-3 py-2 text-sm">
                      <span className="text-muted-foreground">Total current allocation</span>
                      <span className="font-medium">
                        {selected.allocations.reduce((sum, al) => sum + al.allocation, 0).toLocaleString()}{" "}
                        <span className="text-xs font-normal text-muted-foreground">
                          ({selected.allocations.reduce((sum, al) => sum + al.balance, 0).toLocaleString()} remaining)
                        </span>
                      </span>
                    </div>
                    <div className="space-y-2">
                      {allocDrafts.map((row) => {
                        const server = selected.allocations.find((al) => al.cycle === row.cycle)
                        return (
                          <div key={row.key} className="flex items-center gap-2 rounded-lg border p-2.5">
                            <Select
                              value={row.cycle || undefined}
                              onValueChange={(v) => updateAllocRow(row.key, { cycle: v as AllocationCycle })}
                              disabled={busy}
                            >
                              <SelectTrigger className="h-9 w-32 shrink-0">
                                <SelectValue placeholder="Cycle" />
                              </SelectTrigger>
                              <SelectContent>
                                {cycleOrder.map((c) => (
                                  <SelectItem
                                    key={c}
                                    value={c}
                                    disabled={c !== row.cycle && usedCycles.includes(c)}
                                  >
                                    {cycleLabels[c]}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                            <Input
                              className="h-9 min-w-0 flex-1"
                              type="number"
                              min="0"
                              placeholder="Amount"
                              value={row.amount}
                              onChange={(e) => updateAllocRow(row.key, { amount: e.target.value })}
                              disabled={busy}
                            />
                            {server && (
                              <span className="shrink-0 whitespace-nowrap text-xs text-muted-foreground">
                                {server.balance.toLocaleString()} / {server.allocation.toLocaleString()} left
                              </span>
                            )}
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-9 w-9 shrink-0 text-muted-foreground hover:text-destructive"
                              aria-label="Remove allocation"
                              onClick={() => removeAllocRow(row.key)}
                              disabled={busy}
                            >
                              <X className="h-4 w-4" />
                            </Button>
                          </div>
                        )
                      })}
                      {allocDrafts.length === 0 && (
                        <p className="py-4 text-center text-sm text-muted-foreground">
                          No allocations — saving now removes all funding for this organization.
                        </p>
                      )}
                    </div>
                    <div className="flex items-center justify-between gap-2">
                      <Button variant="outline" size="sm" onClick={addAllocRow} disabled={busy || !nextFreeCycle}>
                        <Plus className="mr-1 h-4 w-4" /> New allocation
                      </Button>
                      <Button
                        size="sm"
                        onClick={() => void saveAllocations()}
                        disabled={busy || !allocDirty || !allocRowsValid}
                      >
                        Save
                      </Button>
                    </div>
                  </TabsContent>
                )}

                {/* Events (approved affiliates only) */}
                {selectedAffiliateApproved && (
                  <TabsContent value="events" className="min-h-0 flex-1 space-y-3 overflow-y-auto pt-3">
                    <div className="flex flex-col gap-2 sm:flex-row">
                      <div className="relative flex-1">
                        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                        <Input
                          className="pl-9"
                          placeholder="Search events…"
                          value={orgEventsSearch}
                          onChange={(e) => {
                            setOrgEventsSearch(e.target.value)
                            setOrgEventsPage(0)
                          }}
                        />
                      </div>
                      <Select
                        value={orgEventsActiveOnly ? "active" : "all"}
                        onValueChange={(v) => {
                          setOrgEventsActiveOnly(v === "active")
                          setOrgEventsPage(0)
                        }}
                      >
                        <SelectTrigger className="w-full sm:w-[130px]">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="active">Active</SelectItem>
                          <SelectItem value="all">All events</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    {orgEventsLoading || !orgEventsLoaded ? (
                      <p className="py-6 text-center text-sm text-muted-foreground">Loading events…</p>
                    ) : (
                      <div className="space-y-2">
                        {orgEvents.map((event) => {
                          const expired = event.expiration > 0 && event.expiration * 1000 < Date.now()
                          return (
                            <div
                              key={event.id}
                              className="flex cursor-pointer items-center justify-between gap-3 rounded-lg border p-3 transition-colors hover:bg-secondary/50"
                              onClick={() => {
                                setDeleteOrgEventError(null)
                                setOrgEventDetail(event)
                              }}
                              onKeyDown={(e) => {
                                if (e.key === "Enter" || e.key === " ") {
                                  e.preventDefault()
                                  setDeleteOrgEventError(null)
                                  setOrgEventDetail(event)
                                }
                              }}
                              role="button"
                              tabIndex={0}
                            >
                              <div className="min-w-0">
                                <p className="truncate text-sm font-medium">{event.title}</p>
                                <p className="flex items-center gap-1 text-xs text-muted-foreground">
                                  <CalendarDays className="h-3.5 w-3.5 shrink-0" />
                                  {new Date(event.start_at * 1000).toLocaleDateString()} –{" "}
                                  {new Date(event.expiration * 1000).toLocaleDateString()}
                                </p>
                              </div>
                              <div className="flex shrink-0 items-center gap-2">
                                {expired && (
                                  <Badge variant="outline" className="text-muted-foreground">
                                    Expired
                                  </Badge>
                                )}
                                <Badge variant="secondary">
                                  {event.amount.toLocaleString()} × {event.codes}
                                </Badge>
                              </div>
                            </div>
                          )
                        })}
                        {orgEvents.length === 0 && (
                          <p className="py-6 text-center text-sm text-muted-foreground">No events found.</p>
                        )}
                      </div>
                    )}
                    {(orgEventsPage > 0 || orgEvents.length >= EVENTS_PAGE_SIZE) && (
                      <div className="flex items-center justify-center gap-3">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setOrgEventsPage((p) => Math.max(0, p - 1))}
                          disabled={orgEventsLoading || orgEventsPage === 0}
                        >
                          <ChevronLeft className="h-4 w-4" />
                          Previous
                        </Button>
                        <span className="text-sm text-muted-foreground">Page {orgEventsPage + 1}</span>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setOrgEventsPage((p) => p + 1)}
                          disabled={orgEventsLoading || orgEvents.length < EVENTS_PAGE_SIZE}
                        >
                          Next
                          <ChevronRight className="h-4 w-4" />
                        </Button>
                      </div>
                    )}
                  </TabsContent>
                )}
              </Tabs>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* Event details + QR code downloads (reuses the admin events modal). */}
      <EventModal
        open={orgEventDetail != null}
        onOpenChange={() => setOrgEventDetail(null)}
        event={orgEventDetail || undefined}
        handleDeleteEvent={handleDeleteOrgEvent}
        deleteEventError={deleteOrgEventError}
        ownerLabel={selected ? selected.organization.name : undefined}
      />
    </Card>
  )
}
