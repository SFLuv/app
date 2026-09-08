/**
 * Printed QR cards, drawn straight onto a canvas.
 *
 * The export used to screenshot the React card with html2canvas. That is slow —
 * it clones the DOM, reloads styles and rasterises text for every single card —
 * and it clipped: the clone lays text out with whatever fonts the cloned
 * document happens to have, so a heading that wrapped to three lines on screen
 * could wrap to four in the capture and lose its last line to the `overflow:
 * hidden` that keeps the card a fixed size. The corner title and the last
 * instruction went the same way.
 *
 * Drawing the card here removes both problems at once. Text is measured with
 * `measureText` before it is drawn, so wrapping and shrink-to-fit are decided
 * on the real metrics of the real font; nothing is ever laid out and then
 * cropped. And there is no DOM work per card — the static half is painted once
 * per event and reused, leaving only the QR and the code number to draw.
 *
 * The QR geometry comes from lib/qr-geometry, the same module the on-screen
 * component uses, so a card and the app draw the same code.
 */

// Same import shape the on-screen component uses, so the export draws through
// exactly the code path that is already proven in the browser.
import { buildQRDrawing, EYE_COLOR, LOGO_RATIO, LOGO_SRC, MODULE_COLOR } from "@/lib/qr-geometry"

/** The card's pixel size. The PDF page format is derived from this ratio. */
export const CARD_WIDTH = 425
export const CARD_HEIGHT = 550

/**
 * Supersampling factor for the bitmap handed to the PDF.
 *
 * The page is ~55mm wide, so 2x lands around 390dpi — comfortably past what a
 * card printer resolves, without quadrupling the memory a batch holds.
 */
const SCALE = 2

const EDGE_TOP = 18
const EDGE_BOTTOM = 24
const SIDE = 16

const INK = "#000000"
const NUMBER_COLOR = "#0b303b"
const TITLE_COLOR = "#6b7f87"
const DATE_COLOR = "#8a9aa1"

const FONT_STACK = `"Inter", "Helvetica Neue", Helvetica, Arial, sans-serif`

const TITLE_BOX_HEIGHT = 32
const TITLE_BASE_FONT = 9
const TITLE_MIN_FONT = 6
const TITLE_LINE_HEIGHT = 1.35

const HEADING_BASE_FONT = 21
const HEADING_MIN_FONT = 15
const HEADING_LINE_HEIGHT = 1.35
const HEADING_MAX_ROWS = 3

const QR_SIDE = 170
/** Floor for a comfortably scannable code; see the budget in buildCardTemplate. */
const QR_MIN_SIDE = 120
const LOGO_WIDTH = 136

/**
 * The paired-logo row, matching affiliate-qr-code-card: the SFLuv mark, an "X",
 * and the organizer's mark, each contained in a square box. Containing rather
 * than scaling to a fixed width is what keeps a portrait logo from setting the
 * row hundreds of pixels tall and pushing the QR through the footer.
 */
const LOGO_BOX = 88
const LOGO_ROW_GAP = 12
const LOGO_MIN_HEIGHT = 56

const MIN_GAP = 6
/** Four gaps sit below the body's top: logo, heading, instructions, QR. */
const BODY_GAPS = 4

export interface CardContent {
  codeValue: string
  codeNumber: number
  eventTitle: string
  eventDate?: string
  /** Paired with the SFLuv mark when the organizer has a logo. */
  logoUrl?: string | null
  organization?: string
}

const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

/** Matches formatCardDate in card-chrome, so print and screen agree. */
export function formatCardDate(unixSeconds: number): string {
  const at = new Date(unixSeconds * 1000)
  return `${WEEKDAYS[at.getDay()]}, ${MONTHS[at.getMonth()]} ${at.getDate()}, ${at.getFullYear()}`
}

function font(size: number, weight: number | string = 400): string {
  return `${weight} ${size}px ${FONT_STACK}`
}

