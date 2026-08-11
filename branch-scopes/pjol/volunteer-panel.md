# Branch scope — `pjol/volunteer-panel`

Aug 5–10 2026 · app + mobile-app + webpage · **~32.4h active**

Hours are active working time inferred from commit clustering.

- **Round 1 (Aug 5–8) — 22.0h.** ~16h measured, adjusted upward for two large batch commits
  (`dd007b9` +3,098, `6dbf50c` +2,522) whose work landed with no intermediate commits.
- **Round 2 (Aug 9–10) — 10.4h.** Measured: 1.4h on the 9th (17:44–19:08) and 8.1h on the 10th
  (10:48–18:56), plus ~0.8h of work before the first commit of each day. An earlier revision of this
  file put the round at 10.5h while its last stretch was uncommitted and had to be estimated; the
  measured figure came in at 9.8h, and the QR export rewrite that closed the round added the last
  0.6h. Itemised to the nearest 0.1h, and the items sum to the measured figure — an earlier draft
  floored every item at 0.5h and reached 14.2h, which was the floor talking rather than the work.

---

# Round 1 — Aug 5–8

## Large features

### Volunteer events platform — 4.5h · app
- Event CRUD, approval queue, admin/affiliate request flows
- Recurrence (daily/weekly/monthly) with successor generation
- Faucet allocation on approval; QR code minting per event
- Org binding, org-scoped access, signup capacity tracking
- Replaced the legacy affiliate event + standing-allocation model

### QR code system overhaul — 3.5h · app + mobile
- Dropped `react-qrcode-logo`; own renderer shared across all surfaces
- Shared geometry module + generated logo silhouette mask
- Silhouette-shaped centre clearing, brand eyes, dotted modules
- Printed card redesign: layout, shrink-to-fit headings, code number + title + date
- Truthful preview generator that reads values from the components

### Mobile app UI overhaul — 2.8h · mobile
- Bottom bar reorganisation, activity tab, notifications tab
- Contacts restyle, volunteer panel cleanup
- Animation/visual polish passes, error handling

### Structured opening hours — 2.5h · app + mobile
- Free text → structured times; real time-picker inputs
- Split hours (lunch/dinner) end to end
- Nightly Google sync at midnight Pacific, DST-safe
- Manual-mode toggle; sync skips manual listings and thin responses
- Migrations 1.34/1.35 with parser-based backfill

### Event blasts & transactional email — 1.5h · app
- In-app composer with formatting, image attachments, server-rendered preview
- Signup confirmation emails; HTML-escaped inputs, house styling

### Admin merchant location editing — 1.0h · app
- Admin edit of name, type, address, hours, contact for any listing
- Google listing re-point (server-refetched, never client-supplied)

### Webpage volunteer portal — 0.8h · web
- Event carousel on home, event details restructure, sidebar

---

## Tweaks & fixes

| Item | Repo | Hours |
|---|---|---|
| Partners carousel admin panel | app | 0.7 |
| Workflow payout / step-rollover / `context canceled` fixes | app | 0.7 |
| Ponder hook reconciliation on boot | app | 0.5 |
| Value-ordering audit (value writes last) + refund bug | app | 0.5 |
| Merchant onboarding: reject address-only Google places | app | 0.4 |
| `dev-up.sh`: webpage boot, faucet provisioning, pg resolution | app | 0.4 |
| Legacy allocation/event system removal | app | 0.4 |
| Transaction memo chain-id divergence fix | app | 0.3 |
| Map marker flicker (unstable component identity) | app | 0.3 |
| Event route auth/spoofing audit + 3 findings fixed | app | 0.3 |
| Push notification subscription sync diagnosis | app | 0.3 |
| Numeric inputs: scroll-to-change, leading zeros | app | 0.2 |
| Drain faucet: typed confirmation + destructive styling | app | 0.2 |
| Current-day hours bolding in merchant popups | app + mobile | 0.2 |

**Subtotal 5.4h**

---

# Round 2 — Aug 9–10

## Large features

### Merchant map pins & icon system — 1.2h · app + mobile + web
- Backend: `location_icons` table (migration 1.37), upload / serve / delete endpoints, `icon_url` on every
  location payload, owner-or-admin writes, version-stamped URLs for cache busting
- Merchant-facing uploader: square crop enforced with drag/zoom, live pin preview, 512px re-encode
- Procedurally generated default icon (initials) for merchants with no logo
- One shared pin geometry across all three codebases; several rounds of iteration on silhouette, size,
  tip shape, colour split and optical centring

### Webpage merchant map — 0.8h · web
- Merchants API client with fixtures, types, mapping, config
- Map component, slide-up popup with click-away dismissal, fit-to-bounds framing
- Searchable, collapsible merchant list panel; map/list toggle below `lg`
- Placed on the home page (replacing "Why SFLuv") and inside the merchants page intro

### Open/closed merchant status — 0.5h · app + mobile + web
- Shared three-state hours logic in all three codebases: overnight spans, merchant timezone,
  "unknown" kept distinct from "closed"
- Pulsing open/closed indicator on all four merchant popup surfaces; pin colour follows state
- List ordering by open state, with proximity as the secondary sort

### Staged event photo uploads — 0.6h · app
- Migration 1.38: photos can exist before their event; owner-scoped staging
- Upload starts on file selection; attach happens inside the event's creation transaction
- Creation is now all-or-nothing — a photo that cannot be attached fails the whole event
- Progress gate on submit, per-photo status, orphan sweep for abandoned forms

### Web app map rework — 0.4h · app
- Map at two-thirds width with a searchable, collapsible merchant panel beside it
- List View tab removed; type filter moved to the page title row
- Fit-to-pins framing on load; merchant modal now animates out

