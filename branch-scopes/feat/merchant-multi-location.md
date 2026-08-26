# Branch scope — `feat/merchant-multi-location`

Aug 5–21 2026 · app + mobile-app + webpage · **~32.2h active**

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
- **Round 3 (Aug 18–21) — 13.7h.** Measured from commit clustering across 36 commits in both repos,
  the strongest figure on this branch: the work and its commits are the same sittings.
- **Round 2 (Aug 11–12) — 14.3h. Provisional, and still provisional.** This round is **entirely uncommitted**, so there are
  no timestamps to cluster and the convention's usual method does not apply. The figure is derived
  from volume and the shape of the work — five new backend modules, 2,600+ changed lines in `app`,
  700+ in `mobile-app`, ~25 tests, and repeated live verification against a cloned production
  database — not from a clock. It asked to be re-measured from commit timestamps once committed. It
  has since been committed, on Aug 18, and re-measuring it that way does not work: those timestamps
  record the landing, not the doing. The work sat uncommitted for six days, so clustering its commits
  yields 2.9h — the length of the commit session — and that stretch also carried new work, which is
  counted in Round 3. Restating 14.3h as 2.9h would be less true, not more, so the estimate stands
  and stays labelled as one.

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

---

# Round 3 — Aug 18–21

**13.7h measured**, from commit clustering across 36 commits in `app` and `mobile-app`
(90-minute gap threshold, 25 minutes of lead-in per stretch):

| stretch | commits | hours |
|---|---|---|
| Aug 18 08:37–11:08 | 6 | 2.9 |
| Aug 18 16:25 | 1 | 0.4 |
| Aug 19 11:13–12:57 | 8 | 2.1 |
| Aug 19 15:10 | 1 | 0.4 |
| Aug 19 16:45 | 1 | 0.4 |
| Aug 20 06:15–06:16 | 2 | 0.4 |
| Aug 20 12:36–13:57 | 4 | 1.8 |
| Aug 21 08:52–13:40 | 13 | 5.2 |

**Round 2 could not be re-measured, and its 14.3h figure is left as it was.** The previous
revision asked for that once the work was committed. It has been committed — inside the Aug 18
commits — but those timestamps measure the *landing*, not the doing: the work happened around Aug
11–12 and sat uncommitted for six days, so clustering them yields the length of the commit session
(2.9h) and not the length of the work. Restating 14.3h as 2.9h would be worse than leaving it
provisional, so it stays provisional. The Aug 18 stretch is counted here, in Round 3, because the
same sitting also produced the escalating-tier rebuild, which was new.

Round 3 is the second-largest round on the branch and the first with test coverage attached to it.

## Large features

### W-9 escalating warning tiers, and escrow that cannot accumulate — 2.6h · app + mobile
- Four tiers replace one hard gate: notice at 400, firmer warning at 500, held at 600, refused after
- `decideEscrow` became `decidePayout`, returning pay/escrow/block plus the tier
- A blocked payout no longer consumes its redemption code — `UndoRedeem` and a 409 the app explains
- Escrow bounded to a single payment, which deleted back pay entirely: the expiry sweeper, the admin
  back-pay queue, `expired` and `back_pay_requested`, and the coverage arithmetic that supported them
- `w9_tier_notices` table; `GET /w9/status` gained tier, raw base units and blocked
- `W9TierModal` on mobile, one component with four presentations

### Merchant account refactor — 2.5h · app + mobile
- Account type chosen at signup, during the privacy-policy step; no "apply to be a merchant" button
- Merchants forced through onboarding before anything else, and locked out of the volunteer surfaces
- Wallets moved onto the locations table — a till's money belongs to the shop, not the owner
- Merchant wallets barred from the faucet, checked by identity rather than address
- `UNWRAP_ENABLED` gates the unwrap affordance, default off
- Mobile always in merchant mode, with the PIN guarding which counter a device is on

### Mobile test suite (Maestro) — 2.4h · mobile
- Maestro installed and driven against an iOS simulator for the first time on this project
- Six flows over three account states, tagged `volunteer` / `merchant` / `merchant-setup` / `w9`
- Release-build recipe established after `expo run:ios` proved unusable here
- Two seed scripts so the states are reproducible: merchant (with a second shop) and W-9 tiers
- Wrote up the traps: composite accessibility labels, the PIN slider, per-branch geometry

