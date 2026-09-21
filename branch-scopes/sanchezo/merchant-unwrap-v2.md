# Branch scope — `sanchezo/merchant-unwrap-v2`

Sep 14 2026 · app (backend + frontend) · **2.9h active** (round 1: 1.9h, round 2: 1.0h)

Hours are wall-clock time measured from the session transcript with the
`time-accounting` skill (`measure_sittings.py`, 30-minute sitting gap). One
sitting, 12:12–15:19 PDT, measured at 15:19 (3.1h); round 1 was cut at the
first commit (14:08, 1.9h), round 2 is the rest less 0.1h of unrelated
billing paperwork (14:53–15:00). The branch was cut and every file written
inside the sitting. Itemised to the nearest 0.1h from the per-message
`UserPromptSubmit` stamps in the same transcript, and the items sum to the
measured figure. The earlier branch this replaces (`sanchezo/merchant-cashout-liquidation`,
July) is not counted here; it was abandoned at 165 commits behind main.

Weakness of the measurement: the sitting includes ~1.2h of research and design
conversation before any file was written. That is real working time on this
branch (the decisions below came out of it), but a reader wanting "typing time"
should read the two Build lines only.

---

# Round 1 — Sep 14

## What this branch is

Merchants unwrap SFLUV to their business bank account. The location's own
wallet calls `withdrawTo` on the token, which burns SFLUV and sends the backing
USDC to a Bridge liquidation address; Bridge drains that to the merchant's bank
over ACH. Nothing in the app moves money or holds a bank account number.

Decisions, all Sanchez's, recorded so the shape is explicable later:

- One **Unwrap** button per location (not "Cash out"), web only — never the
  mobile app or merchant-mode devices.
- Any amount; first unwrap each month is free, further unwraps that month must
  be ≥ $100 (the existing `/unwrap/eligibility` rule, kept).
- **Fees are absorbed.** No developer fee is ever set on a liquidation address.
  Verified against a real SFLUV transfer (`celo/usdc → ach/usd`): 25.00 in,
  25.00 out, every fee field 0.
- **Merchants never type a crypto address.** After a bank is linked through
  Plaid, the backend provisions a Bridge liquidation address and binds it to
  each location. Bridge allows exactly one address per (customer, bank, chain,
  currency, rail) — found in round 2 — so locations paying into the same bank
  share one address; a drain is matched to its location through our ledger by
  tx hash, never by address. The manual override is admin-only and is refused
  unless the address is one Bridge issued for that business.
- Bank accounts belong to the **business** (Bridge customer, keyed by owner);
  each location's address binds to one of them, so a second location can pay a
  different account without a second onboarding.
- Tips unwrap as a **second `withdrawTo` from the tipping wallet** — tipping
  wallets already hold REDEEMER_ROLE via `SyncLocationWallets`, so there is no
  consolidation hop.
- **KYB starts at approval.** Approving a location creates the Bridge customer
  via `POST /v0/kyc_links` and puts the hosted verification link in the existing
  approval email. The email is ours; Bridge never emails a customer.

## Large features

### Readiness assessment + Bridge capability research — 0.3h · app
- Confirmed Bridge supports Celo USDC as a liquidation source routing to ACH
- Confirmed Plaid Link is the bank-linking path (no customer portal exists)
- Confirmed `kyc_links` API and `redirect_uri` for approval-time onboarding
- Read-only production check: Azalina's (MamakFood LLC) is KYB-approved with
  no bank linked — the exact state the flow starts from
- Verified fee behaviour from a real transfer receipt

### Live-key security self-audit — 0.1h
- Two unrestricted production Bridge keys found in plaintext on the laptop;
  neither in git. Documented capabilities (transfers, PII, address redirect).
- Outcome: this branch is built and tested against sandbox only; production
  credentials go on the server, never a checkout. Cleanup is tracked separately.

### Design Q&A and decisions — 1.1h
- The decisions listed above, plus multi-bank handling and status visibility

### Build — backend — 0.2h · app
- `backend/bridge/`: typed client (sandbox default, `Idempotency-Key` on every
  write, no developer fee possible), RSA webhook verifier over
  `SHA256(t.body)` trying the three digest encodings Bridge's docs leave
  ambiguous, 10-minute replay window
- Migration **1.55**: `merchant_payout_profiles`, `merchant_bank_accounts`,
  `location_liquidation_addresses` (one row per location, override replaces),
  `unwraps` ledger (unique on tx hash)
- `handlers/merchant_payout.go`: status, KYB link, Plaid link-token + exchange
  (polls for the async bank record, then provisions), provision, per-location
  payout bank, admin override / attach-customer / list
- `handlers/bridge_webhook.go`: verify, ack, then re-read from Bridge — never
  act on a delivery's claimed state
- `handlers/merchant_payout_sweep.go`: 10-minute KYB + drain sweep, the
  fallback for missed webhooks; matches drains to ledger rows by deposit hash
- `RecordUnwrap` now writes the ledger; `GET /unwrap/history`
- Approval hook + KYB section in the approval email; bootstrap wiring; env
  documented in `.env.example`

