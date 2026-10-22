# Branch scope — `pjol/bot-smart-wallet-hotfix`

Oct 22 2026 · app (backend + frontend) · **2.6h** (1.3h measured, 1.3h stated)

Railway's vulnerability means the backend's signing keys have to be rotated. This branch makes that
possible without breaking anything: the protocol accounts become smart wallets whose addresses never
change, their signers move into the database encrypted, every address check learns to ask *when*, and
rotation becomes a button.

## How these hours were measured

**0.7h measured** from session-transcript timestamps, clustered into sittings on a 30-minute gap,
unioned across every project worked in — the `time-accounting` method.

| Sitting | Span | Measured |
|---|---|---|
| A | 11:18–11:25 | 0.12h |
| B | 12:06–12:41 | 0.57h |
| | | **0.69h → 0.7h** |

**Two weaknesses, named rather than smoothed over:**

- **Sitting B was still open when this was written.** 12:41 is the last transcript entry, not the end
  of the work, so the figure is a floor.
- **The per-feature split below is attributed, not separately measured.** The clock measures the
  session, not the feature; the split is where the session's turns actually went, and it sums to the
  measured total rather than being rounded up item by item.

No other project had transcript activity today, so this sitting's clock is this work alone — unlike
the previous branch, where a concurrent session made the figure an upper bound.

## What was found before anything was written — 0.2h · app

Four facts decided the shape of all three parts, and three of them contradict what the code looks
like from the outside:

- **The smart accounts in this system are Gnosis Safe v1.4.1**, threshold 1, one owner, with the
  Citizen Wallet module enabled. Verified by reading a deployed account on Celo: singleton
  `0x41675c09…`, `getOwners()` answering, `owner()` reverting. The frontend's `toSimpleSmartAccount`
  says otherwise and is misleading — a SimpleAccount's owner is immutable, which would have made
  rotation impossible and this whole branch a dead end.
- **The backend had no user-operation path at all.** It derives counterfactual addresses with a view
  call and otherwise signs plain EOA transactions. Smart accounts were deployed client-side.
- **The "entry point" is Citizen Wallet's `CommunityModule`** (`0x1DAa4607…` behind the proxy), and it
  **emits no `UserOperationEvent`** — verified from its ABI. So a submitted operation cannot be
  resolved to a transaction through an event, which is what forced the design decision below.
- **W-9 earnings come from `payout_ledger`, not from address matching**, so the tax tiers and escrow
  are untouched by the rotation. Worth knowing before worrying about them.

The live chain is Celo (42220); factory `0x7cC54D54…`, paymaster `0x825b77eE…`, type `cw-safe`.

## Large features

### A user-operation path in Go — 0.15h · app

`backend/protocol/`: the operation type, the engine's wire format, Safe and factory call encoding,
key generation and envelope encryption. No bundler client existed in the backend, so this is new
rather than extended.

The wire format is exact rather than approximately right, because an operation that is one byte
different from what the engine expects fails validation in a way that reads like a bad key. It was
checked against the Citizen Wallet SDK's own output, value by value: the paymaster call
(`pm_ooSponsorUserOperation`), the submit (`eth_sendUserOperation`), the number encoding (minimal hex
with the SDK's leading-zero quirk — which Go's `%#x` reproduces exactly), and the signing, which is an
EIP-191 personal_sign over the operation hash with the recovery id shifted to 27/28.

**The signature matched the SDK byte-for-byte**, which was the single riskiest assumption in the
branch. The operation hash is read from the contract rather than computed locally, because it folds in
the entry point's address and chain id and a local reimplementation that drifted would sign a valid
signature over the wrong thing.

### Historical address checks — 0.1h · app

**The concrete breakage the rotation would have caused**, found in `bot/bot.go`: the payout verifier
derived the expected sender from the key in hand and compared every past transfer's `from` against it.
Rotate the key and every workflow payout ever made fails verification — the money plainly went out,
the step reads as unpaid, and the series it belongs to stays blocked. Exactly what was described as
"comparing to the wrong account".

