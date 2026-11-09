/**
 * The printed card's reverse: the same redemption steps as the front, in
 * Spanish, Traditional Chinese, Arabic and Vietnamese.
 *
 * Drawn here rather than shipped as artwork for the same reasons the front is
 * (see qr-card-canvas): text measured before it is drawn, so nothing is laid
 * out and then cropped, and no DOM work per card. It is painted once and the
 * same bitmap is reused for every card on the sheet, because the reverse
 * carries nothing card-specific.
 *
 * The wording is transcribed verbatim from the printed reference,
 * "SFLuv Gratitude Perks-back side 05-16-26 (4.25 x 5.5).pdf". Only the layout
 * changed: the three blocks now sit on a shared grid with consistent spacing
 * and a rule between them, and the Arabic is live text rather than the
 * low-resolution bitmap the reference embedded it as. The Vietnamese is not in
 * that reference; it was supplied on its own on 2026-11-08 and is transcribed
 * verbatim from that. Do not reword any of it without a new translation —
 * these are the only instructions a volunteer who does not read English gets.
 */
import { LOGO_SRC } from "@/lib/qr-geometry"
import { CARD_HEIGHT, CARD_WIDTH } from "@/lib/qr-card-canvas"

/** Latin/UI stack, matching the front. */
const FONT_LATIN = `"Inter", "Helvetica Neue", Helvetica, Arial, sans-serif`
/** Canvas falls back per glyph, but naming the faces keeps the result the same across machines. */
const FONT_CJK = `"PingFang TC", "Hiragino Sans TC", "Hiragino Sans", "Microsoft JhengHei", "Noto Sans TC", ${FONT_LATIN}`
const FONT_ARABIC = `"Geeza Pro", "Noto Naskh Arabic", "Al Bayan", "Baghdad", "Damascus", ${FONT_LATIN}`

const INK = "#000000"
const RULE = "#d9dfe2"

const EDGE_TOP = 20
const EDGE_BOTTOM = 22
const SIDE = 20

/**
 * 92 when the back carried three languages. A fourth needs the height: at 92
 * the four blocks would have to shrink to about 84% of their size to fit, and
 * at 64 they keep about 91% (steps print at 7.5pt, headings at 9.8pt).
 */
const LOGO_WIDTH = 64

export type BackDirection = "ltr" | "rtl"

export interface BackBlock {
  /** "¡Gracias de parte de SFLuv!" and its siblings. */
  heading: string
  /** "Para canjear tus tokens:" */
  lead: string
  /** The four numbered steps, already carrying their own numerals. */
  steps: string[]
  font: string
  direction: BackDirection
  /** Point size nudges, so three scripts with different x-heights read as one size. */
  scale?: number
}

/**
 * The translations, verbatim from their sources (see the top of the file).
 *
 * The Chinese is written with the standard ideographs (支, 二, 用). The
 * reference PDF's font encoded three of them as Kangxi radicals (⽀, ⼆, ⽤),
 * which look identical in print but are different characters and would not
 * match if anyone searched or re-translated from it.
 */
export const BACK_BLOCKS: BackBlock[] = [
  {
    heading: "¡Gracias de parte de SFLuv!",
    lead: "Para canjear tus tokens:",
    steps: [
      "1. Escanea el código QR",
      "2. Descarga la app de SFLuv",
      "3. Escanea de nuevo",
      "4. ¡Recibe tu SFLuv!",
    ],
    font: FONT_LATIN,
    direction: "ltr",
  },
  {
    heading: "SFLuv 感謝您的支持！",
    lead: "兌換代幣步驟：",
    steps: [
      "1. 掃描二維碼",
      "2. 下載 SFLuv 應用",
      "3. 再次掃描二維碼",
      "4. 領取您的 SFLuv！",
    ],
    font: FONT_CJK,
    direction: "ltr",
    scale: 0.97,
  },
  {
    heading: "شكراً من SFLuv!",
    lead: "لاسترداد رموزك:",
    steps: [
      "١. امسح رمز QR.",
      "٢. حمّل تطبيق SFLuv.",
      "٣. امسح مرة أخرى.",
      "٤. استلم رموز SFLuv الخاصة بك.",
    ],
    font: FONT_ARABIC,
    direction: "rtl",
    scale: 1.04,
  },
  {
    heading: "Lời cảm ơn từ SFLuv!",
    lead: "Để đổi token của quý vị:",
    steps: [
      "1. Quét mã QR",
      "2. Tải xuống ứng dụng SFLuv",
      "3. Quét mã lần nữa",
      "4. Nhận SFLuv của quý vị!",
    ],
    font: FONT_LATIN,
    direction: "ltr",
  },
]

