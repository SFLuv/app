# Branch scope — `sanchezo/kyb-under-review-state`

Oct 23 2026 · app (frontend) · **~0.1h active**

Hours are wall-clock time from this session's timestamps (about 11:49 to 11:55 AM PDT), not measured with the
`time-accounting` script.

## What this branch is

Niebla SF's owners finished Bridge verification and the card showed "Under review", but still offered
"Continue verification". That opens Bridge's hosted link for the existing customer, which looks like a blank
copy of the whole form, and resubmitting it can overwrite their answers or restart the review.

While `kyb_status` is `under_review`, the Bank & Unwrap card now hides "Continue verification" and explains that
there's nothing more to fill in: Bridge will email if they need anything, and the card moves on to connecting a
bank once approved. The button still shows for `not_started`, `incomplete` and `awaiting_*`, where the merchant
does have something to do. The banking-terms button is unaffected.

| Item | Hours | Repo |
|---|---|---|
| Hide "Continue verification" while under review; under-review copy | 0.1 | app |

Volume: 1 file (+ this scope file). 0 migrations, 0 routes.
