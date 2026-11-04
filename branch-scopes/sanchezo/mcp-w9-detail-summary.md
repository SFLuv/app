# sanchezo/mcp-w9-detail-summary

**Repos touched:** `SFLuv/app` (this branch).
**Status:** built; run against a local database through a throwaway test. Not merged, not deployed.
**Dates:** 2026-11-04. **0.1h active, measured** (12:10–12:12 PST, prompt time to commit by the shell clock).

## What it is

Two fixes to `w9_person_detail` from its first use on production data (`sanchezo/mcp-client-versions`).

| Fix | Hours | Repo |
|---|---|---|
| Its note said amount decimals depend on the chain. Ledger amounts are 6-decimal base units on every chain, Berachain imports included, as production rows show (a 210 SFLUV import is stored as 210000000). The note is corrected, and `amount_base` is now `amount_sfluv_base`, the suffix the server's instructions already define | 0.05 | app |
| One active volunteer's answer ran past 80 payouts. It now returns `payout_summary` (count, counted total, totals by state, last payout) and lists the newest 5 plus any not yet paid. `all_payouts` lists up to the newest 100 | 0.05 | app |

Read-only as before: every query runs in `withReadOnlyTx`.

The test also ran the URL stripping on sample error strings, which #221 had not exercised.

**Volume:** 3 files, no migrations, no new routes. One response field renamed; the tool shipped earlier today and nothing in the repo reads it.
