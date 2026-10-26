# Branch scope — `sanchezo/terms-dialog-every-step`

Oct 23 2026 · app (frontend) · **~0.2h active**

Hours are wall-clock time from this session's timestamps (about 11:15 to 11:25 AM PDT), not measured with the
`time-accounting` script.

## What this branch is

During Niebla SF's Bridge onboarding, "Accept banking terms" did nothing. The backend logs show both clicks
returned 200 with a Bridge terms link (`POST /merchant/payout/tos-link`, 18:10:39 and 18:13:44 UTC). The bug was
in the card itself. `LocationPayoutCard` rendered the terms `<Dialog>` only in its final (unwrap) return, but
each earlier step (verify business, connect bank, finish setup) returns early with its own markup. Opening the
dialog from those steps set `tosOpen` with no dialog on the page. The same gap broke "Connect bank account"
whenever Bridge reported terms still pending, because that path opens the same dialog.

The dialog is now a single `termsDialog` element rendered in every step's return. Before verification is done,
its confirm button reads "I've accepted" and re-reads status instead of calling `connectBank()`, which would
fail until Bridge verifies the business.

| Item | Hours | Repo |
|---|---|---|
| Trace the silent button through the backend logs, render the terms dialog in every step, and make the confirm button match the step | 0.2 | app |

Volume: 1 file (+ this scope file). 0 migrations, 0 routes.
