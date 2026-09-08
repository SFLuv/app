"use client"

import { useEffect, useLayoutEffect, useRef, useState } from "react"
import type { CSSProperties, ReactNode } from "react"

/**
 * Shared shell for the printed QR cards.
 *
 * The card is a fixed 425x550: the PDF page format is derived from that ratio,
 * so it cannot change. Everything here exists to keep content inside that box
 * no matter how long the event title or organization name runs.
 *
 * The layout is a flex column of three parts — header, body, footer — where
 * only the body flexes. EDGE is real padding on a border-box element, so the
 * top and bottom margins are literally the same value at every content length;
 * growth is absorbed by the body's gaps instead of eating into them.
 */

/** Top and bottom inset. Identical by construction, never consumed. */
const EDGE_TOP = 18
const EDGE_BOTTOM = 24
const SIDE = 16

export const cardShellStyle: CSSProperties = {
  boxSizing: "border-box",
  height: "550px",
  width: "425px",
  margin: "auto",
  padding: `${EDGE_TOP}px ${SIDE}px ${EDGE_BOTTOM}px`,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  textAlign: "center",
  color: "black",
  overflow: "hidden",
}

/** Body: the only part that gives. Contents stay optically centred within it. */
export const cardBodyStyle: CSSProperties = {
  flex: "1 1 auto",
  minHeight: 0,
  width: "100%",
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "flex-start",
}

/**
 * Vertical space that gives way before anything else does.
 *
 * Shrinks in proportion to its size, so a wrapping heading tightens every gap a
 * little rather than pushing the footer off the bottom edge.
 *
 * `grow` decides where leftover height collects. Only the gaps above the QR
 * grow: the card is taller than its content on the affiliate variant, whose
 * logo row is less than half the height of the standalone mark, and letting
 * that slack distribute evenly parked ~46px of nothing between the code and the
 * footer. Keeping the QR's own gaps ungrown holds it tight to the instructions
 * above and the footer below at every content length, and sends the spare
 * height to the airy top of the card where it reads as spacing rather than a
 * hole.
 */
export const Gap = ({ size, grow = 0 }: { size: number; grow?: number }) => (
  <div aria-hidden="true" style={{ flex: `${grow} 1 ${size}px`, width: "100%" }} />
)

/** Real content never shrinks — only the gaps do. */
export const rigid: CSSProperties = { flexShrink: 0 }

/**
 * The title's fixed box, and the type that has to live in it.
 *
 * 32px is the most the card can spare once the date line is accounted for —
 * derived from the worst case, an affiliate card whose heading runs to three
 * rows. Two lines fit at full size; a third makes the type shrink rather than
 * be cut off, and 32px is generous enough that three lines still land at about
 * 7.9px. The floor exists so a pathological title degrades to small-but-there
 * instead of unreadable.
 */
const TITLE_BOX_HEIGHT = 32
const TITLE_BASE_FONT = 9
const TITLE_MIN_FONT = 6
const TITLE_LINE_HEIGHT = 1.35

/** useLayoutEffect warns when a client component is server-rendered. */
const useIsomorphicLayoutEffect = typeof window === "undefined" ? useEffect : useLayoutEffect

/**
 * Shrinks text to its box rather than letting it be truncated.
 *
 * Nothing on a printed card should be cut: the heading is the card's message
 * and the corner title is how a stack gets sorted back into piles. CSS cannot
 * size text to its container, so this measures and steps down — cheap, and it
 * settles before paint, which matters because the PDF export rasterises these
 * offscreen shortly after mount.
 */