### Webpage scroll-aware top bar — 0.4h · web
- Docks at the top, pins on a deliberate scroll up (half a viewport in a second), collapses in one go
- Gradient veil so content fades into the site background rather than sliding under the bar

### Admin events panel rework — 0.3h · app
- Pending-approval count as a sidebar tab badge, polled independently of the panel
- Faucet balance promoted into the panel header; standalone Faucet card removed
- Primary-coloured approval badge, action button on the title line, paginated list

### Mobile navigation & gesture fixes — 0.4h · mobile
- Blank event page after returning from an organizer's list
- Photo carousel no longer triggers the pane's back-swipe
- Detail rendered as an overlay so back-navigation reveals the list instead of flickering

### Event editing, admin and affiliate — 0.8h · app
- Create form reused as the edit form, so a field can never be creatable but not editable
- Prefill round-trips instants back to wall clock in the event's own timezone, and rebuilds the
  recurrence rule — reading only its summary would have turned a repeating event into a one-off
- Admin edits apply immediately; affiliate edits park for review, with approve/reject in the panel
- `pending_edit` added to the management payload: the approve/reject endpoints existed but nothing
  could discover there was anything to act on

### Printable QR code PDFs — 0.7h · app
- Replaced the CSV download with the card-rendered PDF export, so printed codes carry the current
  QR styling instead of a second, drifting design
- JSON codes endpoints for both panels, sharing one loader with the CSV so authorisation cannot drift
- Batched at 15 — the batch is what is mounted as well as what is written, which is what bounds peak
  memory on a large event

### QR card export redrawn on a canvas — 0.5h · app
- The export above screenshotted the React card with html2canvas. It clipped, and the clipping was not
  a sizing bug that could be tuned out: html2canvas clones the document and lays the text out with the
  clone's fonts, so a heading that wrapped to three rows on screen could wrap to four in the capture
  and lose the last one to the `overflow: hidden` holding the card at a fixed size. The corner title
  and the final instruction went the same way. Three rounds of fixes to the on-screen fit did not
  reach it, because the on-screen fit was never what was being printed.
- Cards are now drawn straight onto a canvas. Text is measured with `measureText` and wrapped before
  it is drawn, so nothing is ever laid out and then cropped — the failure mode the old path was built
  around cannot occur. Shrink-to-fit compares fractional measured heights rather than the integer
  `scrollHeight` the DOM version used, which reported a box overflowing by a fraction of a pixel as
  fitting exactly.
- The freeze went with it, for the same reason: the static half of the card is painted once per event
  and reused, leaving only the QR and the code number per code. No DOM is cloned, mounted or restyled,
  so the offscreen render target and the batching that bounded its memory are both gone.
- QR geometry comes from `lib/qr-geometry`, the module the on-screen code already uses, so the print
  and the app draw the same code from one source.
- **Printed page size changed**: the page format is now derived from the card's own 425×550 ratio,
  55mm × 71.2mm, rather than the previous 55 × 42.5mm which did not match the artwork and squashed it.

**Subtotal 6.6h**

---

## Fixes & smaller features

| Item | Repo | Hours |
|---|---|---|
| Affiliate event creation unblocked — `approved_by` NOT NULL, photo upload 403, pending-event image links | app | 0.5 |
| Card vertical budget made unfalsifiable — logo contained rather than width-scaled, and the squeeze taken from the logo before the QR floor | app | 0.1 |
| Webpage layout & polish — desktop gutters, equal card widths, carousel hover, dropdown spacing, mobile menu, flywheel corners | web | 0.5 |
| QR logo interior clearing (filled silhouette, size-independent) | app + mobile | 0.4 |
| Mobile map recentre on tab re-tap, pin sizing, native callouts removed | mobile | 0.4 |
| Branch merge + schema migration renumbering | app | 0.3 |
| Mobile bottom tab bar alignment + map/list toggle animation | mobile | 0.3 |
| QR card fit-to-box regression + export responsiveness | app | 0.3 |
| Event detail modal — cover photo gallery, full schedule, location, sign-up mode and QR state (admin + affiliate) | app | 0.2 |
| Event request error surfacing — no silent failures, client or server | app | 0.2 |
| `dev-up.sh`: reinstall dependencies when a manifest changes | app | 0.2 |
| Partial star ratings wherever a rating is shown | app | 0.2 |
| CSP: backend origins allowed as image sources | app | 0.2 |

**Subtotal 3.8h**

Two overflow paths were still live after the QR export rewrite, and were found by evaluating the layout maths
across its worst cases rather than by looking at a card. A three-line title above a three-line heading
left less slack than the QR's 120px floor, pushing the code 35px through the footer; and an organizer
logo was scaled to a fixed width, so a portrait one set a 408px-tall row and pushed it through by 230px.
The logo is now contained in a box and pairs with the SFLuv mark the way the on-screen affiliate card
does, and when space is tight the logo gives way first — a smaller mark costs nothing, a code below
~120px starts to fight the phone camera. All eight layout cases now bottom out exactly at the body's
490px line.

---

## Totals

| | Round 1 | Round 2 | Total |
|---|---|---|---|
| Large features | 16.6 | 6.6 | 23.2 |
| Tweaks & fixes | 5.4 | 3.8 | 9.2 |
| **Total** | **22.0** | **10.4** | **32.4** |

Volume — Round 1: 151 files, ~27.3k insertions / ~3.8k deletions · 12 DB migrations · 52 new routes · 122 new Go tests.
Volume — Round 2: 82 files, ~7.8k insertions / ~0.9k deletions · 2 DB migrations (1.37, 1.38) · 7 new routes · 1 new dependency (`jspdf`), replacing the export's use of `html2canvas`.
