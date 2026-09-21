# Branch scope — `pjol/merchant-celo-unwrap-tooling`

Sep 17–21 2026 · app (backend + frontend) · **7.0h active**

Records **my own** work on the merchant Celo unwrap tooling. The branch it lands
on is `sanchezo/merchant-celo-unwrap-tooling`, and the bulk of that branch is
Sanchez's — the Bridge payout system itself (`17e1eea`, `8a466f7`, `56ee39c`,
`a9b35cc`, Sep 14–16). This file covers the three commits that are mine and the
review passes around them; see **Not in this branch** for what it leaves out.

## How the hours were arrived at

Sittings come from session-transcript timestamps (`measure_sittings.py`,
30-minute gap). Each **prompt** is then given a floor of 10 minutes, covering
the time spent writing it and reading the result back — work that is real but
falls outside the span between two timestamps, and which a span-based
measurement systematically loses on sittings with few prompts. A sitting is
counted at whichever is larger, its measured span or its prompt floor.

| sitting | prompts | measured span | prompt floor | counted |
|---|---:|---:|---:|---:|
| Sep 17 17:05–17:42 | 4 | 0.61 | 0.67 | **0.67** |
| Sep 17 23:52–23:53 | 1 | 0.03 | 0.17 | **0.17** |
| Sep 18 13:50–13:52 | 1 | 0.03 | 0.17 | **0.17** |
| Sep 20 23:28–23:29 | 1 | 0.01 | 0.17 | **0.17** |
| Sep 21 14:02–14:21 | 4 | 0.32 | 0.67 | **0.67** |
| Sep 21 14:55–15:43 | 6 | 0.80 | 1.00 | **1.00** |
| Sep 21 15:46–15:52 (this scope document) | 2 | 0.10 | 0.33 | **0.13** |
| | | | **subtotal** | **2.98** |

Sep 21 is capped at **1.8h** rather than the 1.93h the floors would give. The
day's first prompt on this branch was 14:02 and the last was 15:52, so 1.83h is
the entire elapsed window and the floors cannot exceed it. The cap costs 0.13h,
taken off the scope-document sitting above.

Three stated adjustments cover work that leaves no transcript at all:

| Adjustment | Hours |
|---|---:|
| Boundless meeting, Thu Sep 17 | 2.0 |
| Off-keyboard time across the branch, not captured by the sittings above | 1.5 |
| Deploying the tooling, Sep 20 evening | 0.5 |
| **stated subtotal** | **4.0** |

**Total: 3.0h from sittings + 4.0h stated = 7.0h.**

Weakness worth naming, and it is the main thing to know about this figure: only
**1.9h** of the 7.0h is measured keyboard time. The 10-minute floor adds 1.1h,
the stated adjustments add 4.0h, and neither is a clock reading. On Sep 17,
Sep 18 and Sep 20 the floor is doing most of the work — those four sittings
measure 0.68h between them and are counted at 1.18h. Sep 21 is the strongest
day on the branch, because there the floors and the measured spans agree to
within a few minutes. Corroborated where possible: the Sep 17 and Sep 21
sittings each end within minutes of the commit they produced (`6ad08a7` 23:58,
`b271617` 15:43).

Itemised to the nearest 0.1h; items sum to the total.

---

## Large features

### Bridge webhook self-registration — 0.7h · app

`BRIDGE_WEBHOOK_PUBLIC_KEY` had to be copied out of the Bridge dashboard by
hand, and deliveries were refused without it.

- Bridge mints an RSA signing key per webhook endpoint and returns it from both
  the create and the list call, so given `BRIDGE_WEBHOOK_URL` and an API key
  carrying `webhook:read` (plus `webhook:create` to register), the backend finds
  or registers its own endpoint and adopts the key Bridge issued.
- Runs in the background at boot with a retry ladder (0s, 30s, 2m, 10m). A
  third party must never hold up the server starting; until resolution lands,
  deliveries are refused with a 401, Bridge retries, and the payout sweep covers
  anything lost.
- Migration **1.56** caches the resolved key, so a restart while Bridge is
  unreachable still verifies deliveries instead of refusing all of them.
- `BRIDGE_WEBHOOK_URL` is optional and falls back to the backend's own public
  origin (`PUBLIC_BACKEND_URL` → `NEXT_PUBLIC_BACKEND_URL` →
  `MCP_PUBLIC_BASE_URL`) + `/bridge/webhook`. Deliberately **not** derived from
  `APP_BASE_URL`, which is the frontend and a different host in production.
  Must be https either way, so a local checkout can never register a dead
  endpoint on the Bridge account.
- `BRIDGE_WEBHOOK_PUBLIC_KEY` still wins when set, and no webhook API calls are
  made in that case — a deployment that would rather not grant webhook scopes
  keeps working exactly as before.

### Per-location monthly redemption allowance — 0.7h · app

Replaced a flat `minimumFollowupUnwrapAmountSFLUV` of 100 with a named rule:
each location may make one redemption under **500 SFLUV** per calendar month,
and every further redemption that month has to clear that floor.

- It is a floor on repeat redemptions, not a ceiling on withdrawal — a location
  can redeem its whole balance any day of the month. The rule exists because
  every redemption is a separate ACH item whether it is for $5 or $5,000.