The fix is a registry of which address held which role when, and a lookup keyed on the item's own
time. **The timestamp comes off the transfer's own block**, not from the caller: it is the
authoritative moment, it needed no threading through the five call sites, and it cannot drift from the
item the way a separately recorded date can. With no registry rows the verifier falls back to exactly
what it did before, so the change is inert until the migration runs.

The analytics and MCP classification already had a time-aware index (`rolesAt`, over
`analytics_wallet_role_history`) whose *source* was the environment — one answer to a question that
has a history. It now reads the registry, and its reconciler does the rest.

### The cutover — 0.1h · app

Migration **1.68** — four tables: the account registry, the encrypted signer history, the address
windows, and a step ledger.

On boot, under the jobs leader: register the accounts, mint a signer for any that lacks one, derive
each Safe from it, record the old addresses as historical starting at the epoch, and deploy any Safe
that is not on chain. All idempotent; nothing moves.

**Every step claims itself in the ledger before running**, and a done step is never re-entered — the
claim is one `INSERT … ON CONFLICT DO UPDATE … WHERE status IN ('pending','failed') RETURNING`, so a
restart, a crash loop or a second instance finds it done. Verified against a throwaway Postgres: first
claim takes it, a second while running is refused, a failed step is retryable, a done one is not.

**Moving the money is gated separately** (`PROTOCOL_SWEEP_ENABLED`, or a button), because it spends
with keys that are presumed exposed and must not fire because a staging image booted. Token first,
then gas — the EOA needs its remaining CELO to pay for the token transfer, and the gas sweep leaves
itself exactly its own fee.

The redeemer admin's *authority* moves too, not just its balance: a role is granted to an address, so
sweeping the money without moving the role would leave a funded Safe that can do nothing. Grant,
confirm from the token, and only then revoke — in that order, because revoking against an unconfirmed
grant would leave nobody able to grant `REDEEMER_ROLE` and no way to fix it from this codebase.

### Key rotation and the Protocol tab — 0.1h · app

`GET /admin/protocol`, rotate one, rotate all, sweep. Two capabilities rather than one —
`admin.protocol.view` and `admin.protocol.rotate` — so seeing which Safe pays for what can be given to
someone without giving them the most consequential button in the panel. A platform admin has both; the
superadmin hands the capabilities out one at a time from the Administrative section, which is the
"scope option for other users".

The tab lists each wallet with its address and current signer, both copiable in full, a state badge, a
Rotate button, and Rotate all at the top behind a confirmation that names what it will do. It also
renders the address history and the step ledger, because "which wallet was this in March" is the
question the tab will get asked.

**Rotation is one `swapOwner`, not add-then-remove.** The request described adding the new signer and
removing the old one; those are two operations, and between them the Safe trusts both keys — including
the one being rotated away from. When the reason for rotating is that the old key may be in someone
else's hands, that window is the one thing worth engineering away. The new signer is written to the
database *before* it is used, because a key that owns a Safe and is not in the database is a key
nobody can ever decrypt again.

New keys come from `crypto.GenerateKey`, which draws on `crypto/rand`.

## Smaller items

| What | Hours | Repo |
|---|---|---|
| Faucet balance and sends follow the money to the Safe, with the EOA as fallback | 0.03h | app |
| `Drain` resolves its destination from the registry and reuses the ordinary send path | 0.01h | app |
| `.env.example`: the master key, how to generate it, and why it must be new | 0.01h | app |

## Totals

| Source | Hours |
|---|---|
| Discovery and on-chain verification | 0.20h |
| User-operation path | 0.15h |
| Historical address checks | 0.10h |
| Cutover | 0.10h |
| Rotation and the Protocol tab | 0.10h |
| Smaller items | 0.05h |
| **Total measured** | **0.7h** |

