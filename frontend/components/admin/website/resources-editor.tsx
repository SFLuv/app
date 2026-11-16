"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { ArrowDown, ArrowUp, History, ImageIcon, Loader2, Plus, Trash2, Upload, Video } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useAuthFetch } from "./use-auth-fetch"
import { checkSiteFile, formatWhen, jsonBody, resolveSiteUrl, siteFetch, uploadSiteFile } from "@/lib/site-admin"
import type { SiteResourceBook, SiteResources, SiteResourcesAdmin } from "@/types/site"
import { useNotice } from "./notice"

type ListKey = keyof SiteResources
type Item = { id: string; enabled: boolean }

function newId(prefix: string) {
  return `${prefix}-${Math.random().toString(16).slice(2, 10)}`
}

const YOUTUBE_ID = /^[A-Za-z0-9_-]{11}$/

/**
 * The video id in a YouTube link, for the preview only. The server reads it
 * again on save and is the one that decides; this mirrors its rules.
 */
function youTubeId(raw: string): string | null {
  const value = raw.trim()
  if (YOUTUBE_ID.test(value)) return value
  try {
    const url = new URL(value)
    const host = url.hostname.toLowerCase().replace(/^www\./, "").replace(/^m\./, "")
    const parts = url.pathname.replace(/^\/+|\/+$/g, "").split("/")
    let id = ""
    if (host === "youtu.be") id = parts[0]
    else if (["youtube.com", "youtube-nocookie.com", "music.youtube.com"].includes(host)) {
      if (url.pathname === "/watch") id = url.searchParams.get("v") || ""
      else if (parts.length === 2 && ["embed", "shorts", "live", "v"].includes(parts[0])) id = parts[1]
    }
    return YOUTUBE_ID.test(id) ? id : null
  } catch {
    return null
  }
}

/** The part of the page that is saved. Preview URLs are for display only. */
function comparable(res: SiteResources) {
  return JSON.stringify({ ...res, books: res.books.map(({ cover_preview_url: _preview, ...rest }) => rest) })
}

function forSaving(res: SiteResources) {
  return { ...res, books: res.books.map(({ cover_preview_url: _preview, ...rest }) => rest) }
}

const SECTION_ORDER: { key: ListKey; title: string; description: string; noun: string }[] = [
  {
    key: "how_to",
    title: "How to videos",
    description: "Short videos that show people how to use SFLuv. Each one needs a title and a YouTube link; the description is optional.",
    noun: "video",
  },
  { key: "books", title: "What we’re reading", description: "Books, each with a link and a cover picture.", noun: "book" },
  { key: "podcasts", title: "Podcasts we love", description: "Each podcast links to its page and shows one featured episode from YouTube.", noun: "podcast" },
  { key: "technical_links", title: "Get technical", description: "Links for people who want to go deeper into how SFLuv works.", noun: "link" },
]

function blank(key: ListKey): SiteResources[ListKey][number] {
  switch (key) {
    case "how_to":
      return { id: newId("howto"), enabled: false, title: "", description: "", video_url: "", video_id: "" }
    case "books":
      return {
        id: newId("book"), enabled: false, title: "", author: "", href: "", cover_file_id: null, cover_url: null,
        cover_width: 0, cover_height: 0, cover_alt: "", cover_preview_url: null,
      }
    case "podcasts":
      return { id: newId("podcast"), enabled: false, name: "", href: "", video_url: "", video_id: "" }
    case "technical_links":
      return { id: newId("link"), enabled: false, label: "", href: "" }
  }
}

function itemTitle(key: ListKey, item: SiteResources[ListKey][number]): string {
  switch (key) {
    case "how_to":
      return (item as SiteResources["how_to"][number]).title
    case "books":
      return (item as SiteResources["books"][number]).title
    case "podcasts":
      return (item as SiteResources["podcasts"][number]).name
    case "technical_links":
      return (item as SiteResources["technical_links"][number]).label
  }
}

