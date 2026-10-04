# Branch scope — `pjol/merchant-unwrap-pending-states`

Sep 28 – Oct 2 2026 · app + animations · **9.7h**

Picks up where `pjol/merchant-unwrap-deployment` left off (Sep 22–28). Its last sitting ran to
Mon 10:54; the 0.05h already counted there is not counted again here.

## How these hours were measured

**5.74h measured** from session-transcript timestamps clustered into sittings on a 30-minute gap
(`time-accounting/scripts/measure_sittings.py`). The animations work is corroborated by file mtimes
running Oct 1 13:40 → Oct 2 15:40, inside the measured sittings.

**Two sessions ran concurrently on Friday afternoon, and their spans are not additive.** The
animations sitting (13:41–15:40, 1.99h) wholly contains the app sitting (14:03–15:08, 1.07h): two
project transcripts covering the same wall-clock minutes, because the work was interleaved rather
than sequential. Summing them would claim 3.06h for a 1.99h afternoon. **The union is the
measurement**; what follows apportions that 1.99h across what shared it.

**3.91h of meetings**, from 4.0h stated less 0.09h the transcripts already account for inside two of
those windows.

| Meeting | Day | Stated | Counted |
|---|---|---|---|
| Boundless video check-in w/ Jacky | Fri 2 Oct | 1.0h | 1.0h |
| Boundless video check-in w/ Jacky | Mon 28 Sep, ~4pm | 0.5h | 0.45h |
| Boundless video check-in w/ Jacky | Wed 30 Sep, ~4pm | 0.5h | 0.5h |
| Dev meeting | Mon 28 Sep, 2–4pm | 2.0h | 1.96h |
| Check-in w/ Beth | Fri 2 Oct, 2:30–3pm | 0.5h | **0h added** |

**The Beth meeting adds nothing to the total, and that is not an oversight.** It sits at 14:30–15:00,
entirely inside the Friday 13:41–15:40 span that is already measured. Its half hour is counted once,
as part of that span, and appears in the apportionment below. Adding it again would be the double
count this document exists to avoid.

**One figure I could not check.** Friday's 1.0h Boundless call has no stated time. Friday's measured
spans are 11:29–13:00 and 13:41–15:40, which cover most of the working day — if the call fell inside
either, that hour is partly double-counted and the total should come down. It is the one number here
resting on an assumption.

The weakness worth naming: the per-item split is an apportionment of measured sitting totals, not an
independent measurement of each item. For Friday afternoon the three things that shared the span are
split on their known boundaries — the Beth meeting's stated half hour, and the app transcript's own
1.07h less that overlap — leaving the remainder to animations. The sitting totals are the truth; the
split is approximate below 0.1h.

**No webpage work in this window.** Its last sittings were Sep 25 and are counted in the previous
scope.

### Measured sittings

| Sitting | Repo | Span | Measured |
|---|---|---|---|
| Mon 28 Sep | app | 10:47–10:54 (0.13h, less 0.05h counted previously) | 0.08h |
| Mon 28 Sep | app | 13:29–13:36 | 0.12h |
| Mon 28 Sep | app | 14:23–14:26 | 0.04h |
| Mon 28 Sep | app | 16:09–16:12 | 0.05h |
| Thu 1 Oct | animations | 13:19–14:04, 14:48–14:49 | 0.75h |
| Thu 1 Oct | animations | 15:20–16:32 | 1.20h |
| Fri 2 Oct | animations | 11:29–13:00 | 1.51h |
| Fri 2 Oct | animations + app + Beth meeting (one span) | 13:41–15:40 | 1.99h |
| | | **measured** | **5.74h** |

---

# Large features

### Boundless grant video animation studio — 4.38h · animations

Motion graphics for the Boundless San Francisco grant video, built to the *Draft for Jacky* script.
Not a git repository, so volume and apportionment both come from file mtimes.

**Studio scaffold — 0.75h.** Shared library (`lib/`): brand tokens and brand art, charts, config,
facts, map and map data, people, perk, the studio runtime and UI. Deterministic timelines so a
composition renders the same frame every time.