const HEADING_SIZE = 15
const LEAD_SIZE = 12.5
const STEP_SIZE = 11.5
/** 1.42 with three blocks; tightened so four keep more of their type size. */
const STEP_LINE = 1.3
const HEADING_GAP = 5
const LEAD_GAP = 4
/** The least room left above, between and below the blocks; the rules sit in it. */
const MIN_GAP = 12

function loadImage(src: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const image = new Image()
    image.crossOrigin = "anonymous"
    image.onload = () => resolve(image)
    image.onerror = () => resolve(null)
    image.src = src
  })
}

/**
 * The part of one block's height that grows and shrinks with its type, at full
 * size. The two fixed gaps inside a block are BLOCK_FIXED, and do not scale.
 */
function blockTextHeight(block: BackBlock): number {
  const s = block.scale ?? 1
  return (HEADING_SIZE * 1.3 + LEAD_SIZE * 1.3 + block.steps.length * STEP_SIZE * STEP_LINE) * s
}

const BLOCK_FIXED = HEADING_GAP + LEAD_GAP

/**
 * Paint the reverse once, at the same pixel size and supersampling as the front
 * (3x = 300dpi at the printed 4.25 x 5.5in), and hand back a data URL ready for
 * jsPDF. One bitmap serves every card on the sheet.
 */
export async function buildCardBack(scale = 3): Promise<string> {
  const canvas = document.createElement("canvas")
  canvas.width = CARD_WIDTH * scale
  canvas.height = CARD_HEIGHT * scale
  const ctx = canvas.getContext("2d")
  if (!ctx) throw new Error("Could not get a 2d context for the card back.")
  ctx.scale(scale, scale)
  ctx.fillStyle = "#ffffff"
  ctx.fillRect(0, 0, CARD_WIDTH, CARD_HEIGHT)

  const centre = CARD_WIDTH / 2
  let y = EDGE_TOP

  const logo = await loadImage(LOGO_SRC)
  if (logo) {
    const height = (LOGO_WIDTH / logo.width) * logo.height
    ctx.drawImage(logo, centre - LOGO_WIDTH / 2, y, LOGO_WIDTH, height)
    y += height
  }

  // One gap above each block and one below the last.
  const bottom = CARD_HEIGHT - EDGE_BOTTOM
  const room = bottom - y
  const gaps = BACK_BLOCKS.length + 1
  const text = BACK_BLOCKS.reduce((sum, block) => sum + blockTextHeight(block), 0)
  const fixed = BACK_BLOCKS.length * BLOCK_FIXED

  // Shrink the type, all blocks alike, only as far as it takes for every gap
  // to keep MIN_GAP. Never enlarge it: with room to spare, the spare goes into
  // the gaps instead.
  const fit = Math.min(1, (room - gaps * MIN_GAP - fixed) / text)

  // Share whatever room is left between the blocks and the rules, so the card
  // fills its height evenly instead of bunching at the top.
  const slack = Math.max(0, room - fixed - text * fit)
  const gap = slack / gaps

  ctx.textBaseline = "middle"
  BACK_BLOCKS.forEach((block, index) => {
    y += gap
    if (index > 0) {
      // the rule sits in the middle of the gap above this block
      ctx.strokeStyle = RULE
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.moveTo(SIDE + 34, y - gap / 2)
      ctx.lineTo(CARD_WIDTH - SIDE - 34, y - gap / 2)
      ctx.stroke()
    }

    const s = (block.scale ?? 1) * fit
    ctx.textAlign = "center"
    ctx.direction = block.direction
    ctx.fillStyle = INK

    ctx.font = `700 ${HEADING_SIZE * s}px ${block.font}`
    ctx.fillText(block.heading, centre, y + (HEADING_SIZE * s * 1.3) / 2)
    y += HEADING_SIZE * s * 1.3 + HEADING_GAP

    ctx.font = `600 ${LEAD_SIZE * s}px ${block.font}`
    ctx.fillText(block.lead, centre, y + (LEAD_SIZE * s * 1.3) / 2)
    y += LEAD_SIZE * s * 1.3 + LEAD_GAP

    ctx.font = `400 ${STEP_SIZE * s}px ${block.font}`
    for (const step of block.steps) {
      ctx.fillText(step, centre, y + (STEP_SIZE * s * STEP_LINE) / 2)
      y += STEP_SIZE * s * STEP_LINE
    }
  })

  ctx.direction = "ltr"
  return canvas.toDataURL("image/jpeg", 0.92)
}