function VideoField({ id, value, onChange }: { id: string; value: string; onChange: (value: string) => void }) {
  const videoId = value.trim() ? youTubeId(value) : null
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>YouTube link</Label>
      <Input id={id} value={value} maxLength={500} placeholder="https://www.youtube.com/watch?v=…" onChange={(e) => onChange(e.target.value)} />
      {value.trim() && !videoId && (
        <p className="text-xs text-destructive">That isn’t a YouTube video link. Use the link from the video’s Share button.</p>
      )}
      {videoId && (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={`https://i.ytimg.com/vi/${videoId}/hqdefault.jpg`} alt="" className="mt-2 h-24 rounded-md border object-cover" />
      )}
    </div>
  )
}

function BookCover({ book, onChange }: { book: SiteResourceBook; onChange: (next: SiteResourceBook) => void }) {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const input = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)
  const preview = resolveSiteUrl(book.cover_preview_url)

  const upload = async (file: File) => {
    const problem = await checkSiteFile(file, "image")
    if (problem) {
      toast({ title: "Can’t use that file", description: problem, variant: "destructive" })
      return
    }
    setUploading(true)
    try {
      const uploaded = await uploadSiteFile(authFetch, file)
      if (!uploaded.content_type.startsWith("image/")) throw new Error("Please choose a picture (PNG, JPEG, WebP or GIF).")
      onChange({ ...book, cover_file_id: uploaded.id, cover_url: null, cover_width: 0, cover_height: 0, cover_preview_url: uploaded.url })
    } catch (err) {
      toast({ title: "Could not upload the cover", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setUploading(false)
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-3">
      <div className="flex h-24 w-16 items-center justify-center overflow-hidden rounded-md border bg-muted">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        {preview ? <img src={preview} alt="" className="h-full w-full object-cover" /> : <ImageIcon className="h-4 w-4 text-muted-foreground" />}
      </div>
      <Button type="button" variant="outline" size="sm" disabled={uploading} onClick={() => input.current?.click()}>
        {uploading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Upload className="mr-2 h-4 w-4" />}
        {preview ? "Replace cover" : "Upload cover"}
      </Button>
      <input
        ref={input}
        type="file"
        accept="image/png,image/jpeg,image/webp,image/gif"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0]
          e.target.value = ""
          if (file) void upload(file)
        }}
      />
    </div>
  )
}

type AnyItem = SiteResources[ListKey][number]

