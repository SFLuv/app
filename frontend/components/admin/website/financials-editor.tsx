"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { ExternalLink, FileText, Loader2, Pencil, Trash2, Undo2, Upload } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useAuthFetch } from "./use-auth-fetch"
import { useNotice } from "./notice"
import {
  FINANCIAL_KIND_LABELS,
  PERIOD_LABELS,
  checkSiteFile,
  formatSize,
  jsonBody,
  resolveSiteUrl,
  siteFetch,
  uploadSiteFile,
} from "@/lib/site-admin"
import type { FinancialDocument, FinancialDocumentRequest, FinancialKind, FinancialPeriod } from "@/types/site"

const STATEMENT_KINDS: FinancialKind[] = ["activity", "cash_flows", "financial_position", "activity_comparison"]
const PERIOD_ORDER: FinancialPeriod[] = ["Q4", "Q3", "Q2", "Q1", "FULL"]

/** The fiscal year runs July–June and is named for the year it ends in. */
function fiscalYearOf(date: Date): number {
  return date.getMonth() >= 6 ? date.getFullYear() + 1 : date.getFullYear()
}

/** From the day a statement's period ends: which fiscal year and quarter it belongs to. */
function inferPlacement(asOf: string): { fiscal_year: number; period: FinancialPeriod } | null {
  const match = /^(\d{4})-(\d{2})-\d{2}$/.exec(asOf)
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  if (month >= 7 && month <= 9) return { fiscal_year: year + 1, period: "Q1" }
  if (month >= 10 && month <= 12) return { fiscal_year: year + 1, period: "Q2" }
  if (month >= 1 && month <= 3) return { fiscal_year: year, period: "Q3" }
  if (month >= 4 && month <= 6) return { fiscal_year: year, period: "Q4" }
  return null
}

interface Draft {
  kind: FinancialKind
  fiscal_year: number
  period: FinancialPeriod
  as_of: string
  label: string
}

function blankDraft(): Draft {
  return { kind: "activity", fiscal_year: fiscalYearOf(new Date()), period: "Q1", as_of: "", label: "" }
}

function previewLabel(draft: Draft): string {
  if (draft.label.trim()) return draft.label.trim()
  switch (draft.kind) {
    case "form_990n":
      return `${draft.fiscal_year} 990N`
    case "form_199n":
      return `${draft.fiscal_year} 199N Confirmation`
    case "impact_report":
      return `${draft.fiscal_year - 1}–${draft.fiscal_year} Annual Impact Report`
    case "other":
      return ""
    default:
      return draft.as_of ? `${draft.as_of} ${FINANCIAL_KIND_LABELS[draft.kind]}` : ""
  }
}

