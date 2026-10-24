# Branch scope: `sanchezo/faucet-confirmation-fix`

Oct 23 2026 · app (backend) · **~0.3h active** · based on `main` at `bd17a3a`

Hours are wall-clock time from this session's timestamps, about 16:29 to 16:48 PDT in one sitting. They were not
measured with the `time-accounting` script, because running the downloaded script was blocked in this session.

## What this branch is

These are two payout bugs found on Oct 23. Two volunteers were paid by the faucet Safe (Vivian Tiet, 55 SFLUV, tx
`0x782076c1…`; Miqdaad Bhuriwala, 50 SFLUV, tx `0x01a2cafe…`), but the backend recorded both payouts as **failed**
and emailed admins to retry.

1. **The confirmation search gave up on the first RPC error.** `protocol.AwaitTransfers` returned the first
   `BlockNumber`/`FilterLogs` error. Celo's public RPC (forno) is several nodes behind one address. A search that
   lands on a node one block behind the head it just reported answers `400 {"code":-32019,"message":"block is out
   of range"}`. **Fix:** the search keeps retrying RPC errors until the 90s deadline, re-dialling if needed, and
   names the last RPC error if it does time out. Claim errors (the DB) are still fatal.
2. **Lost confirmations and retries were handled in a way that could double-pay or fake a success.**
   - A send whose outcome was unknown was marked `failed`. A `failed` row stops counting toward the W-9 threshold,
     and it invited a retry that would pay a second time.
   - A retry of a `failed` row went down the "already settled" branch of `Pay()` and **returned success with
     nothing sent**. That is how Miqdaad's step became `paid_out` with no tx: his app's `/payout-request`
     arrived six seconds after the failure.

   **Fix:**
   - An unconfirmed send (`!bot.ShouldRevertRedemption`) now stays `pending`, still counted, with
     `last_error = "unconfirmed: …"`. A retry of it is refused with a non-releasable error, and the bot is not
     called.
   - A definite not-sent failure is `failed` with `last_error = "not sent: …"`. A retry re-arms it to `pending`,
     goes through the whole W-9 decision again, and sends.
   - A `failed` row with neither prefix predates this change and may hide a send. A retry is refused with an error
     telling an admin to check the chain, instead of being reported as paid.
   - If the tx hash can't be recorded after a successful submit, the result is now unconfirmed rather than not sent.

| Item | Hours | Repo |
|---|---|---|
| Tracing the Miqdaad/Vivian payouts to the two bugs (ledger row 429, chain transfers, forno error) | 0.1 | app |
| Both fixes, plus throwaway tests that are not committed per the repo's testing rule. **Search:** a fake JSON-RPC node that fails `eth_blockNumber` once and `eth_getLogs` twice with forno's exact 400 body finds the transfer on the third poll (about 9s), and a DB claim error still fails immediately. **Payouts**, against a real Postgres 18 with the schema from `cmd/init` and a fake bot: (a) an unconfirmed send leaves the row `pending`/`unconfirmed:` and the retry is refused with 1 bot call total; (b) not sent → `failed`/`not sent:`, then the retry pays it (`paid`, tx recorded); (c) a legacy unprefixed `failed` row is refused with 0 bot calls; (d) the happy path is paid, and its retry returns the same tx without sending | 0.2 | app |

Volume: 4 files, +171 / −28. 0 migrations (the state CHECK constraint is unchanged; the distinction lives in a
`last_error` prefix), 0 routes.

## Not in this branch

- **The data fix for the two false failures**: ledger rows to `paid` with their hashes, and Vivian's step to
  `paid_out`. These are SQL for an admin to run.
- **Historic audit**: ledger rows that are `failed` while their workflow step is `paid_out` (the bug-2 shape).
- **Wording of the "Workflow Payout Error" email.** It still says "failed"; the error text it carries now says
  "unconfirmed" or "not sent".
- **An unconfirmed row has no automatic reconciler.** It stays pending, refusing retries, until an admin checks
  the chain and settles it.