**Compositions, early script beats — 1.20h.** 47 comp files across the first beats, plus the catalog
entries binding each beat to its script row, and the `tools/` builders (`build-art.py`,
`build-heart.py`, `build-map.py`) that generate the heart silhouette and Tenderloin map art.

**Compositions, remaining beats and overlay variants — 1.51h.** 63 further comp files and 11 catalog
entries, bringing the set to **13 script beats / 102 compositions** across 14 directories. Each beat
carries standalone options (full-frame, no footage behind) and overlay options — some interactive,
taking a target position so they can point at something in a clip, others generic b-roll furniture.
Plus `index.html`, the review player (space/arrows to scrub, `p` to cycle the stand-in photo, `g` for
title-safe guides, `c` for a transparency checkerboard), and `README.md` / `AUTHORING.md`.

**Stitches and rendering — 0.92h.** A new `Stitch` runtime (`comps/stitch/stitch.js`,
`catalog/stitch.js`) that plays existing compositions inside one parent timeline, each in its own
frame seeked from the parent's clock, so beats that sit back to back in the script can be handed over
as a single continuous shot. `Stitch.ramp(u, accel)` gives a clock that starts at rest and reaches
full speed over a set interval, so a camera resuming after a hold does not jump. Embedded comps run
in render mode and keep their own determinism. Three stitched comps (07→11, 08→09, 08→09d), revisions
to all eight `02-conversion` comps and to `09-expansion`, and `render.mjs` work.

Output: **16 files, ~660 MB** — 12 standalone mp4s, 4 transparent `.mov` overlays (the largest 441 MB)
and 4 stitch mp4s.

### Unwrap pending states and payout detail — 0.12h · app

Checked whether Bridge's three pending drain states were being followed, and found one was not.

- `in_review` collapsed into `funds_received` in `UnwrapStatusFromDrainState`, so a merchant saw
  "Processing" — which reads as *your money arrived and is on its way*, the opposite of what a review
  means for when they get paid. Now its own status with an "In review" label, picked up by the admin
  panel for free since both use `unwrapStatusLabel`.
- **More seriously, `in_review` was missing from the open-unwrap worklist** — both the query and the
  partial index listed only three states. The collapse was masking it: a reviewed drain was written as
  `funds_received` and so stayed open by accident. Any row reaching `in_review` by another path would
  have fallen out of the set entirely, with nothing following it to the bank. `OpenUnwrapStatuses` now
  documents that the query, the index and the migration have to agree.
- Migration **1.59** recreates `unwraps_open_idx` to include `in_review` (a partial index predicate
  cannot be altered in place). Rows already collapsed are left alone deliberately: the sweep re-reads
  every open drain and will set the right state, and guessing here would invent a review that may
  since have cleared.
- New detail modal on the unwrap history, opened by clicking a row: the state with a sentence on what
  it means for the merchant, the bank it landed in, and the transaction and payout address each
  linking to the block explorer. Bridge's own raw state sits next to our label so support can quote it.
- The bank is joined through the **destination address the unwrap was sent to**, not the location's
  current liquidation address — a location re-pointed at a different bank would otherwise retitle every
  past payout as having gone somewhere it never went. Where that no longer resolves it says "Bank no
  longer on file" rather than guessing.
- Caught before shipping: the new join made `unwrapColumns` ambiguous, since the joined tables also
  carry `owner_id`, `created_at`, `location_id` and `updated_at`. Go compiled it; Postgres would have
  rejected every unwrap-history read at runtime. Added `unwrapColumnsQualified`.

### Workflow finalization diagnosis — 0.57h · app

Investigation only; **no code written**. Two distinct bugs found behind one reported symptom.

**Bug A — 12 workflows stuck at `completed` whose payouts already succeeded.** All single-step, all
`completed_step_count: 1, paid_out_step_count: 0`, all carrying a `payout_tx_hash`. Checked one
(`9b626f1c`, Elm Alley Gardens, 55 SFLUV) on chain: **not found on Celo** against two independent
RPCs, **found on Berachain with status `0x1`**. These are pre-cutover payouts from May–June 2026,
before the Celo token deployed on 1 June. `reconcileWorkflowStepPayoutByHash` bails when the recorded
chain differs from `activeChainID()`, and the community config still declares 80094 while the live
token is on 42220 — so they can never settle. This is the accounting discrepancy: work that was paid
reads as unpaid.

