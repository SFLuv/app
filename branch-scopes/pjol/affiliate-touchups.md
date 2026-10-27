# Branch scope — `pjol/affiliate-touchups`

Oct 14–18 2026 · app + animations + webpage · **10.7h** (round 1 **7.2h**, round 2 **2.3h**, round 3 **1.2h**)

Three things that all serve affiliates: a narrated tutorial for requesting a volunteer event, a
print-ready QR code export, and fine-grained organization permissions so an affiliate can give a
volunteer coordinator read access without handing over the whole organization.

Round 2 adds two featuresets that came out of the same week's incidents: partner volunteer
availability, read from St. Anthony's own Salesforce site so our cards show their real numbers, and
the faucet keeping the protocol's other accounts in gas, so the sponsored-payment outage that opened
the week cannot happen unannounced again.

# Round 1 — Oct 14–15 · 7.2h

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

## Large features

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

## Smaller items

| Item | Hours | Repo |
|---|---|---|
| Merge with main (PRs #180, #183) checked: no conflicts, backend builds, frontend at the same 25 pre-existing errors. Route parity proven rather than assumed — 305 → 308, nothing removed, exactly the three new permission endpoints added. | 0.10h | app |
| DB versioning audited: 65 migrations, 1.1 → 1.65, strictly ascending, no duplicates, and `latestDBVersion()` returns the last slice entry, which matters because the runner trusts slice position rather than sorting. `compareVersions` parses integer parts, so there is no string-sort trap at this range. | — | app |
| **13 GB of disk recovered** before any of this could render: build outputs from THASSA, DOGPATCH and BULLSIO. `node_modules` left intact so those repos still work. | 0.05h | — |
| **Chrome would not boot.** An orphaned headless Chrome tree (ppid 1, running 5 days 20 hours on a puppeteer temp profile) was starving new launches — its GPU helper had burned **29.5 CPU-hours** and one renderer 5.3. Killed the tree, reclaimed 14 leftover puppeteer profiles (70 MB), and confirmed Chrome starts again (155.0.8059.79, headless smoke test clean). **No orphaned VS Code processes existed** — pid 508 is one live 7-day session, so nothing there was touched. | 0.12h | — |
| Merge and DB versioning verified after main was pulled (below). | 0.10h | app |
| **`make.sh` failed a gate the tutorial was never subject to.** It hardcoded the grant video's 90-second limit, while `build.py` honours each timeline's own `max_duration` — so a correct 94.3s master rendered, then the build reported `!! 94.300 s IS LONGER THAN 90.0 s` and exited 4. The gate now reads the timeline's value, defaulting to 90.0. | 0.08h | animations |
| **A regression I introduced, found by the volunteer cards.** Adding `IsHouse` to the `Organization` struct, I updated two of the three SELECTs that scan it — `GetOrganizationsByIds` kept a 5-column SELECT against a 6-destination Scan, because the edit that added the destination replaced every occurrence while the edit that added the column did not. Go compiles it; pgx fails at runtime, so the organization batch load returned nothing and every partner event on sfluv.org fell back to the generic "Partner organization" with no logo. Column added; all four SELECT/Scan pairs re-checked. | 0.15h | app |
| `isHouseOrganization` now reads `organizations.is_house` rather than matching the normalized name, which is what the column was added for — a rename can no longer move an event's attribution between SFLuv and a partner. | — | app |
| This branch scope. | 0.05h | app |

---

## Totals

| Source | Hours |
|---|---|
| Measured — app | 1.96h |
| Measured — animations | 1.60h |
| **Measured subtotal** | **3.56h** |
| Stated — Wed before the session | 1.00h |
| Stated — Wed 13:38–15:15, away from the keyboard | 1.62h |
| Stated — Mon 12 Oct dev meeting, 2–3pm | 1.00h |
| **Total** | **7.2h** |

## Volume

**app** — 14 files, +1,702/−152 in `a6396fe`. Backend 7 files (+1,025/−123), frontend 6 (+656/−29).
1 migration (**1.65**), 3 new backend files, 2 new frontend files, 3 new routes, 30 routes re-guarded,
4 dead guards removed.

**animations** — 17 authored files across the comps and the new tooling: a `13-affiliate-event` piece of
8 beats plus a thumbnail still, with its shared kit, and `tts.py`, `make_bed.py` and
`make_captions_affiliate.py`; one fix to `make.sh`. Output: a **94.3s** 1080p master at −14.0 LUFS, a
**32-cue** SRT, and a 1280×720 thumbnail. Not under version control.

## A contradiction in the app, found while fixing the video

The redemption window is described two different ways, and they do not agree:

| Where | What it says |
|---|---|
| The affiliates page tooltip | "become redeemable **24 hours before the event starts**" |
| The event form | "go live at **midnight on the day of the event** and stay redeemable until midnight the day after it ends" |
| The form's defaults | `qr_live_offset_hours` **0**, `qr_expiry_offset_hours` **24** (hours *after end*) |

The tooltip is the odd one out: nothing sets liveness to 24 hours before the start. The video now follows
the form and the defaults. **The tooltip should be corrected**, and it is worth checking which of the two
an affiliate has been believing when they planned a hand-out.

---

# Round 2 — Oct 16–18 · 2.3h

Two featuresets, neither of which touches the database: partner volunteer availability read from
St. Anthony's own Salesforce site, and the faucet keeping the protocol's other accounts in gas.

## How these hours were measured

**2.3h measured** from session-transcript timestamps clustered into sittings on a 30-minute gap
(`time-accounting/scripts/measure_sittings.py`, unioned across the SFLuv project transcripts), taken at
**2026-10-18 13:27 PDT**. Nothing in this round is stated rather than measured.

| Source | Hours | Basis |
|---|---|---|
| Sun 18 Oct 11:10–13:27 | 2.28h | measured — one sitting, see below |
| Fri 16 Oct 14:35–14:43 | 0.14h | measured — already booked in round 1, not counted again |
| | **2.28h → 2.3h** | |

**Friday is not counted twice.** That 8-minute sitting is where the volunteer-card regression was found
and fixed *and* where the St. Anthony's research was done. Round 1's smaller-items table already carried
0.15h for it — written ahead of the measurement that would have shown 0.14h — so this round adds nothing
for Friday, and the research described below is credited to a window already paid for.

**Sunday is one sitting on evidence, not on a looser threshold.** At a 30-minute gap the transcripts
split it in two, 11:10–12:38 and 13:16–13:27, on a 38-minute silence. Commit `effdbc6` was authored
at **13:10**, inside that silence, which is the corroboration the method asks for: the gap is work, not
absence. The span is therefore 11:10 → 13:27. A 45-minute threshold reaches the same span on its
own, but the commit is why it is right rather than merely larger.

**The sitting was still open at the stamp.** Writing this document is inside it, so anything after
13:27 is uncounted — the figure is a floor, not a ceiling. The stamp is given because the previous
branch scope was measured mid-sitting without one and had to be corrected afterwards.

**0.85h of concurrent activity in another project** (`defi-bot`, 11:41–12:27 and 13:09–13:15) falls
inside Sunday's sitting. Both were going on. The span is recorded as SFLuv time because that is what the
sitting was for; the concurrency is named so that whoever reads this later knows what the records show.

## Large features

### Partner volunteer availability — 0.93h · app + webpage

St. Anthony's publishes volunteer sign-ups through Volunteers for Salesforce (V4S), a Visualforce app on
Salesforce Sites. The research first: **there is no REST API to integrate with.** The `GW_Volunteers__`
pages are the only machine-readable surface, sign-ups land as `Status__c = 'Web Sign Up'`, and contacts
are deduplicated on `Firstname;Lastname;Email` by default. Of the three options that leaves, the one
chosen reads the partner's own job listing on demand, shows their real availability on our card, and
deep-links the sign-up button to the exact shift — so the partner keeps their list and their form, and
nothing has to be kept in step.

- **Pull, not poll** (`backend/v4s/client.go`). One fetch per job per TTL (60s for St. Anthony's);
  concurrent askers share a single fetch; a stale snapshot is served while a refresh runs behind the
  request. A job nobody is looking at costs nothing, and a slow partner site degrades to slightly old
  numbers rather than to a slow page. Measured against the live site: a cold fetch of the real listing
  took **853ms** and parsed **42 shifts** out of one 59 KB request, with cached reads at 0–14µs. One
  request may spend at most 2.5s in total on this, however many events it is rendering.
- **The parser was wrong in a way that looked right.** The first version split the page on
  `JobShiftDateTimeString(`, which also matches the *function definition* — so every shift's fields
  landed one row out of step and it reported 45 shifts with every count zero. Rewritten to anchor on the
  shift id and attribute tokens by document order, with the pattern requiring an argument that starts
  with a capital letter, which is what skips the definition.
- **Same-day recurring shifts, asked about before they bit.** Surveying five of St. Anthony's real jobs:
  four carry at most one shift a day, but FCP Clothing Distribution has **58 shifts over two months with
  22 days carrying more than one** — an 8:30am and a 1:00pm sitting. Matching on date alone would have
  put the afternoon's numbers on the morning's card. `MatchOccurrence` matches on time of day inside a
  2-hour window and commits only when it is **confident** (within 45 minutes) or **decisive** (twice as
  near as the runner-up); otherwise it declines and the card falls back to what we knew. An earlier
  version matched an 11:00am occurrence — dead between two sittings — to the 1:00pm shift on a 30-minute
  difference. That is now refused rather than guessed.
- **Only a configured partner can cause a fetch.** `backend/v4s-integrations.json` pairs one
  organization with the one site readable for it, so a partner's event can never send us to another's
  host, or to an arbitrary one if a link is mistyped or hostile. Names are matched ignoring case and
  punctuation; SFLuv's own organization is matched on `organizations.is_house` rather than its name, so
  a rename cannot change what is synced.
- **Closed by default in the code, open by deliberate choice in the shipped file.** No file, an
  unreadable file, the master switch off, or an organization with no entry all mean the same thing: the
  link is not parsed and no request is made. The file checked in here is the opposite — `enabled: true`
  with St. Anthony's on — because it was asked to be ready on deployment. The house entry sits beside it
  switched off, as the worked example for adding the next partner. The boot log states which
  integrations are live, so whether we are reaching out to a third party is visible without reading the
  file back.
- Config lives in JSON rather than the environment because each entry is a *pair* — organization and
  site — which a flat variable cannot express without encoding a list in a string.
- **One line on the website** (`src/lib/volunteer-events/format.ts`): `formatSpots` excluded external
  sign-ups outright, on the reasoning that the partner owned the list and we could not know. For a
  partner who publishes availability we now can, so the test is whether there is a number rather than
  where it came from. A partner event reads "8 of 10 spots" like our own; a partner who publishes
  nothing is unchanged.

### Protocol account funding — 1.17h · app

This is the feature the week's first incident asked for. The account that sponsors user transactions ran
out of CELO, every merchant payment stopped at once, and the first anyone knew of it was a merchant
saying payments "weren't working" — nothing watched the balance, so nothing connected the symptom to the
cause.

- **The faucet becomes the only account a person has to think about.** It tops the others up from its own
  CELO and emails when *it* is low, because that is the one balance no automation can refill.
- **A 1 CELO floor on every account**, as asked. At roughly 0.03 CELO a transaction that is about thirty
  transactions of warning on the paymaster, and its 6 CELO target is about two hundred.
- **The paymaster is first in line, always.** Verified with the faucet nearly empty: with 0.2 CELO to
  spare it funded `Paymaster sponsor +5.8000 CELO (0.2000 → 6.0000)` and skipped the redeemer admin,
  reporting why. An account added later without a priority sorts last, so it cannot displace it.
- **One top-up transaction per account per minute, whatever the balances say.** Verified: `sweep 1:
  funded=1`, then `sweep 2: funded=0 skipped=1 — Paymaster sponsor was topped up recently; waiting for
  it to confirm`. This is the guard against a balance read that is wrong or a loop that is misbehaving:
  the worst case is one transaction a minute rather than a drained faucet.
- **Back-to-back top-ups raise an alert rather than being funded quietly.** An account topped up again
  inside two minutes means it is being drained, not used. Verified: `RAPID: Paymaster sponsor was topped
  up again` → "SFLuv account is draining faster than it is being funded". Throttled per account so a
  draining account produces a warning rather than a mailstorm.
- **The faucet holds 1 CELO back.** Without a reserve a faucet can spend itself down to where it cannot
  send the transaction that would fix anything.
- **Alerts are on by default, and stay on while top-ups are off.** `alerts` absent means enabled, which
  is deliberately the opposite of how the top-up switch behaves: moving money should take an opt-in,
  being told it is about to run out should not. They also fire for a *stranded* account — one below its
  floor with nothing configured to fix it — which is the state a watch-only deployment is actually in.
  Verified against the shipped config: `top-ups enabled=false, alerts enabled=true`, 0 funds moved,
  1 email sent. Recipients come from `FUNDING_ALERT_EMAILS`, falling back to the site admin address so
  an unconfigured deployment still tells somebody rather than nobody.
- **One environment variable flips the transfers.** `FUNDING_AUTO_TOPUP` overrides the file either way,
  so it can be switched on a running deployment without editing anything, and setting it false is the
  kill switch. Verified across every state: unset defers to the file (off), `true`/`on`/`TRUE` move
  funds, `false`/`0` stop them, and an unrecognised value is treated as off — a typo in a switch that
  moves money fails closed. The boot log names which source decided it:
  `funding: auto top-up enabled, alerts on (top-ups set by FUNDING_AUTO_TOPUP, accounts from
  funding-accounts.json)`.
- **An account that cannot be located is named loudly rather than dropped quietly**
  (`funding: NOT WATCHING 2 account(s) with no resolvable address: …`), because an unwatched account
  looks exactly like a healthy one. `REDEEMER_ADMIN_ADDRESS` and `ADMIN_ADDRESS` have to be set or those
  two are not watched.
- **A Balances tab in the admin panel**, ordered by urgency rather than tidiness — anything under its
  floor is pulled to the top — with each balance shown against the floor it is judged by, because a
  number alone does not tell you whether to act. An unreadable balance reads "Unknown", never
  `0.0000`, which would look like an emergency.
- Four primitives added to the bot so it can do this at all: `NativeBalance`, `TokenBalance`,
  `SendNative` and `Address`. The sweep runs every 15 minutes in the background, after a 2-minute delay
  at boot, and the tab's "Top up now" runs the same code path by hand.
- The units are stated at both conversions, since this package mixes them: CELO has 18 decimals, SFLUV
  has **6**, and neither is wei.

## Smaller items

| Item | Hours | Repo |
|---|---|---|
| **No database changes at all this round** — still 65 migrations, 1.1 → 1.65. Both featuresets are config files and code, so there is nothing to run on deploy and nothing to roll back. | — | app |
| Route parity: **308 → 310**, exactly the two new admin balance endpoints, nothing removed. | — | app |
| Backend builds; `go vet` clean on both new packages; `gofmt` clean. The 7 remaining vet findings are all in `handlers/app_contact.go`, untouched since Aug 2025 and untouched on this branch. | — | app |
| Test scaffolding (`NewServiceForTest`) was added to reach the priority and rate-limit paths, then removed — this repo's testing is human-in-the-loop and production code does not carry test hooks. The verification was re-run through the real constructor with the env addresses set, which is also what caught that the first priority run had only loaded 1 of 3 accounts. | — | app |
| This round's scope document. | 0.18h | app |

## Totals

| Source | Hours |
|---|---|
| Partner volunteer availability — app + webpage | 0.93h |
| Protocol account funding — app | 1.17h |
| This scope document | 0.18h |
| **Round 2 measured** | **2.28h → 2.3h** |

## Volume

**app** — 22 files, **+2,618/−11** across `0a3c79a` and `effdbc6`: backend 19 files (+2,347/−11),
frontend 2 (+269), this document (+2). **12 new files** (11 backend, 1 frontend) in 2 new packages,
`backend/v4s` and `backend/funding`, 1,683 lines between them. 2 new JSON config files, 2 new routes,
**0 migrations**.

**webpage** — 1 file, +6/−1, uncommitted on `pjol/volunteer-gallery`.

---

---

# Round 3 — Oct 18 evening · 1.2h

The Railway migration, and bringing this branch up to the main it now has to land on.

## How these hours were measured

**1.1h stated** by the owner for the Railway migration, plus **0.1h measured** for the merge, taken at
**2026-10-18 22:00 PDT**.

| Source | Hours | Basis |
|---|---|---|
| Sun 18 Oct 20:45–21:53 — railway migration | 1.13h → 1.1h | stated |
| Sun 18 Oct 21:54–22:00 — merging main | 0.10h | measured |
| | **1.2h** | |

**The Railway hour is stated, and the records neither support nor contradict it.** There is no session
transcript in that window, and no file in any SFLuv repo was modified during it — which is what work on
a deploy platform looks like, since it happens in Railway's own dashboard and not in the repository.
Recorded as stated.

## Merging main

Main had moved a long way ahead of this branch: Railway preparation, background jobs behind a leader
lock, a `/healthz` endpoint, an advisory lock around the migration runner, and volunteer event photo
edits.

- **One conflict, in `bootstrap/runtime.go`.** Main moved every background job inside
  `StartJobsAsLeader`, so only the process holding the jobs lock runs them; this branch still had the
  old flat list with the new funding sweep on the end of it. Took main's block whole and moved
  `fundingSvc.Start` into the closure, on `jobsCtx`.
- **Leader-only turns out to be required for the funding sweep rather than merely tidy.** The
  once-a-minute top-up guard is in-process state, so two replicas each sweeping on their own timer would
  permit two transfers a minute — exactly double the rate the guard exists to cap, and the guard was
  asked for against a vulnerability. Running on Railway with more than one replica would have defeated
  it quietly. The code now says so where the sweep starts.
- `handlers/bot.go` and `frontend/app/admin/page.tsx` auto-merged.
- **DB versioning re-checked:** still **65 migrations, 1.1 → 1.65**, strictly ascending, no duplicates.
  Main's change to `schema_migrations.go` is not a migration but a Postgres advisory lock around the
  runner, so two processes starting together take turns rather than both applying the same migration —
  which is precisely the hazard a platform that overlaps deploys introduces.
- **The merge is not verified by a build.** The machine is out of disk and the owner asked not to spend
  the session on it. `gofmt` is clean and the resolution is a block move with one call relocated, but
  the build is unconfirmed and is the first thing to run next.

## A correction to round 1

Round 1 records the `GetOrganizationsByIds` regression as found and fixed. It was — *on this branch*,
which was never merged. Main carried the bug from **Oct 15 to Oct 18**: for three days the public
volunteer events list served no organiser names and no logos, and it was fixed on main by someone else
(`8c6d523`) at 21:54 tonight, minutes before this merge. Both sides had made the identical one-line
change, so it merged without a conflict.

The entry in round 1 is not wrong, but it reads as though the problem was dealt with, and in production
it was not. A fix sitting on an unmerged branch is not a fix. Worth noting against the next regression
this branch finds in shipped code: the fix wants to go to main on its own, not to wait for the feature
it was found beside.

## Volume

**app** — merge commit `07c709b`: **12 commits, 18 files** in from main. One file resolved by hand,
`bootstrap/runtime.go`, which now differs from main's by 16 lines — the v4s boot log line, the funding
service's construction, and its leader-gated start. No migrations, no new routes.

---

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
- **Automatic top-ups arrive switched off.** `funding-accounts.json` ships `enabled: false`, which is the
  way to watch the Balances tab for a while before trusting it with transfers. Turning them on is
  `FUNDING_AUTO_TOPUP=true` and a restart; no file edit and no deploy.
- **`FUNDING_ALERT_EMAILS` is unset**, so alerts currently go to the site admin address alone.
  `REDEEMER_ADMIN_ADDRESS` and `ADMIN_ADDRESS` must be set too or those accounts are not watched at all
  — the boot log names any it had to drop.
- **The paymaster sponsor still needs its first funding by hand.** It is an EOA belonging to Citizen
  Wallet, so anyone may fund it but we cannot sign for it; the floor only helps once there is something
  above it.
- **V4S availability ships enabled for St. Anthony's.** First deploy will start reading their listing
  page on demand. If they would rather we did not, `enabled: false` on that entry stops it with no code
  change, and the User-Agent identifies us so they can tell us rather than block an anonymous agent.
- **The website change is uncommitted** on `pjol/volunteer-gallery` — one line in
  `src/lib/volunteer-events/format.ts`. Partner spot counts will not appear until it ships, whatever the
  backend returns.
- **The merge with main is unbuilt.** Resolved by hand in `bootstrap/runtime.go` and `gofmt` clean, but no compile has run against it.
- **The manual "Top up now" button is not leader-gated**, because it runs wherever the request lands. The per-minute guard is per-process, so a manual sweep on a replica that is not the jobs leader cannot see what the leader topped up. Harmless at one replica; worth closing before scaling out on Railway.
- **Partner sign-up counts are read, not reconciled.** Someone who signs up through St. Anthony's is
  counted on our card but is not a user of ours, so a partner event's attendance cannot be rewarded or
  credentialed through the platform. That is the next question this opens rather than a defect.
