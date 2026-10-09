"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { ArrowLeft, Download, Loader2, Printer, Search } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { useAuthFetch } from "./use-auth-fetch"
import { useNotice } from "./notice"
import { formatWhen, jsonBody, siteFetch } from "@/lib/site-admin"
import type { SiteFormChoice, SiteFormSummary, SiteSignatureRecord } from "@/types/site"

const CHOICES_TOKEN = "{{choices}}"

const escapeHtml = (value: string) =>
  value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;")

/** The form's plain-text body as HTML, following the same convention the website uses. */
function bodyToHtml(body: string, choices: SiteFormChoice[], chosen: string | undefined): string {
  const choiceHtml = choices
    .map(
      (c) =>
        `<p class="choice">${c.id === chosen ? "&#9745;" : "&#9744;"} <strong>${escapeHtml(c.label)}</strong> ${escapeHtml(c.description)}</p>`,
    )
    .join("")

  const html = body
    .replace(/\r\n/g, "\n")
    .split(/\n{2,}/)
    .map((block) => block.trim())
    .filter(Boolean)
    .map((block) => {
      if (block === CHOICES_TOKEN) return choiceHtml
      if (block.startsWith("## ")) return `<h3>${escapeHtml(block.slice(3))}</h3>`
      const lines = block.split("\n")
      if (lines.every((l) => l.trimStart().startsWith("- "))) {
        return `<ul>${lines.map((l) => `<li>${escapeHtml(l.trimStart().slice(2))}</li>`).join("")}</ul>`
      }
      return `<p>${escapeHtml(block).replace(/\n/g, "<br>")}</p>`
    })
    .join("")

  return body.includes(CHOICES_TOKEN) || choices.length === 0 ? html : html + choiceHtml
}

/** A one-page record of a signature, for saving as a PDF or printing. */
function printRecord(record: SiteSignatureRecord) {
  const signed = record.signed
  const rows: [string, string][] = [
    ["Signed by", record.signer_name],
    ["Preferred name", record.answers.preferred_name ?? ""],
    ["Contact", record.contact],
    ["Event / project", record.answers.event ?? ""],
    ["Event date", record.answers.event_date ?? ""],
    ["Under 18", record.is_minor ? "Yes" : "No"],
    ["Parent / guardian", record.is_minor ? `${record.guardian_name} (${record.guardian_relationship})` : ""],
    ["Signed on", formatWhen(record.signed_at)],
    ["Agreed to sign electronically", record.esign_consent ? "Yes" : "No"],
    ["Form version", `${record.version} (fingerprint ${record.text_sha256.slice(0, 12)})`],
    ["Record id", record.id],
  ]
  if (record.withdrawn_at) rows.push(["Withdrawn on", formatWhen(record.withdrawn_at)])

  const win = window.open("", "_blank")
  if (!win) return
  win.document.write(`<!doctype html><html><head><meta charset="utf-8"><title>${escapeHtml(record.form_title)} — ${escapeHtml(record.signer_name)}</title>
<style>
 body{font:14px/1.5 -apple-system,Segoe UI,Arial,sans-serif;color:#111;max-width:760px;margin:32px auto;padding:0 24px}
 h1{font-size:20px;margin:0 0 4px} h2{font-size:13px;text-transform:uppercase;letter-spacing:.06em;color:#8f2e2e;margin:28px 0 8px}
 h3{font-size:13px;text-transform:uppercase;letter-spacing:.04em;color:#8f2e2e;margin:18px 0 4px}
 table{border-collapse:collapse;width:100%} td{padding:5px 0;border-bottom:1px solid #eee;vertical-align:top} td:first-child{color:#666;width:210px}
 .sig{border:1px solid #ccc;border-radius:6px;padding:8px;display:inline-block;margin:6px 12px 6px 0;background:#fff} .sig img{height:80px}
 .choice{margin:6px 0} .text{border-top:1px solid #ccc;margin-top:8px}
</style></head><body>
<h1>${escapeHtml(record.form_title)}</h1>
<p style="margin:0;color:#666">Signed electronically · record of the exact text agreed to</p>
<h2>Signature record</h2><table>${rows
    .filter(([, v]) => v)
    .map(([k, v]) => `<tr><td>${escapeHtml(k)}</td><td>${escapeHtml(v)}</td></tr>`)
    .join("")}</table>
<h2>Signatures</h2>
${record.signature_image ? `<div class="sig"><div style="font-size:11px;color:#666">Participant</div><img src="${record.signature_image}"></div>` : ""}
${record.guardian_signature_image ? `<div class="sig"><div style="font-size:11px;color:#666">Parent / guardian</div><img src="${record.guardian_signature_image}"></div>` : ""}
<h2>The text that was signed (version ${record.version})</h2>
<div class="text">${signed ? bodyToHtml(signed.body, signed.config.choices, record.answers.choice) : "<p>(text unavailable)</p>"}</div>
<script>window.onload=function(){setTimeout(function(){window.print()},300)}</script>
</body></html>`)
  win.document.close()
}

