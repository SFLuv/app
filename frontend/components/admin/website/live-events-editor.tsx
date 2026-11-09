"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { ArrowLeft, ExternalLink, Loader2, Pencil, Plus, Star, Trash2, Upload } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useAuthFetch } from "./use-auth-fetch"
import { useNotice } from "./notice"
import { PastEventGallery, Thumb } from "./past-events-editor"
import { SITE_ORIGIN, checkSiteFile, jsonBody, siteFetch } from "@/lib/site-admin"
import type { SiteLiveEvent, SitePastEvent, SitePhoto } from "@/types/site"

/** Matches the backend's limits for cover photos (maxEventPhotos, maxEventPhotoBytes). */
const MAX_COVER_PHOTOS = 6
const MAX_COVER_BYTES = 8 * 1024 * 1024

const liveEventUrl = (ev: SiteLiveEvent) => `${SITE_ORIGIN}/volunteers/${ev.slug}-${ev.id}`

function formatStart(ev: SiteLiveEvent): string {
  return new Intl.DateTimeFormat("en-US", {
    weekday: "short",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: ev.timezone || "America/Los_Angeles",
  }).format(new Date(ev.start_at))
}

const coverThumb = (photo: { url: string } | undefined) => (photo ? ({ url: photo.url } as SitePhoto) : null)

/**
 * Photos on the volunteer events people can sign up for right now: the cover
 * photos, and a gallery of what past occurrences looked like. The event's
 * details stay with whoever runs it; only its photos are edited here.
 */
