# sanchezo/w9-correction-prompt-on-payout

**Repos touched:** `SFLuv/app` (this branch).
**Status:** built; each case run against a local database through a throwaway test, not on the dev stack or a phone. Not merged, not deployed. **Deploy after the mobile over-the-air update**, so the one showing carries the corrected wording.
**Dates:** 2026-11-04. **0.1h active, measured** (12:36–12:38 PST, prompt time to commit by the shell clock).

## What it is

A tier prompt is news once, so somebody with a failed TIN match who had dismissed the prompt never saw it again, however much they earned. A rejection from before `onW9TINMismatch` shipped never armed it at all. Seen in production: a volunteer was paid three times after that shipped and nothing came back.

| Change | Hours | Repo |
|---|---|---|
| `onPayoutAfterTINMismatch`: a payout to somebody with one failed match and a dismissed prompt brings the prompt and bell entry back, once. Runs in the path that pays, including shadow mode | 0.1 | app |
| `correction_prompt` marker in `w9_tier_notices`, like `admin_request`: never a prompt of its own, skipped by the bell, deleted when the filing clears | 0.0 | app |

## Decisions

- **Once.** A volunteer can be paid many times a week. The bell entry stays either way, and once enforcement is on, somebody with a failed match will not be getting many more payouts.
- **No form and no email.** `StartW9` makes the fresh form, one vendor credit, only if they tap through.
- **A prompt not yet dismissed does not use the showing.** Two failures go to support, which has its own message.
- **Armed before marked.** Failing between the two shows it again on the next payout rather than never. Nothing here can affect the payout; failures are logged.

**Volume:** 4 code files, +85/−2, no migrations, no new routes.
