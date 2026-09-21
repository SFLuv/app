# Branch scope — `pgo/signet-sdk-pin`

Sep 21 2026 · app (frontend) · **0.2h active**

Hours are measured wall-clock time from session-transcript timestamps
(`measure_sittings.py`, 30-minute gap), per the `time-accounting` skill. One
sitting, 2026-09-21 12:39–12:48, measuring 0.16h → **0.2h**. Corroborated by
file mtimes: `frontend/package.json` 12:42, `frontend/pnpm-lock.yaml` 12:43,
both inside the sitting.

Weakness worth naming: the sitting is the whole session, which opened with a
review of where the Signet PoC stood before the pin was decided on. The edit
itself is a two-line change; most of the 0.2h is that review plus the
verification below. Splitting it finer than the sitting would be invention — at
this size the sitting total is the only honest figure.

---

## Features

### Pin `@oleary-labs/signet-sdk` to the published `^0.4.0` — 0.2h · app

- Replaced `link:../../../oleary-labs/signet-sdk` in `frontend/package.json`
  with `^0.4.0`, and resolved `frontend/pnpm-lock.yaml` against the registry.
- The `link:` specifier resolved on one machine and nowhere else, so any build
  that was not that laptop had no SDK at all. `632fc61` stated it could not ship
  and had to become a published pin before merging anywhere that builds in CI;
  it merged regardless via `cf3f67b` and had been on `main` since Aug 30.
- The reason for the link is now gone: the SDK was linked because `signEvmDigest`
  was designed against this integration and the then-published `0.3.0` predated
  it. `0.4.0` is published and carries it.
- Verified rather than assumed:
  - all five imported subpaths (`keygen`, `signature`, `session`,
    `resolver-session`, `types`) are present in the published tarball and in its
    `exports` map;
  - every named import resolves — `keygen`, `signEvmDigest`,
    `generateSessionKeypair`, `authenticateWithResolver`,
    `preflightResolverNodes`, `ResolverAuthError`, `SessionKeypair`, `BlockPin`,
    `NodeAuthOutcome`;
  - the published `.d.ts` for all five modules is **byte-identical** to the
    working tree the link pointed at. The working tree is one commit past the
    release (`6156b4c`, Arc USDC presets), which touches nothing imported here.
  - `npx tsc --noEmit` reports zero errors in any Signet file. The 26 errors it
    does report are pre-existing and in unrelated merchant files.
- Adds one transitive dependency, `@noble/secp256k1@3.2.0`. The SDK's three ZK
  peers (`@aztec/bb.js`, `@noir-lang/noir_js`, `@oleary-labs/signet-circuits`)
  are declared optional and are not pulled in.

No tweaks or fixes beyond the above.

---

## Totals

| | Round 1 |
|---|---|
| Features | 0.2 |
| Tweaks & fixes | 0.0 |
| **Total** | **0.2** |

Volume: 2 files, 30 insertions / 3 deletions · 0 DB migrations · 0 new routes ·
0 new direct dependencies (one specifier repointed; one transitive addition).

---

## Not in this branch

`feat/signet-signer` merged to `main` on Aug 30 without a scope doc of its own,
and one has not been written since. This branch does not supply it — it records
only the pin. The PoC's remaining blockers are unchanged: the Signet node fleet
is unconfigured (`NEXT_PUBLIC_SIGNET_NODE_URLS` empty), the resolver-to-group
binding is not done, so `/v1/auth` answers "no auth resolver configured", and
`_beforeTx` in `frontend/lib/wallets/wallets.ts` still returns the Privy signer
unconditionally — the signer swap has not happened.