## Volume

| Repo | Files | Lines | Other |
|---|---|---|---|
| app (backend) | 10 modified, 10 new | +497 / −18 modified, 3,795 new | 1 migration (66 total), 4 new routes (312 total) |
| app (frontend) | 1 modified, 2 new | +11 modified, 536 new | 1 admin tab, 2 capabilities |

## What was verified, and what was not

Verified by execution, against the Citizen Wallet SDK's own encoders and a throwaway Postgres:

- The Safe module call, the factory init code, `swapOwner` with its predecessor logic, and an ERC-20
  transfer all encode byte-identically to the SDK.
- **The signature over an operation hash matches ethers' `signMessage` exactly.**
- The engine's number encoding, including zero, and a JSON round trip.
- Seal/open of a signer key, that a blob sealed for one account will not open as another, that a wrong
  master key fails, that two seals of one key differ, and that 64 generated keys are 64 distinct keys.
- Migration 1.68 applies cleanly; the step claim runs once and only once; a March payout resolves to
  March's address and today's to the Safe, with the cutover instant answering exactly one of them; and
  the database refuses to hold two live signers for one account.

**Not verified, and this is the important line: no operation has ever been submitted to the engine.**
Everything above checks that what would be sent is correctly built, signed and recorded. Nothing has
checked that the engine accepts it, that the paymaster sponsors it, or that a Safe deploys. The first
real test is a human one, and it should be done with a wallet that holds nothing.

## Decisions worth finding later

- **The accounts are Safes, and the frontend's `toSimpleSmartAccount` is misleading.** Anything
  reasoning about these wallets should read `getOwners`/`swapOwner`, not `owner`/`transferOwnership`.
- **A submitted operation is matched to its transaction through the token's own `Transfer` log**,
  because the CommunityModule emits no event tying an operation hash to a transaction. This keeps the
  payout ledger storing real transaction hashes, so the reconciler and the verifier needed no change
  at all. The one case it cannot distinguish is two identical transfers in the same window — same
  sender, recipient and amount — which the payout service's idempotency already prevents upstream.
- **Sponsored execution is what makes sweeping CELO into the Safes safe.** On a direct-execution
  design it would not be: a Safe cannot pay for its own `execTransaction`, so moving all the gas in
  would strand everything inside. Through the paymaster a Safe never needs gas, including to move its
  own CELO back out.
- **The master key lives in the environment, and that is a stated limit rather than an oversight.** A
  database dump alone is now useless, which is the attack this answers. An environment leak *plus* a
  database dump is still fatal, and the environment is what was just compromised — so the key must be
  a value Railway has never held. A KMS would remove even that, and was the alternative considered.
- **Migration 1.68 skips two numbers deliberately.** On main the highest is 1.65, but 1.66 and 1.67
  are claimed by the bulletin board and internal notifications on their own branches. 1.68 is free in
  every branch in flight.
  **Wrong, and corrected in round 2**: skipping ahead would have stranded 1.66 and 1.67 permanently,
  because the runner only applies versions above the database's own. This migration is 1.66, and the
  branches behind it renumber up.
- **The analytics reconciler's `source` string stays `env.<role>`** even though the addresses now come
  from the registry. It is part of the row's identity, so changing it would close every existing
  window and reopen it dated today — moving every historical boundary to the moment of the deploy.

## Open

- **Nothing is pushed and nothing has been run.** `PROTOCOL_SWEEP_ENABLED` is off, and with no
  `PROTOCOL_KEY_ENCRYPTION_KEY` set the protocol service disables itself and logs why.
- **The master key has to be generated and set before any of this does anything** — 32 bytes, and not
  a value that has been on Railway before.
- **The faucet keeps paying from its EOA until its Safe is deployed *and* funded.** Checked at boot
  rather than assumed, so a half-finished cutover leaves the old path in charge instead of pointing
  payouts at an empty account.
