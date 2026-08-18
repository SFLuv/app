# Branch scope — `feat/merchant-multi-location`

Aug 5–12 2026 · app + mobile-app · **~18.5h active**

Branched as `fix/merchant-approval-hardening`; renamed once the work outgrew the name. What began as
hardening the merchant approval path turned into the groundwork for a merchant running more than one
shop, which is now the bulk of the branch.

Hours are active working time. The method differs by round, and the second round's figure is weaker
than the first's:

- **Round 1 (Aug 5–11) — 4.2h.** Inferred from commit clustering across four working commits
  (`b8cad0b`/`ea00f1b` on the 5th, `93ba07f` on the 7th, `7f66c77` on the 11th). Those commits are
  isolated points rather than clusters — each day holds one sitting that landed in a single commit —
  so the figure leans on the diffs, adjusted for `93ba07f` being a 12-file, +744 line batch that
  landed with nothing in between.
- **Round 2 (Aug 11–12) — 14.3h. Provisional.** This round is **entirely uncommitted**, so there are
  no timestamps to cluster and the convention's usual method does not apply. The figure is derived
  from volume and the shape of the work — five new backend modules, 2,600+ changed lines in `app`,
  700+ in `mobile-app`, ~25 tests, and repeated live verification against a cloned production
  database — not from a clock. **It should be re-measured from commit timestamps once this round is
  committed**, and this file updated rather than left as an estimate that was never checked.

Itemised to the nearest 0.1h; items sum to each round's figure.

---

# Round 1 — Aug 5–11

## Large features

### Merchant onboarding without a Google listing — 2.0h · app
- Google Places stays the primary path; manual entry added as a fallback
- Address-only Google results rejected, so a merchant cannot onboard as a street address
- Manual listings prompt for a real business name rather than accepting the address as one
- Schema allows many manual listings while still rejecting a duplicate Google place (migration 1.33)
- Location write path fixed — several columns were being written to the wrong positions

### `dev-up.sh` boot fixes — 1.4h · app
- Database clone moved ahead of the chain fork; the ~18 min dump let Celo advance past the fork
  point, leaving the cloned index claiming blocks the fork never saw
- Fork back-off (`ANVIL_FORK_LAG`) so the fork lands behind the live head
- Chain clock synced to wall clock after boot, which was the cause of the AA32 failures
- Package-manager detection from whichever lockfile is present
- Google Maps keys and CSP image source threaded through to the webpage
- Ponder guard that warns, rather than repairing, when a cloned index runs past the fork

## Smaller fixes

| Fix | Hours | Repo |
|---|---|---|
| Place finder reduced to a single search field; `locationRestriction` → `locationBias` | 0.5 | app |
| Volunteer event page: one-screen layout, inline organiser and reward | 0.3 | app |

**Round 1 total — 4.2h**

---

# Round 2 — Aug 11–12 *(uncommitted)*

## Large features

### Merchant-mode day view and payment confirmation — 3.6h · app + mobile-app
- New till screen: today's payments large, today's tips small, transaction history beneath
- Payment/tip pairing by global nearest-first matching within a 120s window, so a tip is attributed
  to the payment it actually followed rather than the first candidate scanned
- Ownership gate on tip attribution — an address the owner does not hold is somebody else's money
- Business day is midnight-to-midnight in the location's zone, using calendar arithmetic so a
  spring-forward day is 23 hours rather than a silently truncated 24
- Full-screen green confirmation on payment, driven by the incoming push rather than a poll, so it
  fires when the money lands instead of up to an interval later
- Refunds and voids shown as negatives
- 14 tests, including one that caught the pairing bug and one covering the DST boundary

### Multi-location wallet provisioning at approval — 3.2h · app
- Approving a merchant's second or later location derives its own payment and tipping wallets from
  the account factory and attaches them in the approving transaction
- Derivation happens before the transaction opens, so no write locks are held awaiting the chain;
  an unreachable RPC fails the approval cleanly and leaves it retryable
- The first location keeps the owner's existing primary wallet — that is where they are already
  paid, and moving it would orphan their history
- Wallets named after the street address (`900 Innes Avenue - Payments` / `- Tips`)
- Migration 1.40 backfills `location_payment_wallets` for every active location
- Replaced an earlier design that recorded a provisioning intent for the client to fulfil later,
  once the counterfactual address proved derivable server-side

### Merchant mode across multiple locations — 2.9h · app + mobile-app
- Device wallet resolved from the location on every read; it was a snapshot written at enrolment, so
  a wallet swap never reached the till until someone re-enrolled the device
- Enrolment no longer honours a device-supplied wallet, which would re-pin the address it just stopped pinning
- Location toggle on the till where the wallet chooser used to be; no PIN, since switching counters
  is a shift change
- Forced exit when a location is deactivated, unapproved, or loses its payment wallet — checked on
  every poll, because approval can be pulled long after setup
- A till idle more than three days asks which counter it is on before the first sale
- Picker fed by a server list of shops that are approved *and* payable
- Three device queries collapsed into one shared SELECT so they cannot drift

### Wallet swap machinery — 2.8h · app
- Unhooking a payment wallet no longer exists as an operation; replacement is a single atomic call
- Picker lists the merchant's smart accounts, showing unavailable ones with the reason
- Cross-role uniqueness across the whole estate — no address serves two shops, or both roles at one
- Smart accounts only: the paymaster and bundler act on smart accounts, so a till on a bare signing
  key could take payments and then be unable to spend them
- The settings endpoint can no longer empty a location's wallet list

## Smaller fixes

| Fix | Hours | Repo |
|---|---|---|
| Retired the `primary_wallet_address` fallback — 7 COALESCE chains and 6 dead LATERAL joins | 0.9 | app |
| `npm run build` diagnosed and unblocked (invalid `--turbo` flag on `next build`) | 0.6 | app |
| Error classifier extended so swap rejections return 400 with their text, not a bare 500 | 0.3 | app |

**Round 2 total — 14.3h**

---

# Totals

| Round | Dates | Hours | Repos |
|---|---|---|---|
| 1 | Aug 5–11 | 4.2 | app |
| 2 | Aug 11–12 | 14.3 | app + mobile-app |
| **Total** | **Aug 5–12** | **18.5** | |

**Volume** — app: ~35 files, ~2,655 insertions, ~651 deletions. mobile-app: ~6 files, ~718
insertions, ~19 deletions. 3 migrations (1.24, 1.33, 1.40). 5 new routes.

---

# Known gaps at time of writing

- **Nothing in Round 2 has run on a physical device.** TypeScript is clean, but the till screen, the
  location toggle, the forced-exit notice, the three-day prompt and the payment confirmation are all
  untested at runtime; push does not fire on the simulator.
- **The day view counts one payment wallet.** A location may hold several; only the default is
  summed, so a second would under-report takings. Not currently reachable — every location has
  exactly one — but the settings UI can still create the situation.
- **`npm run build` is intermittently flaky.** A minify worker occasionally fails and Next 15.2.6
  reports it as `WebpackError is not a constructor`, because its own compiled webpack shim does not
  export `WebpackError`. The misleading message is upstream and still present in 15.2.9; a retry
  passes. Unrelated to anything on this branch.
- `merchant_mode_devices.wallet_address` is now vestigial — written at enrolment, never read.
