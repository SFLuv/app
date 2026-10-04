# Branch scope — `pjol/merchant-unwrap-pending-states`

Sep 28 – Oct 3 2026 · app + animations + mobile-app · **11.1h**

Picks up where `pjol/merchant-unwrap-deployment` left off (Sep 22–28). Its last sitting ran to
Mon 10:54; the 0.05h already counted there is not counted again here.

## How these hours were measured

**7.15h measured** from session-transcript timestamps clustered into sittings on a 30-minute gap
(`time-accounting/scripts/measure_sittings.py`). The animations work is corroborated by file mtimes
running Oct 1 13:40 → Oct 3 22:18, inside the measured sittings.

**Concurrent sessions are not additive, and this happened twice.** On both Friday and Saturday an app
session and an animations session covered the same wall-clock minutes, because the work was
interleaved rather than sequential:

- **Fri 2 Oct** — animations 13:41–15:40 (1.99h) wholly contains app 14:03–15:08 (1.07h).
- **Sat 3 Oct** — app 22:17–23:39 (1.36h) wholly contains animations 22:17–22:24 (0.12h).

Summing each pair would claim 3.06h and 1.48h for afternoons that lasted 1.99h and 1.36h. **The union
is the measurement**; the apportionment below divides each span across what shared it.

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
inside the Friday span that is already measured. Its half hour is counted once, within that span, and
appears in the apportionment. Adding it again would be the double count this document exists to avoid.

**One figure I could not check.** Friday's 1.0h Boundless call has no stated time, and Friday's
measured spans cover most of the working day. If it fell inside either, that hour is partly
double-counted and the total should come down. It is the one number here resting on an assumption.

The weakness worth naming: the per-item split is an apportionment of measured sitting totals, not an
independent measurement of each item. Friday's span is split on its known boundaries (the Beth
meeting's stated half hour, and the app transcript's 1.07h less that overlap), Saturday's on the
animations transcript's own 0.12h. The sitting totals are the truth; the split is approximate below
0.1h.

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
| Fri 2 Oct | app | 15:47–15:50 | 0.04h |
| Fri 2 Oct | app | 22:34–22:35 | 0.01h |
| Sat 3 Oct | app + animations (one span) | 22:17–23:39 | 1.36h |
| | | **measured** | **7.15h** |

---

# Large features

### Boundless grant video animation studio — 4.50h · animations

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
full speed over a set interval, so a camera resuming after a hold does not jump. Three stitched comps
(07→11, 08→09, 08→09d), revisions to all eight `02-conversion` comps and to `09-expansion`, and
`render.mjs` work. Output: **16 files, ~660 MB** — 12 standalone mp4s, 4 transparent `.mov` overlays
(the largest 441 MB) and 4 stitch mp4s.

**Stitch revision and QA frames — 0.12h.** A pass over `comps/stitch/07-11.html`, with QA stills
regenerated at timestamped intervals for `07-sforganica`, `08-merchant-network`, `10-skilled-workers`
and the 07→11 stitch — the frame captures used to check a composition without re-rendering video.

### Workflow finalization and recurrence — 0.60h · app

Began as the reported bug "workflows with one step complete and the next not never finalize". The
investigation found two unrelated faults behind one symptom, and then found that a third of what I
built was unnecessary. All three are recorded here because the net result is smaller than the work.

**Bug A — payouts that succeeded on a chain we have left.** 12 workflows stuck at `completed`, all
single-step, all `paid_out_step_count: 0`, all carrying a `payout_tx_hash`. Checked one (`9b626f1c`,
Elm Alley Gardens, 55 SFLUV): **not found on Celo** against two independent RPCs, **found on Berachain
with status `0x1`**. Pre-cutover payouts from May–June 2026, before the Celo token deployed on 1 June.
`reconcileWorkflowStepPayoutByHash` bailed on any payout whose chain differed from `activeChainID()`,
so they could never settle — work that was paid reading as unpaid. **This is the accounting
discrepancy.**

Fixed by verifying against the chain the payout was recorded on: `verifyTransferReceipt` is now
parameterised by client and token, `VerifyTransferOnChain` dials a past chain from per-chain config
(`WORKFLOW_PAYOUT_RPC_<id>`, `WORKFLOW_PAYOUT_TOKEN_<id>`), and `ErrNoLegacyChainConfig` keeps "cannot
check" distinct from "checked and absent" so nothing is ever marked paid on trust. Applied to the step
and manager paths; documented in `.env.example`.