- **A rotation whose operation lands late leaves a pending signer row.** Deliberate: that key may be
  the only one that can sign for the Safe, so it is kept and reused rather than discarded. It is
  visible in the panel as an account whose signer has not changed.
- **`go test ./handlers/` still fails to build** on seven pre-existing non-constant-format-string vet
  findings in `app_contact.go`, untouched since August 2025. `go vet` on every package this branch
  touches is otherwise clean.

# Round 2 — Oct 22, review fixes

Oct 22 2026 · **0.2h measured** · app (backend + frontend)

Six issues from review, plus two changes to how the cutover is triggered. Applied before the branch
behind this one is rebased onto it.

## How these hours were measured

**0.2h measured** from session-transcript timestamps on the `time-accounting` method: a 30-minute
sitting gap, unioned across every project worked in.

| Sitting | Span | Measured |
|---|---|---|
| C | 13:24–13:35 | 0.19h → **0.2h** |

Still open when written, so the figure is a floor. No other project had activity in the window.

## The migration number was a real bug, not a collision to be dodged

Round 1 took 1.68 to avoid colliding with two branches in flight. **That would have stranded them
permanently.** The runner applies a migration only when its version is strictly greater than the one
recorded in the database (`isVersionGreater`), so a database taken to 1.68 by this branch would never
run 1.66 or 1.67 when those merged afterwards. Their tables would simply not exist; the version check
would report the schema already current; nothing would say a word until a feature touched a missing
table.

Now **1.66**, the next free number after main, with the ordering written into the migration's own
comment. The two branches behind renumber up to 1.67 and 1.68.

## Identical payouts can no longer record the same transaction

The sharpest of the findings, and it had a real example behind it: three 5-SFLUV rewards to one wallet
inside a minute, on chain, yesterday. A Safe answers with an operation id rather than a transaction, so
the transaction was being found by searching for a transfer of the right amount from the right sender to
the right person — which, for identical payouts, is the same search three times. Two payouts could
record one transaction, and a payout that actually failed could take credit for one that worked:
somebody underpaid, nothing flagged.

Fixed in three places at once:

- **A transaction map** (`protocol_transactions`), keyed on the hash. Claiming a transaction is an
  insert that either succeeds or reports it already spoken for, so candidates are offered in the order
  they landed and the first unowned one wins. A payout with nothing left to claim reports failure,
  which is the truth and leaves the ledger retryable.
- **Sends are serialised.** One faucet operation at a time, which removes the competition at source
  rather than only resolving it afterwards — and closes the nonce question below with it.
- **The verifier asks the map first.** Which account sent a transaction is now a recorded fact looked
  up by hash, rather than a deduction from the block's timestamp and a window. The windows stay for
  everything sent before the map existed, which is everything so far.

Verified against a throwaway Postgres: three identical payouts claim three distinct transactions; a
payout offered an owned hash moves on; and a fourth payout with no unclaimed transfer gets nothing
instead of taking another's.

## The nonce question, answered

Every operation is built with nonce 0 and the engine fills in the real one. **This is exactly what the
Citizen Wallet client does** — the SDK never calls `getNonce` either, and signs whatever the sponsor
response comes back with, which is what this code does. So the behaviour matches the client that runs
in production today.

The residual risk was two operations being sponsored at the same instant and handed the same nonce.
Serialising the sends removes it.

## Nothing on chain unless asked, and nothing in the way of other jobs

Round 1 deployed Safes and granted the redeemer role automatically at boot, with only the fund sweep
gated. **Main auto-deploys, so merging was deploying** — a pull request landing would have put
contracts on chain and moved a token role as a side effect.

Now there are two gates, both off by default: `PROTOCOL_DEPLOY_ENABLED` for putting Safes on chain and
handing over the role, and `PROTOCOL_SWEEP_ENABLED` for moving funds and revoking. What stays automatic
is only what writes to our own database — registering, minting a signer, deriving the addresses (a view
call) and recording the history — because the panel cannot say anything useful until that has happened.
Both steps are also buttons in the Protocol tab, which is where an operator does this deliberately.

