"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { ImagePlus, Loader2, Trash2, ZoomIn, ZoomOut } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import { MerchantMapPin } from "@/components/locations/merchant-pin"
import {
  ICON_FACE,
  PIN_EDGE_COLOR,
  PIN_GLYPH_RADIUS,
  PIN_RING_RADIUS,
  PIN_HEAD_CENTRE,
  PIN_PATH,
  PIN_VIEWBOX_HEIGHT,
  PIN_VIEWBOX_WIDTH,
  PIN_OPEN_COLOR,
} from "@/lib/merchant-icon"
import type { AuthedLocation } from "@/types/location"

/** Edge length of the square the crop is saved at. */
const OUTPUT_SIZE = 512
/** Edge length of the on-screen crop window. */
const VIEWPORT = 260
const MAX_UPLOAD_BYTES = 2 * 1024 * 1024
/**
 * Bigger than the map draws it. The map's pin is deliberately small; a preview
 * at that size would not show a merchant whether their logo survives the crop.
 */
const PREVIEW_PIN_WIDTH = 46

interface MerchantIconCardProps {
  location: AuthedLocation
  /** Called after a successful save or removal so the caller can refetch. */
  onSaved?: () => void | Promise<void>
}

interface CropState {
  /** Multiplier on top of the scale that just covers the viewport. */
  zoom: number
  /** Top-left of the scaled image relative to the viewport, in CSS pixels. */
  x: number
  y: number
}

/**
 * A merchant's map icon: upload, square crop, and a preview of the actual pin.
 *
 * The crop is enforced rather than requested. A map pin is a circle, and an
 * uploader that accepts any aspect ratio only moves the decision to the
 * renderer, which then has to guess which part of a wide photo is the logo.
 * Cropping here means the merchant chooses, and sees exactly what they chose.
 */
