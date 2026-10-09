"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { ArrowLeft, ArrowRight, ExternalLink, ImageIcon, Loader2, Pencil, Plus, Star, Trash2, Upload } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { useAuthFetch } from "./use-auth-fetch"
import { useNotice } from "./notice"
import { SITE_ORIGIN, checkSiteFile, formatWhen, jsonBody, resolveSiteUrl, siteFetch, uploadSiteFile } from "@/lib/site-admin"
import type { SitePastEvent, SitePastEventRequest, SitePhoto } from "@/types/site"

const pastEventUrl = (slug: string) => `${SITE_ORIGIN}/volunteers/past/${slug}`

/** Today in San Francisco, as YYYY-MM-DD, so a past event can't be dated tomorrow. */
function todayInSF(): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "America/Los_Angeles" }).format(new Date())
}

function formatDate(date: string): string {
  const [y, m, d] = date.split("-").map(Number)
  if (!y || !m || !d) return date
  return new Intl.DateTimeFormat("en-US", { month: "long", day: "numeric", year: "numeric", timeZone: "UTC" }).format(
    new Date(Date.UTC(y, m - 1, d)),
  )
}

function Thumb({ photo, className }: { photo: SitePhoto | null; className?: string }) {
  const src = resolveSiteUrl(photo?.url)
  return (
    <div className={`overflow-hidden rounded-md border bg-muted ${className ?? ""}`}>
      {src ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={src} alt="" className="h-full w-full object-cover" loading="lazy" />
      ) : (
        <div className="flex h-full w-full items-center justify-center text-muted-foreground">
          <ImageIcon className="h-4 w-4" />
        </div>
      )}
    </div>
  )
}

type View = { mode: "list" } | { mode: "new" } | { mode: "edit"; id: string }

/**
 * The tiles in Past events on the volunteers page, each with a photo gallery.
 * A past event is only a tile and its photos: making one creates no volunteer
 * event, QR codes or rewards.
 */
