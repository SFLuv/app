"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { ImagePlus, Loader2, Trash2, ZoomIn, ZoomOut } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import type { AuthedLocation } from "@/types/location"

/**
 * Saved size. 16:9 because that is the shape the listing card reserves, and
 * saving at the display aspect means no viewer has to crop again.
 */
const OUTPUT_WIDTH = 1280
const OUTPUT_HEIGHT = 720
/** On-screen crop window, same aspect as the output. */
const VIEWPORT_WIDTH = 320
const VIEWPORT_HEIGHT = 180
/**
 * Matches the server cap. A 1280x720 JPEG at quality 0.85 lands two orders of
 * magnitude below this; the limit is only here to fail early and clearly.
 */
const MAX_UPLOAD_BYTES = 8 * 1024 * 1024
/** Original-file ceiling, before the crop is re-encoded. */
const MAX_SOURCE_BYTES = 24 * 1024 * 1024
const JPEG_QUALITY = 0.85

interface MerchantPhotoCardProps {
  location: AuthedLocation
  /** Called after a successful save or removal so the caller can refetch. */
  onSaved?: () => void | Promise<void>
}

interface CropState {
  /** Multiplier on top of the scale that just covers the window. */
  zoom: number
  /** Top-left of the scaled image relative to the window, in CSS pixels. */
  x: number
  y: number
}

/**
 * A merchant's storefront photo: upload, framing crop, and a preview.
 *
 * This is a different picture from the map icon and is uploaded separately. The
 * icon is a mark a few pixels wide inside a pin and wants a logo; this is a
 * photograph of the place shown at card width. Deriving one from the other in
 * either direction gives a bad result — a logo letterboxed across a banner, or
 * a shopfront squeezed into a circle — so merchants set them independently.
 *
 * The crop is enforced rather than requested, for the same reason as the icon:
 * an uploader that takes any aspect only moves the decision to the renderer,
 * which then has to guess which part of the picture matters.
 */