export function MerchantIconCard({ location, onSaved }: MerchantIconCardProps) {
  const { authFetch, refreshUserRecord } = useApp()
  const { toast } = useToast()

  const [editing, setEditing] = useState(false)
  const [image, setImage] = useState<HTMLImageElement | null>(null)
  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [crop, setCrop] = useState<CropState>({ zoom: 1, x: 0, y: 0 })
  const [saving, setSaving] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [error, setError] = useState("")
  // Set on a successful upload so the new icon shows immediately, before the
  // user record round-trips.
  const [localIconUrl, setLocalIconUrl] = useState<string | null>(null)

  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const dragRef = useRef<{ pointerId: number; startX: number; startY: number; originX: number; originY: number } | null>(null)

  const iconUrl = localIconUrl ?? location.icon_url ?? ""

  useEffect(() => {
    setLocalIconUrl(null)
  }, [location.id, location.icon_url])

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
    return VIEWPORT / Math.min(image.naturalWidth, image.naturalHeight)
  }, [image])

  const clampOffset = useCallback(
    (next: CropState): CropState => {
      if (!image) return next
      const width = image.naturalWidth * baseScale * next.zoom
      const height = image.naturalHeight * baseScale * next.zoom
      return {
        zoom: next.zoom,
        // The image must always cover the window — never let a gap open at an
        // edge, or the saved crop would contain transparent bands.
        x: Math.min(0, Math.max(VIEWPORT - width, next.x)),
        y: Math.min(0, Math.max(VIEWPORT - height, next.y)),
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
    if (file.size > MAX_UPLOAD_BYTES * 4) {
      // Generous: the crop is re-encoded before upload, so a large original is
      // fine — this only rejects files big enough to stall the browser.
      setError("That image is too large. Try one under 8 MB.")
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
      const scale = VIEWPORT / Math.min(element.naturalWidth, element.naturalHeight)
      setCrop({
        zoom: 1,
        x: (VIEWPORT - element.naturalWidth * scale) / 2,
        y: (VIEWPORT - element.naturalHeight * scale) / 2,
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
      const centre = VIEWPORT / 2
      return clampOffset({
        zoom,
        x: centre - (centre - current.x) * ratio,
        y: centre - (centre - current.y) * ratio,
      })
    })
  }

  const renderedWidth = image ? image.naturalWidth * baseScale * crop.zoom : 0
  const renderedHeight = image ? image.naturalHeight * baseScale * crop.zoom : 0

  const cropToBlob = async (): Promise<Blob | null> => {
    if (!image) return null

    const canvas = document.createElement("canvas")
    canvas.width = OUTPUT_SIZE
    canvas.height = OUTPUT_SIZE
    const context = canvas.getContext("2d")
    if (!context) return null

    // Map the window back onto the source image: the visible square starts at
    // the negated offset and is one window wide at the current scale.
    const scale = baseScale * crop.zoom
    const sourceSize = VIEWPORT / scale
    context.imageSmoothingQuality = "high"
    context.drawImage(
      image,
      -crop.x / scale,
      -crop.y / scale,
      sourceSize,
      sourceSize,
      0,
      0,
      OUTPUT_SIZE,
      OUTPUT_SIZE,
    )

    return await new Promise((resolve) => canvas.toBlob(resolve, "image/png"))
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
      body.append("icon", blob, "icon.png")

      const response = await authFetch(`/locations/${location.id}/icon`, { method: "POST", body })
      if (!response.ok) {
        const payload = await response.json().catch(() => null)
        throw new Error(payload?.error || "Unable to save your icon.")
      }
      const payload = (await response.json()) as { icon_url?: string }

      setLocalIconUrl(payload.icon_url ?? null)
      setEditing(false)
      setImage(null)
      toast({ title: "Icon saved", description: "Your map pin now shows your icon." })
      await refreshUserRecord()
      await onSaved?.()
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "Unable to save your icon.")
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    setRemoving(true)
    setError("")
    try {
      const response = await authFetch(`/locations/${location.id}/icon`, { method: "DELETE" })
      if (!response.ok && response.status !== 204) {
        const payload = await response.json().catch(() => null)
        throw new Error(payload?.error || "Unable to remove your icon.")
      }
      setLocalIconUrl("")
      toast({ title: "Icon removed", description: "Your pin is back to your generated initials." })
      await refreshUserRecord()
      await onSaved?.()
    } catch (removeError) {
      setError(removeError instanceof Error ? removeError.message : "Unable to remove your icon.")
    } finally {
      setRemoving(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-5">
        <div className="flex flex-col items-center gap-1.5">
          <MerchantMapPin name={location.name} iconUrl={iconUrl} state="open" width={PREVIEW_PIN_WIDTH} />
          <span className="text-[11px] text-muted-foreground">Open</span>
        </div>
        <div className="flex flex-col items-center gap-1.5">
          <MerchantMapPin name={location.name} iconUrl={iconUrl} state="closed" width={PREVIEW_PIN_WIDTH} />
          <span className="text-[11px] text-muted-foreground">Closed</span>
        </div>

        <div className="min-w-[12rem] flex-1 space-y-2">
          <p className="text-sm text-muted-foreground">
            {iconUrl
              ? "This is how your pin appears on the merchant map."
              : "No icon yet — your pin shows your initials in SFLuv colours. Upload a square logo to replace it."}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" size="sm" onClick={openFilePicker}>
              <ImagePlus className="mr-2 h-4 w-4" />
              {iconUrl ? "Replace icon" : "Upload icon"}
            </Button>
            {iconUrl ? (
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
            <DialogTitle>Crop your icon</DialogTitle>
            <DialogDescription>
              Drag to reposition and zoom to frame. Icons are square — what you see in the circle is what appears on your pin.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col items-center gap-5 sm:flex-row sm:items-start">
            <div className="space-y-3">
              <div
                className="relative cursor-grab touch-none overflow-hidden rounded-xl border bg-muted active:cursor-grabbing"
                style={{ width: VIEWPORT, height: VIEWPORT }}
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
                    className="pointer-events-none absolute select-none"
                    style={{
                      width: renderedWidth,
                      height: renderedHeight,
                      transform: `translate(${crop.x}px, ${crop.y}px)`,
                      left: 0,
                      top: 0,
                    }}
                  />
                ) : null}
                {/* The circle the pin clips to. One element: the huge outward
                    shadow dims everything outside it, and the border draws the
                    ring — so the merchant frames against the real shape rather
                    than a square they will not get. */}
                <div className="pointer-events-none absolute inset-[6%] rounded-full border-2 border-white/80 shadow-[0_0_0_9999px_rgba(11,48,59,0.35)]" />
              </div>

              <div className="flex items-center gap-3">
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

            <div className="flex flex-col items-center gap-4 pt-1">
              <div className="flex flex-col items-center gap-1.5">
                <div className="h-16 w-16 overflow-hidden rounded-2xl border shadow-sm">
                  <CropPreview objectUrl={objectUrl} crop={crop} width={renderedWidth} height={renderedHeight} size={64} />
                </div>
                <span className="text-[11px] text-muted-foreground">Icon</span>
              </div>
              <div className="flex flex-col items-center gap-1.5">
                <PinPreview objectUrl={objectUrl} crop={crop} width={renderedWidth} height={renderedHeight} />
                <span className="text-[11px] text-muted-foreground">On the map</span>
              </div>
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
              Save icon
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

/**
 * The current crop at an arbitrary size.
 *
 * Rendered by scaling the whole crop window rather than recomputing offsets, so
 * the preview cannot drift out of agreement with what will be saved.
 */
function CropPreview({
  objectUrl,
  crop,
  width,
  height,
  size,
}: {
  objectUrl: string | null
  crop: CropState
  width: number
  height: number
  size: number
}) {
  if (!objectUrl) return <div className="h-full w-full bg-muted" />

  return (
    <div
      className="relative overflow-hidden"
      style={{ width: VIEWPORT, height: VIEWPORT, transform: `scale(${size / VIEWPORT})`, transformOrigin: "top left" }}
    >
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={objectUrl}
        alt=""
        draggable={false}
        className="absolute left-0 top-0 select-none"
        style={{ width, height, transform: `translate(${crop.x}px, ${crop.y}px)` }}
      />
    </div>
  )
}

/** The live crop inside the real pin silhouette, at preview size. */
function PinPreview({
  objectUrl,
  crop,
  width,
  height
}: {
  objectUrl: string | null
  crop: CropState
  width: number
  height: number
}) {
  const pinWidth = PREVIEW_PIN_WIDTH
  const pinHeight = Math.round((pinWidth * PIN_VIEWBOX_HEIGHT) / PIN_VIEWBOX_WIDTH)
  const unit = pinWidth / PIN_VIEWBOX_WIDTH
  const glyphSize = Math.round(PIN_GLYPH_RADIUS * 2 * unit)

  return (
    <div className="relative" style={{ width: pinWidth, height: pinHeight }}>
      <svg
        viewBox={`0 0 ${PIN_VIEWBOX_WIDTH} ${PIN_VIEWBOX_HEIGHT}`}
        width={pinWidth}
        height={pinHeight}
        className="absolute inset-0"
        style={{ filter: "drop-shadow(0 1px 2px rgba(11, 48, 59, 0.3))" }}
        aria-hidden
      >
        <path d={PIN_PATH} fill={ICON_FACE} stroke={PIN_EDGE_COLOR} strokeWidth={0.6} />
        <circle cx={PIN_HEAD_CENTRE.x} cy={PIN_HEAD_CENTRE.y} r={PIN_RING_RADIUS} fill={PIN_OPEN_COLOR} />
      </svg>
      <div
        className="absolute overflow-hidden rounded-full"
        style={{
          width: glyphSize,
          height: glyphSize,
          left: PIN_HEAD_CENTRE.x * unit - glyphSize / 2,
          top: PIN_HEAD_CENTRE.y * unit - glyphSize / 2,
          backgroundColor: ICON_FACE
        }}
      >
        <CropPreview objectUrl={objectUrl} crop={crop} width={width} height={height} size={glyphSize} />
      </div>
    </div>
  )
}