**Bug B — a workflow with an unfinished step never finalizes and never recurs.** `60aa49a6`
("Insta Post_Weekly Tenderloin Clean-up", weekly): `step_count: 2`, step 1 completed and paid, step 2
outstanding. It sits at **`in_progress`**, not `completed` — a workflow only reaches `completed` when
every step is done, and the recurrence successor is created *only* at that transition. So the series
stops dead. Different mechanism from Bug A, same reported symptom.

Two decisions are open before either can be written: what ends a workflow's availability window
(workflows carry only `start_at`, so recurring ones can use the next occurrence but one-time ones have
no boundary), and whether to add a `skipped` step status so the admin panel can show why a workflow is
marked partial — the `workflow_steps` CHECK currently allows no such state.

---

# Smaller items

| Item | Hours | Repo |
|---|---|---|
| Previous branch scope completed (tail of the sitting it was written in). | 0.08h | app |
| Check-in w/ Beth (inside the Friday afternoon span, counted once there). | 0.50h | — |
| Bridge unwrap configuration audited and charted — chain `celo`, currency `usdc`, rail `ach`, destination `usd`, memo `SFLUV`, no developer fee set. Confirmed drains are automatic and the rail is fixed at address-creation time, so changing it would need addresses re-minted. | 0.04h | app |
| Investigated what Bridge actually charges. **Not answerable from our own data: we discard it.** Bridge returns `initial_amount`, `developer_fee`, `subtotal_amount`, `converted_amount`, `exchange_rate`, `gas_fee` and `outgoing_amount` on every drain; our `Drain` struct models none of them, and `UpdateUnwrapFromDrain` persists only status, drain id, state, bank reference and hash. Also established that `developer_fee` is **our** revenue line, not Bridge's charge — Bridge's take is in the spread, so no pricing page could state it per transaction. | 0.05h | app |
| This branch scope, and its revision. | 0.01h | app |

---

# Totals

| Source | Hours |
|---|---|
| Measured — animations | 4.38h |
| Measured — app | 0.86h |
| Measured — Beth check-in (inside the Friday span) | 0.50h |
| Measured — webpage | 0.00h |
| **Measured subtotal** | **5.74h** |
| Meetings as stated, less 0.09h already measured inside two of the windows | 3.91h |
| **Total** | **9.7h** |

# Volume

**animations** — 151 files, ~14,700 lines touched Oct 1–2 (excluding `node_modules` and rendered
output). 13 script beats, 105 compositions across 15 directories (3 of them stitches), 12 library
modules, 5 build/serve tools, a review player, a `Stitch` runtime and a headless-Chrome renderer.
16 rendered files, ~660 MB. Not under version control.

**app** — 7 files, +302/−22. 1 migration (1.59). 1 new component
(`unwrap-detail-modal.tsx`, 154 lines). Uncommitted at the time of writing. The workflow finalization
work is diagnosis only; nothing written.

# Open

- **Workflow finalization, Bugs A and B above.** Blocked on the two decisions; Bug A is the one
  distorting payout accounting today.
- **Mobile multi-step role access** — whether an improver assigned to a role covering more than one
  step can reach the later step in the mobile app. Not yet checked.
- **Bridge fee capture.** Add the seven drain fields to the struct, persist them (migration 1.60), and
  surface them in the detail modal — turning "what does Bridge cost us" from a support question into a
  number per payout. Recommended; not started.
- **Check `default_liquidation_address_fee_percent` in the Bridge dashboard.** Nothing in this codebase
  sets it. If it is non-zero, merchants are paying a fee that accrues to an SFLUV payout account and no
  part of our system knows about it — and the payout card currently tells them "No fees are taken from
  your payout."
- **Verify the "1–2 business days" claim.** It appears in three places in the merchant UI and traces to
  nothing on Bridge's side; `created_at` → `updated_at` on `payment_processed` rows will test it,
  bounded by the 10-minute sweep.
- **Mobile merchant mode** still has neither refunds nor the payout detail modal.
