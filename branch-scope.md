# Branch scope — `pjol/volunteer-panel`

Aug 5–8 2026 · app + mobile-app + webpage · **~22h active**

Hours are active working time inferred from commit clustering (~16h measured), adjusted upward for two
large batch commits (`dd007b9` +3,098, `6dbf50c` +2,522) whose work landed with no intermediate commits.

---

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

## Totals

| | Hours |
|---|---|
| Large features | 16.6 |
| Tweaks & fixes | 5.4 |
| **Total** | **22.0** |

Volume: 151 files, ~27.3k insertions / ~3.8k deletions · 12 DB migrations · 52 new routes · 122 new Go tests.
