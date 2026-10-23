# Branch scope — `sanchezo/kyb-awaiting-owners`

Oct 23 2026 · app (frontend) · **~0.1h active**

Hours are wall-clock time from this session's timestamps (about 12:52 to 12:58 PM PDT), not measured with the
`time-accounting` script.

## What this branch is

Z Zoul finished Bridge's business form, and the card then showed "More information needed" with a "Continue
verification" button. Bridge's status was waiting on the owners to verify themselves from Bridge's emails
(`awaiting_ubo`). The button opened what looks like a blank copy of the whole form, which reads as starting a
second verification. Niebla SF sat in the same state until its co-owner verified.

- `kybStatusLabel` now labels `awaiting_ubo` "Waiting on owners to verify". `awaiting_questionnaire` keeps
  "More information needed".
- For `awaiting_ubo` the card hides "Continue verification" and explains that Bridge emailed each listed owner
  a link to verify their identity, and to check inboxes, including spam. This matches the `under_review`
  treatment from #196.
- The button still shows for `not_started`, `incomplete` and `awaiting_questionnaire`, where the business form
  itself needs input. The admin Payouts panel shares the label, so it also reads "Waiting on owners to verify".

| Item | Hours | Repo |
|---|---|---|
| awaiting_ubo label, copy, and no Continue button | 0.1 | app |

Volume: 2 files (+ this scope file). 0 migrations, 0 routes.