### Build — frontend, verification, scope doc — 0.1h · app
- `AppWallet.cashOut()` via `withdrawTo`, signed by the location's derived
  wallet; legacy Stargate `bridge()` / `unwrapAndBridge()` and their revert
  helpers deleted (−339 lines)
- `lib/bridge/plaid.ts`: Plaid Link loader, promise API, cancel ≠ error
- `components/merchant/location-payout-card.tsx`: verify → connect bank →
  unwrap, rendered inside each approved location's settings card; payout-bank
  dropdown when a business has several; "also unwrap tips"; confirm dialog;
  recent unwraps with merchant-facing status
- CSP: `cdn.plaid.com` for script and frame
- Verified: `go vet`/`go test` clean on touched packages, `tsc` clean on every
  touched file (9 pre-existing errors elsewhere, unchanged), migration 1.55
  applied to the local dev DB, server boots with payouts disabled and every
  new route refuses unauthenticated calls (403; unsigned webhook 401)

## Smaller fixes

| Fix | Hours | Repo |
|---|---|---|
| — | | |

## Round 1 totals

| Area | Hours |
|---|---|
| Research + audit | 0.4 |
| Design | 1.1 |
| Build (backend) | 0.2 |
| Build (frontend) + verification | 0.1 |
| **Total (measured)** | **1.9** |

Volume: 22 files, +3,326 / −352, 1 migration (1.55), 11 new routes.

---

# Round 2 — Sep 14 (sandbox verification, admin tooling) — 1.0h

Sandbox keys are environment-bound (a live key cannot become or mint one;
the dashboard's Sandbox toggle is the only source), and Bridge's sandbox
cannot run Plaid, KYC links, or payment webhooks. So the sandbox was used
for what it is good for — exercising the real handlers against real Bridge
responses — and it found two things the docs do not say.

### Sandbox research, key handling, round-1 wrap-up — 0.7h · app
- Confirmed from Bridge docs and by probing: live key → sandbox host is 401;
  the `sk-test_xxx` in `celo_onramp/.env.example` is a placeholder
- Sandbox key received from Sanchez, verified (`/v0/api_keys/whoami`), wired
  into the gitignored worktree `.env` only
- Sandbox business customer created via API, KYB simulated, physical address
  patched, dummy US bank attached (Bridge requires the address first)

### Handler-level smoke, two Bridge constraints, admin Payouts tab — 0.3h · app
- Throwaway harness drove the real handlers (attach, status, provision,
  payout-bank, admin override ×2, record unwrap, history, sweep, webhook)
  against sandbox; removed before commit per the no-test-scaffolding rule
- **Found:** Bridge allows one liquidation address per (customer, bank,
  chain, currency, rail). Provisioning rewritten so locations on the same
  bank share it (`findLiquidationAddressForBank`, race-safe re-list on
  "already exists"). All 16 test locations provision; re-provision skips all;
  bogus override → 422; Bridge-issued override → 200 with `source=admin`
- **Found:** Bridge rejects `Idempotency-Key` on PUT (422). Client now sends
  it on POST only
- `GET /admin/merchant-payouts` now returns `businesses[]` (profile, linked
  banks, every approved location with its name and payout address) plus
  `unwraps[]`
- New admin **Payouts** tab (`components/admin/merchant-payouts-panel.tsx`):
  attach a Bridge customer to an owner, per-business KYB/ToS badges, banks,
  per-location address with source and an override box, recent unwraps with
  Bridge state, trace number and Celoscan link
- **Fixed a round-1 break:** the merchant wallet page's Unwrap button opened
  the old typed-destination modal, which still called the deleted
  `unwrapAndBridge` (Vercel build would have failed). The modal now hosts the
  bank-backed `LocationPayoutCard` (new `onUnwrapped` callback refreshes the
  page balance). `tsc` error count is back to main's 31, none in touched files
- Verified: backend builds, local server boots with `bridge payouts enabled
  (sandbox)`, payout routes 403 unauthenticated, webhook 401 unsigned;
  `/locations`, `/settings`, `/admin?tab=payouts` compile with no console errors

## Totals

| Area | Hours |
|---|---|
| Round 1 (research, design, build) | 1.9 |
| Round 2 — sandbox research + key handling | 0.7 |
| Round 2 — smoke, fixes, admin tab | 0.3 |
| **Total (measured)** | **2.9** |

## Not in this branch (needed before a merchant can use it)

- `BRIDGE_*` env on the production VM with a **fresh** key, never a laptop one
- The Bridge webhook registered against `POST /bridge/webhook`, its public key
  in `BRIDGE_WEBHOOK_PUBLIC_KEY` (Bridge returns a normal PEM; the verifier
  also accepts the `\n`-flattened form)
- Production dry run with SFLUV's own Bridge customer: connect a bank, provision,
  one small unwrap — sandbox cannot do this
- Attach the already-verified first merchant from the admin Payouts tab once
  their bank is linked