/**
 * Greedy word wrap against real measured widths.
 *
 * A single word longer than the line is broken by character rather than allowed
 * to run past the edge — the CSS did this with `overflow-wrap: anywhere`, and a
 * pathological title should still land inside its box.
 */
export function wrapText(
  ctx: CanvasRenderingContext2D,
  text: string,
  maxWidth: number,
): string[] {
  const lines: string[] = []
  let line = ""

  const pushWordChars = (word: string) => {
    let chunk = ""
    for (const char of word) {
      if (chunk !== "" && ctx.measureText(chunk + char).width > maxWidth) {
        lines.push(chunk)
        chunk = char
      } else {
        chunk += char
      }
    }
    line = chunk
  }

  for (const word of text.split(/\s+/).filter(Boolean)) {
    const candidate = line === "" ? word : `${line} ${word}`
    if (ctx.measureText(candidate).width <= maxWidth) {
      line = candidate
      continue
    }
    if (line !== "") {
      lines.push(line)
      line = ""
    }
    if (ctx.measureText(word).width > maxWidth) {
      pushWordChars(word)
    } else {
      line = word
    }
  }

  if (line !== "") lines.push(line)
  return lines
}

/**
 * Below any caller's floor, but above illegible. See fitText.
 */
const EMERGENCY_MIN_FONT = 10

/**
 * The largest size at which the text fits the given box, and its lines there.
 *
 * Steps down rather than clamping, because a printed card that has quietly lost
 * half a line is worse than one whose type is a point smaller. The comparison
 * is on fractional measured heights — the old DOM version compared integer
 * `scrollHeight` values, which reports a box overflowing by a fraction of a
 * pixel as fitting exactly, and that fraction is a clipped row of descenders.
 *
 * `minFont` is the size the design would rather not go below, not a hard stop.
 * Once it is reached the search keeps going to EMERGENCY_MIN_FONT, because the
 * alternative at that point is dropping words: the row limit here is set by how
 * wide the text is, not how tall, so smaller type buys whole lines back. Only a
 * name too long to fit even there loses anything. The line height is unchanged
 * either way, so the card's vertical budget is never affected by this.
 */
export function fitText(
  ctx: CanvasRenderingContext2D,
  text: string,
  options: {
    maxWidth: number
    maxHeight: number
    maxLines: number
    baseFont: number
    minFont: number
    lineHeight: number
    weight?: number | string
  },
): { size: number; lines: string[] } {
  const { maxWidth, maxHeight, maxLines, baseFont, minFont, lineHeight, weight = 400 } = options

  const floor = Math.min(minFont, EMERGENCY_MIN_FONT)

  let size = baseFont
  let lines: string[] = []
  while (size >= floor) {
    ctx.font = font(size, weight)
    lines = wrapText(ctx, text, maxWidth)
    if (lines.length <= maxLines && lines.length * size * lineHeight <= maxHeight) {
      return { size, lines }
    }
    size -= 0.25
  }

  // At the floor, keep what fits rather than drawing past the box.
  ctx.font = font(floor, weight)
  lines = wrapText(ctx, text, maxWidth).slice(0, maxLines)
  return { size: floor, lines }
}

function drawLines(
  ctx: CanvasRenderingContext2D,
  lines: string[],
  x: number,
  top: number,
  size: number,
  lineHeight: number,
): number {
  const row = size * lineHeight
  lines.forEach((line, index) => {
    // Baseline sits on the text box's alphabetic line within its row.
    ctx.fillText(line, x, top + index * row + row / 2)
  })
  return lines.length * row
}

function loadImage(src: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const image = new window.Image()
    image.crossOrigin = "anonymous"
    image.onload = () => resolve(image)
    image.onerror = () => resolve(null)
    image.src = src
  })
}

