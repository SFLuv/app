"use client"

import { memo } from "react"

import {
  ICON_TEXT_COLOR,
  ICON_TEXT_NUDGE_EM,
  PIN_EDGE_COLOR,
  PIN_GLYPH_RADIUS,
  PIN_RING_RADIUS,
  PIN_HEAD_CENTRE,
  PIN_PATH,
  PIN_VIEWBOX_HEIGHT,
  PIN_VIEWBOX_WIDTH,
  PIN_WIDTH,
  ICON_FACE,
  merchantInitials,
  pinColor,
} from "@/lib/merchant-icon"
import type { OpenState } from "@/lib/opening-hours"
import { cn } from "@/lib/utils"

interface MerchantIconProps {
  name: string
  iconUrl?: string | null
  /** Rendered edge length in pixels. */
  size?: number
  className?: string
  /** Open state, which decides the face colour behind a generated mark. */
  state?: OpenState
}

/**
 * A merchant's square mark: their upload when they have one, otherwise a
 * generated initials tile.
 *
 * The generated tile is not a placeholder awaiting a real logo — most merchants
 * will never upload one, and a map of identical grey dots is worse than a map
 * of distinct initials on a clean white face.
 */
export const MerchantIcon = memo(function MerchantIcon({
  name,
  iconUrl,
  size = 40,
  className,
  state = "open",
}: MerchantIconProps) {
  const trimmed = (iconUrl || "").trim()
  const closed = state === "closed"

  if (trimmed !== "") {
    return (
      // Plain <img>: these are arbitrary merchant uploads served from the API
      // origin, which next/image would need configured per-host.
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={trimmed}
        alt=""
        width={size}
        height={size}
        className={cn("h-full w-full object-cover", closed && "grayscale-[0.65] opacity-90", className)}
        loading="lazy"
        decoding="async"
      />
    )
  }

  const initials = merchantInitials(name)

  return (
    <div
      aria-hidden
      className={cn("flex h-full w-full items-center justify-center font-bold leading-none", className)}
      style={{
        backgroundColor: ICON_FACE,
        color: ICON_TEXT_COLOR,
        // Two characters need to fit inside a circle that is mostly padding.
        fontSize: Math.max(8, Math.round(size * (initials.length > 1 ? 0.4 : 0.5))),
        letterSpacing: "-0.01em",
      }}
    >
      <span style={{ transform: `translateY(${ICON_TEXT_NUDGE_EM}em)` }}>{initials}</span>
    </div>
  )
})

interface MerchantMapPinProps {
  name: string
  iconUrl?: string | null
  state: OpenState
  /** Rendered pin width in pixels; height follows the silhouette's ratio. */
  width?: number
  className?: string
}

/**
 * The map pin.
 *
 * A teardrop in the merchant's state colour with their mark set into the head.
 * Drawn here rather than with Google's PinElement because that shape is fixed:
 * it cannot be made shorter, and it finishes on a needle point that reads badly
 * at this size against a busy street map.
 */
export const MerchantMapPin = memo(function MerchantMapPin({
  name,
  iconUrl,
  state,
  width = PIN_WIDTH,
  className,
}: MerchantMapPinProps) {
  const height = Math.round((width * PIN_VIEWBOX_HEIGHT) / PIN_VIEWBOX_WIDTH)
  const unit = width / PIN_VIEWBOX_WIDTH
  const glyphSize = Math.round(PIN_GLYPH_RADIUS * 2 * unit)

  return (
    <div className={cn("relative", className)} style={{ width, height }}>
      <svg
        viewBox={`0 0 ${PIN_VIEWBOX_WIDTH} ${PIN_VIEWBOX_HEIGHT}`}
        width={width}
        height={height}
        className="absolute inset-0"
        style={{ filter: "drop-shadow(0 1px 2px rgba(11, 48, 59, 0.3))" }}
        aria-hidden
      >
        <path d={PIN_PATH} fill={ICON_FACE} stroke={PIN_EDGE_COLOR} strokeWidth={0.6} />
        <circle
          cx={PIN_HEAD_CENTRE.x}
          cy={PIN_HEAD_CENTRE.y}
          r={PIN_RING_RADIUS}
          fill={pinColor(state)}
        />
      </svg>
      <div
        className="absolute overflow-hidden rounded-full"
        style={{
          width: glyphSize,
          height: glyphSize,
          left: PIN_HEAD_CENTRE.x * unit - glyphSize / 2,
          top: PIN_HEAD_CENTRE.y * unit - glyphSize / 2,
          backgroundColor: ICON_FACE,
        }}
      >
        <MerchantIcon name={name} iconUrl={iconUrl} size={glyphSize} state={state} />
      </div>
    </div>
  )
})

/**
 * The open/closed line on a merchant card.
 *
 * The dot pulses only while open. A steady dot beside "Open now" reads as a
 * status light that might be stale; a pulsing one reads as live, which is what
 * it is — recomputed against the clock every minute.
 */
export function OpenStatusBadge({ state, className }: { state: OpenState; className?: string }) {
  if (state === "unknown") {
    return (
      <span className={cn("inline-flex items-center gap-1.5 text-xs text-muted-foreground", className)}>
        <span className="h-2 w-2 rounded-full bg-muted-foreground/40" />
        Hours not available
      </span>
    )
  }

  const open = state === "open"

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-xs font-medium",
        open ? "text-[#eb6c6c]" : "text-muted-foreground",
        className,
      )}
    >
      <span className="relative flex h-2 w-2">
        {open ? (
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[#eb6c6c] opacity-75" />
        ) : null}
        <span className={cn("relative inline-flex h-2 w-2 rounded-full", open ? "bg-[#eb6c6c]" : "bg-muted-foreground/50")} />
      </span>
      {open ? "Open now" : "Closed"}
    </span>
  )
}
