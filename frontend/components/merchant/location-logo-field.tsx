"use client"

import { useEffect, useRef, useState } from "react"
import { ImagePlus, Info, Trash2, ZoomIn, ZoomOut } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { MerchantMapPin } from "@/components/locations/merchant-pin"
import {
  clampLogoCrop,
  cropLogoToPngBlob,
  initialLogoCrop,
  logoRenderedSize,
  readLogoFile,
  zoomLogoCrop,
  LOGO_FILE_INPUT_ACCEPT,
  LOGO_VIEWPORT,
  type LogoCrop,
} from "@/lib/location-logo"

/** Bigger than the map draws it, so the merchant can see what survives the crop. */
const PREVIEW_PIN_WIDTH = 46

/**
 * The optional logo on the Location Approval Form's first step.
 *
 * It uploads nothing. The listing does not exist while the form is being filled
 * in, and a logo belongs to a location — there is no merchant-level logo to
 * attach it to in the meantime, and inventing one would be exactly the
 * user-bound store this is meant not to be. So the crop is held here as a PNG
 * blob and the caller posts it to `/locations/{id}/icon` once the location has
 * an id.
 *
 * That also means a cancelled or abandoned application leaves no orphaned
 * image behind, and two locations applied for in one sitting each carry their
 * own logo rather than the second overwriting the first.
 */