### W-9 provider swap to TaxBandits — 2.0h · app + mobile + webpage
- New `w9provider` adapter: JWS→JWT auth, `FormW9/RequestByUrl`, `FormW9/Status`
- All eleven vendor statuses mapped, including a failed match arriving as INVALID with TINMatching erased
- `cmd/w9probe` sandbox probe, redacting TIN/SSN/EIN/AccessToken
- Three clients realigned on the same copy

### Browser and API test suites — 1.9h · app
- Playwright specs including a real on-chain merchant payment and tip
- Seven API scenarios, made runnable twice rather than once
- Three skips that were hiding a real break, named and removed

## Smaller fixes

| fix | hours | repo |
|---|---|---|
| `locations` text/number columns NOT NULL — one NULL 500'd the public map for everyone (migration 1.48) | 0.6 | app |
| Blocked W-9 tier stayed outstanding after acknowledgement; the modal could not re-arm | 0.5 | app |
| Accessibility: modal backdrops and cards collapsed whole screens into one element; unlabelled controls | 0.5 | mobile |
| Till sheet copy contradicted the Switch location button sitting under it | 0.2 | mobile |
| dev-up faucet key handling, and saying plainly it is never a production signer | 0.2 | app |
| `.gitignore` widened to `.dev.env.*` after a backup slipped past the enumerated suffixes | 0.1 | app |
| Credential types documented as data rather than two hardcoded constants | 0.1 | app |
| Corrected the testing log: Privy was never the blocker, a stale LAN IP was | 0.1 | app |

## Totals

| | hours |
|---|---|
| Large features | 11.4 |
| Smaller fixes | 2.3 |
| **Round 3** | **13.7** |

**Volume.** 187 files changed, 24,005 insertions, 3,088 deletions (162/20,661/2,733 in `app`;
25/3,344/355 in `mobile-app`). 9 migrations, 19 new backend routes, 6 Maestro flows, 3 seed scripts.

## Worth knowing

- **Three bugs in this round were found by UI testing and could not have been found any other way.**
  The NULL-website map outage, the blocked modal that never came back, and the accessibility
  collapse all present as working code under unit tests.
- **Two diagnoses in this round were wrong before they were right**, and the corrections are recorded
  in the testing log rather than quietly fixed: mobile login was blamed on a Privy allowlist when the
  cause was a stale LAN IP, and the blocked modal was blamed on client-side ordering when the cause
  was a server-side query filtering acknowledged rows.
- **The suite cannot be run as one set.** It covers three mutually exclusive account states, so it
  runs by tag with a seed script between. That is a property of the product, not of the harness.
- **Still open at the end of the round**, restated after checking each rather than trusting the note
  that first recorded them — two of the three were described wrongly:
  - **The refused-payout trigger has no test.** It is not missing, as an earlier draft of this line
    said: `App.tsx` already clears the tier dismissal and refreshes status when a redeem comes back
    `blocked`, so the modal is meant to appear at the moment a scan fails. It could not have worked
    before this round, because the server retired the tier on acknowledgement; that is fixed, so the
    path should now work. Nothing exercises it — it needs a live refused redemption, not a seeded row.
  - **`UpdateLocationGooglePlace` binds `google_id` raw** where the INSERT uses `NULLIF($1, '')`. In
    practice unreachable: the handler 400s on a blank id, and the verifier falls back to the
    requested place id if Google returns none, so an empty string cannot reach the UPDATE through any
    caller that exists. It is an inconsistency between two writers of the same column with a partial
    unique index on it, not a live defect — worth aligning before someone adds a third writer.
  - **The two W-9 flows need a re-seed between runs**, and one of them always will: acknowledging a
    tier is what retires tiers 1-3, so 05 changes the state it depends on by passing. 06 does not,
    now that the blocked tier survives acknowledgement.

# Round 4 — Aug 23–26

**Repos:** `app`, `mobile-app` · **Total active hours: 14.0**

