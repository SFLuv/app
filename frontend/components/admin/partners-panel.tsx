"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { ArrowDown, ArrowUp, ExternalLink, ImageIcon, Loader2, Pencil, Plus, Trash2, Upload } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import type { Partner, PartnersResponse } from "@/types/partner"

const MAX_LOGO_BYTES = 5 * 1024 * 1024

function isValidPartnerLink(value: string): boolean {
  const trimmed = value.trim()
  if (trimmed === "") return true // optional
  try {
    const parsed = new URL(trimmed)
    return parsed.protocol === "http:" || parsed.protocol === "https:"
  } catch {
    return false
  }
}

/**
 * Row for a single partner. Editable fields are held locally and saved
 * explicitly so a half-typed URL is never persisted, and the logo preview is
 * cache-busted on `updated_at` because a replacement logo reuses the same URL.
 */
function PartnerRow({
  partner,
  index,
  total,
  onSave,
  onDelete,
  onMove,
  onUploadLogo,
  busy,
}: {
  partner: Partner
  index: number
  total: number
  onSave: (id: string, values: { name: string; link_url: string; active: boolean }) => Promise<boolean>
  onDelete: (partner: Partner) => Promise<void>
  onMove: (index: number, direction: -1 | 1) => Promise<void>
  onUploadLogo: (id: string, file: File) => Promise<void>
  busy: boolean
}) {
  const [name, setName] = useState(partner.name)
  const [linkUrl, setLinkUrl] = useState(partner.link_url)
  const [active, setActive] = useState(partner.active)
  const [expanded, setExpanded] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  // Re-sync when the row is replaced by a server response (reorder, upload).
  // Skipped while the editor is open: a background poll landing mid-edit would
  // otherwise overwrite what the admin is currently typing.
  useEffect(() => {
    if (expanded) return
    setName(partner.name)
    setLinkUrl(partner.link_url)
    setActive(partner.active)
  }, [partner.name, partner.link_url, partner.active, expanded])

  const dirty = name !== partner.name || linkUrl !== partner.link_url || active !== partner.active
  const linkValid = isValidPartnerLink(linkUrl)

  return (
    <div className="group">
      <div className="flex items-center gap-3 p-3">
        {/* Reorder handles sit outside the content so the row height never
            depends on whether they are shown. */}
        <div className="flex shrink-0 flex-col">
          <Button
            variant="ghost" size="icon" className="h-5 w-6"
            disabled={index === 0 || busy}
            onClick={() => onMove(index, -1)}
            aria-label={`Move ${partner.name} earlier`}
          >
            <ArrowUp className="h-3 w-3" />
          </Button>
          <Button
            variant="ghost" size="icon" className="h-5 w-6"
            disabled={index === total - 1 || busy}
            onClick={() => onMove(index, 1)}
            aria-label={`Move ${partner.name} later`}
          >
            <ArrowDown className="h-3 w-3" />
          </Button>
        </div>

        <div className="flex h-12 w-24 shrink-0 items-center justify-center rounded-md border bg-muted/30 p-1.5">
          {partner.logo_url ? (
            // eslint-disable-next-line @next/next/no-img-element -- API host, arbitrary ratios
            <img
              src={`${partner.logo_url}?v=${partner.updated_at}`}
              alt={`${partner.name} logo`}
              className="max-h-full w-auto max-w-full object-contain"
            />
          ) : (
            <ImageIcon className="h-4 w-4 text-muted-foreground" />
          )}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium">{partner.name}</span>
            {!partner.active && <Badge variant="outline" className="shrink-0">Hidden</Badge>}
            {!partner.logo_url && (
              <Badge variant="outline" className="shrink-0 border-amber-500 text-amber-600">Needs logo</Badge>
            )}
          </div>
          <div className="truncate text-sm text-muted-foreground">
            {partner.link_url || "No link"}
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          <input
            ref={fileInputRef}
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml"
            className="hidden"
            onChange={(event) => {
              const file = event.target.files?.[0]
              event.target.value = ""
              if (file) void onUploadLogo(partner.id, file)
            }}
          />
          <Button variant="ghost" size="icon" className="h-8 w-8" disabled={busy}
            onClick={() => fileInputRef.current?.click()} title="Replace logo">
            <Upload className="h-3.5 w-3.5" />
          </Button>
          {partner.link_url && (
            <Button variant="ghost" size="icon" className="h-8 w-8" asChild title="Visit site">
              <a href={partner.link_url} target="_blank" rel="noreferrer">
                <ExternalLink className="h-3.5 w-3.5" />
              </a>
            </Button>
          )}
          <Button variant="ghost" size="icon" className="h-8 w-8" disabled={busy}
            onClick={() => setExpanded((open) => !open)} title="Edit">
            <Pencil className="h-3.5 w-3.5" />
          </Button>
          <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive" disabled={busy}
            onClick={() => onDelete(partner)} aria-label={`Remove ${partner.name}`}>
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {expanded && (
        <div className="space-y-3 border-t bg-muted/20 p-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor={`partner-name-${partner.id}`}>Name</Label>
              <Input id={`partner-name-${partner.id}`} value={name}
                onChange={(event) => setName(event.target.value)} placeholder="Partner name" />
            </div>
            <div className="space-y-1">
              <Label htmlFor={`partner-link-${partner.id}`}>Link</Label>
              <Input id={`partner-link-${partner.id}`} value={linkUrl}
                onChange={(event) => setLinkUrl(event.target.value)}
                placeholder="https://example.org" aria-invalid={!linkValid} />
              {!linkValid && <p className="text-xs text-destructive">Must be a full http(s) URL.</p>}
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <div className="flex items-center gap-2">
              <Switch id={`partner-active-${partner.id}`} checked={active} onCheckedChange={setActive} />
              <Label htmlFor={`partner-active-${partner.id}`} className="cursor-pointer font-normal">
                Shown on site
              </Label>
            </div>
            {partner.logo_url && partner.logo_width > 0 && (
              <span className="text-xs text-muted-foreground">
                {partner.logo_width}×{partner.logo_height}
              </span>
            )}
            <div className="ml-auto flex items-center gap-2">
              <Button variant="ghost" size="sm" onClick={() => setExpanded(false)}>Close</Button>
              <Button size="sm"
                disabled={!dirty || !linkValid || name.trim() === "" || busy}
                onClick={async () => {
                  // Only collapse on success — closing after a failed save
                  // would throw away what the admin just typed.
                  if (await onSave(partner.id, { name: name.trim(), link_url: linkUrl.trim(), active })) {
                    setExpanded(false)
                  }
                }}
              >
                Save
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

/**
 * Admin control for the partner carousel on the public marketing site.
 *
 * Order here is the order visitors see. A partner without a logo is kept out of
 * the public list by the API (a missing logo would render as a gap in the
 * scrolling strip), which the row surfaces explicitly rather than letting an
 * admin wonder why a saved partner never appears.
 */
export function PartnersPanel() {
  const { authFetch, status, user } = useApp()
  const { toast } = useToast()
  const [partners, setPartners] = useState<Partner[]>([])
  // First-load-only spinner: a background refresh must never blank a list the
  // admin is mid-edit on.
  const [initialLoading, setInitialLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  // Signature of the last rendered payload. State is replaced only when this
  // changes, so a poll that finds nothing new causes no re-render — no flash,
  // and no clobbering of a row the admin is typing in.
  const signatureRef = useRef("")
  const [error, setError] = useState("")
  const [newName, setNewName] = useState("")
  const [newLink, setNewLink] = useState("")

  const loadPartners = useCallback(async () => {
    setError("")
    try {
      const res = await authFetch("/admin/partners")
      if (!res.ok) throw new Error("Unable to load partners.")
      const data = (await res.json()) as PartnersResponse
      const next = data.partners || []

      const signature = JSON.stringify(
        next.map((partner) => [
          partner.id,
          partner.name,
          partner.link_url,
          partner.active,
          partner.position,
          partner.logo_url,
          partner.updated_at,
        ]),
      )
      if (signature !== signatureRef.current) {
        signatureRef.current = signature
        setPartners(next)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load partners.")
    } finally {
      setInitialLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    if (status !== "authenticated" || !user?.isAdmin) return
    void loadPartners()
  }, [loadPartners, status, user?.isAdmin])

  // The carousel is edited from more than one admin session, so poll for
  // changes rather than requiring a reload. Paused while a write is in flight
  // so a poll cannot race the admin's own edit.
  useEffect(() => {
    if (status !== "authenticated" || !user?.isAdmin) return
    const timer = setInterval(() => {
      if (!busy) void loadPartners()
    }, 30_000)
    return () => clearInterval(timer)
  }, [loadPartners, status, user?.isAdmin, busy])

  const handleCreate = async () => {
    const name = newName.trim()
    if (name === "") return
    if (!isValidPartnerLink(newLink)) {
      toast({ title: "Invalid link", description: "Must be a full http(s) URL.", variant: "destructive" })
      return
    }

    setBusy(true)
    try {
      const res = await authFetch("/admin/partners", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, link_url: newLink.trim() }),
      })
      if (!res.ok) throw new Error((await res.text()) || "Unable to add partner.")
      const created = (await res.json()) as Partner
      setPartners((current) => [...current, created])
      signatureRef.current = ""
      setNewName("")
      setNewLink("")
      toast({ title: "Partner added", description: "Upload a logo to show it on the site." })
    } catch (err) {
      toast({
        title: "Could not add partner",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
    } finally {
      setBusy(false)
    }
  }

  const handleSave = async (id: string, values: { name: string; link_url: string; active: boolean }): Promise<boolean> => {
    setBusy(true)
    try {
      const res = await authFetch(`/admin/partners/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(values),
      })
      if (!res.ok) throw new Error((await res.text()) || "Unable to save partner.")
      const updated = (await res.json()) as Partner
      setPartners((current) => current.map((partner) => (partner.id === id ? updated : partner)))
      // Keep the poll's baseline in step with the write we just made, so the
      // next tick does not see a "change" and re-render for nothing.
      signatureRef.current = ""
      toast({ title: "Partner saved" })
      return true
    } catch (err) {
      toast({
        title: "Could not save partner",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
      return false
    } finally {
      setBusy(false)
    }
  }

  const handleDelete = async (partner: Partner) => {
    if (!window.confirm(`Remove ${partner.name} from the partner carousel?`)) return

    setBusy(true)
    try {
      const res = await authFetch(`/admin/partners/${partner.id}`, { method: "DELETE" })
      if (!res.ok && res.status !== 204) throw new Error("Unable to remove partner.")
      setPartners((current) => current.filter((entry) => entry.id !== partner.id))
      signatureRef.current = ""
      toast({ title: "Partner removed" })
    } catch (err) {
      toast({
        title: "Could not remove partner",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
    } finally {
      setBusy(false)
    }
  }

  const handleUploadLogo = async (id: string, file: File) => {
    if (file.size > MAX_LOGO_BYTES) {
      toast({ title: "Logo too large", description: "Maximum size is 5 MB.", variant: "destructive" })
      return
    }

    setBusy(true)
    try {
      const form = new FormData()
      form.append("logo", file)
      // No Content-Type header: the browser must set the multipart boundary.
      const res = await authFetch(`/admin/partners/${id}/logo`, { method: "POST", body: form })
      if (!res.ok) throw new Error((await res.text()) || "Unable to upload logo.")
      const updated = (await res.json()) as Partner
      setPartners((current) => current.map((partner) => (partner.id === id ? updated : partner)))
      signatureRef.current = ""
      toast({ title: "Logo updated" })
    } catch (err) {
      toast({
        title: "Could not upload logo",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
    } finally {
      setBusy(false)
    }
  }

  const handleMove = async (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= partners.length) return

    const reordered = [...partners]
    const [moved] = reordered.splice(index, 1)
    reordered.splice(target, 0, moved)
    // Optimistic: the strip reorders instantly, and a failure reloads the
    // server's authoritative order rather than leaving the UI lying.
    setPartners(reordered)

    setBusy(true)
    try {
      const res = await authFetch("/admin/partners/order", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ordered_ids: reordered.map((partner) => partner.id) }),
      })
      if (!res.ok) throw new Error("Unable to save order.")
      const data = (await res.json()) as PartnersResponse
      setPartners(data.partners || reordered)
    } catch (err) {
      toast({
        title: "Could not save order",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
      void loadPartners()
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Partner Carousel</CardTitle>
        <CardDescription>
          Organizations shown in the scrolling partner banner on sfluv.org. Order here is the order visitors
          see. Partners without a logo stay hidden until one is uploaded.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="rounded-lg border border-dashed p-4">
          <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
            <div className="space-y-1">
              <Label htmlFor="new-partner-name">Name</Label>
              <Input
                id="new-partner-name"
                value={newName}
                onChange={(event) => setNewName(event.target.value)}
                placeholder="Citizen Wallet"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="new-partner-link">Link</Label>
              <Input
                id="new-partner-link"
                value={newLink}
                onChange={(event) => setNewLink(event.target.value)}
                placeholder="https://citizenwallet.xyz"
              />
            </div>
            <Button onClick={handleCreate} disabled={busy || newName.trim() === ""}>
              <Plus className="mr-2 h-4 w-4" />
              Add partner
            </Button>
          </div>
        </div>

        {initialLoading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading partners…
          </div>
        )}

        {error !== "" && <p className="text-sm text-destructive">{error}</p>}

        {!initialLoading && error === "" && partners.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No partners yet. Add one above, then upload its logo.
          </p>
        )}

        <div className="divide-y overflow-hidden rounded-lg border">
          {partners.map((partner, index) => (
            <PartnerRow
              key={partner.id}
              partner={partner}
              index={index}
              total={partners.length}
              onSave={handleSave}
              onDelete={handleDelete}
              onMove={handleMove}
              onUploadLogo={handleUploadLogo}
              busy={busy}
            />
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
