# Branch scope — `pjol/affiliate-touchups`

Oct 14–15 2026 · app + animations · **7.2h**

Three things that all serve affiliates: a narrated tutorial for requesting a volunteer event, a
print-ready QR code export, and fine-grained organization permissions so an affiliate can give a
volunteer coordinator read access without handing over the whole organization.

## How these hours were measured

**3.56h measured** from session-transcript timestamps clustered into sittings on a 30-minute gap
(`time-accounting/scripts/measure_sittings.py`), taken at **2026-10-15 11:35 PDT**, plus **3.62h stated**
by the owner for work the transcripts cannot see.

| Source | Hours | Basis |
|---|---|---|
| Wed 14 Oct 11:49–13:38 | 1.82h | measured |
| Wed 14 Oct 19:55–20:38 | 0.71h | measured |
| Thu 15 Oct 10:33–11:35 | 1.04h | measured |
| Wed 14 Oct, before the session | 1.00h | stated |
| Wed 14 Oct 13:38–15:15, away from the keyboard | 1.62h | stated |
| Mon 12 Oct 2–3pm, dev meeting | 1.00h | stated |
| | **7.18h → 7.2h** | |

The per-feature hours below are an apportionment of the measured sittings, split on the boundaries of
when each request arrived. They sum to 3.5h against a measured 3.56h; the measured figure is the one
that counts, and the split is approximate below 0.1h.

**The stated hours are the owner's, and they are the point of this section rather than a caveat on it.**
A transcript records keystrokes, not work: design decided before opening the editor, and a long stretch
of thinking through the permissions model away from the machine, leave no trace in it and are no less
real for that. Where the owner states a figure, it is recorded as stated.

What the transcripts *can* say about those windows, for whoever reads this later:

- The Wednesday 13:38–15:15 window has 1.61h of concurrent activity in another project (`MOX`). Both
  were going on; the SFLuv share of it is stated, not inferred from the records.
- Nothing was written to the SFLuv repos in that window, which is what being away from the keyboard
  looks like.

**Render time is counted, and sits inside the measured sittings.** A studio render is minutes of real
waiting and reviewing. The four runs, from their own logs: Wed 12:21–12:26 and 12:27–12:36, Wed
20:26–20:37, Thu 10:34–10:36 — about 27 minutes, all inside the sittings above, so none of it extends
anything.

**The sitting gap makes no difference here.** The measured figure is the same at 30 minutes and at 120,
because the breaks between sittings are 6.3 hours and 13.9 hours. Widening the threshold was tried and
changed nothing; the stated hours above are what covers that time.

---

# Large features

### Organization permissions — 0.50h measured, plus the stated Wednesday afternoon · app

Membership was all-or-nothing: every member of an organization inherited every platform role it held, in
full. A volunteer coordinator who only needed to read the workflow list had the same power as the person
running the organization.

- **Nothing changes on the day it ships.** A member's `permissions_mode` defaults to `inherit`, which
  resolves every capability of every role their organization holds — exactly what membership meant
  before. Only switching a member to `custom` narrows them. Org admins and superadmins are always
  unrestricted, since they are the ones handing grants out.
- Proven rather than asserted: the real schema was dumped into a throwaway database, migration **1.65**
  applied twice (idempotent), and the old `IsAffiliate` query compared against the new resolution row by
  row. Ordinary member, org admin and inactive user all resolve identically; only a member deliberately
  set to `custom` with no grants comes back false.
- **30 routes** moved from `withAffiliate` / `withProposer` / `withIssuer` / `withSupervisor` to
  `withCapability(...)`, with capabilities derived from what each endpoint does rather than invented.
  The four old guards are now dead and removed, so there is one mechanism rather than two. The org-level
  role approval is still checked underneath, so revoking an organization's affiliate status kills every
  affiliate grant inside it with no cleanup.
- The capability catalog is code, not data — a capability only means something once a route checks it —
  but adding one needs no migration, since grants store the key as text.
- **The SFLuv administrative section** is never inherited and only the house organization's superadmin
  can configure it. Its website toggles write the existing `website_*` credentials underneath, so site
  editing is enforced exactly as it was and credentials granted through the issuer panel still show as
  granted.
- **`organizations.is_house` added.** The codebase identified SFLuv by matching the normalized name, and
  `isHouseOrganization`'s own comment asked for a column "the next time that table is migrated". For
  branding a rename was cosmetic; for gating admin powers it would hand them to a partner or strip them
  from staff. Backfilled from the same name match, so the answer today is identical.