/** Draws the QR from the shared geometry, filling the given square. */
function drawQR(ctx: CanvasRenderingContext2D, value: string, x: number, y: number, side: number, mark: HTMLImageElement | null) {
  const drawing = buildQRDrawing(value)
  const unit = side / drawing.units

  ctx.save()
  ctx.translate(x, y)

  ctx.fillStyle = MODULE_COLOR
  for (const dot of drawing.dots) {
    ctx.beginPath()
    ctx.arc(dot.cx * unit, dot.cy * unit, dot.r * unit, 0, Math.PI * 2)
    ctx.fill()
  }

  for (const eye of drawing.eyes) {
    const radius = eye.rx * unit
    const ex = eye.x * unit
    const ey = eye.y * unit
    const ew = eye.width * unit
    const eh = eye.height * unit
    ctx.beginPath()
    // roundRect is in every browser this app supports; the manual path keeps
    // an older one from throwing mid-export.
    if (typeof ctx.roundRect === "function") {
      ctx.roundRect(ex, ey, ew, eh, radius)
    } else {
      ctx.rect(ex, ey, ew, eh)
    }
    if (eye.fill !== "none") {
      ctx.fillStyle = eye.fill
      ctx.fill()
    }
    if (eye.stroke !== "none") {
      ctx.strokeStyle = eye.stroke
      ctx.lineWidth = eye.strokeWidth * unit
      ctx.stroke()
    }
  }

  if (mark) {
    const box = side * LOGO_RATIO
    const left = (parseFloat(drawing.logo.leftPercent) / 100) * side
    const top = (parseFloat(drawing.logo.topPercent) / 100) * side
    ctx.drawImage(mark, left, top, box, box)
  }

  ctx.restore()
}

/**
 * The logo band: its natural height, and how to draw it at any scale.
 *
 * Returning a draw closure rather than a height lets the caller settle the
 * whole vertical budget before a pixel is committed, and lets the band shrink
 * as one piece when the budget is tight.
 */
interface LogoRow {
  height: number
  draw(centre: number, top: number, scale: number): void
}

/** Largest box of the given aspect that fits inside `box` square. */
function contain(image: HTMLImageElement, box: number): { width: number; height: number } {
  const ratio = image.width / image.height
  return ratio >= 1 ? { width: box, height: box / ratio } : { width: box * ratio, height: box }
}

function planLogoRow(
  ctx: CanvasRenderingContext2D,
  mark: HTMLImageElement | null,
  brand: HTMLImageElement | null,
): LogoRow {
  const empty: LogoRow = { height: 0, draw: () => {} }

  // Paired band, matching the affiliate card on screen: mark, "X", organizer.
  if (mark && brand) {
    const left = contain(mark, LOGO_BOX)
    const right = contain(brand, LOGO_BOX)
    ctx.font = font(24, 700)
    const crossWidth = ctx.measureText("X").width
    const width = left.width + LOGO_ROW_GAP + crossWidth + LOGO_ROW_GAP + right.width

    return {
      height: LOGO_BOX,
      draw(centre, top, scale) {
        let x = centre - (width * scale) / 2
        const middle = top + (LOGO_BOX * scale) / 2
        const place = (image: HTMLImageElement, size: { width: number; height: number }) => {
          ctx.drawImage(image, x, middle - (size.height * scale) / 2, size.width * scale, size.height * scale)
          x += size.width * scale + LOGO_ROW_GAP * scale
        }

        place(mark, left)
        ctx.save()
        ctx.textAlign = "left"
        ctx.textBaseline = "middle"
        ctx.font = font(24 * scale, 700)
        ctx.fillStyle = INK
        ctx.fillText("X", x, middle)
        ctx.restore()
        x += crossWidth * scale + LOGO_ROW_GAP * scale
        place(brand, right)
      },
    }
  }

  // Solo mark, sized to the card's width the way qr-code-card does.
  const solo = brand ?? mark
  if (!solo) return empty

  const height = (LOGO_WIDTH / solo.width) * solo.height
  return {
    height,
    draw(centre, top, scale) {
      const width = LOGO_WIDTH * scale
      ctx.drawImage(solo, centre - width / 2, top, width, height * scale)
    },
  }
}