export function LocationLogoField({
  locationName,
  value,
  onChange,
  disabled = false,
}: {
  /** Only for the pin preview — the pin falls back to initials without a logo. */
  locationName: string
  value: Blob | null
  onChange: (logo: Blob | null) => void
  disabled?: boolean
}) {
  const [editing, setEditing] = useState(false)
  const [image, setImage] = useState<HTMLImageElement | null>(null)
  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [crop, setCrop] = useState<LogoCrop>({ zoom: 1, x: 0, y: 0 })
  const [error, setError] = useState("")
  /** A preview of the accepted crop, which is all the caller's blob can show. */
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)

  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const dragRef = useRef<{
    pointerId: number
    startX: number
    startY: number
    originX: number
    originY: number
  } | null>(null)

  // An object URL pins the whole decoded file in memory until it is released,
  // and this component can go through several of them in one sitting.
  useEffect(() => {
    return () => {
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [objectUrl])

  useEffect(() => {
    if (!value) {
      setPreviewUrl(null)
      return
    }
    const url = URL.createObjectURL(value)
    setPreviewUrl(url)
    return () => URL.revokeObjectURL(url)
  }, [value])

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
      clampLogoCrop(image, {
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

  const { width: renderedWidth, height: renderedHeight } = logoRenderedSize(image, crop)

  /** Zoom about the centre of the window, so the subject does not drift away. */
  const setZoomFromRange = (zoom: number) => {
    setCrop((current) => zoomLogoCrop(image, current, zoom))
  }

  const acceptCrop = async () => {
    setError("")
    try {
      const blob = await cropLogoToPngBlob(image, crop)
      if (!blob) throw new Error("The crop could not be prepared.")
      onChange(blob)
      setEditing(false)
      setImage(null)
    } catch (cropError) {
      setError(cropError instanceof Error ? cropError.message : "The crop could not be prepared.")
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5">
        <Label className="text-black dark:text-white">
          Location Logo <span className="ml-1 font-normal text-muted-foreground">(optional)</span>
        </Label>
        <TooltipProvider delayDuration={200}>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label="Each location has its own logo. Without one the pin shows your initials."
                className="text-muted-foreground transition-colors hover:text-foreground"
              >
                <Info className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent className="max-w-[16rem]">
              Each location has its own. Without one the pin shows your initials.
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>

      {/* Pin on the left, the actions squared up against it on the right. The
          buttons previously sat in a flex-1 block that centred them in whatever
          space was left, so they drifted with the width of the card instead of
          lining up with anything. */}
      <div className="flex items-center gap-4 rounded-lg border border-border/70 bg-muted/20 p-3">
        <div className="flex shrink-0 flex-col items-center gap-1">
          <MerchantMapPin
            name={locationName || "Your location"}
            iconUrl={previewUrl ?? ""}
            state="open"
            width={PREVIEW_PIN_WIDTH}
          />
          <span className="text-[10px] leading-none text-muted-foreground">On the map</span>
        </div>

        <div className="flex min-w-0 flex-1 flex-col items-start gap-1.5">
          <p className="text-xs text-muted-foreground">
            {value ? "Ready to upload with your application." : "Square works best."}
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled}
              onClick={() => {
                setError("")
                fileInputRef.current?.click()
              }}
            >
              <ImagePlus className="mr-2 h-4 w-4" />
              {value ? "Replace" : "Upload logo"}
            </Button>
            {value ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-muted-foreground"
                disabled={disabled}
                onClick={() => onChange(null)}
              >
                <Trash2 className="mr-2 h-4 w-4" />
                Remove
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      {error !== "" && !editing ? <p className="text-xs text-red-600 dark:text-red-300">{error}</p> : null}

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
          if (!open) {
            setEditing(false)
            setImage(null)
          }
        }}
      >
        <DialogContent className="sm:max-w-[420px]">
          <DialogHeader>
            <DialogTitle>Crop your logo</DialogTitle>
            <DialogDescription>Drag to reposition, zoom to frame.</DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            <div
              className="relative mx-auto cursor-grab touch-none overflow-hidden rounded-xl border bg-muted active:cursor-grabbing"
              style={{ width: LOGO_VIEWPORT, height: LOGO_VIEWPORT }}
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
                  // max-w-none is load-bearing: Tailwind's preflight sets
                  // img { max-width: 100% }, which clamps the width to the crop
                  // window while the inline height keeps growing — the image
                  // stretched vertically as it was zoomed. The canvas crop works
                  // from the same numbers as maths rather than from the DOM, so
                  // the saved logo was correct and only the preview lied, which
                  // is what put logos off-centre from what was framed.
                  className="pointer-events-none absolute left-0 top-0 max-w-none select-none"
                  style={{
                    width: renderedWidth,
                    height: renderedHeight,
                    transform: `translate(${crop.x}px, ${crop.y}px)`,
                  }}
                />
              ) : null}
              {/* The circle the pin clips to. One element: the huge outward
                  shadow dims everything outside it, and the border draws the
                  ring — so the merchant frames against the real shape rather
                  than a square they will not get. */}
              <div className="pointer-events-none absolute inset-[6%] rounded-full border-2 border-white/80 shadow-[0_0_0_9999px_rgba(11,48,59,0.35)]" />
            </div>

            <div className="mx-auto flex items-center gap-3" style={{ width: LOGO_VIEWPORT }}>
              <ZoomOut className="h-4 w-4 shrink-0 text-muted-foreground" />
              <input
                type="range"
                aria-label="Zoom"
                min={1}
                max={4}
                step={0.01}
                value={crop.zoom}
                onChange={(event) => setZoomFromRange(Number(event.target.value))}
                className="h-1.5 w-full cursor-pointer appearance-none rounded-full bg-secondary accent-[#eb6c6c]"
              />
              <ZoomIn className="h-4 w-4 shrink-0 text-muted-foreground" />
            </div>
          </div>

          {error !== "" ? <p className="text-sm text-red-600 dark:text-red-300">{error}</p> : null}

          <DialogFooter className="grid grid-cols-2 gap-2 sm:flex-row sm:justify-end sm:gap-2">
            <Button
              type="button"
              variant="outline"
              className="w-full sm:w-40"
              onClick={() => {
                setEditing(false)
                setImage(null)
              }}
            >
              Cancel
            </Button>
            <Button
              type="button"
              className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c] sm:w-40"
              onClick={() => void acceptCrop()}
            >
              Use logo
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
