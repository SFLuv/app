"use client"

import { useCallback, useEffect, useRef, useState } from "react"
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
import {
  clampLogoCrop,
  cropLogoToPngBlob,
  initialLogoCrop,
  logoRenderedSize,
  readLogoFile,
  uploadLocationLogo,
  zoomLogoCrop,
  LOGO_FILE_INPUT_ACCEPT,
  LOGO_VIEWPORT,
  type LogoCrop,
} from "@/lib/location-logo"
import type { AuthedLocation } from "@/types/location"

/**
 * The crop maths lives in lib/location-logo, shared with the Location Approval
 * Form's first step. Two implementations of it would mean a logo that framed
 * one way when the location was applied for and another way when it was
 * replaced here.
 */
const VIEWPORT = LOGO_VIEWPORT
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
  const [crop, setCrop] = useState<LogoCrop>({ zoom: 1, x: 0, y: 0 })
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

  const clampOffset = useCallback((next: LogoCrop): LogoCrop => clampLogoCrop(image, next), [image])

  const openFilePicker = () => {
    setError("")
    fileInputRef.current?.click()
  }

  const handleFile = async (file: File | undefined) => {
    if (!file) return
    setError("")

    try {
      const element = await readLogoFile(file)
      setObjectUrl((previous) => {
        if (previous) URL.revokeObjectURL(previous)
        return element.src
      })
      setImage(element)
      setCrop(initialLogoCrop(element))
      setEditing(true)
    } catch (readError) {
      setError(readError instanceof Error ? readError.message : "That image could not be read.")
    }
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

  const setZoom = (zoom: number) => {
    setCrop((current) => zoomLogoCrop(image, current, zoom))
  }

  const { width: renderedWidth, height: renderedHeight } = logoRenderedSize(image, crop)

  const save = async () => {
    setSaving(true)
    setError("")
    try {
      const blob = await cropLogoToPngBlob(image, crop)
      if (!blob) throw new Error("The crop could not be prepared.")

      setLocalIconUrl(await uploadLocationLogo(authFetch, location.id, blob))
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
        accept={LOGO_FILE_INPUT_ACCEPT}
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
                    // See location-logo-field: without max-w-none the preflight
                    // img rule clamps the width and the preview stretches.
                    className="pointer-events-none absolute max-w-none select-none"
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
  crop: LogoCrop
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
        className="absolute left-0 top-0 max-w-none select-none"
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
  crop: LogoCrop
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
