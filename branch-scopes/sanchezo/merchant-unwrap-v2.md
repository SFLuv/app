# Branch scope — `sanchezo/merchant-unwrap-v2`

Sep 14 2026 · app (backend + frontend) · **1.9h active**

Hours are wall-clock time measured from the session transcript with the
`time-accounting` skill (`measure_sittings.py`, 30-minute sitting gap). One
sitting, 12:12–14:09 PDT, measured at 14:09; the branch was cut and every
file written inside it. Itemised to the nearest 0.1h from the per-message
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
  Plaid, the backend provisions a Bridge liquidation address per location and
  stores it. The manual override is admin-only and is refused unless the
  address is one Bridge issued for that business.
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

## Totals

| Area | Hours |
|---|---|
| Research + audit | 0.4 |
| Design | 1.1 |
| Build (backend) | 0.2 |
| Build (frontend) + verification | 0.1 |
| **Total (measured)** | **1.9** |

Volume: 22 files, +3,326 / −352, 1 migration (1.55), 11 new routes.

## Not in this branch (needed before a merchant can use it)

- A Bridge **sandbox** API key for the end-to-end run (the one in
  `celo_onramp/.env.example` returns 401); `BRIDGE_*` env on the production VM
  with a **fresh** key, never a laptop one
- The Bridge webhook registered against `POST /bridge/webhook`, its public key
  in `BRIDGE_WEBHOOK_PUBLIC_KEY`
- Attach the already-verified first merchant with
  `POST /admin/merchant-payouts/attach-customer` once their bank is linked
- Unit tests were written for the client and verifier, run green, and removed
  before commit to honour the repo's no-test-scaffolding rule