It also **ran inline and blocked the rest of the jobs leader**: a chain round trip and two 90-second
waits in front of the payout sweep and workflow maintenance, on every deploy. It now returns
immediately and works on its own goroutine.

## The on-chain work is skipped once the wallets exist

`CutoverComplete` — every account has a recorded, deployed Safe with a live signer — short-circuits
deployment, the sweep and the handover entirely. Every later release, including the branches queued
behind this one that boot this same sequence, reaches the chain not at all and only points the faucet
at what is already there.

Read from the recorded addresses rather than the schema version: the version only says the tables
exist, while the addresses say the work is done.

## The engine dependency is real and now stated

Once the balances are in the Safes, every payout is a sponsored operation: the engine submits it and
the paymaster pays. **An engine outage means no payouts, with no fallback** — the EOAs no longer hold
anything to pay from. That is inherent to the sponsored path, not an oversight, and the Protocol panel
now says so on the card that appears once the cutover is complete.

The fallback that would exist is the direct one: `execTransaction` carrying the owner's signature,
submitted by any EOA holding CELO. It is a different encoder from the module path used here and is not
built. Named as the option rather than left as an unknown.

## Volume

| Repo | Files | Lines |
|---|---|---|
| app (backend) | 9 modified | +582 / −96 |
| app (frontend) | 2 modified | +145 / −0 |

## Open

- **`PROTOCOL_DEPLOY_ENABLED` and `PROTOCOL_SWEEP_ENABLED` are both off**, and with no
  `PROTOCOL_KEY_ENCRYPTION_KEY` the service disables itself and logs why. Nothing has been run.
- **Still nothing submitted to the engine.** Everything verified here is encoding, SQL and control
  flow; the first real test is a human one.
- **An engine outage stops payouts** once the cutover is complete, with no fallback built.

# Round 3 — Oct 22, the review that produced round 2

Oct 22 2026 · **1.3h stated, not measured** · app

Chronologically this comes *before* round 2: it is the review of round 1 that found the six issues
round 2 fixed. It is logged after because round 2 was written first, and the convention appends rather
than folds, so that an earlier figure is never silently restated.

## How these hours were recorded

**1.3h stated by the owner — nothing measured, and the clock cannot corroborate it.**

The session transcript for this project today starts at 11:18, which is the message carrying the review
report. The work that produced that report leaves no transcript at all: reading the branch, checking
the migration runner's behaviour, and finding the three identical payouts on chain happened outside any
session. The owner states the day began around 10:00, so the window is roughly 10:00–11:18.

Stated rather than derived. There is no measurement here to round, and "about an hour and a quarter
before the first message" is the owner's account of their own morning, not something the clock saw.

| Source | Hours | Basis |
|---|---|---|
| Oct 22 ~10:00–11:18 — review of round 1 | 1.30h | stated |

**Its weakness is that it is unverifiable by the method this folder uses.** It is recorded because the
work plainly happened — the report exists, and it is specific enough that it could not have been
written without reading the branch — not because anything measured it.

## What the review produced

Six findings, all of which held up. Two were wrong in round 1 in ways that would have been expensive:
the migration number would have stranded two branches permanently, and identical payouts could record
the same transaction. Both are fixed in round 2, along with the automatic on-chain work at boot, the
blocking of other jobs, and the nonce question.

The three identical 5-SFLUV payouts inside a minute were found on chain, which is the detail that
turned "this could collide" into a demonstrated bug.

## Totals across this branch

| Round | Hours | Basis |
|---|---|---|
| Round 1 — building it | 0.70h | measured |
| Round 2 — review fixes | 0.20h | measured |
| Round 3 — the review itself | 1.30h | stated |
| Round 4 — so the leaked keys can be deleted | 0.43h | measured |
| **Total** | **2.6h** | 1.3h measured, 1.3h stated |

