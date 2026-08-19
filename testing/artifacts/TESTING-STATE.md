# Where testing stands — 2026-08-19, before the merchant refactor

Written deliberately as a checkpoint, immediately before a large change to
merchant onboarding. Read the "What this refactor will invalidate" section
before trusting anything green below.

## What exists and passes

**Browser suite — `e2e/`, 11 specs, ~41s, all green.**

| Spec | Proves |
|---|---|
| `smoke` | the seeded session authenticates; prank forwarding flips role-gated nav |
| `admin-tax` | the tax panel renders and offers no back-pay control |
| `faucet-redeem` | an invalid code reaches a terminal state; held/refused copy survives |
| `merchant-map` | the map renders; **both shops of a multi-location merchant appear** |
| `pay-merchant` | map → Pay → confirmation addressed to that merchant's till; tipping wallet survives the handoff and differs from the till |
| `send-and-tip` | **a real payment and a real tip, verified on chain** — tip lands in the tipping wallet, till does not move |

**API scenarios — `testing/scripts/`, 6 scripts.** W-9 status and tiers, events
lifecycle, QR redemption, merchant onboarding and multi-location wallet
derivation, workflows, and a full W-9 threshold crossing that holds, refuses,
releases and settles on chain.

**Backend Go tests** pass: `w9provider`, `db`, `handlers`, `router`, `structs`.

## Preconditions a run depends on

- A seeded Privy session: `cd e2e && npm run auth` (human, captcha). Lasts about
  an hour for API calls; the browser session lasts longer but is unmeasured.
- The fork's clock within ~5 min of wall time — `sync-chain-time.sh`. Drift makes
  every account-abstraction operation retry, or hang the app entirely.
- A backend built from current source — `restart-backend.sh`. `dev-up` never
  rebuilds it.
- A funded sending wallet for `send-and-tip` — `fund-wallet.sh`.

## What was never covered

- **Mobile, entirely.** Maestro is unbuilt. The W-9 tier modal and merchant mode
  have never been exercised by any automated test.
- The affiliate event-approval path (admin-created events skip the queue).
- Workflow claim → start → complete (needs a workflow whose `start_at` has
  passed).
- `cmd/backfill_payouts`, which does not exist — the payout ledger has no
  history, so the W-9 gate under-counts everyone in production.
- The TaxBandits path end to end: the adapter is verified against the sandbox,
  but the crossing scenario has only ever run against the fake provider.

## Known and unfixed

- Workflow validation errors return **500** with an opaque message instead of
  400. Low priority: the UI blocks the reachable path.
- `W9_ENFORCEMENT` defaults to **shadow**, so nothing withholds in production.

## What this refactor will invalidate

The merchant work changes onboarding, the wallet model, the sidebar, and the
mobile client. Expect these to need re-running and probably rewriting:

- **`04-merchant-onboarding.sh`** — creates a location by POSTing `/locations`
  as an existing user. If onboarding moves to signup and a location's wallet
  becomes a column on the location row, both the flow and the wallet assertion
  change.
- **`pay-merchant` and `send-and-tip`** — both resolve a merchant's till from
  `location_payment_wallets`. If the wallet moves to the locations table, the
  fixture queries break even though the product still works.
- **`merchant-map`** — depends on approved locations having a payment wallet.
- **`smoke`** — asserts on sidebar nav. "Connected Wallets" becoming "Locations"
  for merchants will change what a merchant account sees.
- **`03-qr-payout.sh`** — merchants are to be barred from faucet payouts, so a
  merchant-owned wallet redeeming should begin to fail on purpose.

**Recommendation:** after the refactor, re-run the API scenarios first (seconds,
and they cover the data model), then the browser suite. Treat any failure in the
two payment specs as a fixture problem until proven otherwise — they assert real
on-chain movement and were green immediately before this change.
