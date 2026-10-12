<!-- Invoice export, formatted to branch-scopes/report_template.csv.
     Machine-readable copy: branch-scopes/pjol/boundless-touchups.csv -->

| INVOICE | |
|---|---|
| CONTRACTOR NAME: | PJ O'Leary |
| SERVICE/TITLE: | Development and motion graphics - pjol/boundless-touchups |
| INVOICE DATE: | 2026-10-12 |
| SERVICE PERIOD: | 2026-10-05 to 2026-10-11 |
| HOURLY RATE: | — |
| % SFLUV | 10.00% |

| DATE | DESCRIPTION | HOURS | AMOUNT BILLED |
|---|---|---:|---:|
| 2026-10-05 | Boundless grant video: analysis pipeline, draft 2, notes pass, draft 3, 4K asset handoff (includes the 2-3pm dev meeting and two app check-ins falling inside these spans) | 6.63 | 0.00 |
| 2026-10-08 | Working block as stated, 12:00-15:00, less the 0.08h logged inside it | 2.92 | 0.00 |
| 2026-10-08 | How it Works: expanding ring replaces the circular loop; 1.43x in the ring, tagline below | 0.81 | 0.00 |
| 2026-10-09 | How it Works: legend and label placement, slower draw, first-year merchant spend from the ledger ($16,560); three option write-ups | 1.23 | 0.00 |
| 2026-10-09 | Past-event gallery modal with full-resolution downloads; dev-up prefers a sibling webpage checkout; hand-tested against the local dev stack | 0.75 | 0.00 |
| 2026-10-09 | Merge feat/forms-and-site-editing into pjol/volunteer-gallery | 0.10 | 0.00 |
| 2026-10-10 | Falafelland payment failure diagnosed: sponsored-transaction EOA out of gas (diagnosis only, no code) | 0.15 | 0.00 |
| 2026-10-11 | Branch scope document for the week | 0.21 | 0.00 |
| **TOTAL:** |  | **12.80** | **$ -** |
| **PAYMENT IN SFLUV** |  |  | **$ -** |
| **PAYMENT IN USD** |  |  | **$ -** |

# Branch scope — `pjol/boundless-touchups`

Oct 5–11 2026 · animations + webpage + app · **12.8h**