export function MerchantPhotoCard({ location, onSaved }: MerchantPhotoCardProps) {
  const { authFetch, refreshUserRecord } = useApp()
  const { toast } = useToast()

  const [editing, setEditing] = useState(false)
  const [image, setImage] = useState<HTMLImageElement | null>(null)
  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [crop, setCrop] = useState<CropState>({ zoom: 1, x: 0, y: 0 })
  const [saving, setSaving] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [error, setError] = useState("")
  // Set on a successful upload so the new photo shows immediately, before the
  // user record round-trips.
  const [localPhotoUrl, setLocalPhotoUrl] = useState<string | null>(null)

  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const dragRef = useRef<{ pointerId: number; startX: number; startY: number; originX: number; originY: number } | null>(null)

  const photoUrl = localPhotoUrl ?? location.photo_url ?? ""

  useEffect(() => {
    setLocalPhotoUrl(null)
  }, [location.id, location.photo_url])

  // Revoke on unmount and on replacement: an object URL pins the whole decoded
  // file in memory until it is released.
  useEffect(() => {
    return () => {
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [objectUrl])

  /** Scale at which the image exactly covers the crop window. */
  const baseScale = useMemo(() => {
    if (!image) return 1
    return Math.max(VIEWPORT_WIDTH / image.naturalWidth, VIEWPORT_HEIGHT / image.naturalHeight)
  }, [image])

  const clampOffset = useCallback(
    (next: CropState): CropState => {
      if (!image) return next
      const width = image.naturalWidth * baseScale * next.zoom
      const height = image.naturalHeight * baseScale * next.zoom
      return {
        zoom: next.zoom,
        // The image must always cover the window — never let a gap open at an
        // edge, or the saved crop would contain empty bands.
        x: Math.min(0, Math.max(VIEWPORT_WIDTH - width, next.x)),
        y: Math.min(0, Math.max(VIEWPORT_HEIGHT - height, next.y)),
      }
    },
    [baseScale, image],
  )

  const openFilePicker = () => {
    setError("")
    fileInputRef.current?.click()
  }

  const handleFile = async (file: File | undefined) => {
    if (!file) return
    setError("")

    if (!/^image\/(png|jpeg|webp|gif)$/.test(file.type)) {
      setError("Choose a PNG, JPEG, or WebP image.")
      return
    }
    if (file.size > MAX_SOURCE_BYTES) {
      // The crop is re-encoded before upload, so a large original is fine —
      // this only rejects files big enough to stall the browser.
      setError("That image is too large. Try one under 24 MB.")
      return
    }

    const url = URL.createObjectURL(file)
    const element = new window.Image()
    element.onload = () => {
      setObjectUrl((previous) => {
        if (previous) URL.revokeObjectURL(previous)
        return url
      })
      setImage(element)
      // Start centred at the cover scale, which is the crop most people want.
      const scale = Math.max(VIEWPORT_WIDTH / element.naturalWidth, VIEWPORT_HEIGHT / element.naturalHeight)
      setCrop({
        zoom: 1,
        x: (VIEWPORT_WIDTH - element.naturalWidth * scale) / 2,
        y: (VIEWPORT_HEIGHT - element.naturalHeight * scale) / 2,
      })
      setEditing(true)
    }
    element.onerror = () => {
      URL.revokeObjectURL(url)
      setError("That image could not be read.")
    }
    element.src = url
  }

  const onPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!image) return
    event.currentTarget.setPointerCapture(event.pointerId)
    dragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      originX: crop.x,
      originY: crop.y,
    }
  }

  const onPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    setCrop((current) =>
      clampOffset({
        zoom: current.zoom,
        x: drag.originX + (event.clientX - drag.startX),
        y: drag.originY + (event.clientY - drag.startY),
      }),
    )
  }

  const endDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    if (dragRef.current?.pointerId === event.pointerId) {
      dragRef.current = null
    }
  }

  /** Zoom about the centre of the window, so the subject does not drift away. */
  const setZoom = (zoom: number) => {
    setCrop((current) => {
      const ratio = zoom / current.zoom
      return clampOffset({
        zoom,
        x: VIEWPORT_WIDTH / 2 - (VIEWPORT_WIDTH / 2 - current.x) * ratio,
        y: VIEWPORT_HEIGHT / 2 - (VIEWPORT_HEIGHT / 2 - current.y) * ratio,
      })
    })
  }

  const renderedWidth = image ? image.naturalWidth * baseScale * crop.zoom : 0
  const renderedHeight = image ? image.naturalHeight * baseScale * crop.zoom : 0

  const cropToBlob = async (): Promise<Blob | null> => {
    if (!image) return null

    const canvas = document.createElement("canvas")
    canvas.width = OUTPUT_WIDTH
    canvas.height = OUTPUT_HEIGHT
    const context = canvas.getContext("2d")
    if (!context) return null

    // Map the window back onto the source image: the visible rectangle starts
    // at the negated offset and is one window wide at the current scale.
    const scale = baseScale * crop.zoom
    context.imageSmoothingQuality = "high"
    context.drawImage(
      image,
      -crop.x / scale,
      -crop.y / scale,
      VIEWPORT_WIDTH / scale,
      VIEWPORT_HEIGHT / scale,
      0,
      0,
      OUTPUT_WIDTH,
      OUTPUT_HEIGHT,
    )

    // JPEG rather than the icon's PNG: this is a photograph, where PNG would be
    // an order of magnitude larger for no visible gain.
    return await new Promise((resolve) => canvas.toBlob(resolve, "image/jpeg", JPEG_QUALITY))
  }

  const save = async () => {
    setSaving(true)
    setError("")
    try {
      const blob = await cropToBlob()
      if (!blob) throw new Error("The crop could not be prepared.")
      if (blob.size > MAX_UPLOAD_BYTES) {
        throw new Error("That crop is too detailed to upload. Try a simpler image.")
      }

      const body = new FormData()
      body.append("photo", blob, "photo.jpg")

      const response = await authFetch(`/locations/${location.id}/photo`, { method: "POST", body })
      if (!response.ok) {
        const payload = await response.json().catch(() => null)
        throw new Error(payload?.error || "Unable to save your photo.")
      }
      const payload = (await response.json()) as { photo_url?: string }

      setLocalPhotoUrl(payload.photo_url ?? null)
      setEditing(false)
      setImage(null)
      toast({ title: "Photo saved", description: "Your listing now shows your photo." })
      await refreshUserRecord()
      await onSaved?.()
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "Unable to save your photo.")
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    setRemoving(true)
    setError("")
    try {
      const response = await authFetch(`/locations/${location.id}/photo`, { method: "DELETE" })
      if (!response.ok && response.status !== 204) {
        const payload = await response.json().catch(() => null)
        throw new Error(payload?.error || "Unable to remove your photo.")
      }
      setLocalPhotoUrl("")
      toast({ title: "Photo removed", description: "Your listing no longer shows a photo." })
      await refreshUserRecord()
      await onSaved?.()
    } catch (removeError) {
      setError(removeError instanceof Error ? removeError.message : "Unable to remove your photo.")
    } finally {
      setRemoving(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-5">
        <div
          className="flex shrink-0 items-center justify-center overflow-hidden rounded-xl border bg-muted"
          style={{ width: 176, height: 99 }}
        >
          {photoUrl ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={photoUrl} alt={`${location.name} storefront`} className="h-full w-full object-cover" />
          ) : (
            <ImagePlus className="h-6 w-6 text-muted-foreground" />
          )}
        </div>

        <div className="min-w-[12rem] flex-1 space-y-2">
          <p className="text-sm text-muted-foreground">
            {photoUrl
              ? "This photo appears on your listing."
              : "No photo yet — add a picture of your space so people know it when they arrive."}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" size="sm" onClick={openFilePicker}>
              <ImagePlus className="mr-2 h-4 w-4" />
              {photoUrl ? "Replace photo" : "Upload photo"}
            </Button>
            {photoUrl ? (
              <Button type="button" variant="ghost" size="sm" onClick={remove} disabled={removing}>
                {removing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
                Remove
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      {error !== "" && !editing ? <p className="text-sm text-destructive">{error}</p> : null}

      <input
        ref={fileInputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={(event) => {
          void handleFile(event.target.files?.[0])
          // Reset so picking the same file twice still fires a change.
          event.target.value = ""
        }}
      />

      <Dialog
        open={editing}
        onOpenChange={(open) => {
          if (!open && !saving) {
            setEditing(false)
            setImage(null)
          }
        }}
      >
        <DialogContent className="sm:max-w-[560px]">
          <DialogHeader>
            <DialogTitle>Frame your photo</DialogTitle>
            <DialogDescription>
              Drag to reposition and zoom to frame. What you see in the box is what appears on your listing.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div
              className="relative mx-auto cursor-grab touch-none overflow-hidden rounded-xl border bg-muted active:cursor-grabbing"
              style={{ width: VIEWPORT_WIDTH, height: VIEWPORT_HEIGHT }}
              onPointerDown={onPointerDown}
              onPointerMove={onPointerMove}
              onPointerUp={endDrag}
              onPointerCancel={endDrag}
            >
              {objectUrl && image ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={objectUrl}
                  alt=""
                  draggable={false}
                  className="pointer-events-none absolute left-0 top-0 select-none"
                  style={{
                    width: renderedWidth,
                    height: renderedHeight,
                    transform: `translate(${crop.x}px, ${crop.y}px)`,
                  }}
                />
              ) : null}
            </div>

            <div className="mx-auto flex items-center gap-3" style={{ width: VIEWPORT_WIDTH }}>
              <ZoomOut className="h-4 w-4 shrink-0 text-muted-foreground" />
              <input
                type="range"
                aria-label="Zoom"
                min={1}
                max={4}
                step={0.01}
                value={crop.zoom}
                onChange={(event) => setZoom(Number(event.target.value))}
                className="h-1.5 w-full cursor-pointer appearance-none rounded-full bg-secondary accent-[#eb6c6c]"
              />
              <ZoomIn className="h-4 w-4 shrink-0 text-muted-foreground" />
            </div>
          </div>

          {error !== "" ? <p className="text-sm text-destructive">{error}</p> : null}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setEditing(false)
                setImage(null)
              }}
              disabled={saving}
            >
              Cancel
            </Button>
            <Button type="button" className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={save} disabled={saving}>
              {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Save photo
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
