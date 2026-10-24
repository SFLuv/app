# Branch scope: `sanchezo/payout-confirmation-sweep`

Oct 23 2026 · app (backend) · **~0.4h active** · based on `main` at `24ec8b4` (#201)

Hours are wall-clock time from this session's timestamps, not from the `time-accounting` script (running the
downloaded script was blocked in this session). There were two stretches: about 17:00–17:12 PDT tracing every
caller of a faucet send and deciding the design, and about 17:28–17:40 writing and testing it. Between them was
16 minutes waiting on a decision. The faucet-allocation question answered in the same sitting is not counted.

## What this branch is

#201 stopped a lost confirmation from paying twice or faking a success, but such a payout then had nowhere to go:
it stayed "unconfirmed" until someone checked the chain by hand. A workflow step stayed unpaid in the app (with a
"please retry" email that a retry could not honour), and the series behind it stayed blocked. This branch makes
those payouts resolve themselves, and makes them rarer.

1. **Fewer lost confirmations.** `AwaitTransfers` queried logs up to the head block that one node reported, and
   forno load-balances across nodes. A node one block behind refused the query as "block is out of range". The
   query is now open-ended, so each node answers up to its own latest block.
2. **The confirmation sweep** (`handlers/payout_confirmation_sweep.go`) runs on the existing 5-minute workflow
   maintenance job. It picks up every ledger row whose outcome is unknown: rows marked `unconfirmed:`, and old
   `failed` rows without the `not sent:` marker. Rows younger than 5 minutes are left to their own send, and only
   rows from the Safe-faucet era are considered.
   - **Transfer found on chain:** the sweep claims it in `protocol_transactions` (so one transfer can never settle
     two payouts) and marks the row `paid` with its hash. A workflow step gets the hash and is marked `paid_out`,
     the workflow is finalized, and the series moves on. A step that a retry had already marked paid with no hash
     gets its hash backfilled. A transfer the dispatcher claimed after its caller gave up (claimed, but recorded by
     no ledger row) is adopted.
   - **Never landed:** a row concludes `not sent:` only once all three hold: it is ≥1 hour old, its transfer is
     absent from the chain, and a faucet operation submitted after it has been mined. Operations take nonces in
     order, so after that the payout cannot land. It then goes back through the normal path:
     - a workflow step gets a retry request and is paid again once;
     - a redemption code is handed back so the volunteer can scan it again;
     - a step that a bad retry already marked paid is not reopened; admins get a "Workflow Payout Never Sent"
       email instead.
3. **Calmer workflow handling.** An unconfirmed send marks the step "Workflow payout sent; awaiting on-chain
   confirmation." with no admin error email. That status holds the step out of the payout loop, so it is not
   retried until the sweep settles it. A pre-#201 failed row refused on retry now counts as unconfirmed, not as a
   plain failure, so it doesn't send a "please retry" email on every attempt.

On deploy, the sweep also settles the two Oct 23 false failures (ledger 429 for Miqdaad, and Vivian's row): both
become `paid` with their real hashes, and the hashes are backfilled onto their already-paid steps. No manual SQL
is needed.

| Item | Hours | Repo |
|---|---|---|
| Tracing every faucet-send caller (workflow loop, retry gating, QR redeem, recovery, escrow settlement, claim table, dispatcher ordering) and the design | 0.2 | app |
| Search change, block-by-time search, paged transfer search, claim/orphan logic, sweep, workflow handling. Throwaway tests, not committed per the repo rule, run against real Postgres 18 with the `cmd/init` schema and a fake 1s-block Celo node, all passing: (1) unconfirmed+landed → paid; (2) legacy failed+landed → paid; (3) 2h, absent, later op → not sent; (4) 2h, absent, no later op → stays, then concluded once a later op lands; (5) 30m absent → stays; (6) 3m old → untouched; (7) pre-Safe row → untouched; (8) not-sent row → untouched; (9) identical twins, one transfer → only the older settles, the younger waits, and a second pass changes nothing; (10) orphaned dispatcher claim adopted, another payout's claim refused; (11) three "block is out of range" answers → still settles; (12) workflow steps: awaiting → paid_out with workflow finalized, paid-without-hash → backfilled, never landed → retry requested, never landed but marked paid → left and emailed; (13) lagging node answers the open-ended query on the first poll; (14) block-by-time exact at 0s/1s/7m/3h/40d, 13 header reads each | 0.2 | app |

Volume: 10 files, +775 / −6. 0 migrations, 0 routes.

## Not in this branch / known limits

- **Recovery claims** (`handlers/recovery.go`) send directly, not through the ledger, so the sweep doesn't cover
  them. A claim whose confirmation is lost stays "claiming", as before.
- **QR redemption response.** A volunteer whose payout is unconfirmed still sees an error, though the money
  usually arrives. Returning a "pending" status needs mobile/web support first.
- **A step that a bad retry marked paid** is reported to admins, not reopened. Paying it is a person's call.
- **The sweep's RPC use** per unresolved row per pass is about 26 header reads plus one log query per 4,000
  blocks. That's fine for a handful of rows. A dedicated Celo RPC would make both this and #201's confirmation
  wait sturdier than forno.
