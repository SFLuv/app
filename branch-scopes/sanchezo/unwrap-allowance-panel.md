# Branch scope — `sanchezo/unwrap-allowance-panel`

Oct 23 2026 · app (frontend) · **~0.2h active**

Hours are wall-clock time from this session's timestamps (about 12:01 to 12:12 PM PDT), not measured with the
`time-accounting` script.

## What this branch is

Merchants onboarding to Bridge payouts needed to see the unwrap rules for their own location, not a generic
sentence. The rules (`backend/handlers/unwrap.go`): the first unwrap in a UTC month can be any amount; further
unwraps that month must be at least $500; the till and tips share one allowance per location.

The Unwrap screen of the Bank & Unwrap card now shows an allowance box in place of the static rule text:
- **Unwrapped this month:** the last unwrap date, when the next any-amount unwrap opens (the start of next month
  in UTC, shown in the merchant's local time), and that they can still unwrap $500 or more before then.
- **Not unwrapped this month:** the next unwrap can be any amount, and further ones this month need $500 or more.

The data comes from the existing `POST /unwrap/eligibility`, probed with a 1-base-unit amount. It returns
`last_unwrap_at` and `minimum_followup_amount_wei` whether or not the probe is allowed. If the probe fails, the
old generic sentence shows. No backend change.

| Item | Hours | Repo |
|---|---|---|
| Allowance box on the Unwrap screen, read from the eligibility check | 0.2 | app |

Volume: 1 file (+ this scope file). 0 migrations, 0 routes.
