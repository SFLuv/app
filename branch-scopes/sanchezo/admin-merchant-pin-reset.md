# Branch scope — `sanchezo/admin-merchant-pin-reset`

Oct 23 2026 · app (backend + frontend) · **~0.3h active**

Hours are wall-clock time from this session's timestamps (about 11:26 to 11:45 AM PDT), not measured with the
`time-accounting` script.

## What this branch is

Niebla SF's owner forgot his merchant-mode PIN during onboarding. Changing the PIN and turning merchant mode
off both require the current PIN, and the only "help" path emails techsupport@sfluv.org. The reset was done by
writing a bcrypt hash into `merchant_mode_settings` by hand from the Railway Postgres console.

This adds an admin reset:
- **Backend:** `POST /admin/users/{user_id}/merchant-mode-pin` (`withAdmin`). It takes an optional `{ "pin": "123456" }`;
  with no PIN, it generates a random 6-digit one. It sets the hash without the current PIN, clears
  `failed_attempt_count` and `locked_until`, and refuses non-merchants (403). It returns the PIN once and logs
  who reset whose PIN, never the PIN itself. `AppDB.AdminSetMerchantModePIN` shares the upsert with
  `SetMerchantModePIN`.
- **Frontend:** Admin → Users → user modal → **Merchant mode PIN** section (merchants only). Optional PIN box,
  "Set PIN" / "Generate new PIN", and the result or error shown inline.

| Item | Hours | Repo |
|---|---|---|
| Admin PIN reset endpoint, DB function, user-modal section, build and type-check | 0.3 | app |

Volume: 4 files (+ this scope file). 0 migrations, 1 new route.
