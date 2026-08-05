"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { ArrowDown, ArrowUp, ExternalLink, ImageIcon, Loader2, Plus, Trash2, Upload } from "lucide-react"

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
  onSave: (id: string, values: { name: string; link_url: string; active: boolean }) => Promise<void>
  onDelete: (partner: Partner) => Promise<void>
  onMove: (index: number, direction: -1 | 1) => Promise<void>
  onUploadLogo: (id: string, file: File) => Promise<void>
  busy: boolean
}) {
  const [name, setName] = useState(partner.name)
  const [linkUrl, setLinkUrl] = useState(partner.link_url)
  const [active, setActive] = useState(partner.active)
  const fileInputRef = useRef<HTMLInputElement>(null)

  // Re-sync when the row is replaced by a server response (reorder, upload).
  useEffect(() => {
    setName(partner.name)
    setLinkUrl(partner.link_url)
    setActive(partner.active)
  }, [partner.name, partner.link_url, partner.active])

  const dirty = name !== partner.name || linkUrl !== partner.link_url || active !== partner.active
  const linkValid = isValidPartnerLink(linkUrl)

  return (
    <div className="rounded-lg border p-4">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
        <div className="flex items-center gap-3">
          <div className="flex flex-col gap-1">
            <Button
              variant="outline"
              size="icon"
              className="h-7 w-7"
              disabled={index === 0 || busy}
              onClick={() => onMove(index, -1)}
              aria-label={`Move ${partner.name} earlier`}
            >
              <ArrowUp className="h-3.5 w-3.5" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              className="h-7 w-7"
              disabled={index === total - 1 || busy}
              onClick={() => onMove(index, 1)}
              aria-label={`Move ${partner.name} later`}
            >
              <ArrowDown className="h-3.5 w-3.5" />
            </Button>
          </div>

          <div className="flex h-16 w-32 shrink-0 items-center justify-center rounded-md border bg-muted/40 p-2">
            {partner.logo_url ? (
              // eslint-disable-next-line @next/next/no-img-element -- logo comes from the API host at arbitrary aspect ratios
              <img
                src={`${partner.logo_url}?v=${partner.updated_at}`}
                alt={`${partner.name} logo`}
                className="max-h-full w-auto max-w-full object-contain"
              />
            ) : (
              <div className="flex flex-col items-center gap-1 text-muted-foreground">
                <ImageIcon className="h-4 w-4" />
                <span className="text-[10px]">No logo</span>
              </div>
            )}
          </div>
        </div>

        <div className="min-w-0 flex-1 space-y-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor={`partner-name-${partner.id}`}>Name</Label>
              <Input
                id={`partner-name-${partner.id}`}
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="Partner name"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor={`partner-link-${partner.id}`}>Link</Label>
              <Input
                id={`partner-link-${partner.id}`}
                value={linkUrl}
                onChange={(event) => setLinkUrl(event.target.value)}
                placeholder="https://example.org"
                aria-invalid={!linkValid}
              />
              {!linkValid && (
                <p className="text-xs text-destructive">Must be a full http(s) URL.</p>
              )}
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <div className="flex items-center gap-2">
              <Switch
                id={`partner-active-${partner.id}`}
                checked={active}
                onCheckedChange={setActive}
              />
              <Label htmlFor={`partner-active-${partner.id}`} className="cursor-pointer">
                Shown on site
              </Label>
            </div>

            {!partner.logo_url && (
              <Badge variant="outline" className="text-amber-600">
                Hidden until a logo is uploaded
              </Badge>
            )}
            {partner.logo_url && partner.logo_width > 0 && (
              <span className="text-xs text-muted-foreground">
                {partner.logo_width}×{partner.logo_height}
              </span>
            )}

            <div className="ml-auto flex flex-wrap items-center gap-2">
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
              <Button variant="outline" size="sm" disabled={busy} onClick={() => fileInputRef.current?.click()}>
                <Upload className="mr-2 h-3.5 w-3.5" />
                {partner.logo_url ? "Replace logo" : "Upload logo"}
              </Button>
              {partner.link_url && (
                <Button variant="ghost" size="sm" asChild>
                  <a href={partner.link_url} target="_blank" rel="noreferrer">
                    <ExternalLink className="mr-2 h-3.5 w-3.5" />
                    Visit
                  </a>
                </Button>
              )}
              <Button
                size="sm"
                disabled={!dirty || !linkValid || name.trim() === "" || busy}
                onClick={() => onSave(partner.id, { name: name.trim(), link_url: linkUrl.trim(), active })}
              >
                Save
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="text-destructive"
                disabled={busy}
                onClick={() => onDelete(partner)}
                aria-label={`Remove ${partner.name}`}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      </div>
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
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [newName, setNewName] = useState("")
  const [newLink, setNewLink] = useState("")

  const loadPartners = useCallback(async () => {
    setLoading(true)
    setError("")
    try {
      const res = await authFetch("/admin/partners")
      if (!res.ok) throw new Error("Unable to load partners.")
      const data = (await res.json()) as PartnersResponse
      setPartners(data.partners || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load partners.")
    } finally {
      setLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    if (status !== "authenticated" || !user?.isAdmin) return
    void loadPartners()
  }, [loadPartners, status, user?.isAdmin])

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

  const handleSave = async (id: string, values: { name: string; link_url: string; active: boolean }) => {
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
      toast({ title: "Partner saved" })
    } catch (err) {
      toast({
        title: "Could not save partner",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
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

        {loading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading partners…
          </div>
        )}

        {error !== "" && <p className="text-sm text-destructive">{error}</p>}

        {!loading && error === "" && partners.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No partners yet. Add one above, then upload its logo.
          </p>
        )}

        <div className="space-y-3">
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
