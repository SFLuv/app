"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { ArrowDown, ArrowUp, ChevronDown, ChevronUp, History, ImageIcon, Loader2, Plus, Trash2, Upload } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useAuthFetch } from "./use-auth-fetch"
import { formatWhen, jsonBody, resolveSiteUrl, siteFetch, uploadSiteFile } from "@/lib/site-admin"
import type { SiteSpotlightAdmin, SiteSpotlightSlide } from "@/types/site"
import { useNotice } from "./notice"

const POSITIONS: { value: string; label: string }[] = [
  { value: "center 8%", label: "Top of the photo (keeps faces near the top)" },
  { value: "center 25%", label: "Upper part" },
  { value: "middle", label: "Middle" },
  { value: "center 75%", label: "Lower part" },
  { value: "center 92%", label: "Bottom of the photo" },
]

const LINK_PATTERN = /\[([^\]\n]{1,120})\]\(([^)\s]{1,500})\)/g

function newId() {
  return `slide-${Math.random().toString(16).slice(2, 10)}`
}

function blankSlide(): SiteSpotlightSlide {
  return {
    id: newId(),
    enabled: false,
    label: "",
    title: "",
    body: "",
    image_file_id: null,
    image_url: null,
    image_width: 0,
    image_height: 0,
    image_alt: "",
    image_position: "center 8%",
    image_preview_url: null,
    action: { label: "", href: "", new_tab: false, also_open_file_id: null, also_open_url: null },
    event_match: "",
  }
}

/** The part of a slide that is saved. The preview URL is for display only. */
function comparable(slides: SiteSpotlightSlide[]) {
  return JSON.stringify(slides.map(({ image_preview_url: _preview, ...rest }) => rest))
}

function PreviewText({ text }: { text: string }) {
  const parts: React.ReactNode[] = []
  let last = 0
  for (const match of text.matchAll(LINK_PATTERN)) {
    const start = match.index ?? 0
    if (start > last) parts.push(text.slice(last, start))
    const safe = /^(\/(?!\/)|https?:\/\/|mailto:)/i.test(match[2])
    parts.push(
      safe ? (
        <span key={start} className="font-medium text-primary underline underline-offset-2">
          {match[1]}
        </span>
      ) : (
        match[0]
      ),
    )
    last = start + match[0].length
  }
  if (last < text.length) parts.push(text.slice(last))
  return <>{parts}</>
}

/** Approximates how a slide looks on the site. The website uses its own styling. */
function SlidePreview({ slide }: { slide: SiteSpotlightSlide }) {
  const image = resolveSiteUrl(slide.image_preview_url)
  return (
    <div className="max-w-sm overflow-hidden rounded-xl border bg-background shadow-sm">
      <div className="h-28 w-full overflow-hidden bg-muted">
        {image ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={image} alt="" className="h-full w-full object-cover" style={{ objectPosition: slide.image_position || "center" }} />
        ) : (
          <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
            <ImageIcon className="mr-1.5 h-4 w-4" /> Add a photo
          </div>
        )}
      </div>
      <div className="space-y-1.5 p-3">
        {slide.label && <span className="inline-block rounded-full bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary">{slide.label}</span>}
        <p className="font-semibold leading-tight">{slide.title || "Your title appears here"}</p>
        {slide.body && (
          <p className="text-xs text-muted-foreground">
            <PreviewText text={slide.body} />
          </p>
        )}
        {slide.action.label && (
          <span className="mt-1 inline-block rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground">{slide.action.label}</span>
        )}
      </div>
    </div>
  )
}

