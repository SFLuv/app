"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Checkbox } from "@/components/ui/checkbox"
import { Badge } from "@/components/ui/badge"
import { WorkflowDetailsModal } from "@/components/workflows/workflow-details-modal"
import { formatWorkflowDisplayStatus } from "@/lib/workflow-status"
import { cn } from "@/lib/utils"
import { SupervisorWorkflowExportRequest, SupervisorWorkflowListItem, SupervisorWorkflowListResponse, Workflow } from "@/types/workflow"
import { ChevronLeft, ChevronRight, Download, Loader2, Search } from "lucide-react"

type DateField = "created_at" | "completed_at" | "start_at"

const toLocalDateBoundaryISO = (value: string, boundary: "start" | "end"): string => {
  const trimmed = value.trim()
  if (!trimmed) return ""

  const suffix = boundary === "start" ? "T00:00:00.000" : "T23:59:59.999"
  const local = new Date(`${trimmed}${suffix}`)
  if (Number.isNaN(local.getTime())) return ""
  return local.toISOString()
}

const toMMDDYYYY = (unixSeconds: number): string => {
  const date = new Date(unixSeconds * 1000)
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  const year = String(date.getFullYear())
  return `${month}/${day}/${year}`
}

export default function SupervisorPage() {
  const { authFetch, status, user } = useApp()

  const [items, setItems] = useState<SupervisorWorkflowListItem[]>([])
  const [total, setTotal] = useState<number>(0)
  const [page, setPage] = useState<number>(0)
  const [count, setCount] = useState<number>(20)
  const [search, setSearch] = useState<string>("")
  const [statusFilter, setStatusFilter] = useState<string>("all")
  const [sortBy, setSortBy] = useState<DateField>("created_at")
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc")
  const [dateField, setDateField] = useState<DateField>("start_at")
  const [dateFrom, setDateFrom] = useState<string>("")
  const [dateTo, setDateTo] = useState<string>("")
  const [selectMode, setSelectMode] = useState<boolean>(false)
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState<boolean>(true)
  const [error, setError] = useState<string>("")
  const [notice, setNotice] = useState<string>("")
  const [exporting, setExporting] = useState<boolean>(false)

  const [detailOpen, setDetailOpen] = useState<boolean>(false)
  const [detailWorkflow, setDetailWorkflow] = useState<Workflow | null>(null)
  const [detailLoading, setDetailLoading] = useState<boolean>(false)

  const canUsePanel = Boolean(user?.isSupervisor || user?.isAdmin)

  const loadData = useCallback(async () => {
    if (!canUsePanel) {
      setLoading(false)
      return
    }

    setLoading(true)
    setError("")
    try {
      const params = new URLSearchParams({
        page: String(page),
        count: String(count),
        search,
        status: statusFilter,
        sort_by: sortBy,
        sort_direction: sortDirection,
        date_field: dateField,
      })
      const queryDateFrom = toLocalDateBoundaryISO(dateFrom, "start")
      const queryDateTo = toLocalDateBoundaryISO(dateTo, "end")
      if (queryDateFrom) params.set("date_from", queryDateFrom)
      if (queryDateTo) params.set("date_to", queryDateTo)

      const res = await authFetch(`/supervisors/workflows?${params.toString()}`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load supervisor workflows.")
      }
      const data = (await res.json()) as SupervisorWorkflowListResponse
      setItems(data.items || [])
      setTotal(data.total || 0)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load supervisor workflows.")
      setItems([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [authFetch, canUsePanel, count, dateField, dateFrom, dateTo, page, search, sortBy, sortDirection, statusFilter])

  useEffect(() => {
    if (status !== "authenticated") return
    void loadData()
  }, [status, loadData])

  const totalPages = useMemo(() => {
    if (count <= 0) return 1
    return Math.max(1, Math.ceil(total / count))
  }, [count, total])

  const toggleSelected = (workflowID: string, checked: boolean) => {
    setSelectedIDs((prev) => {
      const next = new Set(prev)
      if (checked) next.add(workflowID)
      else next.delete(workflowID)
      return next
    })
  }

  const clearSelection = () => setSelectedIDs(new Set())

  const openDetails = async (workflowID: string) => {
    setDetailOpen(true)
    setDetailLoading(true)
    setDetailWorkflow(null)
    try {
      const res = await authFetch(`/workflows/${workflowID}`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load workflow details.")
      }
      const workflow = (await res.json()) as Workflow
      setDetailWorkflow(workflow)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load workflow details.")
      setDetailOpen(false)
    } finally {
      setDetailLoading(false)
    }
  }

  const runExport = async () => {
    if (!canUsePanel || exporting) return

    setExporting(true)
    setError("")
    setNotice("")
    try {
      const payload: SupervisorWorkflowExportRequest = {
        workflow_ids: Array.from(selectedIDs),
        date_field: selectedIDs.size > 0 ? "" : dateField,
        date_from: selectedIDs.size > 0 ? "" : toLocalDateBoundaryISO(dateFrom, "start"),
        date_to: selectedIDs.size > 0 ? "" : toLocalDateBoundaryISO(dateTo, "end"),
      }
      const res = await authFetch("/supervisors/workflows/export", {
        method: "POST",
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to export workflow data.")
      }

      const blob = await res.blob()
      const url = window.URL.createObjectURL(blob)
      const anchor = document.createElement("a")
      anchor.href = url
      anchor.download = "supervisor_workflow_export.zip"
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      window.URL.revokeObjectURL(url)

      setNotice("Export generated.")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to export workflow data.")
    } finally {
      setExporting(false)
    }
  }

  if (status === "loading") {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="h-12 w-12 animate-spin rounded-full border-b-2 border-t-2 border-[#eb6c6c]" />
      </div>
    )
  }

  if (!canUsePanel) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Supervisor Panel</CardTitle>
          <CardDescription>Supervisor access is required.</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Supervisor Panel</CardTitle>
          <CardDescription>
            Review assigned workflow submissions, filter by timeframe, and export CSV + photos.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <div className="space-y-1">
              <Label>Search Title</Label>
              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input value={search} onChange={(e) => setSearch(e.target.value)} className="pl-9" placeholder="Workflow title" />
              </div>
            </div>
            <div className="space-y-1">
              <Label>Status</Label>
              <Select value={statusFilter} onValueChange={setStatusFilter}>
                <SelectTrigger><SelectValue placeholder="All statuses" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All</SelectItem>
                  <SelectItem value="pending">Pending</SelectItem>
                  <SelectItem value="approved">Approved</SelectItem>
                  <SelectItem value="rejected">Rejected</SelectItem>
                  <SelectItem value="blocked">Blocked</SelectItem>
                  <SelectItem value="in_progress">In Progress</SelectItem>
                  <SelectItem value="completed">Completed</SelectItem>
                  <SelectItem value="paid_out">Finalized</SelectItem>
                  <SelectItem value="expired">Expired</SelectItem>
                  <SelectItem value="deleted">Deleted</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>Sort By</Label>
              <Select value={sortBy} onValueChange={(value) => setSortBy(value as DateField)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="created_at">Date Created</SelectItem>
                  <SelectItem value="completed_at">Date Completed</SelectItem>
                  <SelectItem value="start_at">Start Time</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>Sort Direction</Label>
              <Select value={sortDirection} onValueChange={(value) => setSortDirection(value as "asc" | "desc")}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="desc">Newest First</SelectItem>
                  <SelectItem value="asc">Oldest First</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid gap-3 md:grid-cols-3">
            <div className="space-y-1">
              <Label>Date Field</Label>
              <Select value={dateField} onValueChange={(value) => setDateField(value as DateField)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="created_at">Date Created</SelectItem>
                  <SelectItem value="completed_at">Date Completed</SelectItem>
                  <SelectItem value="start_at">Start Time</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>Date From</Label>
              <Input type="date" value={dateFrom} onChange={(e) => setDateFrom(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label>Date To</Label>
              <Input type="date" value={dateTo} onChange={(e) => setDateTo(e.target.value)} />
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={() => { setPage(0); void loadData() }} disabled={loading}>
              {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Refresh
            </Button>
            <Button
              variant={selectMode ? "secondary" : "outline"}
              onClick={() => {
                setSelectMode((prev) => !prev)
                if (selectMode) clearSelection()
              }}
            >
              {selectMode ? "Exit Select Mode" : "Select Multiple"}
            </Button>
            <Button onClick={runExport} disabled={exporting || (selectedIDs.size === 0 && !dateFrom && !dateTo)}>
              {exporting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Download className="mr-2 h-4 w-4" />}
              Export
            </Button>
            <Badge variant="outline">
              {selectedIDs.size > 0 ? `${selectedIDs.size} selected` : "Date-range export mode"}
            </Badge>
          </div>

          {error && <p className="text-sm text-red-600">{error}</p>}
          {notice && <p className="text-sm text-green-600">{notice}</p>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Assigned Workflows</CardTitle>
          <CardDescription>
            Showing {items.length} of {total}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {loading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading workflows...
            </div>
          ) : items.length === 0 ? (
            <p className="text-sm text-muted-foreground">No workflows match the current filters.</p>
          ) : (
            items.map((item) => {
              const isChecked = selectedIDs.has(item.id)
              return (
                <div
                  key={item.id}
                  className={cn("rounded-md border p-3 space-y-2", isChecked ? "border-primary bg-primary/5" : "border-border")}
                >
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="space-y-1">
                      <p className="font-medium">
                        {item.title} - {toMMDDYYYY(item.start_at)}
                      </p>
                      <div className="text-xs text-muted-foreground space-y-1">
                        <p>Status: {formatWorkflowDisplayStatus(item)}</p>
                        <p>Start: {new Date(item.start_at * 1000).toLocaleString()}</p>
                        <p>Created: {new Date(item.created_at * 1000).toLocaleString()}</p>
                        {item.completed_at ? <p>Completed: {new Date(item.completed_at * 1000).toLocaleString()}</p> : null}
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {selectMode ? (
                        <Checkbox checked={isChecked} onCheckedChange={(checked) => toggleSelected(item.id, Boolean(checked))} />
                      ) : null}
                      <Button size="sm" variant="outline" onClick={() => void openDetails(item.id)}>
                        Open
                      </Button>
                    </div>
                  </div>
                </div>
              )
            })
          )}

          <div className="flex flex-wrap items-center justify-between gap-2 pt-2">
            <div className="text-xs text-muted-foreground">
              Page {page + 1} of {totalPages}
            </div>
            <div className="flex items-center gap-2">
              <Select value={String(count)} onValueChange={(value) => { setCount(Number(value)); setPage(0) }}>
                <SelectTrigger className="w-24"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="10">10</SelectItem>
                  <SelectItem value="20">20</SelectItem>
                  <SelectItem value="50">50</SelectItem>
                </SelectContent>
              </Select>
              <Button variant="outline" size="sm" onClick={() => setPage((prev) => Math.max(0, prev - 1))} disabled={page <= 0}>
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <Button variant="outline" size="sm" onClick={() => setPage((prev) => Math.min(totalPages - 1, prev + 1))} disabled={page >= totalPages - 1}>
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <WorkflowDetailsModal
        workflow={detailWorkflow}
        open={detailOpen}
        onOpenChange={setDetailOpen}
        loading={detailLoading}
      />
    </div>
  )
}
