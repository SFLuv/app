/**
 * Geometry for every SFLuv QR code.
 *
 * This is the web port of the mobile app's SfluvQRCode renderer, which is now
 * the house style: near-black dot modules, brand-coral finder patterns drawn as
 * a stroked rounded ring around a rounded centre, and the SFLuv mark sitting on
 * cleared white in the middle. We draw the modules ourselves because no
 * off-the-shelf component expresses that combination, and because a single
 * implementation is the only way the printed card, the wallet sheets, the
 * merchant codes and the phone can be guaranteed to look identical.
 *
 * Plain CommonJS on purpose: the React component imports it, and the preview
 * generator requires it from Node. One source of truth, no mirrored maths.
 *
 * Keep in sync with mobile/src/components/SfluvQRCode.tsx.
 */
const QRCode = require("qrcode")
const {
  MASK_GRID,
  MASK_ROWS,
  MARK_CENTROID_OFFSET_X,
  MARK_CENTROID_OFFSET_Y,
} = require("./qr-logo-mask.js")

/** The logo stays inside this level's damage budget. */
const ERROR_CORRECTION = "M"
const MODULE_COLOR = "#161616"
const EYE_COLOR = "#eb6c6c"
/** Quiet zone, in modules. */
const QUIET_MODULES = 2
/**
 * Dot radius, in modules.
 *
 * 0.375 is what react-qrcode-logo drew (cellSize/2 scaled to 75%), and it is
 * the spacing we want: at the mobile renderer's original 0.46 the dots are 0.92
 * of a cell and nearly touch, which reads as a muddy field. At 0.375 each dot
 * is three quarters of its cell with a clean quarter-module of air around it.
 */
const DOT_RADIUS = 0.375
const LOGO_SRC = "/icon.png"

/** Logo box width as a share of the full drawing. */
const LOGO_RATIO = 0.24

/**
 * Clear space between the mark's ink and the nearest drawn dot, in modules.
 *
 * The clearing follows the mark's silhouette rather than a circle, so this is a
 * genuine even margin all the way round a letterform, not the radius of a hole
 * that has to be wide enough for the mark's widest point. That is why the shape
 * can carry a real moat without swallowing a chunk of the code: a circle giving
 * the same visual margin removes far more modules.
 */
const LOGO_MOAT = 0.85

/** Field padding beyond the logo box, as a fraction of the box. */
const FIELD_PAD = 0.4

/** True for the 7x7 finder patterns, which are drawn as shapes, not dots. */
function isFinderModule(row, col, count) {
  return (
    (row < 7 && col < 7) ||
    (row < 7 && col >= count - 7) ||
    (row >= count - 7 && col < 7)
  )
}

/**
 * Distance from every point near the logo to the mark's nearest ink, in mask
 * cells. Built once and kept: it depends only on the baked-in silhouette.
 *
 * Two-pass chamfer, which lands within a couple of percent of true Euclidean —
 * far finer than a moat needs.
 */
let cachedField = null
function inkDistanceField() {
  if (cachedField) return cachedField

  const pad = Math.round(MASK_GRID * FIELD_PAD)
  const span = MASK_GRID + pad * 2
  const field = new Float32Array(span * span).fill(span * 4)

  /*
   * Seed from the mark's FILLED silhouette: its ink, plus everything the ink
   * encloses.
   *
   * Seeding from ink alone measures distance to the nearest stroke, which is
   * the wrong question inside a counter. The middle of an enclosed space can
   * sit further from every stroke than the moat demands, so the dot there
   * survives and prints inside the logo — and whether it does depends on where
   * the module grid happens to land, which is why it only showed at some
   * sizes. Flooding the background inwards from the border marks everything
   * genuinely outside the mark; whatever the flood cannot reach is interior,
   * and is cleared exactly like ink.
   */
  const isInk = (x, y) => {
    const maskX = x - pad
    const maskY = y - pad
    return (
      maskX >= 0 &&
      maskY >= 0 &&
      maskX < MASK_GRID &&
      maskY < MASK_GRID &&
      MASK_ROWS[maskY].charCodeAt(maskX) === 49 /* "1" */
    )
  }

  // 4-connected background, the complement of 8-connected ink: a diagonal seam
  // in a stroke must not let the flood leak into the space it encloses.
  const outside = new Uint8Array(span * span)
  const queue = [0]
  outside[0] = 1
  while (queue.length > 0) {
    const index = queue.pop()
    const x = index % span
    const y = (index - x) / span
    const visit = (nextX, nextY) => {
      if (nextX < 0 || nextY < 0 || nextX >= span || nextY >= span) return
      const next = nextY * span + nextX
      if (outside[next] === 1 || isInk(nextX, nextY)) return
      outside[next] = 1
      queue.push(next)
    }
    visit(x + 1, y)
    visit(x - 1, y)
    visit(x, y + 1)
    visit(x, y - 1)
  }

  for (let y = 0; y < span; y++) {
    for (let x = 0; x < span; x++) {
      const index = y * span + x
      if (isInk(x, y) || outside[index] === 0) field[index] = 0
    }
  }

  const diag = Math.SQRT2
  const relax = (index, from, cost) => {
    const candidate = field[from] + cost
    if (candidate < field[index]) field[index] = candidate
  }
  for (let y = 0; y < span; y++) {
    for (let x = 0; x < span; x++) {
      const i = y * span + x
      if (y > 0) relax(i, i - span, 1)
      if (x > 0) relax(i, i - 1, 1)
      if (y > 0 && x > 0) relax(i, i - span - 1, diag)
      if (y > 0 && x < span - 1) relax(i, i - span + 1, diag)
    }
  }
  for (let y = span - 1; y >= 0; y--) {
    for (let x = span - 1; x >= 0; x--) {
      const i = y * span + x
      if (y < span - 1) relax(i, i + span, 1)
      if (x < span - 1) relax(i, i + 1, 1)
      if (y < span - 1 && x < span - 1) relax(i, i + span + 1, diag)
      if (y < span - 1 && x > 0) relax(i, i + span - 1, diag)
    }
  }

  cachedField = { field, span, pad }
  return cachedField
}

