"use client"

import { forwardRef, useImperativeHandle, useMemo, useRef } from "react"

import { buildQRDrawing, LOGO_RATIO, LOGO_SRC, MODULE_COLOR } from "@/lib/qr-geometry"
import { cn } from "@/lib/utils"

export interface SfluvQRCodeHandle {
  /** Rasterises the code and saves it. Kept as the previous library's shape so
   *  existing "Save QR" buttons keep working. */
  download: (format: "png", filename: string) => Promise<void>
}

interface SfluvQRCodeProps {
  /** Nullable so callers with an in-flight address do not have to guard;
   *  an empty or unencodable value renders nothing. */
  value: string | null | undefined
  /** Edge length in px. Omit to fill the container, which is the usual case. */
  size?: number
  /** The white plate with the brand hairline, as the wallet sheets show it.
   *  Off for the printed card, which sits on the card's own white. */
  framed?: boolean
  className?: string
  /** Exported PNG edge length. Generous by default — these get printed. */
  exportSize?: number
}

/**
 * The one QR code in the product.
 *
 * Every surface — printed volunteer cards, wallet receive sheets, merchant
 * codes — renders through this, so they cannot drift apart. Geometry lives in
 * lib/qr-geometry.js, shared with the preview generator and mirroring the
 * mobile app's renderer.
 *
 * The mark is a sibling <img> rather than an <image> inside the SVG. Keeping it
 * out means the SVG has no external references, so it rasterises cleanly for
 * download and for the PDF export; the mark is composited on afterwards.
 */
export const SfluvQRCode = forwardRef<SfluvQRCodeHandle, SfluvQRCodeProps>(function SfluvQRCode(
  { value, size, framed = true, className, exportSize = 1024 },
  ref,
) {
  const svgRef = useRef<SVGSVGElement | null>(null)
  const drawing = useMemo(() => {
    if (!value) return null
    try {
      return buildQRDrawing(value)
    } catch {
      // An unencodable value renders nothing rather than throwing out the page.
      return null
    }
  }, [value])

  useImperativeHandle(ref, () => ({
    async download(_format, filename) {
      const svg = svgRef.current
      if (!svg) return

      // Serialise with explicit dimensions — some browsers will not rasterise
      // an SVG that only carries a viewBox.
      const clone = svg.cloneNode(true) as SVGSVGElement
      clone.setAttribute("width", String(exportSize))
      clone.setAttribute("height", String(exportSize))
      const markup = new XMLSerializer().serializeToString(clone)

      const canvas = document.createElement("canvas")
      canvas.width = canvas.height = exportSize
      const ctx = canvas.getContext("2d")
      if (!ctx) return
      // Always on white: a transparent QR over a dark viewer does not scan.
      ctx.fillStyle = "#ffffff"
      ctx.fillRect(0, 0, exportSize, exportSize)

      const codeImage = new Image()
      codeImage.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(markup)}`
      await codeImage.decode()
      ctx.drawImage(codeImage, 0, 0, exportSize, exportSize)

      const mark = new Image()
      mark.src = LOGO_SRC
      await mark.decode()
      const markSize = exportSize * LOGO_RATIO
      const offset = (exportSize - markSize) / 2
      ctx.drawImage(mark, offset, offset, markSize, markSize)

      const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/png"))
      if (!blob) return
      const url = URL.createObjectURL(blob)
      const link = document.createElement("a")
      link.href = url
      link.download = `${filename}.png`
      link.click()
      URL.revokeObjectURL(url)
    },
  }))

  if (!drawing) return null

  return (
    <div
      className={cn(
        "relative",
        framed && "rounded-2xl border border-primary/25 bg-white p-3.5 shadow-sm",
        className,
      )}
      style={size ? { width: size } : undefined}
    >
      <div className="relative aspect-square w-full">
        <svg
          ref={svgRef}
          viewBox={`0 0 ${drawing.units} ${drawing.units}`}
          width="100%"
          height="100%"
          xmlns="http://www.w3.org/2000/svg"
          style={{ display: "block" }}
        >
          <rect width={drawing.units} height={drawing.units} fill="#ffffff" />
          {drawing.eyes.map((eye, index) => (
            <rect
              key={`eye-${index}`}
              x={eye.x}
              y={eye.y}
              width={eye.width}
              height={eye.height}
              rx={eye.rx}
              ry={eye.rx}
              fill={eye.fill}
              stroke={eye.stroke}
              strokeWidth={eye.strokeWidth}
            />
          ))}
          {drawing.dots.map((dot) => (
            <circle key={`${dot.cx}:${dot.cy}`} cx={dot.cx} cy={dot.cy} r={dot.r} fill={MODULE_COLOR} />
          ))}
        </svg>
        {/* eslint-disable-next-line @next/next/no-img-element -- must rasterise for print/export */}
        <img
          src={LOGO_SRC}
          alt=""
          aria-hidden="true"
          style={{
            position: "absolute",
            width: drawing.logo.sizePercent,
            height: drawing.logo.sizePercent,
            left: drawing.logo.leftPercent,
            top: drawing.logo.topPercent,
            objectFit: "contain",
          }}
        />
      </div>
    </div>
  )
})