**Bug B — a step nobody started stalls the occurrence.** Migration **1.60** added `workflows.end_at`,
`workflows.partially_completed` and a `skipped` step state; `WorkflowWindowEnd` plus
`FinalizeWorkflowPartiallyIfElapsed` close out an elapsed workflow, marking unstarted steps skipped
and refusing to proceed while a completed step still owes a bounty. One-time workflows gained an
optional proposer-set end date; without one they never elapse, which is what keeps existing workflows
untouched.

**Then the premise collapsed.** `ensureRecurringWorkflowSeriesCatchUpTx` already does this: when an
occurrence's next start comes due it marks the current one `skipped` regardless of unfinished steps
and generates the successor, looping to catch up misses. It is reached every maintenance pass via
`RefreshWorkflowStartAvailability`. My earlier claim that "there is no periodic sweep that generates
missed recurrences" was wrong — I had searched the unblock and completion paths and never found the
catch-up. So 1.60 and its sweep **duplicate** an existing mechanism with a different terminal state
(`paid_out` + `partially_completed` versus `skipped`) on the same trigger, which is worse than leaving
it alone. Recommended for revert; not yet reverted.

**`partially_completed` exposed on the admin list.** Added to `GetAdminWorkflows` only — the 9 SELECT
sites and 5 scanners elsewhere do not pair up, so a blanket edit would have misaligned four money
queries. Verified column by column: 11 columns, 11 scan fields. Amber badge on the admin card so
"Finalized" is never read as "delivered".

### Claimed-role step assignment — 0.40h · app

The real bug behind Jacky's stalled social-media workflow, found after the above.

Her series has two steps under one role ("Instagram Poster"). Step 1 is `paid_out` to her; step 2 sits
`available` with **no assignee** — and the same shape appears on the Sep 14 occurrence, so it repeats
rather than being a race. `ensureRecurringWorkflowSuccessorTx` clones steps with `role_id` but no
`assigned_improver_id`, so every occurrence starts unassigned, and `ClaimWorkflowStep` is the only
thing that ever assigns — covering just the steps that exist and are unassigned at the instant of the
claim.

`BackfillClaimedRoleStepAssignments` makes assignment follow the role continuously: it fills a NULL
assignee on any `locked`/`available` step whose role already has exactly one claimant in that
workflow, and runs first in each maintenance pass so a step about to be offered goes to the person who
owns it. Never touches `completed`/`paid_out` steps, and refuses to guess where a role somehow has two
claimants.

**Open, and it decides whether this fixes her case:** the MCP exposes `role_title` but not `role_id`.
If the series state defines two separate roles both titled "Instagram Poster" — plausible, since the
series was single-step until late August and step 2 arrived by a state edit — then the ids differ, the
backfill will not match, and the state needs its roles merged instead.

### Unwrap pending states and payout detail — 0.12h · app

- `in_review` collapsed into `funds_received` in `UnwrapStatusFromDrainState`, so a merchant saw
  "Processing" — which reads as *your money arrived and is on its way*, the opposite of what a review
  means. Now its own status with an "In review" label, picked up by the admin panel for free.
- **More seriously, `in_review` was missing from the open-unwrap worklist** — the query and the partial
  index listed only three states. The collapse masked it; any row reaching `in_review` another way
  would have dropped out of the set with nothing following it to the bank. `OpenUnwrapStatuses` now
  documents that query, index and migration must agree. Migration **1.59** recreates
  `unwraps_open_idx` (a partial index predicate cannot be altered in place).
- Detail modal on the unwrap history: the state with what it means for the merchant, the bank it
  landed in, and transaction and payout address linking to the block explorer, with Bridge's raw state
  beside our label for support. The bank is joined through the **destination address the unwrap was
  sent to**, not the location's current one, so re-pointing a location cannot retitle past payouts.
- Caught before shipping: the new join made `unwrapColumns` ambiguous (`owner_id`, `created_at`,
  `location_id`, `updated_at` exist on the joined tables). Go compiled it; Postgres would have rejected
  every unwrap-history read at runtime. Added `unwrapColumnsQualified`.

### Mobile multi-step role access — 0.22h · mobile-app

Access was never broken — the step pager and `getInitialStepIndexForWorkflow` already reach the
improver's first actionable step. Discoverability was, in two specific ways:

- `detailStepIndex` was recomputed only on open and refresh, so after completing step 1 an improver
  stayed on their own finished step with no sign step 3 was also theirs. Completion now advances to
  their next open step and says so.
- "Step 2 of 4" said nothing about ownership. The pager now reads "Step 2 of 4 · yours: 1, 3".