function ItemFields({ section, item, onChange }: { section: ListKey; item: AnyItem; onChange: (next: AnyItem) => void }) {
  const field = (name: string) => `${item.id}-${name}`
  switch (section) {
    case "how_to": {
      const v = item as SiteResources["how_to"][number]
      return (
        <div className="grid gap-3">
          <div className="space-y-1">
            <Label htmlFor={field("title")}>Title</Label>
            <Input id={field("title")} value={v.title} maxLength={150} placeholder="How to redeem a perk QR code" onChange={(e) => onChange({ ...v, title: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor={field("description")}>Description (optional)</Label>
            <Textarea id={field("description")} value={v.description} maxLength={1000} rows={3} onChange={(e) => onChange({ ...v, description: e.target.value })} />
          </div>
          <VideoField id={field("video")} value={v.video_url} onChange={(video_url) => onChange({ ...v, video_url })} />
        </div>
      )
    }
    case "books": {
      const b = item as SiteResources["books"][number]
      return (
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor={field("title")}>Title</Label>
            <Input id={field("title")} value={b.title} maxLength={200} onChange={(e) => onChange({ ...b, title: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor={field("author")}>Author</Label>
            <Input id={field("author")} value={b.author} maxLength={150} onChange={(e) => onChange({ ...b, author: e.target.value })} />
          </div>
          <div className="space-y-1 sm:col-span-2">
            <Label htmlFor={field("href")}>Link</Label>
            <Input id={field("href")} value={b.href} maxLength={500} placeholder="https://…" onChange={(e) => onChange({ ...b, href: e.target.value })} />
          </div>
          <div className="space-y-1 sm:col-span-2">
            <Label>Cover</Label>
            <BookCover book={b} onChange={(next) => onChange(next)} />
          </div>
        </div>
      )
    }
    case "podcasts": {
      const p = item as SiteResources["podcasts"][number]
      return (
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor={field("name")}>Name</Label>
            <Input id={field("name")} value={p.name} maxLength={150} onChange={(e) => onChange({ ...p, name: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor={field("href")}>Podcast’s page</Label>
            <Input id={field("href")} value={p.href} maxLength={500} placeholder="https://…" onChange={(e) => onChange({ ...p, href: e.target.value })} />
          </div>
          <div className="sm:col-span-2">
            <VideoField id={field("video")} value={p.video_url} onChange={(video_url) => onChange({ ...p, video_url })} />
          </div>
        </div>
      )
    }
    case "technical_links": {
      const l = item as SiteResources["technical_links"][number]
      return (
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor={field("label")}>Words shown</Label>
            <Input id={field("label")} value={l.label} maxLength={150} placeholder="docs.celo.org — Discover Celo" onChange={(e) => onChange({ ...l, label: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor={field("href")}>Link</Label>
            <Input id={field("href")} value={l.href} maxLength={500} placeholder="https://…" onChange={(e) => onChange({ ...l, href: e.target.value })} />
          </div>
        </div>
      )
    }
  }
}

/** The Resources page on sfluv.org: How to videos, reading list, podcasts, technical links. */
export function ResourcesEditor() {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const [loaded, setLoaded] = useState<SiteResourcesAdmin | null>(null)
  const [draft, setDraft] = useState<SiteResources | null>(null)
  const [error, setError] = useState("")
  const [saving, setSaving] = useState(false)

  const apply = useCallback((data: SiteResourcesAdmin) => {
    setLoaded(data)
    setDraft(data.current)
  }, [])

  const load = useCallback(async () => {
    try {
      apply(await siteFetch<SiteResourcesAdmin>(authFetch, "/admin/site/resources"))
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load the resources page.")
    }
  }, [authFetch, apply])

  useEffect(() => {
    void load()
  }, [load])

  if (error) return <p className="text-sm text-destructive">{error}</p>
  if (!loaded || !draft) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading the resources page…
      </div>
    )
  }

  const dirty = comparable(draft) !== comparable(loaded.current)

  const setList = (key: ListKey, list: Item[]) => setDraft((current) => (current ? { ...current, [key]: list } : current))
  const listOf = (key: ListKey) => draft[key] as Item[]
  const updateItem = (key: ListKey, next: Item) => setList(key, listOf(key).map((item) => (item.id === next.id ? next : item)))
  const move = (key: ListKey, index: number, direction: -1 | 1) => {
    const list = [...listOf(key)]
    const target = index + direction
    if (target < 0 || target >= list.length) return
    ;[list[index], list[target]] = [list[target], list[index]]
    setList(key, list)
  }
  const add = (key: ListKey) => setList(key, [...listOf(key), blank(key) as Item])
  const remove = (key: ListKey, item: Item, noun: string) => {
    const name = itemTitle(key, item as SiteResources[ListKey][number]) || `this ${noun}`
    if (!window.confirm(`Delete “${name}”? To keep it for later, switch it off instead.`)) return
    setList(key, listOf(key).filter((i) => i.id !== item.id))
  }

  const save = async () => {
    setSaving(true)
    try {
      const next = await siteFetch<SiteResourcesAdmin>(authFetch, "/admin/site/resources", {
        method: "PUT",
        // The version this draft started from: the server refuses the save if
        // someone else has saved since, rather than silently overwriting them.
        ...jsonBody({ ...forSaving(draft), base_version: loaded.version }),
      })
      apply(next)
      toast({ title: "Resources page saved", description: "Changes appear on the website within about 30 seconds." })
    } catch (err) {
      toast({ title: "Could not save", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setSaving(false)
    }
  }

  const restore = async (version: number) => {
    if (dirty && !window.confirm("You have unsaved changes. Restoring will discard them. Continue?")) return
    try {
      apply(await siteFetch<SiteResourcesAdmin>(authFetch, "/admin/site/resources/restore", { method: "POST", ...jsonBody({ version }) }))
      toast({ title: `Restored version ${version}`, description: "It appears on the website within about 30 seconds." })
    } catch (err) {
      toast({ title: "Could not restore", description: err instanceof Error ? err.message : "", variant: "destructive" })
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-semibold">Resources page</h3>
        <p className="text-sm text-muted-foreground">
          sfluv.org/resources, section by section. Switch an item off to hide it without losing it. Changes show on the website within about 30 seconds of saving.
        </p>
      </div>

      {SECTION_ORDER.map((section) => {
        const list = listOf(section.key)
        const showing = list.filter((item) => item.enabled).length
        return (
          <Card key={section.key}>
            <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
              <div className="space-y-1.5">
                <CardTitle className="flex items-center gap-2">
                  {section.key === "how_to" && <Video className="h-4 w-4" />}
                  {section.title}
                </CardTitle>
                <CardDescription>{section.description}</CardDescription>
              </div>
              <Button variant="outline" onClick={() => add(section.key)}>
                <Plus className="mr-2 h-4 w-4" /> Add {section.noun}
              </Button>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="text-sm text-muted-foreground">
                {list.length === 0
                  ? "Nothing here yet — the section is hidden on the website."
                  : showing === 0
                    ? "Everything is switched off — the section is hidden on the website."
                    : `${showing} showing, in this order.`}
              </p>
              {list.map((item, index) => (
                <div key={item.id} className={`space-y-3 rounded-lg border p-3 ${item.enabled ? "" : "bg-muted/30"}`}>
                  <div className="flex flex-wrap items-center gap-3">
                    <div className="flex shrink-0 flex-col">
                      <Button variant="ghost" size="icon" className="h-5 w-6" disabled={index === 0} onClick={() => move(section.key, index, -1)} aria-label="Move up">
                        <ArrowUp className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-5 w-6" disabled={index === list.length - 1} onClick={() => move(section.key, index, 1)} aria-label="Move down">
                        <ArrowDown className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                    <p className="min-w-0 flex-1 truncate font-medium">
                      {itemTitle(section.key, item as SiteResources[ListKey][number]) || `Untitled ${section.noun}`}
                    </p>
                    <div className="flex items-center gap-2">
                      <Label htmlFor={`${item.id}-on`} className="text-sm">
                        {item.enabled ? "Showing" : "Off"}
                      </Label>
                      <Switch id={`${item.id}-on`} checked={item.enabled} onCheckedChange={(enabled) => updateItem(section.key, { ...item, enabled })} />
                    </div>
                    <Button variant="ghost" size="icon" aria-label={`Delete ${section.noun}`} onClick={() => remove(section.key, item, section.noun)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                  <ItemFields section={section.key} item={item as AnyItem} onChange={(next) => updateItem(section.key, next as Item)} />
                </div>
              ))}
            </CardContent>
          </Card>
        )
      })}

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

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <History className="h-4 w-4" /> History
          </CardTitle>
          <CardDescription>Every save is kept. Restoring an older version puts it back on the site and adds it to this list.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="divide-y rounded-lg border">
            {loaded.versions.map((v) => (
              <div key={v.version} className="flex flex-wrap items-center justify-between gap-3 p-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm font-medium">
                    {v.note || `Version ${v.version}`}
                    {v.is_current && <Badge variant="success">Current</Badge>}
                  </div>
                  <p className="truncate text-xs text-muted-foreground">
                    {v.value.how_to.filter((i) => i.enabled).length} videos · {v.value.books.filter((i) => i.enabled).length} books ·{" "}
                    {v.value.podcasts.filter((i) => i.enabled).length} podcasts · {v.value.technical_links.filter((i) => i.enabled).length} links ·{" "}
                    {v.saved_by || "Unknown"} · {formatWhen(v.saved_at)}
                  </p>
                </div>
                {!v.is_current && (
                  <Button variant="outline" size="sm" onClick={() => restore(v.version)}>
                    Restore
                  </Button>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
