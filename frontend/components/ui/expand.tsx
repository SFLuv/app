"use client"

import { useEffect, useState, type ReactNode } from "react"

import { cn } from "@/lib/utils"

/** Kept in step with the duration-300 class below and with CHOOSER_MOVE_MS. */
const EXPAND_MS = 300

/**
 * Animates a block open and shut without anyone having to know its height.
 *
 * The trick is the grid: a single-row grid transitioning `grid-template-rows`
 * between `0fr` and `1fr` interpolates to the content's own natural height, so
 * a section whose contents change size — a week of opening-hours pickers, a
 * step that grows a write-in field — animates correctly with no measurement,
 * no ref, and no ResizeObserver. `max-height` guesses cannot do that: too small
 * clips, too large makes the easing look wrong.
 *
 * The children stay mounted while closed, which is what makes the open
 * animation possible at all — there is nothing to reveal if it does not exist
 * yet. `inert` is what stops that being a bug: a collapsed section is skipped
 * by tab order, search and the accessibility tree, so a merchant tabbing
 * through step one never lands in a field they cannot see.
 */
export function Expand({
  open,
  children,
  className,
  /** Extra top margin applied only while open, so a closed block adds no gap. */
  gap = "mt-4",
  /**
   * Slide horizontally as well as collapsing: out to the right on the way
   * closed, in from the right on the way open. For a block that is moving to a
   * different place in the form rather than merely appearing, where a plain
   * collapse reads as two unrelated events.
   */
  slide = false,
}: {
  open: boolean
  children: ReactNode
  className?: string
  gap?: string
  slide?: boolean
}) {
  /**
   * Clip only while the height is moving.
   *
   * The row has to clip for the animation to hide anything, but a permanently
   * clipped box also cuts off whatever an open child renders outside its bounds
   * — which is every dropdown, and is how the location chooser's suggestion
   * list came to be sliced off at the bottom of its own field.
   *
   * So it clips whenever closed or closing, and stops once the opening
   * animation has finished. Radix menus portal to the body and never needed
   * this; anything positioned in place does.
   */
  const [clipping, setClipping] = useState(!open)
  useEffect(() => {
    if (!open) {
      setClipping(true)
      return
    }
    const timer = setTimeout(() => setClipping(false), EXPAND_MS + 40)
    return () => clearTimeout(timer)
  }, [open])

  return (
    <div
      className={cn(
        "grid transition-[grid-template-rows,opacity,margin] duration-300 ease-out motion-reduce:transition-none",
        slide && "transition-[grid-template-rows,opacity,margin,transform]",
        open
          ? cn("grid-rows-[1fr] opacity-100", slide && "translate-x-0", gap)
          : cn("grid-rows-[0fr] opacity-0", slide && "translate-x-8"),
        className,
      )}
    >
      {/* The row itself must clip, or the content keeps its full height and the
          grid animation has nothing to hide. Radix menus and tooltips portal to
          the body, so nothing that needs to escape this box is inside it. */}
      <div className={clipping ? "overflow-hidden" : "overflow-visible"} inert={!open}>
        {children}
      </div>
    </div>
  )
}
