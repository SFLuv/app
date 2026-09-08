/**
 * The look of a merchant's map mark, in one place.
 *
 * The same rules are implemented on the mobile client and the marketing site.
 * They are duplicated rather than shared because the three do not share a
 * package — if you change a colour, a dimension or the initials rule here,
 * change it in `mobile-app/mobile/src/utils/merchantIcon.ts` and
 * `webpage/src/lib/merchants/icon.ts` too.
 */

/** Brand red. A merchant that is open right now. */
export const PIN_OPEN_COLOR = "#eb6c6c"
/**
 * Closed. A muted grey, but warmed a few degrees toward the brand red rather
 * than a neutral slate — a wholly desaturated pin reads as broken or
 * unavailable, where these merchants are simply shut until tomorrow.
 */
export const PIN_CLOSED_COLOR = "#9b8a8a"

/**
 * The face a generated icon is drawn on, open or closed.
 *
 * Deliberately state-independent: the pin's own colour carries open/closed, and
 * tinting the merchant's mark as well made their identity look like a status.
 */
export const ICON_FACE = "#ffffff"

/** Initials are black and bold: the strongest contrast against a white face. */
export const ICON_TEXT_COLOR = "#111111"

/**
 * Pin geometry, shared with the marker SVG.
 *
 * The body is WHITE and the merchant's state is a perfect circle set inside it,
 * rather than the whole teardrop being coloured. A pin that is mostly white
 * reads as a marker on a map; a pin that is mostly brand red reads as an alert,
 * and a street of them drowned out the map underneath.
 *
 * Google's own PinElement could not do this — its shape and its single fill are
 * both fixed — hence the hand-drawn path. It is stubbier than Google's (26:31
 * rather than 27:43) and finishes on a wide rounded arc rather than a point.
 */
export const PIN_VIEWBOX_WIDTH = 26
export const PIN_VIEWBOX_HEIGHT = 31
export const PIN_PATH =
  "M1.74 19.5A13 13 0 1 1 24.26 19.5C21.6 23.6 17 27 14.6 29.4a2.2 2.2 0 0 1-3.2 0C9 27 4.4 23.6 1.74 19.5Z"
/** Head-circle centre in viewBox units — where the colour and glyph sit. */
export const PIN_HEAD_CENTRE = { x: 13, y: 13 }
/**
 * Radius of the state-coloured circle, in viewBox units.
 *
 * Inside the head's own radius of 13, leaving the white body as a thin rim
 * around the colour. Filling the head edge to edge was tried and lost the
 * outline that separates a pin from whatever it happens to sit on.
 */
export const PIN_RING_RADIUS = 10.4
/**
 * Radius of the merchant's artwork, in viewBox units.
 *
 * Smaller than the coloured circle by design: the difference is what turns the
 * colour into a visible ring around the mark instead of a disc behind it.
 */
export const PIN_GLYPH_RADIUS = 7.4
/**
 * Downward nudge for generated initials, as a fraction of their font size.
 *
 * Centring a line box is not the same as centring what is drawn in it: with a
 * line height of 1, the box reserves room below the baseline for descenders
 * that capital letters never use, so "MC" sits visibly above the middle of its
 * circle. This pushes the glyphs back onto the optical centre.
 */
export const ICON_TEXT_NUDGE_EM = 0.06

/** A hairline so a white pin still has an edge on a pale map. */
export const PIN_EDGE_COLOR = "rgba(11, 48, 59, 0.18)"

/** Default rendered pin width in CSS pixels. */
export const PIN_WIDTH = 30



/**
 * Up to two initials for a business name.
 *
 * Leading articles are dropped ("The Mill" reads as M, not TM) and words that
 * are punctuation or a lone ampersand are skipped, because "Bob & Sons" should
 * be BS. Falls back to the first two characters when there is only one word.
 */
export function merchantInitials(name: string): string {
  const cleaned = (name || "").replace(/[^\p{L}\p{N}\s'&-]/gu, " ").trim()
  if (cleaned === "") return "?"

  const words = cleaned
    .split(/\s+/)
    .filter((word) => /[\p{L}\p{N}]/u.test(word))
    .filter((word) => word !== "&")

  const meaningful = words.length > 1 ? words.filter((word) => !/^(the|a|an|el|la|le)$/i.test(word)) : words
  const source = meaningful.length > 0 ? meaningful : words

  if (source.length === 0) return "?"
  if (source.length === 1) {
    return source[0].slice(0, 2).toUpperCase()
  }
  return (source[0][0] + source[1][0]).toUpperCase()
}

/** The pin colour for an open state. Unknown hours keep the brand colour:
 *  absence of data is not a claim of closure. */
export function pinColor(state: "open" | "closed" | "unknown"): string {
  return state === "closed" ? PIN_CLOSED_COLOR : PIN_OPEN_COLOR
}