# Round 4 — Oct 22, so the leaked keys can be deleted

Oct 22 2026 · **0.4h measured** · app

Round 1 moved the money to Safes. It did not finish the job the leak actually
demands, which is being able to **delete `BOT_KEY` and `REDEEMER_ADMIN_KEY` from
Railway**. Three code paths still reached for the old key after the cutover, and
until those were gone the keys had to stay.

Plus the queueing problem the batching fix introduced, and four things found in a
second review pass.

## How these hours were measured

**0.4h measured**, `time-accounting` method, 30-minute sitting gap.

| Sitting | Span | Measured | Booked where |
|---|---|---|---|
| C | 13:24–13:36 | 0.19h | round 2 |
| C | 13:36–13:58 | 0.38h | internal-notifications, rounds 5–6 |
| C | 13:58–14:24 | **0.43h → 0.4h** | this round |
| | | 1.00h | sitting C, accounted for in full |

Still open as written, so a floor.

## The three paths that still needed the old key

**The faucet attach only tried once.** One failed balance read at boot — an RPC
blip is enough — and the process logged "leaving the EOA in charge" and never
tried again. Every payout for the life of that process then signed with
`BOT_KEY` against an account the sweep had emptied: a failure for as long as
nobody redeployed. It now retries with backoff to two minutes, logs the first
few attempts and then every tenth, and **never falls back to the EOA once the
cutover is complete** — it refuses instead, which is retryable, where paying
from an emptied wallet is not.

**The payout verifier derived its own address from the key.** When neither the
transaction map nor the address windows could answer, the last resort parsed
`BOT_KEY` to learn what "we" meant. Delete the key and every payout that really
went out reads as unverified. Verification only ever needed *addresses*, so it
now takes them from `BOT_ADDRESS` and the history table, uses the key's address
only if a key happens to still be present, and never treats its absence as an
error.

**`Bot.Drain` still built a transactor from the key.** Round 1's notes said
draining went through the registry, and `BotService.Drain` did — but the method
underneath it survived, dead, holding the key dependency. Deleted, along with its
interface entry. The handler's version resolves the destination from the registry
and sends through the ordinary path, so there was nothing left for it to do.

**After these, nothing signs with either key.** The only wallet-related secret
the environment still needs is `PROTOCOL_KEY_ENCRYPTION_KEY`.

## A data race, and a window on every deploy

Round 2 moved the boot sequence onto a goroutine, which introduced both.

`SetFaucet` wrote two function fields that every request goroutine reads — a
plain data race, and the kind that surfaces as an impossible nil rather than
anything legible. The faucet's routing is now one struct swapped through an
`atomic.Pointer`.

And since attaching is asynchronous, every deploy had a few seconds where the
cutover had happened but this process did not know it yet, so payouts in that
window were signed with the old key. **Whether the Safe is the faucet is now read
synchronously at wiring**, one database query before the server answers anything,
so the gap closes. The three states are explicit: before the cutover the EOA
pays; after it with the Safe attached the Safe pays; after it without the Safe
attached, paying is refused and the redemption code is released.

## Queueing: batched rather than serialised

Holding the lock through the 90-second settle meant a burst of redemptions queued
behind it and each caller gave up after three minutes — returning a
non-revertable error, so the QR code stayed spent and the volunteer was never
paid. Two changes:

**Payouts are batched.** Everything waiting while an operation settles goes out
as one operation carrying many transfers, through Safe's `MultiSendCallOnly` at
its canonical deployment (verified present on Celo before relying on it). Thirty
redemptions become two operations instead of thirty. A single payout stays on the
plain path, so the common case gains no delegatecall from a feature that exists
for bursts.

**The queue is a channel, and the error says whether the money went.** Waiting
respects the caller's context. A payout abandoned before submission returns a
*releasable* error and its code goes back; one abandoned after submission does
not, because the money may yet land and releasing the code would let the same
reward be claimed twice. That distinction now travels with the error instead of
being guessed at by the bot.