/**
 * Everything on the card that does not change between codes.
 *
 * Painted once per event and reused for every card, which is what makes the
 * export fast: per code, only the QR and the number are drawn.
 */
export interface CardTemplate {
  canvas: HTMLCanvasElement
  qrRect: { x: number; y: number; side: number }
  mark: HTMLImageElement | null
}

export async function buildCardTemplate(content: CardContent): Promise<CardTemplate> {
  const canvas = document.createElement("canvas")
  canvas.width = CARD_WIDTH * SCALE
  canvas.height = CARD_HEIGHT * SCALE
  const ctx = canvas.getContext("2d")
  if (!ctx) throw new Error("Could not prepare the card canvas.")

  ctx.scale(SCALE, SCALE)
  ctx.fillStyle = "#ffffff"
  ctx.fillRect(0, 0, CARD_WIDTH, CARD_HEIGHT)
  ctx.textBaseline = "middle"

  const contentWidth = CARD_WIDTH - SIDE * 2
  const right = CARD_WIDTH - SIDE

  // --- header: title and date, right aligned. The number is per-card. ------
  ctx.textAlign = "right"
  const titleFit = fitText(ctx, content.eventTitle, {
    maxWidth: contentWidth * 0.72,
    maxHeight: TITLE_BOX_HEIGHT,
    maxLines: 3,
    baseFont: TITLE_BASE_FONT,
    minFont: TITLE_MIN_FONT,
    lineHeight: TITLE_LINE_HEIGHT,
  })
  ctx.font = font(titleFit.size)
  ctx.fillStyle = TITLE_COLOR
  let headerBottom = EDGE_TOP + drawLines(ctx, titleFit.lines, right, EDGE_TOP, titleFit.size, TITLE_LINE_HEIGHT)

  if (content.eventDate) {
    ctx.font = font(9)
    ctx.fillStyle = DATE_COLOR
    ctx.fillText(content.eventDate, right, headerBottom + 2 + (9 * 1.35) / 2)
    headerBottom += 2 + 9 * 1.35
  }

  const mark = await loadImage(LOGO_SRC || "/icon.png")
  const brand = content.logoUrl ? await loadImage(content.logoUrl) : null

  // --- body -----------------------------------------------------------------
  const footerHeight = 10 * 1.4 * 2
  const bodyTop = headerBottom + 10
  const bodyBottom = CARD_HEIGHT - EDGE_BOTTOM - footerHeight - 8

  // Wording matches the cards on screen: the affiliate variant credits both
  // parties, since the organizer's logo sits beside the SFLuv mark above it.
  const heading = content.organization
    ? `Thank you from SFLuv and ${content.organization}!`
    : "Thank you from SFLuv!"
  ctx.textAlign = "center"
  const headingFit = fitText(ctx, heading, {
    maxWidth: contentWidth - 48,
    maxHeight: HEADING_BASE_FONT * HEADING_LINE_HEIGHT * HEADING_MAX_ROWS,
    maxLines: HEADING_MAX_ROWS,
    baseFont: HEADING_BASE_FONT,
    minFont: HEADING_MIN_FONT,
    lineHeight: HEADING_LINE_HEIGHT,
    weight: 700,
  })

  const logoRow = planLogoRow(ctx, mark, brand)
  const headingHeight = headingFit.lines.length * headingFit.size * HEADING_LINE_HEIGHT
  const instructionsHeight = 13 * 1.3 + 4 + 4 * (12 * 1.35)

  /*
   * The budget, enforced rather than hoped for.
   *
   * Everything rigid is measured before anything is drawn, and the slack is
   * shared between the gaps. When the content is tall the gaps go to their
   * minimum and then something has to give — the one thing that must never
   * happen is the body running past its bottom, which is what produced the
   * clipped instructions.
   *
   * The order it gives in matters. A shrunken logo costs nothing; a QR below
   * ~120px starts to fight the phone camera. So the logo absorbs the squeeze
   * first, down to LOGO_MIN_HEIGHT, and only then does the code come down. The
   * last line is a hard clamp against bodyBottom, so however the numbers land
   * the card is laid out to fit rather than trusted to.
   */
  const available = bodyBottom - bodyTop
  const fixedHeight = headingHeight + instructionsHeight
  let logoHeight = logoRow.height
  let slack = available - (logoHeight + fixedHeight)
  let qrSide = Math.min(QR_SIDE, slack - BODY_GAPS * MIN_GAP)

  if (qrSide < QR_MIN_SIDE) {
    const shrink = Math.min(QR_MIN_SIDE - qrSide, Math.max(0, logoHeight - LOGO_MIN_HEIGHT))
    logoHeight -= shrink
    slack += shrink
    qrSide = Math.min(QR_SIDE, slack - BODY_GAPS * MIN_GAP)
  }

  const gap = Math.max(MIN_GAP, (slack - qrSide) / BODY_GAPS)

  let y = bodyTop + gap
  const centre = CARD_WIDTH / 2

  if (logoHeight > 0) {
    logoRow.draw(centre, y, logoHeight / logoRow.height)
    y += logoHeight
  }
  y += gap

  ctx.fillStyle = INK
  ctx.font = font(headingFit.size, 700)
  y += drawLines(ctx, headingFit.lines, centre, y, headingFit.size, HEADING_LINE_HEIGHT)
  y += gap

  ctx.font = font(13, 700)
  ctx.fillText("To redeem your tokens:", centre, y + (13 * 1.3) / 2)
  y += 13 * 1.3 + 4

  ctx.font = font(12)
  for (const step of [
    "1. Scan the QR code",
    "2. Download the SFLuv app",
    "3. Scan again",
    "4. Receive your SFLuv!",
  ]) {
    ctx.fillText(step, centre, y + (12 * 1.35) / 2)
    y += 12 * 1.35
  }
  y += gap

  // The clamp the whole budget exists to make redundant. Kept because a card
  // with a slightly small code still scans, and one drawn over its own footer
  // is the bug this file was written to end.
  const drawnQR = Math.max(0, Math.min(qrSide, bodyBottom - y))
  const qrRect = { x: centre - drawnQR / 2, y, side: drawnQR }

  // --- footer ---------------------------------------------------------------
  ctx.font = font(10)
  ctx.fillStyle = INK
  const footerTop = CARD_HEIGHT - EDGE_BOTTOM - footerHeight
  ctx.fillText("Interested in more SFLuv supported events?", centre, footerTop + (10 * 1.4) / 2)
  ctx.fillText("Visit www.sfluv.org/volunteers", centre, footerTop + 10 * 1.4 + (10 * 1.4) / 2)

  return { canvas, qrRect, mark }
}

/**
 * One card: the template, plus this code's QR and number.
 *
 * Returns a data URL ready for jsPDF. The template is copied rather than drawn
 * into, so the same one serves every code in the event.
 */
export function renderCard(template: CardTemplate, content: CardContent): string {
  const canvas = document.createElement("canvas")
  canvas.width = CARD_WIDTH * SCALE
  canvas.height = CARD_HEIGHT * SCALE
  const ctx = canvas.getContext("2d")
  if (!ctx) throw new Error("Could not prepare the card canvas.")

  ctx.drawImage(template.canvas, 0, 0)
  ctx.scale(SCALE, SCALE)
  ctx.textBaseline = "middle"

  ctx.textAlign = "left"
  ctx.font = font(12, 700)
  ctx.fillStyle = NUMBER_COLOR
  ctx.fillText(`#${content.codeNumber}`, SIDE, EDGE_TOP + (12 * 1.2) / 2)

  drawQR(ctx, content.codeValue, template.qrRect.x, template.qrRect.y, template.qrRect.side, template.mark)

  return canvas.toDataURL("image/jpeg", 0.92)
}