/**
 * Resolves a value into ready-to-emit shapes in module units. Callers only
 * iterate and render, so an SVG built in React and one built as a string in
 * Node cannot disagree.
 *
 * @param {string} value
 * @returns {{units:number,count:number,dots:{cx:number,cy:number,r:number}[],
 *   eyes:{x:number,y:number,width:number,height:number,rx:number,fill:string,
 *   stroke:string,strokeWidth:number}[],
 *   logo:{sizePercent:string,leftPercent:string,topPercent:string},
 *   clearedModules:number}}
 */
function buildQRDrawing(value) {
  const modules = QRCode.create(value, { errorCorrectionLevel: ERROR_CORRECTION }).modules
  const count = modules.size
  const units = count + QUIET_MODULES * 2

  const boxSide = units * LOGO_RATIO
  // Place the mark on its optical centre. icon.png's artwork sits high inside
  // its own frame, so centring the frame leaves the mark visibly above centre;
  // shifting by the ink centroid puts the visual mass where it belongs.
  const centreX = units / 2 - MARK_CENTROID_OFFSET_X * boxSide
  const centreY = units / 2 - MARK_CENTROID_OFFSET_Y * boxSide
  const boxLeft = centreX - boxSide / 2
  const boxTop = centreY - boxSide / 2

  const { field, span, pad } = inkDistanceField()
  // A dot is dropped when any part of it would reach into the moat, so the
  // clearing never leaves a module sliced in half against the mark.
  const clearance = LOGO_MOAT + DOT_RADIUS

  const dots = []
  let clearedModules = 0
  for (let row = 0; row < count; row += 1) {
    for (let col = 0; col < count; col += 1) {
      if (!modules.data[row * count + col] || isFinderModule(row, col, count)) continue
      const cx = QUIET_MODULES + col + 0.5
      const cy = QUIET_MODULES + row + 0.5

      const fx = Math.round(((cx - boxLeft) / boxSide) * MASK_GRID) + pad
      const fy = Math.round(((cy - boxTop) / boxSide) * MASK_GRID) + pad
      if (fx >= 0 && fy >= 0 && fx < span && fy < span) {
        // Cell distance -> fraction of the logo box -> modules.
        const distanceInModules = (field[fy * span + fx] / MASK_GRID) * boxSide
        if (distanceInModules < clearance) {
          clearedModules += 1
          continue
        }
      }
      dots.push({ cx, cy, r: DOT_RADIUS })
    }
  }

  const eyes = []
  for (const [row, col] of [[0, 0], [0, count - 7], [count - 7, 0]]) {
    const x = QUIET_MODULES + col
    const y = QUIET_MODULES + row
    // Outer ring: a rounded square stroked one module wide.
    eyes.push({
      x: x + 0.5, y: y + 0.5, width: 6, height: 6, rx: 1.75,
      fill: "none", stroke: EYE_COLOR, strokeWidth: 1,
    })
    eyes.push({
      x: x + 2, y: y + 2, width: 3, height: 3, rx: 0.9,
      fill: EYE_COLOR, stroke: "none", strokeWidth: 0,
    })
  }

  return {
    units,
    count,
    dots,
    eyes,
    clearedModules,
    logo: {
      sizePercent: `${LOGO_RATIO * 100}%`,
      leftPercent: `${(boxLeft / units) * 100}%`,
      topPercent: `${(boxTop / units) * 100}%`,
    },
  }
}

module.exports = {
  buildQRDrawing,
  isFinderModule,
  inkDistanceField,
  ERROR_CORRECTION,
  MODULE_COLOR,
  EYE_COLOR,
  QUIET_MODULES,
  DOT_RADIUS,
  LOGO_RATIO,
  LOGO_MOAT,
  LOGO_SRC,
}