- The allowance belongs to the **location**, not the wallet: a location is one
  shop, so its till and tipping wallets share a single monthly allowance. A
  client that names no location falls back to the wallet's own stamp, which is
  how this worked before.
- Eligibility checks location ownership before reading its last redemption, and
  answers 403 rather than leaking another merchant's timing.

### Per-location bank attachment — 0.7h · app

`provisionLiquidationAddresses` used to give a destination to every approved
location the business had, so connecting a bank for one shop silently routed
every other shop's takings to the same account — a decision about money the
merchant never made, discoverable only after the fact.

- Provisioning now points **one** named location at a bank, or refreshes
  locations that already have a destination. It never attaches a location
  nobody asked about: sweeps, webhooks and "finish setup" pass no target, and
  for them an un-attached location stays un-attached.
- `POST /merchant/payout/provision` takes an optional `location_id` naming the
  one location being set up. The body is optional, so an older client gets the
  refresh-only behaviour rather than an error.
- The merchant panel always sends it, because the card offering the button
  belongs to a location. Copy changed to say so: a second shop now reads
  "Payouts are set up per location — connect this one to finish."
- The Bridge **address** is still shared across locations on the same bank,
  because Bridge allows exactly one per (customer, bank, chain, currency, rail)
  and refuses a second. Sharing the address is forced by the vendor; inheriting
  the destination was not, and only the second one was ever a choice.

### Redeemer role granted at approval, with a boot preflight — 0.4h · app

Locations approved after the last manual grant were holding takings their tills
could not redeem — `withdrawTo` reverted, and nothing said why.

- Approval now grants `REDEEMER_ROLE` to **that location's** till and tipping
  wallet on every approval. The existing account-level grant does not reach
  them: tills are derived per location, so a merchant's second shop gets
  addresses that have never been granted anything. The account-level grant stays
  guarded on the first approved location, since re-running it per shop is a
  chain read for nothing.
- Grant failures are logged, never fatal to the approval. A shop that cannot
  redeem yet is a problem; a shop that could not be approved is worse, and the
  boot sync still catches up.
- `NewRedeemerService` now preflights at boot: read `getRoleAdmin` for
  `REDEEMER_ROLE`, check the configured wallet actually holds it, and refuse to
  announce the service enabled if it does not. Previously the service reported
  itself enabled while every grant reverted with
  `AccessControlUnauthorizedAccount`. A wrong-chain or wrong-key deploy now says
  so at boot instead of failing silently forever.
- Zero gas is a warning rather than a gate — it can be topped up without a
  deploy, and the role reads keep working either way.
- Boot sync and approval-time grant now share one `grantToLocationWallets`
  loop, differing only in which wallets they are handed.

---

## Smaller fixes

| Fix | Hours | Repo |
|---|---|---|
| Ponder-free migrations: the indexer backfill that marks already-recorded transfers as notified moved out of `schema_migrations` into a post-boot seed, so an unreachable Ponder DB can no longer turn a deploy into an nginx 502 loop (it did on Sep 10). Reads Ponder in a READ ONLY transaction, writes only the app DB, `ON CONFLICT DO NOTHING`, safe every boot. | 0.2 | app |
| Review passes that produced no code: the Sep 18 bug/vulnerability sweep over merchant unwrap, and the Sep 20 audit of env vars introduced by the merge to main. | 0.3 | app |

---

## Time not attached to a commit

| | Hours | Repo |
|---|---|---|
| Boundless meeting, Thu Sep 17. | 2.0 | — |
| Off-keyboard time across the branch. | 1.5 | — |
| Deploying the tooling, Sep 20 evening. | 0.5 | app |

---

## Totals

| | Round 1 |
|---|---|
| Large features | 2.5 |
| Smaller fixes | 0.5 |
| Time not attached to a commit | 4.0 |
| **Total** | **7.0** |

Volume: 20 files, +820 / −165 · 1 migration (1.56) · 0 new routes
(`/merchant/payout/provision` gained an optional body rather than a sibling).

---

## Worth knowing

- **`6ad08a7`'s message says "added refunds". No refund code is in it.** The
  feature was asked for in the Sep 17 sitting and never landed — a pickaxe
  search (`git log -S refund` since Sep 10) finds nothing in any commit of mine.
  The message overstates the commit; the work is still outstanding.
- **Ponder is off limits, and that is now enforced rather than remembered.**
  `MigrationPools` has no Ponder handle to offer, so a migration cannot reach
  the indexer even by accident. Ponder stamps `_ponder_meta` with an app
  identity and refuses to start against a database another app has written, so
  the boundary is the indexer's rule, not a preference.
- **The per-location bank rule costs a merchant an extra step on purpose.** A
  second shop asks for its own bank even when it is the same account. That was
  chosen over inheritance because the failure mode of inheritance is silent and
  about money.

---

## Not in this branch

The Bridge payout system this work sits on is Sanchez's, committed Sep 14–16:
`17e1eea` (unwrap to a bank account via Bridge, v2), `8a466f7` (one Bridge
address per bank, admin Payouts tab, wallet-page unwrap fix), `56ee39c` (admin
attach-customer by email) and `a9b35cc` (unwrap ledger reconciliation against
user-operation hashes). Those have no scope document yet and this file does not
supply one — the hours above are mine alone and do not cover them.
