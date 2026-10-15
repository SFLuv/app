/**
 * The printed card's reverse: the same redemption steps as the front, in
 * Spanish, Traditional Chinese and Arabic.
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
 * low-resolution bitmap the reference embedded it as. Do not reword any of it
 * without a new translation — these are the only instructions a volunteer who
 * does not read English gets.
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

const LOGO_WIDTH = 92

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
 * The three translations, verbatim from the printed reference.
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
]

const HEADING_SIZE = 15
const LEAD_SIZE = 12.5
const STEP_SIZE = 11.5
const STEP_LINE = 1.42
const HEADING_GAP = 7
const LEAD_GAP = 6

function loadImage(src: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const image = new Image()
    image.crossOrigin = "anonymous"
    image.onload = () => resolve(image)
    image.onerror = () => resolve(null)
    image.src = src
  })
}

/** One block's height, so the three can be spaced against the real remaining room. */
function blockHeight(block: BackBlock): number {
  const s = block.scale ?? 1
  return (
    HEADING_SIZE * s * 1.3 + HEADING_GAP +
    LEAD_SIZE * s * 1.3 + LEAD_GAP +
    block.steps.length * STEP_SIZE * s * STEP_LINE
  )
}

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

  // Share whatever room is left between the three blocks and the two rules, so
  // the card fills its height evenly instead of bunching at the top.
  const bottom = CARD_HEIGHT - EDGE_BOTTOM
  const blocks = BACK_BLOCKS.map(blockHeight)
  const totalBlocks = blocks.reduce((sum, h) => sum + h, 0)
  const gaps = BACK_BLOCKS.length // one above each block, plus one before the first rule's partner
  const slack = Math.max(0, bottom - y - totalBlocks)
  const gap = slack / (gaps + 1)

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

    const s = block.scale ?? 1
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