/** The fields shared by "add" and "edit". */
function DocumentFields({ draft, onChange, idPrefix }: { draft: Draft; onChange: (next: Draft) => void; idPrefix: string }) {
  const years = useMemo(() => {
    const top = fiscalYearOf(new Date()) + 1
    const list: number[] = []
    for (let y = top; y >= 2024; y -= 1) list.push(y)
    if (!list.includes(draft.fiscal_year)) list.push(draft.fiscal_year)
    return list.sort((a, b) => b - a)
  }, [draft.fiscal_year])

  const isStatement = STATEMENT_KINDS.includes(draft.kind)
  const isImpact = draft.kind === "impact_report"
  const generated = previewLabel(draft)

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1">
        <Label>What is it?</Label>
        <Select value={draft.kind} onValueChange={(kind) => onChange({ ...draft, kind: kind as FinancialKind })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {Object.entries(FINANCIAL_KIND_LABELS).map(([value, label]) => (
              <SelectItem key={value} value={value}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {isStatement && (
        <div className="space-y-1">
          <Label htmlFor={`${idPrefix}-date`}>Statement date (the day the period ends)</Label>
          <Input
            id={`${idPrefix}-date`}
            type="date"
            value={draft.as_of}
            onChange={(e) => {
              const placement = inferPlacement(e.target.value)
              onChange({ ...draft, as_of: e.target.value, ...(placement ?? {}) })
            }}
          />
          <p className="text-xs text-muted-foreground">The fiscal year and quarter fill in from this date.</p>
        </div>
      )}

      <div className="space-y-1">
        <Label>Fiscal year (ends June 30)</Label>
        <Select value={String(draft.fiscal_year)} onValueChange={(y) => onChange({ ...draft, fiscal_year: Number(y) })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {years.map((y) => (
              <SelectItem key={y} value={String(y)}>
                {isImpact ? `${y - 1}–${y}` : `FYE June 30, ${y}`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {!isImpact && (
        <div className="space-y-1">
          <Label>Which part of the year?</Label>
          <Select value={draft.period} onValueChange={(period) => onChange({ ...draft, period: period as FinancialPeriod })}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(PERIOD_LABELS).map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      <div className="space-y-1 sm:col-span-2">
        <Label htmlFor={`${idPrefix}-label`}>
          Name shown on the site {draft.kind === "other" ? "" : "(optional — leave blank to use the standard name)"}
        </Label>
        <Input
          id={`${idPrefix}-label`}
          value={draft.label}
          maxLength={200}
          placeholder={generated || "Name of the document"}
          onChange={(e) => onChange({ ...draft, label: e.target.value })}
        />
        {generated && !draft.label && <p className="text-xs text-muted-foreground">It will appear as “{generated}”.</p>}
      </div>
    </div>
  )
}

function toRequest(draft: Draft, fileId: string): FinancialDocumentRequest {
  return {
    kind: draft.kind,
    fiscal_year: draft.fiscal_year,
    period: draft.kind === "impact_report" ? "FULL" : draft.period,
    as_of: STATEMENT_KINDS.includes(draft.kind) && draft.as_of ? draft.as_of : null,
    label: draft.label.trim(),
    file_id: fileId,
  }
}

function validateDraft(draft: Draft): string {
  if (STATEMENT_KINDS.includes(draft.kind) && !draft.as_of) return "Enter the statement date."
  if (draft.kind === "other" && !draft.label.trim()) return "Give this document a name."
  return ""
}

export function FinancialsEditor() {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()

  const [documents, setDocuments] = useState<FinancialDocument[] | null>(null)
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)

  const [draft, setDraft] = useState<Draft>(blankDraft)
  const [file, setFile] = useState<File | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const [editing, setEditing] = useState<FinancialDocument | null>(null)
  const [editDraft, setEditDraft] = useState<Draft>(blankDraft)
  const [editFile, setEditFile] = useState<File | null>(null)
  const editFileInput = useRef<HTMLInputElement>(null)

  const load = useCallback(async () => {
    try {
      const data = await siteFetch<{ documents: FinancialDocument[] }>(authFetch, "/admin/site/financials")
      setDocuments(data.documents)
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load documents.")
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  // A wrong or oversized file is refused when it is chosen, not at publish.
  const pickPdf = async (chosen: File | null, keep: (file: File | null) => void) => {
    if (!chosen) return
    const problem = await checkSiteFile(chosen, "pdf")
    if (problem) return toast({ title: "Can’t use that file", description: problem, variant: "destructive" })
    keep(chosen)
  }

  const fail = (title: string, err: unknown) =>
    toast({ title, description: err instanceof Error ? err.message : "", variant: "destructive" })

  const add = async () => {
    const problem = validateDraft(draft)
    if (problem) return toast({ title: problem, variant: "destructive" })
    if (!file) return toast({ title: "Choose the PDF to upload.", variant: "destructive" })

    setBusy(true)
    try {
      const uploaded = await uploadSiteFile(authFetch, file)
      const created = await siteFetch<FinancialDocument>(authFetch, "/admin/site/financials", {
        method: "POST",
        ...jsonBody(toRequest(draft, uploaded.id)),
      })
      toast({ title: "Published", description: `“${created.label}” will appear on the website within about 30 seconds.` })
      setFile(null)
      if (fileInput.current) fileInput.current.value = ""
      setDraft(blankDraft())
      await load()
    } catch (err) {
      fail("Could not publish the document", err)
    } finally {
      setBusy(false)
    }
  }

  const saveEdit = async () => {
    if (!editing) return
    const problem = validateDraft(editDraft)
    if (problem) return toast({ title: problem, variant: "destructive" })

    setBusy(true)
    try {
      const fileId = editFile ? (await uploadSiteFile(authFetch, editFile)).id : ""
      await siteFetch<FinancialDocument>(authFetch, `/admin/site/financials/${editing.id}`, {
        method: "PUT",
        ...jsonBody(toRequest(editDraft, fileId)),
      })
      toast({ title: "Saved", description: "The change appears on the website within about 30 seconds." })
      setEditing(null)
      await load()
    } catch (err) {
      fail("Could not save the change", err)
    } finally {
      setBusy(false)
    }
  }

  const setRemoved = async (doc: FinancialDocument, removed: boolean) => {
    if (removed && !window.confirm(`Remove “${doc.label}” from the website? You can restore it afterwards.`)) return
    setBusy(true)
    try {
      await siteFetch<FinancialDocument>(
        authFetch,
        removed ? `/admin/site/financials/${doc.id}` : `/admin/site/financials/${doc.id}/restore`,
        { method: removed ? "DELETE" : "POST" },
      )
      toast({
        title: removed ? "Removed from the website" : "Restored to the website",
        description: removed ? "You can bring it back from “Recently removed” below." : undefined,
      })
      await load()
    } catch (err) {
      fail(removed ? "Could not remove it" : "Could not restore it", err)
    } finally {
      setBusy(false)
    }
  }

  const openEdit = (doc: FinancialDocument) => {
    setEditing(doc)
    setEditFile(null)
    setEditDraft({
      kind: doc.kind,
      fiscal_year: doc.fiscal_year,
      period: doc.period,
      as_of: doc.as_of ?? "",
      label: doc.label,
    })
  }

  if (error) return <p className="text-sm text-destructive">{error}</p>
  if (!documents) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading documents…
      </div>
    )
  }

  const live = documents.filter((d) => !d.removed_at)
  const removed = documents.filter((d) => d.removed_at)
  const impact = live.filter((d) => d.kind === "impact_report")
  const years = Array.from(new Set(live.filter((d) => d.kind !== "impact_report").map((d) => d.fiscal_year))).sort((a, b) => b - a)

  const row = (doc: FinancialDocument) => (
    <div key={doc.id} className="flex flex-wrap items-center justify-between gap-3 p-3">
      <div className="flex min-w-0 items-center gap-3">
        <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0">
          <a
            href={resolveSiteUrl(doc.href) ?? "#"}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
          >
            <span className="truncate">{doc.label}</span>
            <ExternalLink className="h-3 w-3 shrink-0 opacity-60" />
          </a>
          <p className="text-xs text-muted-foreground">
            {FINANCIAL_KIND_LABELS[doc.kind]}
            {doc.source === "legacy" ? " · existing file" : ""}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 gap-1">
        <Button variant="ghost" size="sm" disabled={busy} onClick={() => openEdit(doc)}>
          <Pencil className="mr-1.5 h-3.5 w-3.5" /> Edit
        </Button>
        <Button variant="ghost" size="sm" disabled={busy} onClick={() => setRemoved(doc, true)}>
          <Trash2 className="mr-1.5 h-3.5 w-3.5" /> Remove
        </Button>
      </div>
    </div>
  )

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Add a document</CardTitle>
          <CardDescription>
            Upload a financial statement, tax filing or annual impact report. It is published on the Financials and
            Reports page within about 30 seconds, filed under the right year and quarter automatically.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <DocumentFields draft={draft} onChange={setDraft} idPrefix="add" />

          <div className="space-y-2">
            <Label>The PDF</Label>
            <div className="flex flex-wrap items-center gap-3">
              <Button type="button" variant="outline" onClick={() => fileInput.current?.click()} disabled={busy}>
                <Upload className="mr-2 h-4 w-4" /> {file ? "Choose a different PDF" : "Choose a PDF"}
              </Button>
              {file && (
                <span className="text-sm text-muted-foreground">
                  {file.name} · {formatSize(file.size)}
                </span>
              )}
              <input
                ref={fileInput}
                type="file"
                accept="application/pdf"
                className="hidden"
                onChange={(e) => {
                  const chosen = e.target.files?.[0] ?? null
                  e.target.value = ""
                  void pickPdf(chosen, setFile)
                }}
              />
            </div>
          </div>

          <Button onClick={add} disabled={busy || !file}>
            {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Upload and publish
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>On the website now</CardTitle>
          <CardDescription>Everything visitors can see on the Financials and Reports page.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {impact.length > 0 && (
            <div className="space-y-2">
              <h3 className="text-sm font-semibold">Annual impact reports</h3>
              <div className="divide-y rounded-lg border">{impact.map(row)}</div>
            </div>
          )}

          {years.map((year) => {
            const ofYear = live.filter((d) => d.kind !== "impact_report" && d.fiscal_year === year)
            return (
              <div key={year} className="space-y-3">
                <h3 className="text-sm font-semibold">FYE June 30, {year}</h3>
                {PERIOD_ORDER.filter((p) => ofYear.some((d) => d.period === p)).map((period) => (
                  <div key={period} className="space-y-1.5">
                    <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      {period === "Q4" ? "Q4 / year-end" : period === "FULL" ? "Full year" : period}
                    </p>
                    <div className="divide-y rounded-lg border">{ofYear.filter((d) => d.period === period).map(row)}</div>
                  </div>
                ))}
              </div>
            )
          })}

          {live.length === 0 && <p className="text-sm text-muted-foreground">Nothing is published yet.</p>}
        </CardContent>
      </Card>

      {removed.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Recently removed</CardTitle>
            <CardDescription>These are hidden from the website. Restore one to put it back.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="divide-y rounded-lg border">
              {removed.map((doc) => (
                <div key={doc.id} className="flex items-center justify-between gap-3 p-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{doc.label}</p>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      {doc.kind === "impact_report" ? `${doc.fiscal_year - 1}–${doc.fiscal_year}` : `FYE ${doc.fiscal_year}`}
                      <Badge variant="secondary">Hidden</Badge>
                    </div>
                  </div>
                  <Button variant="outline" size="sm" disabled={busy} onClick={() => setRemoved(doc, false)}>
                    <Undo2 className="mr-1.5 h-3.5 w-3.5" /> Restore
                  </Button>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <Dialog open={editing !== null} onOpenChange={(open) => !open && setEditing(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Edit document</DialogTitle>
            <DialogDescription>Change how it is filed or named, or replace the file. Leave the file alone to keep the current one.</DialogDescription>
          </DialogHeader>
          <DocumentFields draft={editDraft} onChange={setEditDraft} idPrefix="edit" />
          <div className="space-y-2">
            <Label>Replace the PDF (optional)</Label>
            <div className="flex flex-wrap items-center gap-3">
              <Button type="button" variant="outline" size="sm" onClick={() => editFileInput.current?.click()}>
                <Upload className="mr-2 h-4 w-4" /> {editFile ? "Choose a different PDF" : "Choose a PDF"}
              </Button>
              {editFile && <span className="text-sm text-muted-foreground">{editFile.name}</span>}
              <input
                ref={editFileInput}
                type="file"
                accept="application/pdf"
                className="hidden"
                onChange={(e) => {
                  const chosen = e.target.files?.[0] ?? null
                  e.target.value = ""
                  void pickPdf(chosen, setEditFile)
                }}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setEditing(null)}>
              Cancel
            </Button>
            <Button onClick={saveEdit} disabled={busy}>
              {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
