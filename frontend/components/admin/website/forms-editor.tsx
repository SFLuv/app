"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { Copy, ExternalLink, Loader2, Pencil, Plus, QrCode, Users } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { SfluvQRCode, type SfluvQRCodeHandle } from "@/components/ui/sfluv-qr-code"
import { useAuthFetch } from "./use-auth-fetch"
import { useNotice } from "./notice"
import { formatWhen, formPublicUrl, jsonBody, siteFetch } from "@/lib/site-admin"
import type { SiteFormSummary } from "@/types/site"
import { FormEditor } from "./form-editor"
import { FormSignatures } from "./form-signatures"

type View = { mode: "list" } | { mode: "edit"; id: string | null } | { mode: "signatures"; form: SiteFormSummary }

function StatusBadge({ form }: { form: SiteFormSummary }) {
  if (form.status === "open") return <Badge variant="success">Open for signing</Badge>
  if (form.status === "closed") return <Badge variant="secondary">Closed</Badge>
  return <Badge variant="outline">Draft — not open yet</Badge>
}

export function FormsEditor() {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()

  const [view, setView] = useState<View>({ mode: "list" })
  const [forms, setForms] = useState<SiteFormSummary[] | null>(null)
  const [error, setError] = useState("")
  const [busyId, setBusyId] = useState<string | null>(null)

  const [opening, setOpening] = useState<SiteFormSummary | null>(null)
  const [closesAt, setClosesAt] = useState("")
  const [qrFor, setQrFor] = useState<SiteFormSummary | null>(null)
  const qrRef = useRef<SfluvQRCodeHandle>(null)

  const load = useCallback(async () => {
    try {
      const data = await siteFetch<{ forms: SiteFormSummary[] }>(authFetch, "/admin/site/forms")
      setForms(data.forms)
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load forms.")
    }
  }, [authFetch])

  useEffect(() => {
    if (view.mode === "list") void load()
  }, [load, view.mode])

  const fail = (title: string, err: unknown) =>
    toast({ title, description: err instanceof Error ? err.message : "", variant: "destructive" })

  const open = async () => {
    if (!opening) return
    let closes: number | null = null
    if (closesAt) {
      closes = Math.floor(new Date(closesAt).getTime() / 1000)
      if (!Number.isFinite(closes) || closes <= Date.now() / 1000) return toast({ title: "The closing time must be in the future.", variant: "destructive" })
    }
    setBusyId(opening.id)
    try {
      await siteFetch(authFetch, `/admin/site/forms/${opening.id}/open`, { method: "POST", ...jsonBody({ closes_at: closes }) })
      toast({ title: "The form is open", description: "People can sign it within about 10 seconds. Use the QR code or link to share it." })
      setOpening(null)
      setClosesAt("")
      await load()
    } catch (err) {
      fail("Could not open the form", err)
    } finally {
      setBusyId(null)
    }
  }

  const close = async (form: SiteFormSummary) => {
    if (!window.confirm(`Close “${form.title}”? People will no longer be able to sign it. Existing signatures are kept.`)) return
    setBusyId(form.id)
    try {
      await siteFetch(authFetch, `/admin/site/forms/${form.id}/close`, { method: "POST" })
      toast({ title: "The form is closed" })
      await load()
    } catch (err) {
      fail("Could not close the form", err)
    } finally {
      setBusyId(null)
    }
  }

  const copyLink = async (form: SiteFormSummary) => {
    try {
      await navigator.clipboard.writeText(formPublicUrl(form.slug))
      toast({ title: "Link copied" })
    } catch {
      toast({ title: "Could not copy", description: formPublicUrl(form.slug), variant: "destructive" })
    }
  }

  if (view.mode === "edit") {
    return (
      <FormEditor
        formId={view.id}
        onBack={() => setView({ mode: "list" })}
        onSaved={(saved) => {
          // After creating, stay on the new form so it can be edited further.
          if (view.id === null) setView({ mode: "edit", id: saved.id })
        }}
      />
    )
  }

  if (view.mode === "signatures") {
    return <FormSignatures form={view.form} onBack={() => setView({ mode: "list" })} />
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1.5">
            <CardTitle>Forms and waivers</CardTitle>
            <CardDescription>
              Forms people can sign on sfluv.org (About → Forms and Waivers). Open one when you need signatures and close
              it when you are done — closed forms disappear from the list and stop accepting signatures.
            </CardDescription>
          </div>
          <Button onClick={() => setView({ mode: "edit", id: null })}>
            <Plus className="mr-2 h-4 w-4" /> New form
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {error && <p className="text-sm text-destructive">{error}</p>}
          {!forms && !error && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" /> Loading forms…
            </div>
          )}
          {forms && forms.length === 0 && <p className="text-sm text-muted-foreground">No forms yet.</p>}

          {forms?.map((form) => (
            <div key={form.id} className="space-y-3 rounded-lg border p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-semibold">{form.title}</h3>
                    <StatusBadge form={form} />
                    {form.kind === "withdrawal" && <Badge variant="outline">Withdrawal</Badge>}
                  </div>
                  {form.summary && <p className="text-sm text-muted-foreground">{form.summary}</p>}
                  <p className="text-xs text-muted-foreground">
                    <span className="font-mono">{formPublicUrl(form.slug)}</span>
                    {form.status === "open" && form.closes_at ? ` · closes ${formatWhen(form.closes_at)}` : ""}
                    {" · "}version {form.current_version}
                  </p>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  {form.status === "open" ? (
                    <Button variant="outline" size="sm" disabled={busyId === form.id} onClick={() => close(form)}>
                      {busyId === form.id && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                      Close
                    </Button>
                  ) : (
                    <Button size="sm" disabled={busyId === form.id} onClick={() => { setOpening(form); setClosesAt("") }}>
                      Open for signing
                    </Button>
                  )}
                </div>
              </div>

              <div className="flex flex-wrap gap-2">
                <Button variant="ghost" size="sm" onClick={() => setView({ mode: "edit", id: form.id })}>
                  <Pencil className="mr-1.5 h-3.5 w-3.5" /> Edit
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setView({ mode: "signatures", form })}>
                  <Users className="mr-1.5 h-3.5 w-3.5" /> Signatures ({form.signature_count})
                </Button>
                <Button variant="ghost" size="sm" onClick={() => copyLink(form)}>
                  <Copy className="mr-1.5 h-3.5 w-3.5" /> Copy link
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setQrFor(form)}>
                  <QrCode className="mr-1.5 h-3.5 w-3.5" /> QR code
                </Button>
                {form.status === "open" && (
                  <Button variant="ghost" size="sm" asChild>
                    <a href={formPublicUrl(form.slug)} target="_blank" rel="noreferrer">
                      <ExternalLink className="mr-1.5 h-3.5 w-3.5" /> View on the site
                    </a>
                  </Button>
                )}
              </div>
            </div>
          ))}
        </CardContent>
      </Card>

      <Dialog open={opening !== null} onOpenChange={(o) => !o && setOpening(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Open “{opening?.title}” for signing</DialogTitle>
            <DialogDescription>
              It will appear on the Forms and Waivers page and its link and QR code will start working.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-1">
            <Label htmlFor="closes-at">Close it automatically at (optional)</Label>
            <Input id="closes-at" type="datetime-local" value={closesAt} onChange={(e) => setClosesAt(e.target.value)} />
            <p className="text-xs text-muted-foreground">
              Leave this empty to keep it open until you close it yourself. Useful for an event: set it for the day after.
            </p>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setOpening(null)}>
              Cancel
            </Button>
            <Button onClick={open} disabled={busyId !== null}>
              {busyId !== null && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Open for signing
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={qrFor !== null} onOpenChange={(o) => !o && setQrFor(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>QR code</DialogTitle>
            <DialogDescription>
              Anyone who scans this goes straight to “{qrFor?.title}”. Print it or hold it up at the event.
              {qrFor && qrFor.status !== "open" ? " The form is not open yet, so the code won’t work until you open it." : ""}
            </DialogDescription>
          </DialogHeader>
          {qrFor && (
            <div className="space-y-3">
              <div className="mx-auto w-56">
                <SfluvQRCode ref={qrRef} value={formPublicUrl(qrFor.slug)} />
              </div>
              <p className="break-all text-center font-mono text-xs text-muted-foreground">{formPublicUrl(qrFor.slug)}</p>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => qrFor && void qrRef.current?.download("png", `${qrFor.slug}-qr.png`)}>
              Download image
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
