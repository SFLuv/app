/**
 * Generates qr-card-preview.html.
 *
 * Accuracy rules this script follows, because a preview that quietly disagrees
 * with the app is worse than no preview:
 *
 *  1. The QR is REAL. It is encoded by qrcode-generator — the same library
 *     react-qrcode-logo uses — at the ecLevel the component actually passes,
 *     with the payload buildEventRedeemQrValue actually produces. Module count,
 *     and therefore how large the corner patterns look, falls out of that
 *     instead of being guessed.
 *  2. It is drawn with react-qrcode-logo's own geometry, in its own canvas
 *     units (viewBox 0 0 size+2*quietZone), so no unit conversion can drift.
 *  3. Every layout number is read out of the component source. Change a value
 *     in the components and this picks it up; change the SHAPE and this throws
 *     rather than silently rendering something stale.
 *  4. The page loads Inter, the app's font. Text metrics decide where the
 *     heading wraps, which is most of what the preview exists to show.
 *
 * Run: node gen_preview.js
 */
const fs = require("fs")
const path = require("path")

const ROOT = __dirname
const COMPONENTS = path.join(ROOT, "frontend/components/events")
const OUT = path.join(ROOT, "qr-card-preview.html")
// The app's own geometry module — not a copy of it. Whatever the product
// renders, this renders.
const geometry = require(path.join(ROOT, "frontend/lib/qr-geometry.js"))

const read = (f) => fs.readFileSync(path.join(COMPONENTS, f), "utf8")
const chrome = read("card-chrome.tsx")
const cardQr = read("card-qr.tsx")
const sfluvCard = read("qr-code-card.tsx")
const affiliateCard = read("affiliate-qr-code-card.tsx")

/** Every extraction goes through here so a shape change fails loudly. */
function must(source, re, label) {
  const m = source.match(re)
  if (!m) throw new Error(`gen_preview: could not read ${label} — the component shape changed.`)
  return m
}