function useFitToBox(
  ref: { current: HTMLElement | null },
  key: string,
  box: number,
  base: number,
  min: number,
): number {
  /*
   * The fitted size is STATE, not just an inline style written by the effect.
   *
   * Writing only to node.style meant any later re-render re-applied the style
   * prop's base size and silently undid the fit, while this effect — keyed on
   * content — had no reason to run again. The card then rasterised at full size
   * inside a capped box, which is how a long title lost its bottom half and a
   * three-line message lost its third line instead of shrinking. The export
   * re-renders repeatedly (batch swaps, progress, the panel's background poll),
   * so this was reliably hit there and almost never on screen.
   */
  const [size, setSize] = useState(base)

  useIsomorphicLayoutEffect(() => {
    const node = ref.current
    if (!node) return

    const fit = () => {
      // Measure with the cap lifted so the loop does not depend on how
      // scrollHeight reports a clipped box.
      const cap = node.style.maxHeight
      node.style.maxHeight = "none"
      let fitted = base
      node.style.fontSize = `${fitted}px`
      while (node.scrollHeight > box && fitted > min) {
        fitted -= 0.25
        node.style.fontSize = `${fitted}px`
      }
      node.style.maxHeight = cap
      setSize(fitted)
    }

    fit()

    // Refit once the webfont swaps in. Measuring against the fallback face
    // gives the wrong answer — different metrics can push the text onto an
    // extra line after the fit has already settled — and the PDF export
    // rasterises these shortly after mount, so a stale fit gets printed.
    let cancelled = false
    void document.fonts?.ready.then(() => {
      if (!cancelled) fit()
    })
    return () => {
      cancelled = true
    }
  }, [ref, key, box, base, min])

  return size
}

/**
 * The card's "Thank you from ..." line.
 *
 * Shares the box with a shrink rather than a clamp, because a long partner name
 * getting sliced mid-word is the one failure that looks like a bug rather than
 * a design. The box height is unchanged, so the card's budget is unaffected —
 * an over-long name now costs type size instead of characters.
 */
export const CardHeading = ({ fitKey, children }: { fitKey: string; children: ReactNode }) => {
  const ref = useRef<HTMLHeadingElement | null>(null)
  const fontSize = useFitToBox(ref, fitKey, HEADING_BOX_HEIGHT, HEADING_BASE_FONT, HEADING_MIN_FONT)
  return (
    <h1 ref={ref} data-fitted style={{ ...headingStyle, fontSize: `${fontSize}px` }}>
      {children}
    </h1>
  )
}

const FittedTitle = ({ text }: { text: string }) => {
  const ref = useRef<HTMLSpanElement | null>(null)
  const fontSize = useFitToBox(ref, text, TITLE_BOX_HEIGHT, TITLE_BASE_FONT, TITLE_MIN_FONT)

  return (
    <span
      ref={ref}
      data-fitted
      style={{
        display: "block",
        fontSize: `${fontSize}px`,
        color: "#6b7f87",
        lineHeight: TITLE_LINE_HEIGHT,
        textAlign: "right",
        maxHeight: `${TITLE_BOX_HEIGHT}px`,
        overflow: "hidden",
        overflowWrap: "anywhere",
      }}
    >
      {text}
    </span>
  )
}

/**
 * Card heading, shared so both variants stay on the same type.
 *
 * The row cap is what makes overflow structurally impossible rather than merely
 * unlikely. Enlarging the body's gaps cannot buy headroom — a gap adds the same
 * amount to the content height as it adds to the shrink budget — so the only way
 * to guarantee the footer survives an arbitrarily long organization name is to
 * bound the heading itself.
 *
 * Three rows, not four: the looser line-height costs enough height that a
 * four-row heading no longer fits beside a three-row event title. Three still
 * carries "Thank you from" plus roughly 55 characters of name, past which the
 * name truncates — a far better failure than a QR code or footer sliced off at
 * the card edge.
 */
const HEADING_BASE_FONT = 21
const HEADING_LINE_HEIGHT = 1.35
const HEADING_ROW = HEADING_BASE_FONT * HEADING_LINE_HEIGHT
/** ceil, not round: at round() the box lands 0.05px under three rows, which
 *  clips the third line while scrollHeight rounds to the same number and so
 *  never reports the overflow. */
