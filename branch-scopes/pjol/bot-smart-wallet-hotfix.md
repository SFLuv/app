# Branch scope — `pjol/bot-smart-wallet-hotfix`

Oct 22 2026 · app (backend + frontend) · **0.7h measured**

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