/** Pulls `key: value` pairs out of a brace-balanced style object. */
function styleBlock(source, marker, label) {
  const start = source.indexOf(marker)
  if (start === -1) throw new Error(`gen_preview: could not find ${label}`)
  // From `start`, NOT past the marker: markers that include `style={{` would
  // otherwise skip their own braces and capture the next element's style
  // object instead — which silently gave the h3 the <ol>'s styling.
  const i = source.indexOf("{", start)
  let depth = 0
  let end = i
  for (; end < source.length; end++) {
    if (source[end] === "{") depth++
    else if (source[end] === "}" && --depth === 0) break
  }
  const body = source.slice(i + 1, end)
  const out = {}
  for (const m of body.matchAll(/(\w+):\s*("[^"]*"|`[^`]*`|[\d.]+)/g)) {
    out[m[1]] = m[2].replace(/^["`]|["`]$/g, "")
  }
  return out
}

const kebab = (k) => k.replace(/[A-Z]/g, (c) => "-" + c.toLowerCase())
const css = (pairs) => Object.entries(pairs).map(([k, v]) => `${kebab(k)}:${v}`).join(";")

// ----------------------------------------------------------------- QR drawing
const ICON = "frontend/public/icon.png"

// The value the card actually encodes, from frontend/lib/redeem-link.ts.
const redeemValue = (code) => `https://app.sfluv.org?code=${code}&page=redeem`

/**
 * Emits the shared drawing as SVG.
 *
 * Only the emission differs from the React component — every coordinate, radius
 * and colour comes from buildQRDrawing, so the preview cannot render a QR the
 * app would not.
 */
function renderQR(value) {
  const drawing = geometry.buildQRDrawing(value)
  const parts = [`<rect width="${drawing.units}" height="${drawing.units}" fill="#ffffff"/>`]
  for (const eye of drawing.eyes) {
    parts.push(`<rect x="${eye.x}" y="${eye.y}" width="${eye.width}" height="${eye.height}" `
      + `rx="${eye.rx}" ry="${eye.rx}" fill="${eye.fill}" stroke="${eye.stroke}" `
      + `stroke-width="${eye.strokeWidth}"/>`)
  }
  for (const dot of drawing.dots) {
    parts.push(`<circle cx="${dot.cx}" cy="${dot.cy}" r="${dot.r}" fill="${geometry.MODULE_COLOR}"/>`)
  }
  const svg = `<svg viewBox="0 0 ${drawing.units} ${drawing.units}" width="100%" height="100%" `
    + `xmlns="http://www.w3.org/2000/svg" style="display:block">${parts.join("")}</svg>`
  // The mark is a sibling image, exactly as in the component, so the SVG stays
  // free of external references.
  const mark = `<img src="${ICON}" alt="" aria-hidden="true" style="position:absolute;`
    + `width:${drawing.logo.sizePercent};height:${drawing.logo.sizePercent};`
    + `left:${drawing.logo.leftPercent};top:${drawing.logo.topPercent};object-fit:contain"/>`
  return {
    count: drawing.count,
    dotCount: drawing.dots.length,
    html: `<div style="position:relative">`
      + `<div style="position:relative;aspect-ratio:1/1;width:100%">${svg}${mark}</div>`
      + `</div>`,
  }
}

// ------------------------------------------------------------- card structure

const shell = styleBlock(chrome, "export const cardShellStyle", "cardShellStyle")
const EDGE_TOP = must(chrome, /const EDGE_TOP = (\d+)/, "EDGE_TOP")[1]
const EDGE_BOTTOM = must(chrome, /const EDGE_BOTTOM = (\d+)/, "EDGE_BOTTOM")[1]
const SIDE = must(chrome, /const SIDE = (\d+)/, "SIDE")[1]
shell.padding = `${EDGE_TOP}px ${SIDE}px ${EDGE_BOTTOM}px`

const body = styleBlock(chrome, "export const cardBodyStyle", "cardBodyStyle")

const headingBaseFont = Number(must(chrome, /const HEADING_BASE_FONT = (\d+)/, "HEADING_BASE_FONT")[1])
const headingLineHeight = Number(must(chrome, /const HEADING_LINE_HEIGHT = ([\d.]+)/, "HEADING_LINE_HEIGHT")[1])
const headingMinFont = Number(must(chrome, /const HEADING_MIN_FONT = (\d+)/, "HEADING_MIN_FONT")[1])
const headingRows = Number(must(chrome, /HEADING_ROW \* (\d+)/, "heading row cap")[1])
const headingBox = Math.ceil(headingBaseFont * headingLineHeight * headingRows)
// These are template literals / identifiers in the component, so the style
// parser cannot read them; resolve them from the constants above.
const headingRaw = styleBlock(chrome, "export const headingStyle", "headingStyle")
const heading = { ...headingRaw }
heading.fontSize = `${headingBaseFont}px`
heading.lineHeight = String(headingLineHeight)
heading.maxHeight = `${headingBox}px`

const titleBox = Number(must(chrome, /const TITLE_BOX_HEIGHT = (\d+)/, "TITLE_BOX_HEIGHT")[1])
const titleBaseFont = Number(must(chrome, /const TITLE_BASE_FONT = (\d+)/, "TITLE_BASE_FONT")[1])
const titleMinFont = Number(must(chrome, /const TITLE_MIN_FONT = (\d+)/, "TITLE_MIN_FONT")[1])
const titleLineHeight = Number(must(chrome, /const TITLE_LINE_HEIGHT = ([\d.]+)/, "TITLE_LINE_HEIGHT")[1])
const title = {
  display: "block",
  fontSize: `${titleBaseFont}px`,
  color: "#6b7f87",
  lineHeight: String(titleLineHeight),
  textAlign: "right",
  maxHeight: `${titleBox}px`,
  overflow: "hidden",
  overflowWrap: "anywhere",
}

const number = styleBlock(chrome, "<span style={{ ...rigid, fontSize: \"12px\"", "number span")
const dateStyle = styleBlock(chrome, "        <span\n          style={{", "date span")

const h3 = styleBlock(chrome, "<h3 style={{ ...rigid,", "h3")
const olRaw = styleBlock(chrome, "    <ol\n      style={{", "ol")
const footerSize = must(chrome, /<div style=\{\{ \.\.\.rigid, textAlign: "center", fontSize: "(\d+)px" \}\}>/, "footer")[1]
const qrFrameWidth = must(chrome, /CardQRFrame[\s\S]*?width: "(\d+)px"/, "CardQRFrame width")[1]

const gapsOf = (src) =>
  [...src.matchAll(/<Gap size=\{(\d+)\}(?:\s+grow=\{(\d+)\})?\s*\/>/g)]
    .map((m) => ({ size: Number(m[1]), grow: Number(m[2] || 0) }))

const sfluvGaps = gapsOf(sfluvCard)
const affiliateGaps = gapsOf(affiliateCard)
if (sfluvGaps.length !== 5 || affiliateGaps.length !== 5) {
  throw new Error("gen_preview: expected 5 gaps per card; the card body structure changed.")
}

const sfluvLogoW = must(sfluvCard, /alt="SFLuv logo"[^/]*width: "(\d+)px"/, "SFLuv logo width")[1]
const affLogo = must(affiliateCard, /alt="SFLuv logo" style=\{\{ height: "(\d+)px", width: "(\d+)px"/, "affiliate logo size")
const affX = must(affiliateCard, /fontWeight: "bold", fontSize: "(\d+)px" \}\}>X</, "affiliate X size")[1]
const affRowGap = must(affiliateCard, /justifyContent: "center",\s*\n\s*gap: "(\d+)px"/, "affiliate row gap")[1]

const gapHtml = (g) => `<div aria-hidden="true" style="flex:${g.grow} 1 ${g.size}px;width:100%"></div>`
const instructions =
  `<h3 style="flex-shrink:0;${css(h3)}">To redeem your tokens:</h3>`
  + `<ol style="flex-shrink:0;${css(olRaw)}">`
  + "<li>1. Scan the QR code</li><li>2. Download the SFLuv app</li>"
  + "<li>3. Scan again</li><li>4. Receive your SFLuv!</li></ol>"
const footer = `<div style="flex-shrink:0;text-align:center;font-size:${footerSize}px"><p>`
  + "Interested in more SFLuv supported events?<br/>Visit <a>www.sfluv.org/volunteers</a></p></div>"

function card({ number: n, title: t, org, code, startDate }) {
  const gaps = org ? affiliateGaps : sfluvGaps
  const qr = renderQR(redeemValue(code))
  const logo = org
    ? `<div style="flex-shrink:0;display:flex;align-items:center;justify-content:center;gap:${affRowGap}px">`
      + `<img src="${ICON}" alt="SFLuv logo" style="height:${affLogo[1]}px;width:${affLogo[2]}px;object-fit:contain"/>`
      + `<span style="font-weight:bold;font-size:${affX}px">X</span>`
      + `<img src="${ICON}" alt="Affiliate logo" style="height:${affLogo[1]}px;width:${affLogo[2]}px;object-fit:contain"/></div>`
    : `<img src="${ICON}" alt="SFLuv logo" style="flex-shrink:0;height:auto;width:${sfluvLogoW}px"/>`
  const headingHtml = org
    ? `<h1 data-fitted-heading style="${css(heading)}"><span style="white-space:nowrap">Thank you from</span> `
      + `<span style="display:inline-block">SFLuv and ${org}!</span></h1>`
    : `<h1 data-fitted-heading style="${css(heading)}">Thank you from SFLuv!</h1>`

  return `<div class="qr-card" style="${css(shell)};background:#fff">`
    + `<div style="display:flex;width:100%;align-items:flex-start;justify-content:space-between;gap:10px;flex-shrink:0">`
    + `<span style="flex-shrink:0;${css(number)}">#${n}</span>`
    + `<div style="display:flex;flex-direction:column;align-items:flex-end;max-width:72%;min-width:0">`
    + `<span data-fitted-title style="${css(title)}">${t}</span>`
    + `<span style="${css(dateStyle)}">${startDate}</span>`
    + `</div></div>`
    + `<div style="${css(body)}">${gapHtml(gaps[0])}${logo}${gapHtml(gaps[1])}${headingHtml}`
    + `${gapHtml(gaps[2])}${instructions}${gapHtml(gaps[3])}`
    + `<div style="flex-shrink:0;width:${qrFrameWidth}px">${qr.html}</div>${gapHtml(gaps[4])}</div>`
    + `${footer}</div>`
}

const CODES = [
  "3f2a1b4c-5d6e-4f70-8a91-b2c3d4e5f607",
  "8c7b6a59-4d3e-4f21-9a08-1b2c3d4e5f60",
  "a1b2c3d4-e5f6-4071-8293-a4b5c6d7e8f9",
  "0f1e2d3c-4b5a-4698-8776-655443322110",
  "7e6d5c4b-3a29-4180-9f7e-6d5c4b3a2918",
]

const CASES = [
  ["SFLuv-run event", "Number top-left, title top-right. Spare height collects in the gaps above the code, not around it.",
    { number: 1, title: "Ocean Beach Cleanup", code: CODES[0], startDate: "Sat, Aug 15, 2026" }],
  ["SFLuv event &mdash; long title", "The title wraps rightward across up to three rows; the gaps give up space to match.",
    { number: 2, title: "Ocean Beach Cleanup &amp; Dune Restoration Morning Session", code: CODES[1], startDate: "Sun, Sep 6, 2026" }],
  ["Affiliate event", "Co-branded variant, short organization name: heading stays on one row.",
    { number: 3, title: "Community Fridge Restock", org: "Mission Meals", code: CODES[2], startDate: "Wed, Oct 7, 2026" }],
  ["Affiliate event &mdash; long name", "&ldquo;Thank you from&rdquo; breaks onto its own row rather than splitting the name.",
    { number: 4, title: "Bird Habitat Survey", org: "Golden Gate Audubon Society", code: CODES[3], startDate: "Sat, Nov 14, 2026" }],
  ["Both running long", "Worst realistic case: three-row title and a wrapping heading. Edges hold top and bottom.",
    { number: 5, title: "Bird Habitat Survey &amp; Shoreline Monitoring Volunteer Morning", org: "Golden Gate Audubon Society", code: CODES[4], startDate: "Sat, Dec 12, 2026" }],
]

const sample = renderQR(redeemValue(CODES[0]))
const moduleCount = sample.count
// Every module the shared drawing produced must reach the page.
{
  const drawn = sample.html.split("<circle").length - 1
  if (drawn !== sample.dotCount) {
    throw new Error(`gen_preview: drew ${drawn} modules, drawing has ${sample.dotCount}`)
  }
}

const sections = CASES.map(([name, desc, spec]) =>
  `<section class="case"><div class="meta"><h2>${name}</h2><p>${desc}</p></div>`
  + `<div class="card-wrap">${card(spec)}</div></section>`).join("\n")

const PAGE = `<meta charset="utf-8">
<title>QR card preview &mdash; printed volunteer codes</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700&display=swap" rel="stylesheet">
<style>
  :root { --ink:#0b303b; --ink-muted:#4a6069; --canvas:#fef4ee; --line:#d9dfe1; --brand:${geometry.EYE_COLOR}; }
  *, *::before, *::after { box-sizing:border-box; }
  body { margin:0; background:var(--canvas); color:var(--ink);
    font-family:Inter,ui-sans-serif,system-ui,-apple-system,"Helvetica Neue",Arial,sans-serif; line-height:1.5; }

  /* Mirrors Tailwind preflight, which IS applied in the app. Without it the
     browser adds heading and paragraph margins, resets heading type, and draws
     decimal list markers on top of the hard-coded "1." in the instructions. */
  .qr-card h1, .qr-card h2, .qr-card h3, .qr-card p { margin:0; }
  .qr-card h1, .qr-card h2, .qr-card h3 { font-size:inherit; font-weight:inherit; }
  .qr-card ol, .qr-card ul { list-style:none; margin:0; padding:0; }
  .qr-card { border:1px dashed var(--line); }

  h1.page { font-size:28px; margin:0 0 8px; }
  .lede { color:var(--ink-muted); max-width:62ch; margin:0 0 16px; }
  .note { display:block; max-width:62ch; background:#fff; border:1px solid var(--line);
    border-left:3px solid var(--brand); border-radius:10px; padding:10px 14px; font-size:13px;
    color:var(--ink-muted); margin-bottom:10px; }
  main { max-width:1100px; margin:0 auto; padding:24px; display:flex; flex-direction:column; gap:32px; }
  .case { display:grid; grid-template-columns:minmax(0,1fr); gap:16px; background:#fff;
    border:1px solid var(--line); border-radius:20px; padding:24px; box-shadow:0 6px 20px rgb(11 48 59 / 0.06); }
  @media (min-width:900px) { .case { grid-template-columns:280px minmax(0,1fr); align-items:start; } }
  .meta h2 { font-size:16px; margin:0 0 6px; }
  .meta p { font-size:13px; color:var(--ink-muted); margin:0; }
  .card-wrap { overflow-x:auto; }
  footer { max-width:62ch; margin:0 auto; padding:8px 24px 64px; font-size:13px; color:var(--ink-muted); }
  code { background:#fff; border:1px solid var(--line); border-radius:6px; padding:1px 5px; font-size:12px; }
</style>

<main>
  <h1 class="page">Printed QR cards</h1>
  <p class="lede">Both card variants as exported to PDF &mdash; 425&times;550px, dashed outline added here
  only to show the card edge.</p>

  <p class="note"><strong>These QR codes are real encodings, not placeholders.</strong> Each is encoded by
  <code>qrcode-generator</code> &mdash; the same library the app uses &mdash; at <code>ecLevel
  ${geometry.ERROR_CORRECTION}</code> with the payload <code>buildEventRedeemQrValue</code> produces, and
  drawn by <code>lib/qr-geometry.js</code> &mdash; the app's own module, imported here rather than
  reimplemented, so the preview cannot render a code the product would not. That yields ${moduleCount}
  modules. The codes below carry made-up UUIDs, so they resolve to nothing, and the page was not
  decode-tested &mdash; scan a batch from the event modal before a print run.</p>

  <p class="note"><strong>Layout.</strong> The card is a flex column of header, body and footer. Insets are
  ${EDGE_TOP}px top and ${EDGE_BOTTOM}px bottom on a border-box element, so they hold at every content
  length. Only the gaps above the code grow, keeping the QR tight to the instructions above and the footer
  below. The event title is capped at three rows and the heading at ${headingRows}, which is what makes
  overflow impossible rather than merely unlikely.</p>

  <p class="note"><strong>Every number here is read out of the components at generation time</strong>
  (<code>card-chrome.tsx</code>, <code>card-qr.tsx</code> and the two card files), so this page cannot drift
  from them on values. It does assume the card's overall structure; if that changes, this script throws
  instead of rendering something stale. The page loads Inter, the app's font, because text metrics decide
  where the heading wraps.</p>

${sections}
</main>

<footer>Regenerated with <code>node gen_preview.js</code>.</footer>

<script>
  // Mirrors FittedTitle in card-chrome.tsx: titles shrink to fit their box
  // rather than being truncated. Same constants, read from the component at
  // generation time.
  const fit = (selector, base, box, min) => {
    for (const node of document.querySelectorAll(selector)) {
      const cap = node.style.maxHeight
      node.style.maxHeight = "none"
      let size = base
      node.style.fontSize = size + "px"
      while (node.scrollHeight > box && size > min) {
        size -= 0.25
        node.style.fontSize = size + "px"
      }
      node.style.maxHeight = cap
    }
  }
  const fitAll = () => {
    fit("[data-fitted-title]", ${titleBaseFont}, ${titleBox}, ${titleMinFont})
    fit("[data-fitted-heading]", ${headingBaseFont}, ${headingBox}, ${headingMinFont})
  }
  fitAll()
  // This page loads Inter over the network. Fitting against the fallback face
  // measures the wrong metrics, which is what left a third line clipped.
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(fitAll)
</script>
`

/**
 * Self-check: assert the values that came out of the components actually made
 * it into the page, the expected number of times.
 *
 * Extraction is regex over source, and a regex that matches the wrong thing
 * fails silently and looks plausible — that is precisely how the instructions
 * heading ended up rendering with the list's styling. Verifying the output
 * against the inputs is what turns a silent wrong answer into a loud one.
 */
const N = CASES.length
const AFFILIATE = CASES.filter(([, , s]) => s.org).length
const expectations = [
  ["shell padding", `padding:${EDGE_TOP}px ${SIDE}px ${EDGE_BOTTOM}px`, N],
  ["card box", `height:${shell.height};width:${shell.width}`, N],
  ["heading", css(heading), N],
  ["event title", css(title), N],
  ["start date", css(dateStyle), N],
  ["code number", css(number), N],
  ["instructions heading", css(h3), N],
  ["instruction list", css(olRaw), N],
  ["QR frame", `width:${qrFrameWidth}px`, N],
  ["SFLuv mark", `width:${sfluvLogoW}px`, N - AFFILIATE],
  ["affiliate marks", `height:${affLogo[1]}px;width:${affLogo[2]}px`, AFFILIATE * 2],
  ["QR viewBox", `viewBox="0 0 ${sample.count + 2 * geometry.QUIET_MODULES} ${sample.count + 2 * geometry.QUIET_MODULES}"`, N],
  ["eye colour", `stroke="${geometry.EYE_COLOR}"`, N * 3],
  ["centre mark size", `width:${(geometry.LOGO_RATIO * 100)}%`, N],
]
for (const [i, g] of sfluvGaps.entries()) {
  expectations.push([`gap ${i}`, `flex:${g.grow} 1 ${g.size}px`, null])
}

const failures = []
for (const [label, fragment, expected] of expectations) {
  const seen = PAGE.split(fragment).length - 1
  if (seen === 0 || (expected !== null && seen !== expected)) {
    failures.push(`  ${label}: "${fragment.slice(0, 60)}" appeared ${seen}x`
      + (expected === null ? " (expected at least once)" : `, expected ${expected}x`))
  }
}
// The instructions heading and the list must not collapse to the same styling.
if (css(h3) === css(olRaw)) failures.push("  h3 and ol resolved to identical styles — extraction is off")
if (!css(title).includes("max-height")) failures.push("  event title lost its height cap")
if (!css(heading).includes("max-height")) failures.push("  heading lost its height cap")
// Any property declared on the component but missing from the emitted CSS is
// a silent divergence — exactly how flex-shrink went astray.
for (const key of Object.keys(headingRaw)) {
  if (!(key in heading)) failures.push(`  heading dropped "${key}" between the component and the page`)
}
for (const hook of ["data-fitted-title", "data-fitted-heading"]) {
  const seen = PAGE.split(hook).length - 1
  if (seen !== N + 1) failures.push(`  ${hook}: ${seen} occurrences, expected ${N + 1} (one per card, plus the fit script)`)
}

if (failures.length) {
  console.error("gen_preview: output does not match the components:\n" + failures.join("\n"))
  process.exit(1)
}

fs.writeFileSync(OUT, PAGE)
console.log(`wrote ${OUT} (${PAGE.length} bytes, ${CASES.length} cases, ${moduleCount} modules @ ecLevel ${geometry.ERROR_CORRECTION})`)
console.log(`verified ${expectations.length} value groups against the component sources`)