This also forced the claim to become per-*transfer*: a batch is one transaction
with one Transfer log per payout, so keying the claim on the hash alone would let
the first payout claim the transaction and starve the rest. The key is now
`(tx_hash, log_index)`.

Submission is still strictly one operation at a time, and deliberately: the
engine assigns the nonce when it sponsors, so two operations sponsored at the
same instant can be handed the same one. Batching is what buys the throughput
instead. Whether they could be pipelined is a question for the dry run below.

## Two risks that were not code problems until they were

**CELO no longer goes into the Safes.** They never need gas, so it was dead
weight — and stuck weight if the paymaster declines to sponsor a value transfer,
since the direct `execTransaction` path is not built. Only SFLUV goes to the
Safes now; leftover gas goes to `PROTOCOL_CELO_SWEEP_TO`, and the step records a
skip when that is unset rather than guessing at a destination.

**Losing the master key no longer means losing the money.** Each Safe had exactly
one owner whose key exists only encrypted in Postgres; lose or mistype the
encryption key, or restore the database from before the last rotation, and the
faucet is unreachable forever. Now:

- `PROTOCOL_RECOVERY_OWNER` is added as a second owner at deployment, while the
  key the backend holds still works. Threshold stays 1, so it is a way in rather
  than a second signature. It is also what makes the worst rotation possible: if
  the encryption key itself leaks, the recovery owner can rotate away from the
  compromised signer — without it, the only key that can rotate is the one that
  needs rotating.
- **Startup checks the signer against `getOwners()`** and refuses to attach the
  faucet if the key it holds is not an owner. That is the restored-backup case,
  which otherwise looks healthy in every other respect and fails at validation as
  if the engine were broken.
- The restore procedure is written where someone will look for it, next to the
  key in `.env.example`.

## The dry run, because nothing has touched the engine yet

`go run ./cmd/protocoldryrun` asks the paymaster what it will sponsor, for an
empty undeployed Safe belonging to a key minted for that run and never stored.
Seven operation shapes: a deployment, an ERC-20 transfer, a two-transfer batch
through MultiSend, `swapOwner`, `grantRole`, `revokeRole`, and a value-carrying
call. Sponsorship is decided before execution, so the answer comes back with
nothing signed, submitted or spent.

It also sponsors twice in a row and reports both nonces, which answers whether
operations can be pipelined or must stay serialised.

The failure it is designed to distinguish: a refusal may be the paymaster's
policy, or it may be that an empty Safe cannot pay its own sponsorship fee. Only
the error text separates them, so the error text is printed.

`.env.example` now tells you to run it before either gate goes on.

## Volume

| Repo | Files | Lines |
|---|---|---|
| app (backend) | 9 modified, 2 new | +706 / −262 modified, 575 new |

## What was verified, and what was not

Verified by execution: the MultiSend packing matches the reference encoder
byte-for-byte; a batch is a delegatecall into MultiSendCallOnly while a single
transfer stays a plain CALL and never goes near it; an empty batch is refused.
MultiSendCallOnly is deployed on Celo at the canonical address. Full `go vet`
clean across every package.

**Not verified: the paymaster.** That is what the dry run is for, and it has not
been run — it needs the production engine and the live config. Until it has,
every claim here about what will be sponsored is an assumption.

## Open

- **Run the dry run before enabling either gate.** If it refuses any of the seven
  shapes, the cutover plan needs revisiting rather than retrying.
- **Pipelining is unanswered.** If the dry run shows the engine advances the
  nonce between sponsorships, the dispatcher could submit the next batch while
  the previous settles. It is serialised until that is known.
- **`PROTOCOL_RECOVERY_OWNER` must be set before deploying the Safes.** It cannot
  be added later from a key that has already been lost, which is the entire point
  of it.