const HEADING_BOX_HEIGHT = Math.ceil(HEADING_ROW * 3)
/** Floor for the shrink; below this the message stops being the message. */
const HEADING_MIN_FONT = 15
export const headingStyle: CSSProperties = {
  flexShrink: 0,
  margin: "0 24px",
  fontWeight: "bold",
  fontSize: `${HEADING_BASE_FONT}px`,
  lineHeight: HEADING_LINE_HEIGHT,
  maxHeight: `${HEADING_BOX_HEIGHT}px`,
  overflow: "hidden",
}

/**
 * Code number top-left, event title top-right.
 *
 * The title wraps instead of truncating on the first overflow — it is how a
 * stack of printed cards gets sorted back into piles — but it is capped at
 * three rows. The cap is a hard `maxHeight` as well as a line clamp, because
 * the PDF rasteriser does not honour `-webkit-line-clamp`, and without the
 * height cap a pathological title would still push the body.
 */
const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

/**
 * Spelled out rather than left to toLocaleDateString so a card printed on one
 * machine reads the same as one printed on another. Local time is correct here:
 * the date on the card is the date the volunteer turns up.
 */
export const formatCardDate = (unixSeconds: number): string => {
  const at = new Date(unixSeconds * 1000)
  return `${WEEKDAYS[at.getDay()]}, ${MONTHS[at.getMonth()]} ${at.getDate()}, ${at.getFullYear()}`
}

export const CardHeader = ({
  codeNumber,
  eventTitle,
  eventStartAt,
}: {
  codeNumber: number
  eventTitle: string
  eventStartAt?: number
}) => (
  <div
    style={{
      ...rigid,
      display: "flex",
      width: "100%",
      alignItems: "flex-start",
      justifyContent: "space-between",
      gap: "10px",
    }}
  >
    <span style={{ ...rigid, fontSize: "12px", fontWeight: 700, color: "#0b303b", lineHeight: 1.2 }}>
      #{codeNumber}
    </span>
    <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", maxWidth: "72%", minWidth: 0 }}>
      <FittedTitle text={eventTitle} />
      {eventStartAt !== undefined && (
        <span
          style={{
            fontSize: "9px",
            color: "#8a9aa1",
            lineHeight: 1.35,
            textAlign: "right",
            whiteSpace: "nowrap",
            marginTop: "2px",
          }}
        >
          {formatCardDate(eventStartAt)}
        </span>
      )}
    </div>
  </div>
)

/** Shared instruction list and footer, identical across both card variants. */
export const CardInstructions = () => (
  <>
    {/* A sub-heading, so it outranks the steps it introduces: a point larger
        and bold. Preflight resets heading weight to inherit, so the weight has
        to be stated rather than assumed from the <h3>. */}
    <h3 style={{ ...rigid, margin: "0 10px 4px", fontSize: "13px", fontWeight: 700, lineHeight: 1.3 }}>
      To redeem your tokens:
    </h3>
    <ol
      style={{
        ...rigid,
        textAlign: "center",
        width: "70%",
        margin: 0,
        fontSize: "12px",
        lineHeight: 1.35,
      }}
    >
      <li>1. Scan the QR code</li>
      <li>2. Download the SFLuv app</li>
      <li>3. Scan again</li>
      <li>4. Receive your SFLuv!</li>
    </ol>
  </>
)

export const CardFooter = () => (
  <div style={{ ...rigid, textAlign: "center", fontSize: "10px" }}>
    <p>
      Interested in more SFLuv supported events?
      <br />
      Visit <a>www.sfluv.org/volunteers</a>
    </p>
  </div>
)

/** Keeps the code's footprint identical between the two card variants. */
export const CardQRFrame = ({ children }: { children: ReactNode }) => (
  <div style={{ ...rigid, width: "170px" }}>{children}</div>
)