How the hours were arrived at: commit clustering across both repos gives 8.2h
(Aug 23 13:04–15:05, Aug 24 17:30–18:19, Aug 25 08:39–09:35 and 14:49–15:28,
Aug 26 09:39–12:44 and 13:45–14:45). Three stated adjustments add 5.8h: +1.5h
for the Aug 22–23 session whose work all landed in the 13:04 batch commit;
+1.0h for the Aug 24 vendor-doc and signature-probing session that preceded its
first commit; +3.3h for the Aug 25–26 gaps that were live TaxBandits console
walkthrough, webhook activation, and simulator-freeze diagnosis rather than
idle time (recorded in the session, invisible to clustering).

## Features

| Feature | hours | repo |
|---|---|---|
| Live TaxBandits sandbox wired end to end — env swap, ngrok tunnel, console walkthrough, JWS auth, `RequestByUrl`, hosted return page, vendor-root and timestamp fixes | 3.0 | app |
| W-9 confirmation modal and the three freeze fixes — confirm on intent, in-tree overlay, never dismiss a gone sheet, release the button before presenting, wait out the tier modal's dismissal | 3.2 | mobile |
| Notification panel as the only inbox, rows as general-purpose links — server `Action` targets, badge counts outstanding, per-row dismiss, un-dismiss on new tier, two `seen_at` scan fixes | 2.5 | 1.7 mobile / 0.8 app |
| TaxBandits webhook receiver — HMAC verification (their `MM/DD/YYYY … AM/PM` UTC timestamp, not unix), ack-then-work, sweep backoff via `last_polled_at` (migration 1.49), logger tee | 1.8 | app |
| Unwinding automated testing into the human-in-the-loop `scripts/` folder | 1.5 | both |
| Real-money W-9 ladder scripts (`w9-earn`, `reset-w9`, seeding rework to land on 400/500/600) | 1.0 | app |
| W-9 tier correctness from live walkthroughs — escrow tier recorded at the crossing, `pending` counted in the shown total, blocked tier re-arms after acknowledgement | 1.0 | app |

## Totals

| | hours |
|---|---|
| Features | 14.0 |
| **Round 4** | **14.0** |

**Volume.** `app`: 33 files, +2,114/−151 across the feature commits, then 118
files, +363/−11,344 in the unwind (81 deletions, 15 moves); 1 migration, 3 new
routes. `mobile-app`: 19 files, +731/−970.

## Worth knowing

- **The automated-testing effort was unwound by decision, not neglect.** The
  Playwright suite, the Go test files added on this branch, the Maestro flows,
  the `testing/` harness and its repo skill are gone. What remains is
  `scripts/` — one subfolder per human-testing shortcut, each with an accurate
  markdown description, rules in `scripts/MAINTENANCE.md` — with `dev-up`
  moved inside it. `TESTING.md`, `CLAUDE.md` and `AGENTS.md` now describe the
  human-in-the-loop process.
- **The backend `./test` package's W-9 group had never executed.** An earlier
  group's `t.Fatal` (stale schema: `CreateTables` lacks migration-added
  columns) stopped the run before `GroupW9Handlers` was reached — found while
  auditing what the suite actually covered, and part of why it was cut.
- **TaxBandits webhooks are signed**, contrary to the build plan's earlier
  claim: `base64(HMAC-SHA256(ClientId + "\n" + Timestamp, ClientSecret))`,
  retried 9× over 24h, 200 expected within 5s. Both real signatures reproduced
  exactly before the receiver was trusted.
- **All three simulator freezes were one bug**: presenting the Safari sheet
  while the tier modal was still dismissing left an invisible sheet over the
  app, swallowing touches with `openBrowserAsync` unresolved. iOS allows one
  modal presentation at a time; the fix is a 400ms wait, and the evidence was
  Maestro's captured hierarchy listing Safari controls over a frozen screen.

# Round 5 — Aug 26 (second sitting)

**Repos:** `mobile-app` · **Total active hours: 0.3**

One decision, taken and applied: W-9 tier acknowledgement is contingent on a
completed filing. Closing the vendor sheet no longer acknowledges the tier, so
backing out of the form unfiled brings the warning back after the close grace
period instead of retiring it for the year. Explicit dismissal still
acknowledges; a cleared filing removes the tier at the source.

| Change | hours | repo |
|---|---|---|
| Back-out no longer acknowledges the tier (`e830544`) | 0.3 | mobile |