Both are client-side and need a build; neither changes the API, so the release can ship in either
order. `skipped` added to both step-status unions, since the declared types were about to be wrong at
runtime.

---

# Smaller items

| Item | Hours | Repo |
|---|---|---|
| Dependabot triage: 194 open alerts reduced to **32 distinct packages** (17 critical, 71 high, 94 medium, 12 low; 170 npm, 24 Go). Only two frontend direct deps are implicated — `next` (62 alerts) and `jspdf` (20). Six Go bumps were applied and verified building, then **reverted at request** to keep this branch to the workflow fixes; they move to `pjol/vulnerability-fixes`. Noted there: go-ethereum 1.17.0 raises the `go` directive to 1.25.0, and `go.sum` is untracked so a half-applied bump leaves no trace in the diff. | 0.40h | app |
| Disk at 100% (638 MB free of 466 GB) blocked the Go build with `no space left on device`; reclaimed the regenerable build cache and module cache (~14 GB). One `go mod tidy` ran while full, silently rolled `go.mod` back, and was only caught by re-reading resolved versions — worth re-verifying with `go list -m` after any bump. | 0.12h | — |
| Previous branch scope completed (tail of the sitting it was written in), plus three revisions. | 0.20h | app |
| Bridge unwrap configuration audited and charted — chain `celo`, currency `usdc`, rail `ach`, destination `usd`, memo `SFLUV`, no developer fee set. | 0.04h | app |
| Investigated what Bridge actually charges. **Not answerable from our own data: we discard it.** Bridge returns `initial_amount`, `developer_fee`, `subtotal_amount`, `converted_amount`, `exchange_rate`, `gas_fee` and `outgoing_amount` on every drain; our `Drain` struct models none of them. Also established that `developer_fee` is **our** revenue line, not Bridge's charge — Bridge's take is in the spread, so no pricing page could state it per transaction. | 0.05h | app |

---

# Totals

| Source | Hours |
|---|---|
| Measured — animations | 4.50h |
| Measured — app | 2.15h |
| Measured — Beth check-in (inside the Friday span) | 0.50h |
| Measured — webpage | 0.00h |
| **Measured subtotal** | **7.15h** |

The mobile-app edits and the disk recovery carry no separate line: both were done from inside app
sittings, so their time is already in the 2.15h rather than alongside it. There is no mobile-app
transcript of its own.
| Meetings as stated, less 0.09h already measured inside two of the windows | 3.91h |
| **Total** | **11.1h** |

# Volume

**animations** — 151 files, ~14,700 lines touched Oct 1–3 (excluding `node_modules` and rendered
output). 13 script beats, 105 compositions across 15 directories (3 of them stitches), 12 library
modules, 5 build/serve tools, a review player, a `Stitch` runtime and a headless-Chrome renderer.
16 rendered files, ~660 MB, plus QA frame stills. Not under version control.

**app** — 2 migrations (1.59, 1.60). New files: `db/app_refunds.go`-adjacent work aside, this round
added `db/app_workflow_partial.go`, `db/app_workflow_role_assignment.go`,
`frontend/components/merchant/unwrap-detail-modal.tsx`. `go.mod` reverted to HEAD, so no dependency
change ships on this branch.

**mobile-app** — `ImproverScreen.tsx` and `types/app.ts`.

# Open

- **Jacky's two steps: one `role_id` or two?** Decides whether the backfill fixes her case or the
  series state needs its duplicate roles merged.
- **Revert migration 1.60 and `FinalizeWorkflowPartiallyIfElapsed`**, which duplicate
  `ensureRecurringWorkflowSeriesCatchUpTx` with a conflicting terminal state.
- **Set `WORKFLOW_PAYOUT_RPC_80094` and `WORKFLOW_PAYOUT_TOKEN_80094`** — this is what actually clears
  the 12 stuck workflows and the payout accounting.
- **194 Dependabot alerts**, on `pjol/vulnerability-fixes`. Go side is 6 bumps for 24 alerts; npm side
  is 170 alerts behind `next` and `jspdf` plus 24 transitive packages.
- **Bridge fee capture** — add the seven drain fields, persist them, surface them in the detail modal.
- **Check `default_liquidation_address_fee_percent`** in the Bridge dashboard. Nothing in this codebase
  sets it; if it is non-zero, merchants pay a fee the payout card tells them is not taken.
- **Verify the "1–2 business days" claim** — in three places in the merchant UI, tracing to nothing on
  Bridge's side.
- **Mobile merchant mode** still has neither refunds nor the payout detail modal.