function SignatureDetail({
  id,
  onClose,
  onChanged,
}: {
  id: string
  onClose: () => void
  onChanged: () => void
}) {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const [record, setRecord] = useState<SiteSignatureRecord | null>(null)
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let cancelled = false
    siteFetch<SiteSignatureRecord>(authFetch, `/admin/site/signatures/${id}`)
      .then((data) => !cancelled && setRecord(data))
      .catch((err) => !cancelled && setError(err instanceof Error ? err.message : "Unable to load the record."))
    return () => {
      cancelled = true
    }
  }, [id, authFetch])

  const toggleWithdrawn = async () => {
    if (!record) return
    const withdrawing = !record.withdrawn_at
    if (withdrawing && !window.confirm("Mark this person's consent as withdrawn? The record is kept.")) return
    setBusy(true)
    try {
      setRecord(
        await siteFetch<SiteSignatureRecord>(authFetch, `/admin/site/signatures/${id}/withdraw`, {
          method: "POST",
          ...jsonBody({ withdrawn: withdrawing }),
        }),
      )
      onChanged()
    } catch (err) {
      toast({ title: "Could not update the record", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setBusy(false)
    }
  }

  const choiceLabel =
    record?.signed?.config.choices.find((c) => c.id === record.answers.choice)?.label ?? record?.answers.choice ?? ""

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{record ? `${record.signer_name}` : "Signature"}</DialogTitle>
          <DialogDescription>{record ? `${record.form_title} · version ${record.version}` : ""}</DialogDescription>
        </DialogHeader>

        {error && <p className="text-sm text-destructive">{error}</p>}
        {!record && !error && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" /> Loading…
          </div>
        )}

        {record && (
          <div className="space-y-5">
            {record.withdrawn_at && <Badge variant="warning">Consent withdrawn {formatWhen(record.withdrawn_at)}</Badge>}

            <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
              {(
                [
                  ["Signed on", formatWhen(record.signed_at)],
                  ["Contact", record.contact || "—"],
                  ["Preferred name", record.answers.preferred_name || "—"],
                  ["Event / project", record.answers.event || "—"],
                  ["Event date", record.answers.event_date || "—"],
                  ["Scope chosen", choiceLabel || "—"],
                  ["Under 18", record.is_minor ? `Yes — ${record.guardian_name} (${record.guardian_relationship})` : "No"],
                  ["Agreed to e-sign", record.esign_consent ? "Yes" : "No"],
                ] as [string, string][]
              ).map(([label, value]) => (
                <div key={label}>
                  <dt className="text-xs text-muted-foreground">{label}</dt>
                  <dd className="break-words">{value}</dd>
                </div>
              ))}
            </dl>

            <div className="flex flex-wrap gap-4">
              {record.signature_image && (
                <div>
                  <p className="mb-1 text-xs text-muted-foreground">Participant signature</p>
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={record.signature_image} alt="Participant signature" className="h-24 rounded-md border bg-white p-2" />
                </div>
              )}
              {record.guardian_signature_image && (
                <div>
                  <p className="mb-1 text-xs text-muted-foreground">Parent / guardian signature</p>
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={record.guardian_signature_image} alt="Guardian signature" className="h-24 rounded-md border bg-white p-2" />
                </div>
              )}
            </div>

            {record.signed && (
              <div>
                <p className="mb-1 text-xs text-muted-foreground">The exact text this person agreed to (version {record.signed.version})</p>
                <div
                  className="max-h-64 overflow-y-auto rounded-md border bg-muted/30 p-3 text-sm [&_h3]:mt-3 [&_h3]:text-xs [&_h3]:font-semibold [&_h3]:uppercase [&_p]:my-2 [&_ul]:list-disc [&_ul]:pl-5"
                  dangerouslySetInnerHTML={{
                    __html: bodyToHtml(record.signed.body, record.signed.config.choices, record.answers.choice),
                  }}
                />
              </div>
            )}

            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => printRecord(record)}>
                <Printer className="mr-2 h-4 w-4" /> Print / save as PDF
              </Button>
              <Button variant="ghost" onClick={toggleWithdrawn} disabled={busy}>
                {record.withdrawn_at ? "Clear the withdrawn mark" : "Mark consent as withdrawn"}
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

export function FormSignatures({ form, onBack }: { form: SiteFormSummary; onBack: () => void }) {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const [records, setRecords] = useState<SiteSignatureRecord[] | null>(null)
  const [error, setError] = useState("")
  const [search, setSearch] = useState("")
  const [openId, setOpenId] = useState<string | null>(null)
  const [downloading, setDownloading] = useState(false)

  const load = useCallback(async () => {
    try {
      const data = await siteFetch<{ signatures: SiteSignatureRecord[] }>(authFetch, `/admin/site/forms/${form.id}/signatures`)
      setRecords(data.signatures)
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load signatures.")
    }
  }, [authFetch, form.id])

  useEffect(() => {
    void load()
  }, [load])

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase()
    if (!records || !needle) return records ?? []
    return records.filter((r) =>
      [r.signer_name, r.contact, r.answers.preferred_name, r.answers.event, r.guardian_name].some((v) => (v ?? "").toLowerCase().includes(needle)),
    )
  }, [records, search])

  const downloadCsv = async () => {
    setDownloading(true)
    try {
      const res = await authFetch(`/admin/site/forms/${form.id}/signatures.csv`)
      if (!res.ok) throw new Error("The download failed. Please try again.")
      const url = URL.createObjectURL(await res.blob())
      const link = document.createElement("a")
      link.href = url
      link.download = `${form.slug}-signatures.csv`
      link.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      toast({ title: "Could not download", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setDownloading(false)
    }
  }

  return (
    <div className="space-y-6">
      <Button variant="ghost" size="sm" onClick={onBack}>
        <ArrowLeft className="mr-2 h-4 w-4" /> All forms
      </Button>

      <Card>
        <CardHeader>
          <CardTitle>Signatures — {form.title}</CardTitle>
          <CardDescription>
            Only admins can see these. Open a row for the signature itself and the exact wording that person agreed to.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap items-center gap-3">
            <div className="relative min-w-[220px] flex-1">
              <Search className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input className="pl-9" placeholder="Search by name, contact or event" value={search} onChange={(e) => setSearch(e.target.value)} />
            </div>
            <Button variant="outline" onClick={downloadCsv} disabled={downloading || !records?.length}>
              {downloading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Download className="mr-2 h-4 w-4" />}
              Download spreadsheet (CSV)
            </Button>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}
          {!records && !error && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" /> Loading…
            </div>
          )}
          {records && records.length === 0 && <p className="text-sm text-muted-foreground">No one has signed this form yet.</p>}

          {records && records.length > 0 && (
            <>
              <p className="text-sm text-muted-foreground">
                {shown.length === records.length ? `${records.length} signature${records.length === 1 ? "" : "s"}` : `${shown.length} of ${records.length}`}
              </p>
              <div className="overflow-x-auto rounded-lg border">
                <table className="w-full min-w-[640px] text-sm">
                  <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
                    <tr>
                      <th className="p-3 font-medium">Signed</th>
                      <th className="p-3 font-medium">Name</th>
                      <th className="p-3 font-medium">Contact</th>
                      <th className="p-3 font-medium">Event</th>
                      <th className="p-3 font-medium">Notes</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y">
                    {shown.map((r) => (
                      <tr key={r.id} className="cursor-pointer hover:bg-muted/40" onClick={() => setOpenId(r.id)}>
                        <td className="whitespace-nowrap p-3">{formatWhen(r.signed_at)}</td>
                        <td className="p-3 font-medium">{r.signer_name}</td>
                        <td className="p-3">{r.contact || "—"}</td>
                        <td className="p-3">{r.answers.event || "—"}</td>
                        <td className="space-x-1.5 p-3">
                          {r.is_minor && <Badge variant="secondary">Under 18</Badge>}
                          {r.withdrawn_at && <Badge variant="warning">Withdrawn</Badge>}
                          {r.version !== form.current_version && <Badge variant="outline">v{r.version}</Badge>}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {openId && <SignatureDetail id={openId} onClose={() => setOpenId(null)} onChanged={() => void load()} />}
    </div>
  )
}
