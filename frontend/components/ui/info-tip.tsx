"use client"

import * as React from "react"
import * as PopoverPrimitive from "@radix-ui/react-popover"
import { Info } from "lucide-react"

import { cn } from "@/lib/utils"

/**
 * The context that would otherwise be a subtitle: an "i" beside a title or
 * label that shows its text on hover or keyboard focus, and pins it open on a
 * click or a tap (touch has no hover). Escape, or a click elsewhere, closes it.
 *
 * Place it beside what it explains, never inside a button, link or tab: a
 * button inside a button is not valid, and the outer one would swallow it.
 */
export function InfoTip({
  children,
  label = "More information",
  side = "top",
  align = "center",
  className,
  contentClassName,
}: {
  children: React.ReactNode
  /** What a screen reader announces for the icon. */
  label?: string
  side?: "top" | "right" | "bottom" | "left"
  align?: "start" | "center" | "end"
  className?: string
  contentClassName?: string
}) {
  const [open, setOpen] = React.useState(false)
  const [pinned, setPinned] = React.useState(false)
  const closeTimer = React.useRef<number | undefined>(undefined)

  const cancelClose = () => {
    if (closeTimer.current !== undefined) {
      window.clearTimeout(closeTimer.current)
      closeTimer.current = undefined
    }
  }
  const hoverOpen = (event: React.PointerEvent) => {
    if (event.pointerType !== "mouse") return
    cancelClose()
    setOpen(true)
  }
  const hoverClose = (event: React.PointerEvent) => {
    if (event.pointerType !== "mouse" || pinned) return
    cancelClose()
    // A short grace period, so the pointer can cross the gap to the text.
    closeTimer.current = window.setTimeout(() => setOpen(false), 120)
  }

  React.useEffect(() => cancelClose, [])

  return (
    <PopoverPrimitive.Root
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) setPinned(false)
      }}
    >
      <PopoverPrimitive.Trigger asChild>
        <button
          type="button"
          data-info-tip=""
          aria-label={label}
          className={cn(
            "inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full align-middle text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
            className,
          )}
          onPointerEnter={hoverOpen}
          onPointerLeave={hoverClose}
          onFocus={(event) => {
            // Keyboard focus only: a click focuses too, and handles itself.
            if (event.currentTarget.matches(":focus-visible")) setOpen(true)
          }}
          onBlur={() => {
            if (!pinned) setOpen(false)
          }}
          onClick={(event) => {
            // Handled here rather than by the popover, so a click pins an
            // already-open tip instead of closing it, and never reaches a
            // label, row or card it sits in.
            event.preventDefault()
            event.stopPropagation()
            cancelClose()
            if (pinned) {
              setPinned(false)
              setOpen(false)
            } else {
              setPinned(true)
              setOpen(true)
            }
          }}
        >
          <Info className="h-4 w-4" aria-hidden="true" />
        </button>
      </PopoverPrimitive.Trigger>
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          side={side}
          align={align}
          sideOffset={6}
          collisionPadding={16}
          // Opening on hover must not move focus out of the page.
          onOpenAutoFocus={(event) => event.preventDefault()}
          // The tip renders in a portal, but React still bubbles its events
          // through the tree it was declared in: a click on the text would
          // otherwise toggle a collapsible header or select a row it sits in.
          onClick={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
          onPointerEnter={(event) => {
            if (event.pointerType === "mouse") cancelClose()
          }}
          onPointerLeave={hoverClose}
          className={cn(
            "z-50 max-w-xs rounded-lg border border-border/70 bg-popover px-3 py-2 text-sm leading-5 text-popover-foreground shadow-md outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 motion-reduce:animate-none",
            contentClassName,
          )}
        >
          {children}
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  )
}
