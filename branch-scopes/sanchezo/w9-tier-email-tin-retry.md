# sanchezo/w9-tier-email-tin-retry

**Repos touched:** `SFLuv/app` (this branch) and `SFLUV_MobileApp` (branch `sanchezo/w9-tin-correction`).
**Status:** built and type-checked, SQL dry-run against a local database; not hand-tested end to end. Not merged, not deployed.
**Dates:** 2026-11-01. **0.3h active, measured.**

## What it is

Two follow-ups to the TaxBandits go-live, both prompted by what happened to the first real filings:

1. **Tier emails.** Tier prompts fire at redemption events, in a crowd, with no Social Security card
   to hand. On 2026-10-25 five people dismissed the prompt within minutes and none had filed a week
   later. The push and modal are gone once dismissed, so each tier now also emails the
   non-expiring form link, once per tier per year.
2. **Failed TIN match.** A W-9 whose name and number don't match IRS records is marked invalid, but
   the person was only told "could not be verified", and the app kept handing back the rejected
   form's link, which can't be submitted again. Now the first failure explains what to check and
   gives a fresh form on request; a repeat failure sends them to support.

## Time

**0.3h, measured** with the `time-accounting` skill (`measure_sittings.py`, 30-minute gap) from the
session transcript, restricted to this branch's work: from the message that asked for the design
(09:06 MST) to the commit (09:23 MST). The same transcript covers months of unrelated work, and the
same sitting began at 08:54 with an unrelated W-9 report rerun, which is excluded.

| Span (MST) | Hours | Work |
|---|---|---|
| 09:06–09:09 | 0.05 | Design: when to email, how to word it while enforcement is off, why old form data is not reused |
| 09:09–09:23 | 0.25 | Backend, admin card, mobile wording, SQL dry run, type checks, this doc |

The split inside the sitting is apportioned by message times and is approximate; the total is
measured. Work was done with Claude Code.

**Volume:** app 13 files, +632/−51 (1 migration, no new routes); mobile 4 files, +79/−22.

## Features, largest first

| Feature | Repo |
|---|---|
| Tier emails (`handlers/w9_email.go`): the notice, warning, first escrow and first refusal each email the form link once per year, off the request path. The wording follows enforcement: with it off, nothing says money is held, and the warning reads "past the limit" rather than "close to it" for someone already over. Not sent for cleared or rejected filings. Off switch: `W9_TIER_EMAILS_ENABLED=false` | app |
| Failed TIN match, first time: push, email listing what to check (number digit by digit, name exactly as on the Social Security card, business name with the business's EIN), and the in-app prompt re-armed. No form is made then; a new one costs a TaxBandits credit, so it is made when the person taps "Fill out my W-9" (`StartW9`) or an admin sends one | app |
| Failed TIN match, repeat: push and email asking them to email techsupport@sfluv.org; `StartW9` refuses with that message (409). An admin send still issues a new form, which is how support unsticks them | app |
| `ReissueInvalidW9Request` + `ReissueW9ProviderRequest`: a fresh vendor request for an invalid filing, resetting status and TIN fields but keeping the failure count. Only touches an invalid filing, so it cannot undo a completion it raced | app |
| `w9_filings.tin_mismatch_count` (migration 1.77), incremented by `RecordTINMatch` on a rejection; existing rejected rows start at 1 | app |
| `/w9/status`: `correction_needed`, `support_needed`, `tin_mismatch_count`; the rejected form's link is no longer returned | app |
| Mobile prompt: "We need a corrected W-9" with what to check (first failure, notice/warning tiers), or "We couldn't verify your W-9" with the support address and no form button (repeat, any tier). Older builds still get the notice/warning prompt, and the push and email carry the explanation | mobile |
| Admin W-9 card: "Failed the IRS TIN match once / N times", what the person has been told, and "Send a new W-9 form" | app |
| Admin MCP `w9_report`: `tin_mismatches`, and `rejected_at` uses the TIN match time | app |

## Decisions

- **Old form data is not reused.** Pre-filling a corrected form would mean fetching the rejected
  W-9 through our servers, SSN included, which the backend otherwise never sees. It also wouldn't
  help: the mistake is in exactly the fields that would be pre-filled.
- **The payout path never makes a new form.** It still calls `EnsureW9Request`, which only re-links
  the existing request. Neither does the admin send pre-check. A new credit is spent only on an
  explicit tap or admin action.
- **Two failures, then a person.** TaxBandits limits TIN-match attempts per recipient, and a second
  failure is rarely another typo.

## Not done

- Not hand-tested on the dev stack. Worth running: a sandbox filing with a deliberately wrong TIN,
  then the fresh form, then a second wrong TIN.
- No mobile build (builds only when asked).

# Round 2 — 2026-11-02

**0.1h, measured** (08:12–08:15 MST, from the prompt time and the shell clock; this turn's transcript
entries were not yet written when it was measured).

| Fix | Hours | Repo |
|---|---|---|
| A payout or a link refresh put a rejected filing back to `requested` while it still pointed at the rejected form (`EnsureW9FilingRequestedTx`, `SaveW9ProviderRequest` treated `invalid` like any open filing). Seen in production: a volunteer's filing went from invalid to pending when he earned 25 SFLUV on Nov 1, which hid the rejection from the report and the admin card and would have let the app re-open the rejected form instead of issuing a fresh one. Both upserts now keep `invalid`; migration 1.77 also puts any open filing with a rejected TIN match back to `invalid`. SQL dry-run against a local database | 0.1 | app |

**Volume:** 2 files, +17/−4.

# Round 3 — 2026-11-02

**0.1h, measured** (08:17–08:29 MST, prompt time to commit by the shell clock; includes the round-2
follow-up question about another volunteer's check).

| Change | Hours | Repo |
|---|---|---|
| A first failed TIN match now issues the fresh form automatically, instead of waiting for a tap or an admin. The email carries its link ("Fill out a corrected W-9"); if the vendor call fails, the email falls back to "open the app", where tapping still makes it. One credit per person per year at most, bounded by the failure count. Form and email run in a goroutine so the webhook reply isn't held up. `correction_needed` stays set on the fresh form until it clears, so the app keeps the "corrected" wording; the admin card says whether they have the fresh form yet. Supersedes round 1's "no form is made at rejection time" | 0.1 | app |
| Admin MCP `w9_report`: `tin_match` (matched / pending / rejected) and `tin_match_at`, so the IRS result can be checked without the database or the TaxBandits console | 0.0 | app |

**Volume:** 6 files, +95/−48.