### Affiliate tutorial video — 1.60h measured, across three rounds · animations

A narrated 87.5-second explainer for requesting a volunteer event: open the form, title and dates, cover
photos, the two numbers, visibility, sign-up, submit and approval, download the codes. One focus on
screen at a time throughout.

- Content checked against the real flow rather than a simplified story: events are **request-only** for
  affiliates, and approval is what reserves faucet funds and mints the codes, so the tutorial says that
  rather than implying codes appear on submit. Every on-screen string is sourced at the bottom of
  `comps/13-affiliate-event/piece.js`.
- **Narration** is ElevenLabs `eleven_v4`, one file per beat so each line sits on its own beat
  (`edit/audio/tts.py`, new). Verified by transcribing every line back through Scribe and diffing
  against what was written — which caught that "sfluv.org" was being read as one word, and later that
  two transcripts were stale from an earlier version of the script.
- **Music is synthesised** (`edit/audio/make_bed.py`, new), so the bed carries no third-party rights and
  needed no licence or attribution. It fits the cut exactly and ducks under the narration automatically.
- **Round 2** added the cover-photo step and gave Visibility and Sign-up a beat each, walking their
  options one at a time with a plain-English line under each. The highlight timings come from the
  narration's own word timings, so an option lights on the words describing it rather than near them.
- **Captions for YouTube** (`edit/tools/make_captions_affiliate.py`, new): 29 cues, UTF-8, validated for
  sequential indices, no overlaps and at most two lines. Words come from the script and only timing from
  the recogniser, and a line spelled for the voice ("SFLuv dot org", "twenty-four") carries a separate
  `caption` so the subtitle reads properly while the audio is unchanged.
- **Round 3** corrected the ending, and in doing so found the video had been wrong. It said the codes
  "go live twenty-four hours before your event starts". The form's own defaults are `qr_live_offset_hours`
  **0** and `qr_expiry_offset_hours` **24**: the 24 hours is the *expiry after the event ends*, not
  liveness before it starts. The closing beat now states the real default — live from the morning of the
  event, redeemable until 24 hours after it ends — and that both ends are adjustable, which was the
  thing an affiliate most needed to know and the video never said.
- Two more overlaps retired in the same pass: the Download button no longer lingers behind the card fan,
  and the card group was lifted so it never touches the window pills.
- **A thumbnail** (`comps/13-affiliate-event/thumb.html`): 1280×720, the printed card angled beside the
  title, built to survive being shown at 320px wide — few elements, very large type, one coral accent.
  The backdrop's ambient hearts are switched off in it, because one clipped at the frame edge and read
  as a glitch at that size.
- **The captions went stale and were caught in review, not by the tooling.** vo8 was rerecorded and
  Scribe was re-run on it to retime the beat, but the subtitle was not regenerated, so the published SRT
  still carried the line the video no longer said. The generator could not catch it: it reads whichever
  transcripts exist, and these were current. The rule is to re-run it after *any* narration change,
  which the README now says. Final SRT: 32 cues, ending 93.93s against a 94.3s master.

### QR codes print-ready — 0.95h · app

The export put one card on a 55mm page, so whoever printed it had to impose the sheet themselves, and
there was no way to get the translated reverse onto the back at all.

- **US Letter, four cards a page, fronts and backs interleaved.** The card is 4.25 × 5.5in, exactly a
  quarter of Letter, so a 2×2 grid tiles with no scaling and no margin. Page order is front, back,
  front, back, so "Print → Two-Sided" with default options produces finished cards.
- Backs are placed in **column-mirrored** order. Duplex flips about the long edge, so top-left returns as
  top-right. Every back is identical today so it is invisible, but it stays correct the moment one
  carries something card-specific such as the code number.
- **The reverse was trilingual, not bilingual.** Text extraction from the reference returned Spanish and
  Chinese; the **Arabic block was embedded as a 378×212 bitmap** and would have been silently dropped. It
  is live text now, which also fixes it printing at roughly 150dpi. Wording is verbatim, except that the
  reference encoded 支, 二 and 用 as Kangxi radicals — identical in print, different characters, and they
  would not match a search or a re-translation.
- **A resolution regression caught in the same pass.** Printing the card at its full 4.25in meant the
  existing `SCALE = 2` — whose comment justified itself with the old 55mm page — was suddenly producing
  **200dpi**. Raised to 3 for a true 300dpi; the file-size cost is about 25%, since the backs are mostly
  white.
