# Branch scope — `pgo/signet-enrolment-completion`

Sep 21 2026 · app (frontend) + Signet infrastructure (Celo, Ethereum mainnet, node fleet) ·
**4.3h active**

Hours are measured wall-clock time from session-transcript timestamps
(`measure_sittings.py`, 30-minute gap), per the `time-accounting` skill. Two sittings on
2026-09-21: 12:39–16:26 (3.79h) and 17:21–18:06 (0.74h), measuring **4.53h** against 5.45h of
wall clock.

**4.3h is reported here, not 4.53h.** The first ten minutes of sitting one were the SDK pin,
already recorded as 0.2h in [`pgo/signet-sdk-pin.md`](signet-sdk-pin.md). Counting it twice
would inflate the pair.

Weaknesses worth naming:

- The per-feature split below is apportioned by commit boundaries inside the measured sittings,
  not by mtimes, because much of this branch's work produced no files at all — chain reads,
  contract calls, node probing and diagnosis. The sitting totals are measured; the split is an
  apportionment of them and is approximate below 0.1h.
- A large share of the time is **not** frontend work. Binding a resolver, whitelisting a
  paymaster and diagnosing node config are infrastructure, and they dominate the first sitting.
  Reading this as 4.3h of React would badly misdescribe it.

---

## Features

### Signet infrastructure brought from dormant to working — 1.8h · Celo + Ethereum mainnet + nodes

The PoC had been stalled since Aug 30 behind what the CLI called "handoff steps B and D",
which were referenced nowhere and defined nowhere. Replaced that with facts read off chain.

- Found the group's auth resolver **unset** (`getAuthResolver()` → `0x0`) by reading the group
  contract on Ethereum mainnet, after wrongly inferring from node error responses that it was
  already configured. The contract read was available the whole time.
- Queued and executed `queueAuthResolver(42220, 0x903409cB…, true)` through its 10-minute
  timelock; verified `siweDomains()` is `['app.sfluv.org']` and matches the client.
- Confirmed the gate already admitted the staff EOA, and that the Safe was deployed — no writes
  needed for either.
- Diagnosed the first production bind failure to the paymaster: destination not whitelisted,
  proven by the absence of any EntryPoint log for the Safe. Read the paymaster implementation
  to establish that `to == sender` is already permitted, so only the registry needed adding.
- Produced and verified the `updateWhitelist` calldata; confirmed the result by reading the
  whitelist mapping out of contract storage when the explorer's index lagged.
- Identified the missing `chain_rpcs: {42220: …}` on the node fleet from the node source, after
  establishing that every participant re-reads `resolve()` independently.

### Enrolment completed in the browser — 0.5h · app

The card bound a wallet and then promised "setup will continue automatically", which no code
delivered. It now finishes.

- `AppWallet.signMessage` — personal-sign as the owner EOA, the seam SIWE needs
- Session lifecycle in `useSignetEnrolment`: in memory, reused within its hour, dropped on
  account change
- `completeEnrolment()` driving keygen and add-owner, with adapters for the receipt-vs-hash and
  authorization-shape mismatches
- Finish-setup UI with a progress-driven checklist
- Failure copy that is actionable, with the technical detail logged rather than shown

Two fixes fell out of wiring it, both of which would have defeated the feature:

- `signetIsOwner` could never be true, because `readEnrolmentState` required a stored key
  address that nothing stored — making the success state unreachable code. Derived from the
  Safe's owner set instead, refusing to guess past one extra owner.
- `connect-src` had no wildcard and no node origins, so production would have blocked every
  `/v1/auth` call while looking like the fleet was down. Derived from the fleet env var so
  configuring one implies the other.

### Production debugging: the bind that did nothing — 0.9h · app + Celo

The first real click signed twice and changed nothing on chain. Established that the user
operation never reached the EntryPoint, that the EIP-712 domain matched the registry, and that
the sponsor was refusing the destination — then found that `_submitContractCall` only reports
failure when the bundler *throws*, so a refusal is silent by construction.

Also fixed the post-write read: `execSponsored` resolves when the bundler accepts, not when the
operation lands, so the single `refresh()` re-read stale state and a successful write looked
like a button that did nothing — the same symptom as the refusal it followed.

### Whitelist, env and deploy verification — 0.4h · Celo + deploy

Whitelist calldata, storage-level verification, and establishing that `NEXT_PUBLIC_*` values are
inlined at build time — the env var had been set but not rebuilt. Verified from the live CSP
header rather than by asking.

### SIWE chain id, and the first successful enrolment — 0.7h · app + Celo

Auth failed for an account that was demonstrably authorized. The SIWE message carried the
resolver's chain (42220) where the node compares against the group's **home** chain (1). Both
the node's own error string and the SDK's `chain_id_mismatch` doc describe this check backwards,
and the node sanitizes 401s to `{"error":"unauthorized"}`, so the real reason was visible only
in node logs — where the operator found it.

With that corrected, enrolment completed end to end for the first time. Verified on chain: the
Safe now has two owners, the second being a threshold key no machine holds.

---

## Tweaks & fixes

| Item | Repo | Hours |
|---|---|---|
| Included in the feature figures above — this branch produced no separable small fixes | — | 0.0 |

---

## Totals

| | Sitting 1 | Sitting 2 | Total |
|---|---|---|---|
| Features | 3.6 | 0.7 | 4.3 |
| Tweaks & fixes | 0.0 | 0.0 | 0.0 |
| **Total** | **3.6** | **0.7** | **4.3** |

Volume: 7 files, 430 insertions / 30 deletions · 0 DB migrations · 0 new routes · 0 new
dependencies. Excludes the two SDK-pin commits this branch is stacked on, recorded separately.

On-chain writes made outside the repo, none of them reversible by a deploy: the group's auth
resolver binding (Ethereum mainnet), the paymaster whitelist (Celo), the account→Safe binding,
and a threshold key added as a Safe owner.

---

## What is NOT proven

The branch completes enrolment. It does not make Signet sign anything.

- `_beforeTx` still returns the Privy signer unconditionally. **No Signet key has produced a
  signature.** `sign-digest` — the step that establishes whether the fleet can produce a
  signature the bundler accepts, and how slowly — has never run.
- The settings card's "Use Signet to sign" toggle is inert. `preferSignet` is local React state
  read by nothing; a transfer made with it on is signed by the browser key exactly as before.
  It presents a choice that does nothing and should be wired or hidden.
- `openSession` contacts `SIGNET_NODE_URLS[0]` with no failover, while keygen fails over across
  the list. One unhealthy node breaks enrolment for everyone.
- Nothing here has been exercised by anyone but the developer, on one allowlisted account.
