# Branch scope — `sanchezo/faucet-queue-fixes`

Oct 22 2026 · app (backend) · **~0.1h active** · based on `pjol/bot-smart-wallet-hotfix` at `5aa4df7`

Hours are wall-clock time from this session's timestamps (about 15:07 to 15:13 PDT), not measured with the
`time-accounting` script.

## What this branch is

Two double-payment fixes in the faucet Safe queue (`handlers/protocol_faucet.go`), found in review of round 4.

1. **An abandoned queued payout was still paid after its QR code was released.** When a caller's context expired
   while its request sat in the queue, `SendFromFaucet` returned a releasable error, so the redemption was undone, but the
   request stayed in the channel and the dispatcher paid it later. The volunteer could then redeem the same code again.
   There was also a read-then-return race against the dispatcher's `submitted.Store(true)`.
   **Fix:** each request has one atomic state, `queued` → `cancelled` (caller) or `queued` → `submitted`
   (dispatcher), decided by compare-and-swap, so exactly one side wins. The dispatcher drops cancelled requests
   (`takeFaucetBatch`) before building the operation.
2. **A send that failed ambiguously released every code in the batch.** Any `executeRaw` error was treated as "the
   engine refused it". **Fix:** `protocol.Client.rpc` now returns `*protocol.RejectedError` for definite refusals
   (a JSON-RPC error object, or a 4xx other than 408). `executeRaw` wraps any other `Send` failure (a timeout, a
   dropped connection, a 5xx, an undecodable response) in `uncertainSendError` carrying the signed operation hash.
   Those payouts stay non-releasable, and the hash is logged for tracing. Failures before `Send` are still releasable.

| Item | Hours | Repo |
|---|---|---|
| Both fixes. Verified with throwaway tests, not committed per the repo's testing rule: 60,000 racing cancel/take requests under `-race` gave 0 double payments; the same harness on the old logic released *and* paid 19,996 of 20,000 abandoned requests; engine responses classified as rejected (JSON-RPC error, 400, 429) or uncertain (408, 502, garbage, hang, connection refused) | 0.1 | app |

Volume: 2 files, +130 / −24. 0 migrations, 0 routes.

Known remaining edge: if a caller's context expires in the moments between the dispatcher taking its request and a
pre-send failure, the caller has already answered "submitted", so that code stays spent although nothing was sent.
It errs toward not paying twice.

# Round 2 — Oct 22, audit items 1 and 3 (committed directly on `pjol/bot-smart-wallet-hotfix`)

Oct 22 2026 · app (backend) · **~0.1h active** (about 15:15 to 15:21 PDT, wall-clock from session timestamps)

Round 1 was fast-forwarded into `pjol/bot-smart-wallet-hotfix`, so the hotfix is one branch again.

## Item 1: a sweep run from the Protocol panel left payouts on the emptied EOA, and "complete" meant "deployed"

- **The panel buttons now take effect in the running process.** `ProtocolService.SetAfterPanelStep` is wired at
  startup (`bootstrap.WireProtocolPanel`). After a successful panel deploy or sweep it re-reads the cutover state,
  attaches the faucet Safe once it's funded (with the boot retry loop), and points redeemer grants at its Safe. A
  panel deploy may hand the role over, as the deploy gate does at boot. It runs on the process context, not the request's.
- **`CutoverComplete` now requires the faucet's token sweep to be `done` or `skipped`**, not just deployed Safes.
  Before this, every restart between deploy and sweep marked the cutover complete: payouts from the still-funded EOA
  were refused, and boot skipped the very sweep that `PROTOCOL_SWEEP_ENABLED` asked for.
- **`Bot.SetFaucet` / `SetFaucetCutover` update the routing with a compare-and-swap loop**, so the boot retry loop
  and a panel step can't silently drop each other's change (e.g. lose the attached spender).

## Item 3: the recovery owner was optional and never retried after the cutover