Records **my own** work this week. Branched off `sanchezo/website-cms-with-vuln-fixes`, whose two
halves are already written up elsewhere — the website CMS and Forms and Waivers in
[`sanchezo/website-cms.md`](../sanchezo/website-cms.md) (6.3h, Sanchez's, Oct 1–9), and the dependency
work plus Monday morning's accounting in `pjol/merchant-unwrap-pending-states.md` (12.3h, Sep 28 – Oct 5),
which still sits on its own branch and is not merged here. This file is the remainder, and the reason for
the branch name: the Boundless grant video, the How it Works reframing, the past-event gallery, and
Saturday's payment-failure diagnosis.

## How these hours were measured

**Correction to the figure first published here.** The first version of this document reported
**8.29h measured / 11.2h total**, measured at Oct 11 21:09 while the sitting that wrote it was still
open. That sitting closed at 21:13, so its line moves 0.15h → 0.21h and the measured union moves
**8.29h → 8.36h**, total **11.2h → 11.3h** (a 1.007x change). Two feature headings also rounded
upward past what their sittings measured — How it Works read 1.2h against a measured 1.09h, and the
gallery 0.2h against 0.10h — so the itemisation summed 0.3h above the total it sat under. Both are
corrected below. Nothing about the work changed; only the arithmetic describing it.

**Second correction: the week was under-measured.** Asked whether the figure felt short, I checked the
commits and file mtimes against the sittings instead of defending the number, and two Friday spans had
committed output sitting in gaps the 30-minute rule had split out. Measured moves **8.36h → 9.88h** and
the total **11.3h → 12.8h** (a 1.13x change). The cause is named in full under *Where the 30-minute rule
undercounted*; the short version is that hand-testing a web page in a browser generates no transcript
messages, so the webpage work was the most under-reported thing here.

**9.88h measured** from session-transcript timestamps clustered into sittings on a 30-minute gap
(`time-accounting/scripts/measure_sittings.py`), re-taken at **2026-10-12 11:12 PDT**, with two spans
extended to the file writes and commits inside them (see *Where the 30-minute rule undercounted*). Three projects ran
this week — animations, webpage and app — and the figure is the **union** of their sittings, not the
sum: 0.02h of overlap is merged rather than counted twice. Corroborated by file mtimes, which run
Oct 5 12:59 → 20:09 for animations and match the commit times in both git repos.

**Two sittings are excluded because another document already counts them.** Mon 5 Oct app 10:53–11:12
(0.31h) and 12:17–12:19 (0.03h) are recorded in `pjol/merchant-unwrap-pending-states.md` at `abbc0f9`,
which was committed at 12:19 that day and closes at that point. Counting them here would be the double
count this convention exists to prevent.

**The Monday dev meeting adds nothing to the total, and that is not an oversight.** It sits at
14:00–15:00, entirely inside the measured animations sitting 12:53–16:39 — and not merely bracketed by
it: 216 messages were sent during 14:05–14:59 with a largest internal gap of 14 minutes, so the hour is
measured working time already. Its hour is counted once, as part of that span, and appears in the
apportionment below. (The separate 11:15–12:00 meeting on the same day belongs to the other document.)

**Oct 8 is the one stated figure.** 3.0h stated for 12:00–15:00, less the 0.08h the transcripts already
account for inside that window (app 14:05–14:10), so **2.92h is added**.

| Stated | Day | Stated | Counted |
|---|---|---|---|
| Dev meeting | Mon 5 Oct, 2–3pm | 1.0h | **0h added** |
| Working block | Thu 8 Oct, 12:00–15:00 | 3.0h | 2.92h |

Four weaknesses worth naming:

1. **The Oct 8 block is stated, not measured.** Only 0.08h of those three hours appears in any
   transcript. What the other 2.92h covered is not recoverable from the clock; it is taken as stated.
   It is the one number here not resting on a measurement.
2. **0.46h of the measured spans share wall-clock with non-SFLUV sessions** on this machine (Mon 13:18–13:22;
   Fri 11:48–11:56, 12:49–12:52, 15:08–15:13, 15:44–15:46, 18:43–18:49). Those minutes carry SFLUV
   messages, so they are counted, but attention was split across them.
3. **The clock measures attention, not output.** The gallery work landed +350/−47 in webpage against
   0.10h of measured sitting, because agent runtime is not working time. Do not read hours back out of
   the diff — that is the inflation this convention was written to stop.
4. **The per-feature split inside Monday's two animations sittings is apportioned by file mtime**, whose
   brackets sum to the sitting totals. The sitting totals are the truth; the split is approximate below
   0.1h.

### Measured sittings

| Sitting | Repo | Span | Measured |
|---|---|---|---|
| Mon 5 Oct | animations (contains the 2–3pm dev meeting) | 12:53–16:39 | 3.77h |
| Mon 5 Oct | animations | 17:19–20:10 | 2.86h |
| Mon 5 Oct | app | 12:56, 18:23 | *absorbed* |
| Thu 8 Oct | app | 14:05–14:10 | 0.08h |
| Thu 8 Oct | webpage | 15:40–16:24 | 0.73h |
| Fri 9 Oct | webpage | 11:38–**12:52** *(extended)* | 1.23h |
| Fri 9 Oct | webpage + app | 15:08–**15:53** *(extended)* | 0.75h |
| Fri 9 Oct | webpage | 18:43–18:49 | 0.10h |
| Sat 10 Oct | app | 12:57–13:06 | 0.15h |
| Sun 11 Oct | app | 21:00–21:13 | 0.21h |
| | | **measured** | **9.88h** |

Monday's two app check-ins (12:56 and 18:23, 0.02h) fall inside the animations spans, so they are
absorbed into them rather than added alongside.

### Where the 30-minute rule undercounted

The 30-minute gap is the right default — a longer silence usually means somebody left. But it measures
*minutes containing a message*, and reviewing a rendered page in a browser produces no messages. Two
Friday gaps were split by it that were plainly working time, and the corroboration step is what caught
them: **the files and commits fell outside the sittings**, which the method says means the measurement
is describing the wrong span.

| Split gap | Length | What is inside it | Verdict |
|---|---|---|---|
| Fri 11:56 → 12:49 | 53 min | commit `a69f98f` at 12:38; the four `how-it-works-examples/` files written 12:44; 86 app files at 12:45 | **working** — span extended |
| Fri 15:13 → 15:53 | 40 min | gallery commits in both repos at 15:53; webpage `.next/dev` rebuilding at 15:48; `tmp/webpage` dev stack at 15:27 | **working** — span extended |
| Mon 16:39 → 17:19 | 40 min | nothing written, no commits | genuine break |
| Thu 14:10 → 15:40 | 90 min | nothing written, no commits (and 14:10–15:00 sits inside the stated Oct 8 block) | genuine break |
| Mon 11:12 → 12:17 | 65 min | holds the 11:15–12:00 meeting, counted in `merchant-unwrap-pending-states.md` | counted elsewhere |
| Mon 12:19 → 12:53 | 34 min | one `.env` write at 12:40 | too thin to claim |

Extending only the two corroborated spans adds **1.53h**. For scale, had the threshold itself been
moved instead, the same week measures 9.48h at 35 minutes, 10.15h at 45 and 11.03h at 60 — which is
why the gaps were judged individually against what landed in them rather than by turning one dial.

---

# Large features

### Boundless grant video — drafts 2 and 3, and the asset handoff — 6.63h · animations

Monday, two sittings. Not a git repository, so both volume and the split come from file mtimes.

**Analysis pipeline — 0.8h** (12:53–13:40). A new `edit/` subsystem standing up the parts an edit needs
before any cut can be judged: audio `prep`, `coarse`, `feat`, `fine_track`, `xc`, `fitlib`, `eq_curve`,
`residual`, `plot_gain` and `common`, plus `audiolib.py`, `df_denoise.py`, `analyze.py` and `chan.py`;
and on the video side `make_proxies.py`, `cuts.py`, `cuts2.py`, `identify.py`, `initguess.py` and
`lib.py`, with per-scene proxy maps (`map_s01` … `map_s21`), `inventory.json`, `coarse_100_25.json`,
`eq_curves.json` and `segments0.json`. Also pulled `data/sforganica-transactions-2026-04-to-09.json`
so the SFOrganica stats beat reads off the real ledger.

**Draft 2 — 3.0h** (13:41–16:39). Test renders first, then 4K re-renders of every beat carrying a note —
`01-story` standalone-e and overlay-d2, `02-conversion` standalone-c-min, `03-perk-created` overlay-b2,
`05-perk-received` overlay-b2, `07-sforganica` standalone-b, the `08-09ad` stitch, `10-skilled-workers`
standalone-b, `11-propose-vote` standalone-c and `12-closing` overlay-b — and `draft2.mp4` (339.8 MB),
delivered as *boundless grant video - draft 2.mp4*. Exported without music, to be laid in later, and the
stitch progress kept addressable so the pass could be re-stitched after the next round of notes rather
than rebuilt.

**Notes pass and draft 3 — 2.4h** (17:19–19:44). Pacing and audio notes applied: Beth's intro no longer
sits isolated (room noise kept under it, so it reads as a room rather than a booth), the voiceover EQ
backed off toward the original draft's brightness after the newer one traded that for power, and Beth's
name moved out of the caption zone — shifted to the side rather than stacked below. `03-perk-created`
went overlay-b2 → **overlay-b3**, and the affected beats were re-rendered twice more as the notes
settled. Out: `rough-draft-2.mp4` (339.7 MB), then `draft3.mp4` / `rough-draft-3.mp4` (315.4 MB).

**Handoff package — 0.4h** (19:44–20:10). `BOUNDLESS VIDEOS/Draft 3 Assets/` — 7 standalone animations,
4 transparent `.mov` overlays, 4 scenes with the overlay already composited, and the timeline tail.
Every file is named with its timecode and the beat it belongs to
(`00m20s16f perk-cards (03-perk-created overlay-b3).mov`), so an editor can drop them onto the timeline
at the right frame without reading the script alongside.

Output: **81 video files, 3,946 MB**, three drafts and a 16-file handoff set.

### How it Works reframed from circularity to amplification — 1.96h · webpage, 0.08h · app

Thu 15:40–16:24, Fri 11:38–11:56 and 12:49–12:52, with the brief set in the Thu app sitting 14:05–14:10.

The section implied money going round in a circle; it should say the money does more than one job. The
structure and most of the copy were left alone deliberately — the change is the animation and the
framing around it.

- Three options were written up first, as `how-it-works-examples/` (a README plus *second job*,
  *expanding pie* and *stacked impact*) with the animations embedded as HTML, so the direction could be
  chosen before the page was touched.
- The loop became an expanding ring: `AmplificationLoop.tsx` → **`AmplificationRing.tsx`**, with 1.43x
  in the middle of the ring and the tagline below it.
- Then the corrections that only show up once it is on the page: "Amplification" moved out of the circle
  to the top left, the original legend restored, the report's own amounts carried into the ring's legend,
  and the draw slowed down after it read too fast to follow.
- First-year merchant spend corrected to **$16,560**, read off the ledger rather than left as a figure
  typed into the page.
- **Live metrics deliberately not wired up.** There is no way to pull the offchain financials live, so
  anything built now would be a hardcoded number wearing the costume of live data — worse than the
  static figure it replaced, because it would invite trust it cannot earn.

Files: `src/app/(about)/how-it-works/page.tsx`, `src/content/about.ts`,
`src/features/how-it-works/AmplificationRing.tsx`, `AmplificationLoop.tsx`, `src/styles/globals.css`.

### Past-event gallery modal — 0.75h · webpage + app

Fri 15:08–15:13 and 15:44–15:46; committed 15:53 in both repos.

- `ArchiveEventCard.tsx` rewritten (`"use client"`): the card keeps its title picture, and that picture
  becomes the button. A `{n} photos` badge appears only when there is more than one, so single-photo
  events do not advertise a gallery they do not have.
- **`EventGalleryModal.tsx`** (new): Esc and arrow keys, scroll lock, a focus trap that restores focus
  on close, a thumbnail strip, per-photo download and a *Download all* that spaces its requests 400ms
  apart rather than firing a dozen at once.
- Downloads target **`image.src`**, the original upload, not the `next/image` URL — so what lands on disk
  is the full-resolution file somebody can actually use, which is the point of making them downloadable.
- `scripts/dev-up/dev-up.sh` now prefers a sibling `../webpage` checkout over the git clone, with
  `WEBPAGE_DIR=git` to force the clone back. Local website work reaches the dev stack instead of being
  masked by a fresh clone of `main`.

### Falafelland payment failure diagnosed — 0.15h · app

Sat 12:57–13:06. **Investigation only; no code written.** Reported as one merchant's problem; it was not.

- Payments were landing normally — 8 transfers from 8 different customers to the till
  `0xe28df651…`, 151 SFLUV, between 12:17:38 and 12:30:25 — and then stopped. Working for everyone and
  then failing for everyone points at a shared dependency, not a merchant's configuration.
- The tx sender behind a successful payment is the sponsored-transaction EOA **`0x4e346660…`**, which is
  exactly what the Celo paymaster `0x825b77eE…` returns from `sponsor()`. The paymaster holds no balance
  of its own and has no EntryPoint deposit, so every sponsored payment's gas comes from that one EOA.
- It held **0.0872 CELO**, nonce frozen at 790. The transactions are EIP-1559 with maxFeePerGas 402 gwei
  and a gas limit of 250–300k, so a node reserves **0.100–0.121 CELO upfront**. Below that nothing can be
  submitted at all — which is why there was no on-chain trace and no error anywhere in our own logs to
  find. At the 202 gwei actually paid the balance looks like two payments of headroom; the submittable
  count was zero. That gap between "looks nearly empty" and "is already empty" is the trap.
- **Faucet and reward sends kept working throughout**, because the bot wallet `0x21Df0dfC…` signs and
  pays its own gas and holds 11.93 CELO. Rewards flowing while purchases failed is what made a
  system-wide outage look merchant-specific.
- The key is Citizen Wallet's — the paymaster's `owner()` `0x2B5fA761…` appears nowhere in this repo —
  but it is an EOA, so it can be topped up without holding the key. **Nothing monitors its balance**;
  the first signal was a merchant reporting failures mid-service.

---

# Smaller items

| Item | Hours | Repo |
|---|---|---|
| Merged `feat/forms-and-site-editing` into `pjol/volunteer-gallery` so the gallery sits on top of the site-editing work rather than diverging from it (36 files, +1,872/−406 brought across). | 0.10h | webpage |
| Previous week's branch scope closed out, the dev meeting measured from the transcript rather than assumed, and a direct question answered about whether the quiz is live on the website — checked against the deployed site, not the repo. The bulk of that sitting belongs to `pjol/merchant-unwrap-pending-states.md` and is not counted again here. | 0.02h | app |
| This branch scope document, and the invoice export at the top of this file. | 0.21h | app |

---

# Totals

| Source | Hours |
|---|---|
| Measured — animations | 6.63h |
| Measured — webpage | 2.06h |
| Measured — app | 0.45h |
| Measured — webpage + app, Friday's gallery span | 0.75h |
| **Measured subtotal** (0.02h absorbed, 0.34h excluded as already recorded, two spans extended to their commits) | **9.88h** |
| Dev meeting, Mon 5 Oct 2–3pm — inside the animations span, counted once there | 0h |
| Thu 8 Oct 12:00–15:00 as stated, less the 0.08h measured inside the window | 2.92h |
| **Total** | **12.8h** |

# Volume

**animations** — 1,154 authored files touched Oct 5, excluding `node_modules` and the Python venv stood
up that day (the raw count including dependencies is 24,809 and means nothing). A new `edit/` pipeline of
roughly 30 analysis scripts; 81 video files rendered, 3,946 MB; three drafts delivered and a 16-file
handoff package. Not under version control.

**webpage** — 13 files, +1,076/−190 across 7 commits on `pjol/volunteer-gallery`, plus the merge bringing
36 files and +1,872/−406 from `feat/forms-and-site-editing`. One new component (`EventGalleryModal.tsx`),
one replacing another (`AmplificationRing.tsx` for `AmplificationLoop.tsx`), four option write-ups.

**app** — 2 files, +25/−6, one commit (`125472f`). **No migrations and no new routes** on my side of this
branch; 1.61–1.64 and the `/site/*` routes are Sanchez's and are documented in
[`website-cms.md`](../sanchezo/website-cms.md). The Falafelland work produced no code.

# Open

- **Fund the sponsor EOA `0x4e346660…`.** At 202 gwei, 1 CELO buys roughly 25 sponsored payments. Nothing
  watches the balance, which is the part worth fixing rather than the top-up itself — a boot-time and
  periodic check against about 5x the per-transaction reserve would have caught this hours earlier.
- **The three How it Works options were never formally picked.** The ring went in directly, and the
  write-ups remain in `how-it-works-examples/` as a record of the alternatives. Option B may need
  redrawing against the real annual-report artwork; that PDF is image-based and could not be read.
- **`pjol/merchant-unwrap-pending-states` is unmerged.** It carries Monday morning's app time and the
  11:15–12:00 meeting. When it lands, the two documents sit adjacent rather than overlapping — the
  exclusions above are what keeps that true.