export function PastEventsEditor() {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const [events, setEvents] = useState<SitePastEvent[] | null>(null)
  const [error, setError] = useState("")
  const [view, setView] = useState<View>({ mode: "list" })
  const [busyId, setBusyId] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const data = await siteFetch<{ events: SitePastEvent[] }>(authFetch, "/admin/site/past-events")
      setEvents(data.events)
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load past events.")
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  const replace = (updated: SitePastEvent) =>
    setEvents((prev) => (prev ? prev.map((ev) => (ev.id === updated.id ? updated : ev)) : prev))

  const setRemoved = async (ev: SitePastEvent, removed: boolean) => {
    if (removed && !window.confirm(`Take “${ev.title}” off the website? You can restore it from “Recently removed”.`)) return
    setBusyId(ev.id)
    try {
      replace(
        await siteFetch<SitePastEvent>(authFetch, `/admin/site/past-events/${ev.id}${removed ? "" : "/restore"}`, {
          method: removed ? "DELETE" : "POST",
        }),
      )
      toast({ title: removed ? "Removed" : "Restored", description: "The website updates within about 30 seconds." })
    } catch (err) {
      toast({ title: "Couldn’t update the event", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setBusyId(null)
    }
  }

  if (view.mode === "new") {
    return (
      <PastEventDetails
        event={null}
        onBack={() => setView({ mode: "list" })}
        onSaved={(created) => {
          setEvents((prev) => [created, ...(prev ?? [])])
          setView({ mode: "edit", id: created.id })
        }}
      />
    )
  }

  if (view.mode === "edit") {
    const ev = events?.find((e) => e.id === view.id)
    if (ev) {
      return (
        <div className="space-y-6">
          <PastEventDetails event={ev} onBack={() => setView({ mode: "list" })} onSaved={replace} />
          <PastEventGallery event={ev} onChange={replace} />
        </div>
      )
    }
  }

  if (error) return <p className="text-sm text-destructive">{error}</p>
  if (!events) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading past events…
      </div>
    )
  }

  const shown = events.filter((ev) => !ev.removed_at)
  const removed = events.filter((ev) => ev.removed_at)

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-4 space-y-0">
          <div className="max-w-2xl space-y-1.5">
            <CardTitle>Past events</CardTitle>
            <CardDescription>
              The tiles under Past events on the volunteers page. Each opens a gallery of photos from that event. Adding one only adds a
              tile and its photos: it doesn’t create a volunteer event, QR codes or SFLUV rewards. Changes show on the website within
              about 30 seconds.
            </CardDescription>
          </div>
          <Button onClick={() => setView({ mode: "new" })}>
            <Plus className="mr-2 h-4 w-4" /> New past event
          </Button>
        </CardHeader>
        <CardContent>
          {shown.length === 0 ? (
            <p className="text-sm text-muted-foreground">No past events yet. Add one to get started.</p>
          ) : (
            <ul className="divide-y rounded-lg border">
              {shown.map((ev) => (
                <li key={ev.id} className="flex flex-wrap items-center gap-3 p-3">
                  <Thumb photo={ev.cover} className="h-12 w-20 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">{ev.title}</p>
                    <p className="text-xs text-muted-foreground">
                      {formatDate(ev.date)} · {ev.photos.length === 1 ? "1 photo" : `${ev.photos.length} photos`}
                      {!ev.cover && " · no tile photo"}
                    </p>
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-1">
                    <Button variant="ghost" size="sm" onClick={() => setView({ mode: "edit", id: ev.id })}>
                      <Pencil className="mr-1.5 h-3.5 w-3.5" /> Edit
                    </Button>
                    <Button variant="ghost" size="sm" asChild>
                      <a href={pastEventUrl(ev.slug)} target="_blank" rel="noreferrer">
                        <ExternalLink className="mr-1.5 h-3.5 w-3.5" /> View on the site
                      </a>
                    </Button>
                    <Button variant="ghost" size="sm" disabled={busyId === ev.id} onClick={() => void setRemoved(ev, true)}>
                      <Trash2 className="mr-1.5 h-3.5 w-3.5" /> Remove
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      {removed.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Recently removed</CardTitle>
            <CardDescription>Not shown on the website. Restore one to put it back, photos and all.</CardDescription>
          </CardHeader>
          <CardContent>
            <ul className="divide-y rounded-lg border">
              {removed.map((ev) => (
                <li key={ev.id} className="flex flex-wrap items-center justify-between gap-3 p-3 text-sm">
                  <span>
                    {ev.title} <span className="text-muted-foreground">· removed {formatWhen(ev.removed_at)}</span>
                  </span>
                  <Button variant="outline" size="sm" disabled={busyId === ev.id} onClick={() => void setRemoved(ev, false)}>
                    Restore
                  </Button>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}
    </div>
  )
}

/** Title, date, words and tile photo, for a new event or an existing one. */
function PastEventDetails({
  event,
  onBack,
  onSaved,
}: {
  event: SitePastEvent | null
  onBack: () => void
  onSaved: (event: SitePastEvent) => void
}) {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const coverInput = useRef<HTMLInputElement>(null)
  const [title, setTitle] = useState(event?.title ?? "")
  const [date, setDate] = useState(event?.date ?? "")
  const [description, setDescription] = useState(event?.description ?? "")
  const [newCover, setNewCover] = useState<{ id: string; url: string } | null>(null)
  const [busy, setBusy] = useState<"upload" | "save" | null>(null)

  // Follow the saved event (after a save, or the gallery choosing a new tile photo).
  useEffect(() => {
    if (!event) return
    setTitle(event.title)
    setDate(event.date)
    setDescription(event.description)
    setNewCover(null)
  }, [event])

  const chooseCover = async (file: File) => {
    const problem = await checkSiteFile(file, "image")
    if (problem) return toast({ title: "Can’t use that file", description: problem, variant: "destructive" })
    setBusy("upload")
    try {
      const uploaded = await uploadSiteFile(authFetch, file)
      setNewCover({ id: uploaded.id, url: uploaded.url })
    } catch (err) {
      toast({ title: "Could not upload the photo", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setBusy(null)
    }
  }

  const save = async () => {
    // Screen readers describe the tile photo by the event's title, unless an older tile already has a description.
    const body: SitePastEventRequest = { title, date, description, cover_alt: newCover ? "" : (event?.cover?.alt ?? "") }
    if (newCover) body.cover_file_id = newCover.id
    setBusy("save")
    try {
      const saved = await siteFetch<SitePastEvent>(authFetch, event ? `/admin/site/past-events/${event.id}` : "/admin/site/past-events", {
        method: event ? "PUT" : "POST",
        ...jsonBody(body),
      })
      toast({
        title: event ? "Saved" : "Past event added",
        description: event ? "The website updates within about 30 seconds." : "Now add its photos below.",
      })
      onSaved(saved)
    } catch (err) {
      toast({ title: "Couldn’t save", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setBusy(null)
    }
  }

  const cover = newCover ? { url: newCover.url } : event?.cover
  const dirty =
    !event ||
    newCover !== null ||
    title !== event.title ||
    date !== event.date ||
    description !== event.description

  return (
    <Card>
      <CardHeader className="space-y-3">
        <Button variant="ghost" size="sm" className="-ml-2 w-fit" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" /> All past events
        </Button>
        <div className="flex flex-wrap items-center gap-3">
          <CardTitle>{event ? `Edit “${event.title}”` : "New past event"}</CardTitle>
          {event && (
            <Button variant="outline" size="sm" asChild>
              <a href={pastEventUrl(event.slug)} target="_blank" rel="noreferrer">
                <ExternalLink className="mr-1.5 h-3.5 w-3.5" /> View on the site
              </a>
            </Button>
          )}
        </div>
        {!event && (
          <CardDescription>
            This adds a tile under Past events on the volunteers page, with its own photo gallery. It doesn’t create a volunteer event, QR
            codes or SFLUV rewards.
          </CardDescription>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
          <div className="space-y-1">
            <Label htmlFor="past-title">Title</Label>
            <Input id="past-title" value={title} maxLength={150} placeholder="Tenderloin Weekly Clean-up" onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="past-date">Date it took place</Label>
            <Input id="past-date" type="date" value={date} max={todayInSF()} onChange={(e) => setDate(e.target.value)} />
          </div>
        </div>
        <div className="space-y-1">
          <Label htmlFor="past-description">A few words about it (optional, shown on the gallery page)</Label>
          <Textarea id="past-description" rows={3} maxLength={2000} value={description} onChange={(e) => setDescription(e.target.value)} />
        </div>

        <div className="space-y-2 rounded-lg border p-3">
          <Label>Tile photo (shown on the volunteers page)</Label>
          <div className="flex flex-wrap items-center gap-3">
            <Thumb photo={cover ? ({ url: cover.url } as SitePhoto) : null} className="h-16 w-28" />
            <Button type="button" variant="outline" size="sm" disabled={busy !== null} onClick={() => coverInput.current?.click()}>
              {busy === "upload" ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Upload className="mr-2 h-4 w-4" />}
              {cover ? "Replace photo" : "Upload photo"}
            </Button>
            <input
              ref={coverInput}
              type="file"
              accept="image/png,image/jpeg,image/webp,image/gif"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0]
                e.target.value = ""
                if (file) void chooseCover(file)
              }}
            />
          </div>
          {event && event.photos.length > 0 && (
            <p className="text-xs text-muted-foreground">Or pick one from the gallery below with “Use as tile photo”.</p>
          )}
          {!cover && <p className="text-xs text-muted-foreground">Without one, the tile shows a coloured placeholder.</p>}
        </div>

        <Button onClick={() => void save()} disabled={busy !== null || !dirty || !title.trim() || !date}>
          {busy === "save" && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {event ? "Save changes" : "Add past event"}
        </Button>
      </CardContent>
    </Card>
  )
}

/** The event's photos: add many at once, put them in order, caption them, pick the tile photo. */
function PastEventGallery({ event, onChange }: { event: SitePastEvent; onChange: (event: SitePastEvent) => void }) {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const input = useRef<HTMLInputElement>(null)
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  const call = async (path: string, init: RequestInit, done?: string) => {
    const updated = await siteFetch<SitePastEvent>(authFetch, `/admin/site/past-events/${event.id}${path}`, init)
    onChange(updated)
    if (done) toast({ title: done, description: "The website updates within about 30 seconds." })
  }

  const addPhotos = async (files: File[]) => {
    const usable: File[] = []
    const refused: string[] = []
    for (const file of files) {
      if (await checkSiteFile(file, "image")) refused.push(file.name)
      else usable.push(file)
    }
    if (refused.length > 0) {
      toast({
        title: refused.length === 1 ? "One file isn’t a photo" : `${refused.length} files aren’t photos`,
        description: `Skipped: ${refused.slice(0, 5).join(", ")}${refused.length > 5 ? "…" : ""}`,
        variant: "destructive",
      })
    }
    if (usable.length === 0) return

    setProgress({ done: 0, total: usable.length })
    const ids: string[] = []
    try {
      for (const file of usable) {
        ids.push((await uploadSiteFile(authFetch, file)).id)
        setProgress({ done: ids.length, total: usable.length })
      }
      // In batches of 50, the most one request may add.
      for (let i = 0; i < ids.length; i += 50) {
        await call("/photos", { method: "POST", ...jsonBody({ file_ids: ids.slice(i, i + 50) }) })
      }
      toast({ title: ids.length === 1 ? "Photo added" : `${ids.length} photos added`, description: "The website updates within about 30 seconds." })
    } catch (err) {
      toast({ title: "Couldn’t add all the photos", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setProgress(null)
    }
  }

  const move = async (index: number, delta: number) => {
    const order = event.photos.map((p) => p.id!)
    const target = index + delta
    if (target < 0 || target >= order.length) return
    ;[order[index], order[target]] = [order[target], order[index]]
    setBusyId(order[target])
    try {
      await call("/photos/order", { method: "PUT", ...jsonBody({ order }) })
    } catch (err) {
      toast({ title: "Couldn’t move the photo", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setBusyId(null)
    }
  }

  const useAsCover = async (photo: SitePhoto) => {
    setBusyId(photo.id!)
    try {
      await call(
        "",
        {
          method: "PUT",
          ...jsonBody({
            title: event.title,
            date: event.date,
            description: event.description,
            cover_photo_id: photo.id,
            cover_alt: "",
          } satisfies SitePastEventRequest),
        },
        "Tile photo changed",
      )
    } catch (err) {
      toast({ title: "Couldn’t change the tile photo", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setBusyId(null)
    }
  }

  const saveCaption = async (photo: SitePhoto, caption: string) => {
    if (caption.trim() === (photo.caption ?? "")) return
    try {
      await call(`/photos/${photo.id}`, { method: "PUT", ...jsonBody({ caption }) }, "Caption saved")
    } catch (err) {
      toast({ title: "Couldn’t save the caption", description: err instanceof Error ? err.message : "", variant: "destructive" })
    }
  }

  const remove = async (photo: SitePhoto) => {
    if (!window.confirm("Remove this photo from the gallery?")) return
    setBusyId(photo.id!)
    try {
      await call(`/photos/${photo.id}`, { method: "DELETE" }, "Photo removed")
    } catch (err) {
      toast({ title: "Couldn’t remove the photo", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setBusyId(null)
    }
  }

  const isCover = (photo: SitePhoto) =>
    Boolean(event.cover && (photo.file_id ? photo.file_id === event.cover.file_id : !event.cover.file_id && photo.url === event.cover.url))

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-4 space-y-0">
        <div className="space-y-1.5">
          <CardTitle>Gallery</CardTitle>
          <CardDescription>
            {event.photos.length === 0
              ? "No photos yet. Add as many as you like; you can choose several at once."
              : `${event.photos.length} ${event.photos.length === 1 ? "photo" : "photos"}, shown on the gallery page in this order.`}
          </CardDescription>
        </div>
        <Button disabled={progress !== null} onClick={() => input.current?.click()}>
          {progress ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Upload className="mr-2 h-4 w-4" />}
          {progress ? `Uploading ${progress.done} of ${progress.total}…` : "Add photos"}
        </Button>
        <input
          ref={input}
          type="file"
          multiple
          accept="image/png,image/jpeg,image/webp,image/gif"
          className="hidden"
          onChange={(e) => {
            const files = Array.from(e.target.files ?? [])
            e.target.value = ""
            if (files.length > 0) void addPhotos(files)
          }}
        />
      </CardHeader>
      <CardContent>
        {event.photos.length > 0 && (
          <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {event.photos.map((photo, index) => (
              <li key={photo.id} className="space-y-2 rounded-lg border p-2">
                <div className="relative">
                  <Thumb photo={photo} className="aspect-[4/3] w-full" />
                  {isCover(photo) && (
                    <Badge className="absolute left-2 top-2" variant="secondary">
                      <Star className="mr-1 h-3 w-3" /> Tile photo
                    </Badge>
                  )}
                  {busyId === photo.id && (
                    <div className="absolute inset-0 flex items-center justify-center rounded-md bg-background/60">
                      <Loader2 className="h-5 w-5 animate-spin" />
                    </div>
                  )}
                </div>
                <Textarea
                  aria-label={`Caption for photo ${index + 1}`}
                  placeholder="Caption (optional, shown under the photo)"
                  defaultValue={photo.caption ?? ""}
                  maxLength={300}
                  rows={2}
                  onBlur={(e) => void saveCaption(photo, e.target.value)}
                />
                <div className="flex flex-wrap items-center justify-between gap-1">
                  <div className="flex gap-1">
                    <Button variant="ghost" size="icon" className="h-8 w-8" aria-label="Move earlier" disabled={index === 0 || busyId !== null} onClick={() => void move(index, -1)}>
                      <ArrowLeft className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8"
                      aria-label="Move later"
                      disabled={index === event.photos.length - 1 || busyId !== null}
                      onClick={() => void move(index, 1)}
                    >
                      <ArrowRight className="h-4 w-4" />
                    </Button>
                  </div>
                  <div className="flex gap-1">
                    {!isCover(photo) && (
                      <Button variant="ghost" size="sm" disabled={busyId !== null} onClick={() => void useAsCover(photo)}>
                        <Star className="mr-1.5 h-3.5 w-3.5" /> Use as tile photo
                      </Button>
                    )}
                    <Button variant="ghost" size="icon" className="h-8 w-8" aria-label="Remove photo" disabled={busyId !== null} onClick={() => void remove(photo)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