- `DeployAccounts` refuses when `PROTOCOL_RECOVERY_OWNER` is unset, and logs the address it deploys with ("confirm
  this address is ours").
- `EnsureRecoveryOwner` takes `p.mu` (no caller holds it), so `addOwner` can't be sponsored alongside a faucet payout
  or race a `swapOwner`.
- Boot, once the cutover is complete, calls the new `EnsureRecoveryOwners` when `PROTOCOL_DEPLOY_ENABLED` is true;
  with the gate off it only reports (`MissingRecoveryOwners`) and logs a warning.

| Item | Hours | Repo |
|---|---|---|
| Items 1 and 3 as above. Verified with throwaway tests, not committed: `CutoverComplete` against a real Postgres migrated to 1.66, with all 7 states correct (no step, running, failed, only the redeemer swept, faucet done, faucet skipped, an undeployed Safe); deploy refused without a recovery owner; the panel hook firing (and nil-safe); 2,000 racing `SetFaucet` / `SetFaucetCutover` rounds under `-race` with no lost update | 0.1 | app |

Volume: 7 files. 0 migrations, 0 routes.

Not changed, worth knowing: a sweep from the panel still doesn't revoke the legacy redeemer authority. That stays
gated on `PROTOCOL_SWEEP_ENABLED` at boot, as before.

# Round 3 — Oct 22, the batch stall and faucet history labels (on `pjol/bot-smart-wallet-hotfix`)

Oct 22 2026 · app (backend + frontend) · **~0.1h active** (about 15:33 to 15:38 PDT, wall-clock from session timestamps)

## One search per operation, not per payout

`protocol.Client.AwaitTransfers` replaces the per-payout `AwaitTransfer` (removed, no other callers). It runs one
`eth_getLogs` poll for every transfer out of the faucet Safe and claims each payout's log under one shared 90-second
deadline. Before, each payout waited up to 90 seconds in turn, so an operation that never landed held the queue for
the batch's whole 4-minute budget.

An operation is one transaction, and a MultiSend batch is all-or-nothing. So once any payout of the batch is found,
the operation's transaction is known: the rest may only match inside it, and a payout missing from it fails
immediately ("operation landed in … but this payout's transfer is not in it") instead of waiting out the deadline.
Matching is in `claimBatch`, kept separate from the polling.

## Rewards from a previous faucet keep their label

- **Backend:** new `PREVIOUS_FAUCET_ADDRESSES` (comma-separated, validated like `BACKING_ASSETS`), served as
  `extras.previous_faucet_addresses` in `/config`. Documented in `.env.example`.
- **Web:** `ResolvedCommunityConfig.faucetAddresses` holds the current faucet first, then the previous ones,
  deduplicated. `TransactionProvider` labels a transfer `volunteer_reward` if it came from any of them, so after a
  rotation the old wallet's rewards stay rewards in history.
- **Mobile (`SFLuv/mobile-app`), checked, not changed:** it reads `faucet_address` from the same `/config`, so
  changing `FAUCET_ADDRESS` reaches it. Nothing else depends on the faucet's address or type. Its reward label
  (`mobile/src/utils/transactions.ts` `isRewardTransaction`) compares a single address, so it needs the same list
  change, shipped in an app release.

| Item | Hours | Repo |
|---|---|---|
| Both of the above. Verified with throwaway tests, not committed: `claimBatch` with synthetic logs (3 identical payouts claim 3 distinct logs in one tx; a payout missing from the landed tx fails at once; nothing landed stays pending; an older owned transfer is skipped; the operation's tx beats other candidates); `AwaitTransfers` against a fake JSON-RPC node finding both payouts on the second poll; `/config` serving the list, replacing a stale upstream copy, and rejecting a bad address; `tsc --noEmit` shows no errors in the changed frontend files (39 pre-existing elsewhere) | 0.1 | app |

Volume: 6 files. 0 migrations, 0 routes, 1 new env var.

Edge, unchanged: if an earlier operation that timed out lands inside this one's window with an identical
payout, a payout can claim that transaction first, and the rest of the batch then fails as "not in it" (codes stay
spent, nothing is paid twice).

# Round 4 — Oct 22, rewards labelled by the backend; PREVIOUS_FAUCET_ADDRESSES removed

Oct 22 2026 · app (backend + frontend) · **~0.1h active** (about 15:50 to 15:55 PDT, wall-clock from session timestamps)

Supersedes round 3's labelling half. The address history in the database is the record of who the faucet was, so
the env list was the wrong place for it. `PREVIOUS_FAUCET_ADDRESSES` and the web `faucetAddresses` list are reverted.
The batch-stall fix from round 3 stays.

- **The backend labels rewards.** `GET /transactions` sets `type: "volunteer_reward"` on a transfer whose sender held
  the faucet role at the moment of the transfer (`AppDB.ProtocolRoleWindows`, one query per page, checked in memory
  with `ProtocolAddressWindows.HeldAt`). Past rewards keep their label across any rotation, and an old faucet wallet
  sending after its window has closed is not a reward. While the history is empty (before
  `PROTOCOL_KEY_ENCRYPTION_KEY` is set and the service has seeded it), `FAUCET_ADDRESS` stands in for all time,
  which is what the clients did before.
- **The Berachain-era faucet was the same address.** `0x21Df0dfC…` has a nonce of 1,183 on Berachain and 1,041 on Celo,
  the same key on both chains. `0x36BB2a38…`, the `BOT_ADDRESS` in local dev configs, has never sent a transaction on
  either chain. `/transactions` serves the backfilled Berachain rows with the active chain id (single-chain Ponder),
  so the epoch-to-cutover window for `0x21Df…` on 42220 covers them, and no extra history row is needed.
- **Web:** `TransactionProvider` uses the backend's `type`. It falls back to comparing against `/config`'s
  `faucet_address` only when the field is absent (an older backend).
- **Admin faucet balance** comes from the new admin-only `GET /faucet/balance` (`bot.Balance()`, which follows the
  faucet to its Safe) instead of reading `/config`'s faucet address on chain.
- **`/config` still serves `faucet_address`** from `FAUCET_ADDRESS`, so mobile builds that label by comparison keep
  working. It's only needed until those builds age out.

| Item | Hours | Repo |
|---|---|---|
| The above, plus the Berachain lookup. Verified with throwaway tests, not committed: labelling against a real Postgres at 1.66, covering an empty history (env fallback), a Berachain-era reward, a pre-cutover Celo reward, a Safe reward, the old wallet after the cutover (not a reward), the Safe before the cutover (not a reward), a merchant transfer, and case-insensitivity; `tsc --noEmit` shows no errors in the changed files | 0.1 | app |

Volume: 9 files. 0 migrations, 1 new route (`GET /faucet/balance`), and the `type` field on transactions.

Mobile (`SFLuv/mobile-app`): `isRewardTransaction` should prefer `transaction.type` from `/transactions`. Not changed
here; it ships with an app release.

# Round 5 — Oct 22, every transaction gets a type; the mobile counterpart

Oct 22 2026 · app (backend) + mobile-app · **~0.1h active** (about 15:59 to 16:06 PDT, wall-clock from session timestamps)

- **`type` is now always set** on `/transactions` rows: `volunteer_reward` or `transfer`. Round 4 left it empty for
  non-rewards (`omitempty`), so a client couldn't tell "not a reward" from "a backend too old to label". It
  fell back to comparing against `faucet_address` in both cases, which mislabels the old wallet after a rotation. Now
  only a missing field triggers the fallback.
- **Mobile, `SFLuv/mobile-app` branch `sanchezo/server-reward-labels` (`ab98b16`), not merged:**
  `isRewardTransaction` uses the backend's `type` when present (outgoing is never a reward) and compares against
  `faucet_address` only when it's absent. `getTransactions` passes `type` through. Compatible both ways: a new build
  with an old backend compares as before; an old build with the new backend ignores `type` and keeps using
  `faucet_address`, which `/config` still serves.

| Item | Hours | Repo |
|---|---|---|
| The above. Verified with throwaway tests, not committed: labelling against a real Postgres (every row typed; the old wallet after the cutover is `transfer`); mobile `tsc --noEmit` clean (0 errors) and `isRewardTransaction` correct for an old backend (from the faucet / from someone else), a new backend (Safe reward / old wallet after the swap), and outgoing | 0.1 | app, mobile-app |
