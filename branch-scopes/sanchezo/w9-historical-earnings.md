# Branch scope: `sanchezo/w9-historical-earnings`

Oct 25 2026 · app (backend) · **~0.4h active** · based on `main` at `24ec8b4`

Hours are wall-clock time from this session's timestamps, not from the `time-accounting` script. It was one
sitting, about 12:33–12:58 PDT:
- about 0.2h finding why no W-9 warning had ever fired;
- about 0.2h on the fix, the import SQL, and tests.

## Why no automatic W-9 warning ever fired

- **The thresholds sum `payout_ledger`, which only starts on Sept 8 2026**, when the rebuilt W-9 system went live.
  Earnings from Jan 1 to Sept 7, on Berachain and early Celo, were never imported. The `historical_ponder` source
  exists for that import, but nothing ever wrote it.
- **Inside that window**, the only person past 400 already has a legacy-approved W-9. Everyone else is under 400,
  so no tier was ever recorded. Production has 0 rows in `w9_tier_notices`, and no `shadow_decision` values.
- **Meanwhile**, the chain-based report shows several people past 400 or 600 for 2026 with no W-9.

## What this branch is

1. **Shadow mode no longer shows "held" or "blocked" prompts.** With `W9_ENFORCEMENT=false`, a payout that
   would have been escrowed or refused is paid. Previously it still recorded the `escrow_600` or `blocked` tier,
   and the person would be told their reward was being held. The tier is now capped at `warning_500`, which is
   true and still asks for the form. Enforce mode is unchanged.
2. **The history import itself is SQL that an admin runs** in the PostgresProd console, matching how production
   data changes are done here. It isn't code in this branch. Files:
   - `handoff/w9-backfill-dryrun.md`: read-only;
   - `handoff/w9-backfill-apply.sql.tmpl`: all-or-nothing, with an expected-row guard.

   **What it imports:** the frozen `w9_wallet_earnings` table. Its writer was removed in the W-9 rebuild, so it
   holds the old system's per-wallet, per-chain 2026 totals of payments from SFLuv's paying addresses, up to the
   rebuild.
   - **Imported:** each wallet's total, as one `historical_ponder` row in the `paid` state, attributed to the
     wallet's current owner (smart or EOA address, falling back to the old table's own user link).
   - **Skipped:** SFLuv's own addresses (anything in `protocol_address_history`), wallets with no active account,
     rows newer than the ledger start (they would be counted twice), and rows already imported.
   - **Idempotent:** keys are `historical:2026:<chain>:<wallet>`.

| Item | Hours | Repo |
|---|---|---|
| Diagnosis: the decision logic, tier recording, the ledger window, the frozen old table and its removed writer (`f3148d0`), and three read-only production queries | 0.2 | app |
| The shadow-tier cap, plus the import SQL. Throwaway tests, not committed per the repo rule, against real Postgres 18 with the `cmd/init` schema. **Dry run:** both chains summed per person, a mixed-case address matched, 2025 ignored, the EOA-only owner matched, the legacy user fallback used, and SFLuv-address / no-account / newer-than-ledger rows skipped. **Apply:** a wrong expected count changes nothing; the right count imports exactly those rows; a second run inserts nothing. **After import:** the next payout to a person now over 600 records `warning_500` with enforcement off (`would_escrow` noted) and `escrow_600` with enforcement on | 0.2 | app |

Volume: 1 file, +8. 0 migrations, 0 routes. Plus two handoff SQL files (not in the repo).

## Notes

- **After the import, warnings fire on each person's next payout.** People who don't earn again won't be
  prompted; the admin "Request a W-9" button (#203) covers them.
- **Imported rows also feed the 1099 report and the admin W-9 overview.** That's intended: those payments were
  made by SFLuv in 2026.
