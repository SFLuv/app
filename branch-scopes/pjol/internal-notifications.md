# Branch scope — `pjol/internal-notifications`

Oct 19–20 2026 · app + mobile-app + webpage · **2.0h** so far

Internal organization notifications on the mobile app, plus a round of website updates.

**This branch changed direction mid-flight.** It began as `pjol/telegram-notifications` and a working
Telegram bot was built on it — three alert channels, in-chat action buttons, server-side action tokens.
None of it was committed. Sanchez then pushed a bulletin-board and group-chat featureset to both
repositories, and the decision was to build the same notification behaviour on top of that instead of on
a third-party chat service. The Telegram layer was removed rather than left dormant; what survived it is
named under "Salvaged" below, because that work is the reason the replacement was quick.

## How these hours were measured

**2.0h stated** by the owner. **Nothing is measured yet**, because there is no session work on this
branch to measure: the only SFLuv transcript activity since Oct 18 is this session, which opened at
10:48 today.

| Source | Hours | Basis |
|---|---|---|
| Mon 19 Oct 14:00–15:00 — dev meeting | 1.00h | stated |
| Tue 20 Oct 10:00–11:00 — website reviewing | 1.00h | stated |
| | **2.0h** | |

**Both figures are stated and neither is corroborated.** A meeting leaves no trace in a transcript or a
repository, and reviewing a site is reading rather than writing, so there is nothing for the clock to
find. Recorded as stated, per the convention.

One thing the records do show, named because this is where a measurement's weaknesses go: 0.13h of
activity in another, non-SFLuv project overlaps the end of the review window (10:42–10:50).

## Where this branch starts

Cut from `pjol/affiliate-touchups` at `00d1241`, not from main — so it **carries five unmerged commits**
that belong to the other branch:

| Commit | What |
|---|---|
| `0a3c79a` | Salesforce V4S partner availability |
| `effdbc6` | Protocol account funding / auto top-up |
| `07c709b` | Merge of main (Railway prep, jobs leader) |
| `20d942f`, `00d1241` | Affiliate branch scope checkins |

This matters at merge time rather than now: a PR from here against main would carry the affiliate work
with it, and that work's own scope document describes it. Either merge `pjol/affiliate-touchups` first,
or expect both to arrive together. It is recorded here so whoever opens the PR is not surprised by the
diff.

## Large features

*Nothing yet.*

## Smaller items

*Nothing yet.*

## Totals

| Source | Hours |
|---|---|
| Stated — Mon 19 Oct dev meeting | 1.00h |
| Stated — Tue 20 Oct website reviewing | 1.00h |
| **Measured** | **0.00h** |
| **Total** | **2.0h** |

## Volume

Nothing committed on this branch. `app` and `webpage` are both at their branch points.

## Open

- **The website repo is on another branch with uncommitted work in it.** `webpage` is on
  `pjol/volunteer-gallery` with one modified file, `src/lib/volunteer-events/format.ts` — the partner
  spot-count change from the affiliate branch's round 2, still unshipped. Website edits made now would
  sit on top of it, so it wants committing or moving before this branch's work starts.
- **Nothing is pushed.** `pjol/internal-notifications` is local only.