export function LiveEventsEditor() {
  const authFetch = useAuthFetch()
  const [events, setEvents] = useState<SiteLiveEvent[] | null>(null)
  const [error, setError] = useState("")
  const [openId, setOpenId] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const data = await siteFetch<{ events: SiteLiveEvent[] }>(authFetch, "/admin/site/live-events")
      setEvents(data.events)
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load live events.")
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  const replace = (updated: SiteLiveEvent) =>
    setEvents((prev) => (prev ? prev.map((ev) => (ev.id === updated.id ? updated : ev)) : prev))

  const open = events?.find((ev) => ev.id === openId)
  if (open) return <LiveEventDetails event={open} onBack={() => setOpenId(null)} onChange={replace} />

  if (error) return <p className="text-sm text-destructive">{error}</p>
  if (!events) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading live events…
      </div>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Live events</CardTitle>
        <CardDescription>
          The volunteer events people can sign up for right now. Change an event’s cover photos, or add a gallery of what it has looked like
          in the past. Changes show on the website within a few minutes.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {events.length === 0 ? (
          <p className="text-sm text-muted-foreground">No volunteer events are coming up.</p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {events.map((ev) => {
              const count = ev.gallery?.photos.length ?? 0
              return (
                <li key={ev.id} className="flex flex-wrap items-center gap-3 p-3">
                  <Thumb photo={coverThumb(ev.cover_photos[0])} className="h-12 w-20 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">{ev.title}</p>
                    <p className="text-xs text-muted-foreground">
                      {ev.recurrence_summary || formatStart(ev)} · {ev.organizer_name} · {count === 1 ? "1 gallery photo" : `${count} gallery photos`}
                    </p>
                  </div>
                  <Button variant="ghost" size="sm" onClick={() => setOpenId(ev.id)}>
                    <Pencil className="mr-1.5 h-3.5 w-3.5" /> Edit photos
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

function LiveEventDetails({
  event,
  onBack,
  onChange,
}: {
  event: SiteLiveEvent
  onBack: () => void
  onChange: (event: SiteLiveEvent) => void
}) {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const [starting, setStarting] = useState(false)

  const startGallery = async () => {
    setStarting(true)
    try {
      onChange(await siteFetch<SiteLiveEvent>(authFetch, `/admin/site/live-events/${event.id}/gallery`, { method: "POST" }))
    } catch (err) {
      toast({ title: "Couldn’t start the gallery", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="space-y-3">
          <Button variant="ghost" size="sm" className="-ml-2 w-fit" onClick={onBack}>
            <ArrowLeft className="mr-1.5 h-4 w-4" /> All live events
          </Button>
          <div className="flex flex-wrap items-center gap-3">
            <CardTitle>{event.title}</CardTitle>
            <Button variant="outline" size="sm" asChild>
              <a href={liveEventUrl(event)} target="_blank" rel="noreferrer">
                <ExternalLink className="mr-1.5 h-3.5 w-3.5" /> View on the site
              </a>
            </Button>
          </div>
          <CardDescription>
            {event.recurrence_summary ? `${event.recurrence_summary}. Next: ${formatStart(event)}` : formatStart(event)} · {event.organizer_name}
          </CardDescription>
        </CardHeader>
      </Card>

      <CoverPhotos event={event} onChange={onChange} />

      {event.gallery ? (
        <PastEventGallery
          event={event.gallery}
          liveEventId={event.id}
          onChange={(gallery: SitePastEvent) => onChange({ ...event, gallery })}
        />
      ) : (
        <Card>
          <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-4 space-y-0">
            <div className="max-w-2xl space-y-1.5">
              <CardTitle>Gallery</CardTitle>
              <CardDescription>
                Lots of photos, up to 300, shown under the event so people can see what it’s like.
                {event.recurrence_summary && " A repeating event keeps the same gallery every time it comes around."}
              </CardDescription>
            </div>
            <Button disabled={starting} onClick={() => void startGallery()}>
              {starting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Plus className="mr-2 h-4 w-4" />}
              Start a gallery
            </Button>
          </CardHeader>
        </Card>
      )}
    </div>
  )
}

/** The photos at the top of the event's page. The first is also the one on its tile. */
function CoverPhotos({ event, onChange }: { event: SiteLiveEvent; onChange: (event: SiteLiveEvent) => void }) {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const input = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const photos = event.cover_photos
  const room = MAX_COVER_PHOTOS - photos.length
  const base = `/admin/site/live-events/${event.id}/photos`

  const addPhotos = async (files: File[]) => {
    setBusy("add")
    const failed: string[] = []
    let added = 0
    for (const file of files.slice(0, room)) {
      const problem =
        file.size > MAX_COVER_BYTES ? "That photo is too large. Cover photos can be up to 8 MB." : await checkSiteFile(file, "image")
      if (problem) {
        failed.push(`${file.name}: ${problem}`)
        continue
      }
      const form = new FormData()
      form.append("photo", file)
      try {
        onChange(await siteFetch<SiteLiveEvent>(authFetch, base, { method: "POST", body: form }))
        added++
      } catch (err) {
        failed.push(`${file.name}: ${err instanceof Error ? err.message : "The upload failed."}`)
      }
    }
    setBusy(null)
    if (files.length > room) failed.push(`Only ${MAX_COVER_PHOTOS} cover photos fit, so ${files.length - room} weren’t added. The gallery below can hold many more.`)
    if (added > 0) toast({ title: added === 1 ? "Cover photo added" : `${added} cover photos added`, description: "The website updates within a few minutes." })
    if (failed.length > 0) toast({ title: "Some photos couldn’t be added", description: failed.join("\n"), variant: "destructive" })
  }

  const makeMain = async (photoId: string) => {
    setBusy(photoId)
    try {
      const order = [photoId, ...photos.map((p) => p.id).filter((id) => id !== photoId)]
      onChange(await siteFetch<SiteLiveEvent>(authFetch, `${base}/order`, { method: "PUT", ...jsonBody({ order }) }))
      toast({ title: "Main cover changed", description: "The website updates within a few minutes." })
    } catch (err) {
      toast({ title: "Couldn’t change the cover", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setBusy(null)
    }
  }

  const remove = async (photoId: string) => {
    if (!window.confirm("Remove this cover photo from the event?")) return
    setBusy(photoId)
    try {
      onChange(await siteFetch<SiteLiveEvent>(authFetch, `${base}/${photoId}`, { method: "DELETE" }))
      toast({ title: "Cover photo removed", description: "The website updates within a few minutes." })
    } catch (err) {
      toast({ title: "Couldn’t remove the photo", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setBusy(null)
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-4 space-y-0">
        <div className="max-w-2xl space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <CardTitle>Cover photos</CardTitle>
            <Badge variant={room <= 0 ? "secondary" : "outline"}>
              {photos.length} of {MAX_COVER_PHOTOS}
            </Badge>
          </div>
          <CardDescription>
            The few photos at the very top of the event’s page. The main cover is also the photo on its tile. For lots of photos, use the
            gallery below.
            {event.recurrence_summary && " Changes here carry forward to the next dates of this event."}
          </CardDescription>
        </div>
        <Button disabled={busy !== null || room <= 0} onClick={() => input.current?.click()}>
          {busy === "add" ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Upload className="mr-2 h-4 w-4" />}
          Add cover photos
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
      <CardContent className="space-y-4">
        {room <= 0 && (
          <p className="rounded-md border bg-muted/50 p-3 text-sm">
            This event has all {MAX_COVER_PHOTOS} cover photos. To add a different one, remove one first. To add more photos to the event, use
            the gallery below.
          </p>
        )}
        {photos.length === 0 ? (
          <p className="text-sm text-muted-foreground">No cover photos. The event shows a placeholder until one is added.</p>
        ) : (
          <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {photos.map((photo, index) => (
              <li key={photo.id} className="space-y-2 rounded-lg border p-2">
                <div className="relative">
                  <Thumb photo={coverThumb(photo)} className="aspect-[4/3] w-full" />
                  {index === 0 && (
                    <Badge className="absolute left-2 top-2" variant="secondary">
                      <Star className="mr-1 h-3 w-3" /> Main cover
                    </Badge>
                  )}
                  {busy === photo.id && (
                    <div className="absolute inset-0 flex items-center justify-center rounded-md bg-background/60">
                      <Loader2 className="h-5 w-5 animate-spin" />
                    </div>
                  )}
                </div>
                <div className="flex items-center justify-end gap-1">
                  {index > 0 && (
                    <Button variant="ghost" size="sm" disabled={busy !== null} onClick={() => void makeMain(photo.id)}>
                      <Star className="mr-1.5 h-3.5 w-3.5" /> Make main cover
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    aria-label="Remove cover photo"
                    disabled={busy !== null}
                    onClick={() => void remove(photo.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
