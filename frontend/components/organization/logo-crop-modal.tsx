"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"

const VIEWPORT = 280 // on-screen square crop viewport (px)
const OUTPUT = 512 // exported logo size (px)

interface LogoCropModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Source image data URL to crop. */
  src: string | null
  onCropped: (dataUrl: string) => void
}

// Self-contained square crop: drag to pan, slider to zoom, exports a square
// data URL. Enforces the strict square-logo requirement regardless of the
// uploaded image's dimensions.
export function LogoCropModal({ open, onOpenChange, src, onCropped }: LogoCropModalProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const imgRef = useRef<HTMLImageElement | null>(null)
  const [zoom, setZoom] = useState(1)
  const [offset, setOffset] = useState({ x: 0, y: 0 })
  const dragRef = useRef<{ startX: number; startY: number; baseX: number; baseY: number } | null>(null)

  // Base scale fills the square viewport with the image's smaller dimension.
  const baseScale = useCallback(() => {
    const img = imgRef.current
    if (!img) return 1
    return VIEWPORT / Math.min(img.naturalWidth, img.naturalHeight)
  }, [])

  const clampOffset = useCallback((next: { x: number; y: number }, z: number) => {
    const img = imgRef.current
    if (!img) return next
    const scale = baseScale() * z
    const w = img.naturalWidth * scale
    const h = img.naturalHeight * scale
    const minX = VIEWPORT - w
    const minY = VIEWPORT - h
    return {
      x: Math.min(0, Math.max(minX, next.x)),
      y: Math.min(0, Math.max(minY, next.y)),
    }
  }, [baseScale])

  const draw = useCallback(() => {
    const canvas = canvasRef.current
    const img = imgRef.current
    if (!canvas || !img) return
    const ctx = canvas.getContext("2d")
    if (!ctx) return
    const scale = baseScale() * zoom
    ctx.clearRect(0, 0, VIEWPORT, VIEWPORT)
    ctx.drawImage(img, offset.x, offset.y, img.naturalWidth * scale, img.naturalHeight * scale)
  }, [baseScale, offset, zoom])

  useEffect(() => {
    if (!open || !src) return
    const img = new Image()
    img.onload = () => {
      imgRef.current = img
      const scale = VIEWPORT / Math.min(img.naturalWidth, img.naturalHeight)
      // Center the image initially.
      setZoom(1)
      setOffset({
        x: (VIEWPORT - img.naturalWidth * scale) / 2,
        y: (VIEWPORT - img.naturalHeight * scale) / 2,
      })
    }
    img.src = src
  }, [open, src])

  useEffect(() => {
    draw()
  }, [draw])

  const onPointerDown = (e: React.PointerEvent) => {
    ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
    dragRef.current = { startX: e.clientX, startY: e.clientY, baseX: offset.x, baseY: offset.y }
  }
  const onPointerMove = (e: React.PointerEvent) => {
    const drag = dragRef.current
    if (!drag) return
    setOffset(clampOffset({ x: drag.baseX + (e.clientX - drag.startX), y: drag.baseY + (e.clientY - drag.startY) }, zoom))
  }
  const onPointerUp = () => {
    dragRef.current = null
  }

  const handleZoom = (nextZoom: number) => {
    // Zoom around the viewport center.
    const img = imgRef.current
    if (!img) return
    const prevScale = baseScale() * zoom
    const nextScale = baseScale() * nextZoom
    const cx = VIEWPORT / 2
    const cy = VIEWPORT / 2
    const imgCx = (cx - offset.x) / prevScale
    const imgCy = (cy - offset.y) / prevScale
    const next = { x: cx - imgCx * nextScale, y: cy - imgCy * nextScale }
    setZoom(nextZoom)
    setOffset(clampOffset(next, nextZoom))
  }

  const exportCrop = () => {
    const img = imgRef.current
    if (!img) return
    const out = document.createElement("canvas")
    out.width = OUTPUT
    out.height = OUTPUT
    const ctx = out.getContext("2d")
    if (!ctx) return
    const scale = baseScale() * zoom
    // Viewport (0,0,VIEWPORT,VIEWPORT) in image coordinates:
    const sx = -offset.x / scale
    const sy = -offset.y / scale
    const sSize = VIEWPORT / scale
    ctx.drawImage(img, sx, sy, sSize, sSize, 0, 0, OUTPUT, OUTPUT)
    onCropped(out.toDataURL("image/png"))
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[380px]">
        <DialogHeader>
          <DialogTitle>Crop logo</DialogTitle>
          <DialogDescription>Organization logos must be square. Drag to position and zoom to fit.</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col items-center gap-4">
          <canvas
            ref={canvasRef}
            width={VIEWPORT}
            height={VIEWPORT}
            className="cursor-move touch-none rounded-lg border border-border bg-muted/30"
            onPointerDown={onPointerDown}
            onPointerMove={onPointerMove}
            onPointerUp={onPointerUp}
          />
          <div className="w-full space-y-1">
            <Label htmlFor="logo-zoom" className="text-xs text-muted-foreground">Zoom</Label>
            <input
              id="logo-zoom"
              type="range"
              min="1"
              max="4"
              step="0.01"
              value={zoom}
              onChange={(e) => handleZoom(Number(e.target.value))}
              className="w-full accent-[#eb6c6c]"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={exportCrop}>Use logo</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