- Verified end to end by generating a real PDF through jsPDF in headless Chrome and inspecting it: Letter
  pages, every placement exactly 4.250 × 5.500in, and a part-full sheet getting only the backs it needs.

---

# Smaller items

| Item | Hours | Repo |
|---|---|---|
| Merge with main (PRs #180, #183) checked: no conflicts, backend builds, frontend at the same 25 pre-existing errors. Route parity proven rather than assumed — 305 → 308, nothing removed, exactly the three new permission endpoints added. | 0.10h | app |
| DB versioning audited: 65 migrations, 1.1 → 1.65, strictly ascending, no duplicates, and `latestDBVersion()` returns the last slice entry, which matters because the runner trusts slice position rather than sorting. `compareVersions` parses integer parts, so there is no string-sort trap at this range. | — | app |
| **13 GB of disk recovered** before any of this could render: build outputs from THASSA, DOGPATCH and BULLSIO. `node_modules` left intact so those repos still work. | 0.05h | — |
| **Chrome would not boot.** An orphaned headless Chrome tree (ppid 1, running 5 days 20 hours on a puppeteer temp profile) was starving new launches — its GPU helper had burned **29.5 CPU-hours** and one renderer 5.3. Killed the tree, reclaimed 14 leftover puppeteer profiles (70 MB), and confirmed Chrome starts again (155.0.8059.79, headless smoke test clean). **No orphaned VS Code processes existed** — pid 508 is one live 7-day session, so nothing there was touched. | 0.12h | — |
| Merge and DB versioning verified after main was pulled (below). | 0.10h | app |
| **`make.sh` failed a gate the tutorial was never subject to.** It hardcoded the grant video's 90-second limit, while `build.py` honours each timeline's own `max_duration` — so a correct 94.3s master rendered, then the build reported `!! 94.300 s IS LONGER THAN 90.0 s` and exited 4. The gate now reads the timeline's value, defaulting to 90.0. | 0.08h | animations |
| This branch scope. | 0.05h | app |

---

# Totals

| Source | Hours |
|---|---|
| Measured — app | 1.96h |
| Measured — animations | 1.60h |
| **Measured subtotal** | **3.56h** |
| Stated — Wed before the session | 1.00h |
| Stated — Wed 13:38–15:15, away from the keyboard | 1.62h |
| Stated — Mon 12 Oct dev meeting, 2–3pm | 1.00h |
| **Total** | **7.2h** |

# Volume

**app** — 14 files, +1,702/−152 in `a6396fe`. Backend 7 files (+1,025/−123), frontend 6 (+656/−29).
1 migration (**1.65**), 3 new backend files, 2 new frontend files, 3 new routes, 30 routes re-guarded,
4 dead guards removed.

**animations** — 17 authored files across the comps and the new tooling: a `13-affiliate-event` piece of
8 beats plus a thumbnail still, with its shared kit, and `tts.py`, `make_bed.py` and
`make_captions_affiliate.py`; one fix to `make.sh`. Output: a **94.3s** 1080p master at −14.0 LUFS, a
**32-cue** SRT, and a 1280×720 thumbnail. Not under version control.

# A contradiction in the app, found while fixing the video

The redemption window is described two different ways, and they do not agree:

| Where | What it says |
|---|---|
| The affiliates page tooltip | "become redeemable **24 hours before the event starts**" |
| The event form | "go live at **midnight on the day of the event** and stay redeemable until midnight the day after it ends" |
| The form's defaults | `qr_live_offset_hours` **0**, `qr_expiry_offset_hours` **24** (hours *after end*) |

The tooltip is the odd one out: nothing sets liveness to 24 hours before the start. The video now follows
the form and the defaults. **The tooltip should be corrected**, and it is worth checking which of the two
an affiliate has been believing when they planned a hand-out.

# Open

- **Nothing is deployed.** Migration 1.65 runs on next boot; the permissions UI and the QR export ship
  with the frontend build.
- **The dashboard has no way to see a member's effective access from their own side.** `GET
  /organizations/mine/permissions` is readable by any member so they can, but nothing in the UI surfaces
  it to a non-admin yet.
- **The tutorial is 94.3s**, up from 57s as the photo step, the two dropdown beats and the corrected
  ending went in. If it needs to be shorter, the two dropdown beats are 30s of it.
- **The affiliates page tooltip contradicts the form** on when QR codes go live. See above; the video
  now follows the form, and the tooltip is the thing to fix.