function SlideEditor({
  slide,
  onChange,
}: {
  slide: SiteSpotlightSlide
  onChange: (next: SiteSpotlightSlide) => void
}) {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const imageInput = useRef<HTMLInputElement>(null)
  const documentInput = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState<"image" | "document" | null>(null)
  const [documentName, setDocumentName] = useState<string | null>(null)

  const patch = (changes: Partial<SiteSpotlightSlide>) => onChange({ ...slide, ...changes })
  const patchAction = (changes: Partial<SiteSpotlightSlide["action"]>) => onChange({ ...slide, action: { ...slide.action, ...changes } })

  const uploadImage = async (file: File) => {
    setUploading("image")
    try {
      const uploaded = await uploadSiteFile(authFetch, file)
      if (!uploaded.content_type.startsWith("image/")) throw new Error("Please choose a photo (PNG, JPEG, WebP or GIF).")
      patch({
        image_file_id: uploaded.id,
        image_url: null,
        image_width: 0,
        image_height: 0,
        image_preview_url: uploaded.url,
      })
    } catch (err) {
      toast({ title: "Could not upload the photo", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setUploading(null)
    }
  }

  const uploadDocument = async (file: File) => {
    setUploading("document")
    try {
      const uploaded = await uploadSiteFile(authFetch, file)
      patchAction({ also_open_file_id: uploaded.id, also_open_url: null })
      setDocumentName(uploaded.filename)
    } catch (err) {
      toast({ title: "Could not upload the document", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setUploading(null)
    }
  }

  const hasDocument = Boolean(slide.action.also_open_file_id || slide.action.also_open_url)
  // "Middle" is stored as an empty position (centred); the select cannot hold an empty value.
  const positionChoice = slide.image_position === "" ? "middle" : slide.image_position
  const knownPosition = POSITIONS.some((p) => p.value === positionChoice)
  const offSite = /^https?:\/\//i.test(slide.action.href.trim())

  return (
    <div className="grid gap-6 border-t p-4 lg:grid-cols-[1fr_auto]">
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-[1fr_2fr]">
          <div className="space-y-1">
            <Label htmlFor={`${slide.id}-label`}>Small tag (optional)</Label>
            <Input
              id={`${slide.id}-label`}
              value={slide.label}
              maxLength={40}
              placeholder="Now published"
              onChange={(e) => patch({ label: e.target.value })}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor={`${slide.id}-title`}>Title</Label>
            <Input
              id={`${slide.id}-title`}
              value={slide.title}
              maxLength={150}
              placeholder="Our 2026–2027 Annual Impact Report"
              onChange={(e) => patch({ title: e.target.value })}
            />
          </div>
        </div>

        <div className="space-y-1">
          <Label htmlFor={`${slide.id}-body`}>Text (optional — keep it to a sentence)</Label>
          <Textarea
            id={`${slide.id}-body`}
            value={slide.body}
            maxLength={600}
            rows={3}
            onChange={(e) => patch({ body: e.target.value })}
          />
          <p className="text-xs text-muted-foreground">
            To put a link in the text, write [words to click](https://example.org), or [our volunteer page](/volunteers) for a page on the site.
          </p>
        </div>

        <div className="space-y-2 rounded-lg border p-3">
          <Label>Photo (shown as a wide strip across the top of the card)</Label>
          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" variant="outline" size="sm" disabled={uploading !== null} onClick={() => imageInput.current?.click()}>
              {uploading === "image" ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Upload className="mr-2 h-4 w-4" />}
              {slide.image_preview_url ? "Replace photo" : "Upload photo"}
            </Button>
            <input
              ref={imageInput}
              type="file"
              accept="image/png,image/jpeg,image/webp,image/gif"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0]
                e.target.value = ""
                if (file) void uploadImage(file)
              }}
            />
          </div>
          {slide.image_preview_url && (
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label>Which part of the photo to show</Label>
                <Select
                  value={knownPosition ? positionChoice : "__custom"}
                  onValueChange={(v) => v !== "__custom" && patch({ image_position: v === "middle" ? "" : v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {POSITIONS.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        {p.label}
                      </SelectItem>
                    ))}
                    {!knownPosition && <SelectItem value="__custom">Custom ({slide.image_position})</SelectItem>}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">The card shows a wide strip, so part of a tall photo is cut off.</p>
              </div>
              <div className="space-y-1">
                <Label htmlFor={`${slide.id}-alt`}>Describe the photo (for screen readers)</Label>
                <Input
                  id={`${slide.id}-alt`}
                  value={slide.image_alt}
                  maxLength={200}
                  placeholder="Five volunteers in safety vests…"
                  onChange={(e) => patch({ image_alt: e.target.value })}
                />
              </div>
            </div>
          )}
        </div>

        <div className="space-y-3 rounded-lg border p-3">
          <p className="text-sm font-medium">Button</p>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor={`${slide.id}-btn`}>Button words</Label>
              <Input
                id={`${slide.id}-btn`}
                value={slide.action.label}
                maxLength={40}
                placeholder="Read the report"
                onChange={(e) => patchAction({ label: e.target.value })}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor={`${slide.id}-href`}>Where it goes</Label>
              <Input
                id={`${slide.id}-href`}
                value={slide.action.href}
                placeholder="/volunteers or https://…"
                onChange={(e) => patchAction({ href: e.target.value })}
              />
            </div>
          </div>
          {offSite && (
            <div className="flex items-center gap-3">
              <Switch id={`${slide.id}-tab`} checked={slide.action.new_tab} onCheckedChange={(v) => patchAction({ new_tab: v })} />
              <Label htmlFor={`${slide.id}-tab`}>Open this other website in a new tab</Label>
            </div>
          )}
          <div className="space-y-1.5">
            <Label>Also open a document in a new tab (optional)</Label>
            <div className="flex flex-wrap items-center gap-2">
              <Button type="button" variant="outline" size="sm" disabled={uploading !== null} onClick={() => documentInput.current?.click()}>
                {uploading === "document" ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Upload className="mr-2 h-4 w-4" />}
                {hasDocument ? "Replace document" : "Upload a PDF"}
              </Button>
              {hasDocument && (
                <>
                  <Badge variant="secondary">{documentName ?? (slide.action.also_open_url ? "Existing document attached" : "Document attached")}</Badge>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      patchAction({ also_open_file_id: null, also_open_url: null })
                      setDocumentName(null)
                    }}
                  >
                    <Trash2 className="mr-2 h-4 w-4" /> Remove
                  </Button>
                </>
              )}
              <input
                ref={documentInput}
                type="file"
                accept="application/pdf"
                className="hidden"
                onChange={(e) => {
                  const file = e.target.files?.[0]
                  e.target.value = ""
                  if (file) void uploadDocument(file)
                }}
              />
            </div>
            <p className="text-xs text-muted-foreground">When someone clicks the button, the document opens in a new tab while the site goes to the place above.</p>
          </div>
        </div>

        <div className="space-y-1">
          <Label htmlFor={`${slide.id}-event`}>Link to a recurring volunteer event (optional)</Label>
          <Input
            id={`${slide.id}-event`}
            value={slide.event_match}
            maxLength={100}
            placeholder="weekly clean"
            onChange={(e) => patch({ event_match: e.target.value })}
          />
          <p className="text-xs text-muted-foreground">
            Type a few words from the event’s title, in order. The button then goes to the next upcoming event with those words, and its date
            and reward are shown. Leave this empty for an ordinary announcement.
          </p>
        </div>
      </div>

      <div className="space-y-2">
        <Label>Preview (approximate)</Label>
        <SlidePreview slide={slide} />
        {!slide.enabled && <p className="max-w-sm text-xs text-muted-foreground">This highlight is switched off, so visitors will not see it.</p>}
      </div>
    </div>
  )
}

export function HighlightsEditor() {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()

  const [loaded, setLoaded] = useState<SiteSpotlightAdmin | null>(null)
  const [slides, setSlides] = useState<SiteSpotlightSlide[]>([])
  const [expanded, setExpanded] = useState<string | null>(null)
  const [error, setError] = useState("")
  const [saving, setSaving] = useState(false)

  const apply = useCallback((data: SiteSpotlightAdmin) => {
    setLoaded(data)
    setSlides(data.current.slides)
  }, [])

  const load = useCallback(async () => {
    try {
      apply(await siteFetch<SiteSpotlightAdmin>(authFetch, "/admin/site/spotlight"))
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load the highlights.")
    }
  }, [authFetch, apply])

  useEffect(() => {
    void load()
  }, [load])

  const dirty = loaded !== null && comparable(slides) !== comparable(loaded.current.slides)

  const update = (id: string, next: SiteSpotlightSlide) => setSlides((list) => list.map((s) => (s.id === id ? next : s)))
  const move = (index: number, direction: -1 | 1) =>
    setSlides((list) => {
      const target = index + direction
      if (target < 0 || target >= list.length) return list
      const copy = [...list]
      ;[copy[index], copy[target]] = [copy[target], copy[index]]
      return copy
    })

  const add = () => {
    const slide = blankSlide()
    setSlides((list) => [slide, ...list]) // new announcements go first, as on the site
    setExpanded(slide.id)
  }

  const remove = (slide: SiteSpotlightSlide) => {
    if (!window.confirm(`Delete “${slide.title || "this highlight"}”? To keep it for later, switch it off instead.`)) return
    setSlides((list) => list.filter((s) => s.id !== slide.id))
  }

  const save = async () => {
    setSaving(true)
    try {
      const next = await siteFetch<SiteSpotlightAdmin>(authFetch, "/admin/site/spotlight", {
        method: "PUT",
        // The version this draft started from: the server refuses the save if
        // someone else has saved since, rather than silently overwriting them.
        ...jsonBody({ slides: slides.map(({ image_preview_url: _preview, ...rest }) => rest), base_version: loaded?.version ?? 0 }),
      })
      apply(next)
      const showing = next.current.slides.filter((s) => s.enabled).length
      toast({
        title: "Highlights saved",
        description: `${showing} showing on the homepage. Changes appear on the website within about 30 seconds.`,
      })
    } catch (err) {
      toast({ title: "Could not save", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setSaving(false)
    }
  }

  const restore = async (version: number) => {
    if (dirty && !window.confirm("You have unsaved changes. Restoring will discard them. Continue?")) return
    try {
      apply(await siteFetch<SiteSpotlightAdmin>(authFetch, "/admin/site/spotlight/restore", { method: "POST", ...jsonBody({ version }) }))
      toast({ title: `Restored version ${version}`, description: "It appears on the website within about 30 seconds." })
    } catch (err) {
      toast({ title: "Could not restore", description: err instanceof Error ? err.message : "", variant: "destructive" })
    }
  }

  if (error) return <p className="text-sm text-destructive">{error}</p>
  if (!loaded) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading the highlights…
      </div>
    )
  }

  const showing = slides.filter((s) => s.enabled).length

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1.5">
            <CardTitle>Homepage highlights</CardTitle>
            <CardDescription>
              The rotating card beside the title on sfluv.org. Switch a highlight off when it is over — it stays here for next time. New highlights go
              first. Changes show on the website within about 30 seconds.
            </CardDescription>
          </div>
          <Button onClick={add}>
            <Plus className="mr-2 h-4 w-4" /> New highlight
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {showing === 0 ? "Nothing is showing — the card is hidden on the homepage." : `${showing} showing on the homepage, in the order below.`}
          </p>

          {slides.length === 0 && <p className="text-sm text-muted-foreground">No highlights yet. Add one to get started.</p>}

          <div className="space-y-3">
            {slides.map((slide, index) => {
              const open = expanded === slide.id
              const image = resolveSiteUrl(slide.image_preview_url)
              return (
                <div key={slide.id} className={`overflow-hidden rounded-lg border ${slide.enabled ? "" : "bg-muted/30"}`}>
                  <div className="flex flex-wrap items-center gap-3 p-3">
                    <div className="flex shrink-0 flex-col">
                      <Button variant="ghost" size="icon" className="h-5 w-6" disabled={index === 0} onClick={() => move(index, -1)} aria-label="Move up">
                        <ArrowUp className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-5 w-6" disabled={index === slides.length - 1} onClick={() => move(index, 1)} aria-label="Move down">
                        <ArrowDown className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                    <div className="flex h-12 w-20 shrink-0 items-center justify-center overflow-hidden rounded-md border bg-muted">
                      {/* eslint-disable-next-line @next/next/no-img-element */}
                      {image ? <img src={image} alt="" className="h-full w-full object-cover" style={{ objectPosition: slide.image_position || "center" }} /> : <ImageIcon className="h-4 w-4 text-muted-foreground" />}
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-medium">{slide.title || "Untitled highlight"}</p>
                      <p className="truncate text-xs text-muted-foreground">
                        {slide.label || "No tag"} · {slide.action.label ? `Button: ${slide.action.label}` : "No button yet"}
                        {slide.event_match ? " · linked to an event" : ""}
                      </p>
                    </div>
                    <div className="flex items-center gap-2">
                      <Label htmlFor={`${slide.id}-on`} className="text-sm">
                        {slide.enabled ? "Showing" : "Off"}
                      </Label>
                      <Switch id={`${slide.id}-on`} checked={slide.enabled} onCheckedChange={(enabled) => update(slide.id, { ...slide, enabled })} />
                    </div>
                    <div className="flex gap-1">
                      <Button variant="outline" size="sm" onClick={() => setExpanded(open ? null : slide.id)}>
                        {open ? <ChevronUp className="mr-1.5 h-3.5 w-3.5" /> : <ChevronDown className="mr-1.5 h-3.5 w-3.5" />}
                        {open ? "Close" : "Edit"}
                      </Button>
                      <Button variant="ghost" size="icon" aria-label="Delete highlight" onClick={() => remove(slide)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                  {open && <SlideEditor slide={slide} onChange={(next) => update(slide.id, next)} />}
                </div>
              )
            })}
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <Button onClick={save} disabled={saving || !dirty}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Save and publish
            </Button>
            {dirty ? (
              <>
                <span className="text-sm text-amber-600">You have unsaved changes.</span>
                <Button variant="ghost" size="sm" onClick={() => apply(loaded)}>
                  Discard
                </Button>
              </>
            ) : (
              <span className="text-sm text-muted-foreground">Everything is saved.</span>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <History className="h-4 w-4" /> History
          </CardTitle>
          <CardDescription>Every save is kept. Restoring an older version puts it back on the site and adds it to this list.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="divide-y rounded-lg border">
            {loaded.versions.map((v) => {
              const on = v.value.slides.filter((s) => s.enabled).map((s) => s.title || "Untitled")
              return (
                <div key={v.version} className="flex flex-wrap items-center justify-between gap-3 p-3">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-sm font-medium">
                      {v.note || `Version ${v.version}`}
                      {v.is_current && <Badge variant="success">Current</Badge>}
                    </div>
                    <p className="truncate text-xs text-muted-foreground">
                      {on.length > 0 ? `Showing: ${on.join(" · ")}` : "Nothing showing"} · {v.saved_by || "Unknown"} · {formatWhen(v.saved_at)}
                    </p>
                  </div>
                  {!v.is_current && (
                    <Button variant="outline" size="sm" onClick={() => restore(v.version)}>
                      Restore
                    </Button>
                  )}
                </div>
              )
            })}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
