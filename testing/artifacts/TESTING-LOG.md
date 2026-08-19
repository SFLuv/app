# SFLUV testing log

Conclusions, not raw output. Per-run evidence lives in `run-<timestamp>/`; this
file is what stops the same hour being spent twice.

Newest first. For each entry: what ran, what broke, and what the cause turned
out to be — the cause is the part worth writing down.

---

## 2026-08-19 — the smoke spec was wrong in three places, not the app

**Result:** both smoke specs pass. Prank forwarding is now proven **through the
browser** — `sanchez@oleary.com (admin=true) → peej@oleary.com (admin=false) and
back`, with the role-gated nav flipping each way. That was the one link the
shell-level round trip could not reach.

Every failure on the way was in the spec, not the app.

**1. `getByRole` matches the accessible name as a SUBSTRING by default.**
`{ name: "Connect" }` also matched the sidebar's **"Connected Wallets"** entry
(`dashboard/sidebar.tsx:53`), so "not logged in" was true for a logged-in user.
Needs `exact: true`.

**2. `is_admin` came back as `"true"`, not `"t"`.** A bare boolean under
`psql -tA` prints `t`/`f`, but this one is concatenated into text with `||`, and
that cast yields `true`/`false`. Checking only for `"t"` made **every** account
look non-admin, sending the spec down the wrong branch.

**3. The prankee picker asked the wrong question.** It selected the first active
non-admin by id — which was a user with `accepted_privacy_policy = false`. Such
a user never reaches the dashboard at all: `AppProvider` diverts to the policy
gate and returns before setting authenticated
(`context/AppProvider.tsx:744`), so the sidebar never renders. "A row exists"
and "a user the app can render" are different questions, and a fixture picker
has to ask the second.

**Also hardened:** `chain_revert` in `testing/scripts/lib.sh` now restores wall
time after reverting. Without it, every payout scenario that snapshots and
reverts drags the chain clock backwards and breaks the developer's own browser
session — which is exactly what happened earlier today.

---

## 2026-08-19 — chain clock drift hangs the whole web app

**Symptom:** logged in through `npm run auth`, then the browser sat on the
loading spinner indefinitely. The backend answered every request with a 200 —
`/config`, `/locations`, `/users/policy-status`, `/users` — and looked healthy.

**Cause:** the anvil fork's `block.timestamp` was **87,783 seconds (~24h)
behind wall clock**. The paymaster signs each UserOperation with a validity
window from real time; the chain evaluates it against block time; a day apart
means every operation reads as not yet due. `tmp/logs/engine.log` had it plainly:

```
EstimateGasLimit error details: execution reverted: AA32 expired or not due
```

`AppProvider` hangs in `_initWallets` — which talks to the bundler, not the
backend, which is why nothing after `/users` ever appeared in the backend log —
so status never reaches `authenticated` and the sidebar spins forever.

**Self-inflicted, partly.** anvil only advances time when blocks are mined, so
an idle fork falls behind on its own. But `evm_revert` restores an old snapshot
*including its timestamp*, and today's payout scenarios snapshot and revert —
dragging the clock backwards each time.

**Fixed** with `anvil_setTime` + `evm_mine`. Added `sync-chain-time.sh`, and
preflight now fails if the drift exceeds five minutes.

**Worth internalising:** a completely healthy-looking backend can sit behind a
totally broken app. When the UI hangs, check `engine.log` for `AA32` before
suspecting anything else.

---

## 2026-08-19 — the e2e auth setup spawned a tab every second

**Symptom:** `npm run auth` opened the browser, then the Privy window flickered
open and shut about once a second, making login impossible.

**Cause:** `tests/auth.setup.ts` polled `page.context().storageState()` in a
one-second loop waiting for a token to appear. Playwright reads localStorage by
opening a **transient page per origin**, so every poll spawned and closed a tab
— and plausibly stole focus from the login it was waiting for.

**Fix:** wait on a positive UI landmark instead — the "Contacts" nav button,
which only an authenticated user has — then snapshot storage once, with a few
spaced retries in case the token lands just after the UI. Never poll
storageState.

**Also found today:** the `e2e/` Playwright suite already existed and is
**gitignored** (`.gitignore:24`), and the repo skill describing it is
**untracked**. Both are invisible in `git status` and absent from a fresh clone,
which is why this session twice started rebuilding things that were already
there. Worth committing the suite and ignoring only `e2e/.auth/`, which is the
actual credential.

---

## 2026-08-19 — W-9 end to end, and a real bug it caught

**Ran:** `06-w9-threshold-crossing.sh` — a new scenario that drives an uncleared
account across the annual limit with real redemptions. **9/9 after the fix.**

```
crossing payment (650 SFLUV)  → 202 HELD, nothing on chain
second payment                → 409 REFUSED, code handed back
same code again               → 409 (was: false 200)
form filed on the stub        → sweep noticed after ~250s
                              → 650 SFLUV released on chain
```

**Found — a blocked volunteer who rescanned was told they had been paid.**

`Pay`'s retry branch reported `Escrowed` but never `Blocked`. A refusal leaves a
**cancelled** ledger row and hands the code back, so the same code is scanned
again by design. On that second attempt the settled-row branch returned
`Blocked:false, Escrowed:false` — indistinguishable from success — so the
caller answered **HTTP 200 and sent nothing**.

Worse than the original refusal, which at least said what to do. Fixed in
`dae984b` by carrying `Blocked: row.State == cancelled` through the retry.

**Every unit test passed throughout**, because none of them replayed an
idempotency key after a block. It took driving a real account across a real
threshold on a real chain to see it. That is the argument for this whole
exercise.

**Learned:**

- An already-cleared account cannot test any of this — the gate short-circuits.
  Find a subject with `filing_status: not_started`; `/admin/w9/{user}/clear`
  does the *opposite* of what is needed (it marks somebody cleared).
- Events reserve `reward x max_participants` against **unallocated** faucet
  balance. A 650 reward for 5 seats demands 3250 and is refused with a clear
  message. Use `max_participants: 1` for threshold tests.
- The maintenance sweep took ~250s to notice a completed filing — well within
  its ~5 minute cadence, but plan for it in any timed assertion.

---

## 2026-08-19 — workflows, via prank forwarding

**Ran:** `05-workflow.sh` pranking into a real approved proposer and improver
(found with `GET /admin/proposers` and `/admin/improvers` — the field is
`user_id`). **5 passed, 3 skipped, 0 failed.** Proposed, force-approved, read
back as `approved`. **Prank forwarding is proven end to end** — the proposer
guard let the call through as somebody else.

**Found — validation failures return 500, not 400, with an opaque message.**
Creating a workflow with a role that has no credentials, or a work item that
demands nothing, answers:

> 500 "Unable to create workflow because an internal workflow database
> operation failed." · `proposer_workflow_db.internal`

The actual reason is only in `backend/logs/prod/app.log`:

- "workflow role requires at least one credential"
- "workflow work item must require photo, written response, or dropdown"

Both are user-correctable input errors being reported as server faults. A
proposer hitting either in the UI gets a message that suggests our database
broke and gives them nothing to act on.

**Found — CLAUDE.md is wrong about credentials.** It says *"Two credential
types: dpw_certified, sfluv_verifier"*. **Neither exists.** `GET
/credentials/types` returns twelve, none of them those —
`sfluv_certified_volunteer`, `dpw_bufees_volunteer`,
`sfluv_project_coordinator` and so on.

**Learned:** a workflow whose `start_at` is in the future sits at `approved`
with its steps locked, so claiming answers 400. Correct behaviour; exercising
claim → start → complete needs a `start_at` in the past.

**Where the real errors live:** `backend/logs/prod/app.log`, not the stdout log.
The HTTP line in stdout gives only the status code.

---

## 2026-08-19 — authenticated run: W-9, merchant, multi-location

**Ran:** all four runnable scenarios after capturing a token. **W-9 5/5**,
**events 10/10**, **QR payout 2/2 (real on-chain transfer)**, **merchant
onboarding 11/11** — including two locations deriving **distinct** tills
(`0x2c9ED94a…` / `0x83Ba8a64…`), which is the multi-location property migration
1.40 exists to protect.

**Found — the running backend was 24 hours stale**, started before the tier
commit. `/w9/tier/{}/ack` answered 404 and the tier fields were "missing" from
`/w9/status`, all of which read as real bugs in code that was never running.
preflight now compares the backend process start time against the newest commit
touching `backend/` and fails if it is older. `restart-backend.sh` fixes it.

**Found — `set -a; source tmp/backend.dev.env` silently breaks authentication.**
`PRIVY_VKEY` is a PEM public key spanning literal newlines; sourcing truncates
it at the first one, so the backend cannot verify any Privy token and **every**
authenticated request returns a bare 403 with nothing logged. dev-up avoids this
by passing env as an array. `restart-backend.sh` parses the file in python so
multi-line values survive.

**Found — the auth header is `Access-Token`, not `Authorization: Bearer`**
(`utils/middleware/auth.go:17`). And the token cannot be found in Local Storage
— Privy splits its session across storage and cookies with no usefully-named
key. Copy it from the **Network tab** instead.

**Found — a cleared user gets a misleading 503** from `POST /w9/start`: "No tax
form is available right now. Please try again shortly." That describes a
transient outage; the truth is "you already filed". The UI hides the button for
cleared users so it should not be reachable, but the wording is wrong.

**Learned — more API shapes that are not guessable:**

- `POST /locations` answers **201 with the plain text "success"** — no id, no
  JSON. The new row has to be found afterwards via `/locations/user`.
- Location approval is **`PUT /admin/locations`** with `{id, approval}` in the
  **body**. There is no per-id approval route, and `POST /admin/locations/{id}`
  is a 405.
- Required location fields are name, street, city, description,
  contact_firstname, contact_lastname, **admin_email**, **admin_phone** — the
  contact email and phone are the `admin_*` fields, not the `contact_*` ones.
- `listing_source:"manual"` is the no-Google-listing path. It also keeps local
  tests off the live Places API, which the default path calls server-side.
- **`GET /locations/{id}` answers 400**; `pay_to_address` and `tip_to_address`
  are on `/locations/user`.
- `GET /locations` wraps its rows as `{"locations":[…]}`, not a bare array.

**Not tested:** `05-workflow.sh`. The captured account is admin/merchant/voter,
so proposing needs `SFLUV_PROPOSER_DID` and claiming needs
`SFLUV_IMPROVER_DID` to prank into.

---

## 2026-08-19 — first harness run: events + QR payout

**Ran:** `preflight.sh`, `02-events.sh`, `03-qr-payout.sh` against the local
stack. Events **10/10 green**; QR payout **2/2 green with a real on-chain
transfer** (5 SFLUV, then reverted by snapshot).

**Found — harness bug, and it invalidated every status assertion.**
`out="$(api GET /thing)"` is a command substitution, which runs in a
**subshell**. `SFLUV_STATUS` was set inside it and never reached the caller, so
every `expect_status` compared against an empty string. Fixed by writing the
status to a file, which survives the subshell. Worth remembering for any future
bash helper: return values escape a subshell, variable assignments do not.

**Found — the frontend is HTTPS-only** with a self-signed cert. Probing
`http://localhost:3000` gets connection-refused, so preflight reported a
perfectly healthy frontend as down. Every curl at it needs `-k`.

**Found — ponder answers 500 at its root** and 200 at `/health`. Probing the
root reports a healthy indexer as dead.

**Learned — the API is not what a reasonable person would guess.** All of these
cost a round trip:

- Volunteer events want `title`, not `name`; and `start_at_local` / `end_at_local`
  as **local wall clock with a separate `timezone`** — not UTC, no trailing `Z`.
- There is **no DELETE** for a volunteer event. It is approve / reject / cancel.
  The `DELETE /events/{id}` that refuses with 409 once codes are redeemed belongs
  to the *other*, older bot-database event system. Two event systems, easily
  conflated.
- An **admin-created event is born approved** — status `scheduled`, approval
  null — so `POST /approve` correctly answers `409 event is not pending
  approval`. The approval queue exists for *affiliate*-created events. That path
  is still uncovered.
- Codes are **read** with GET, not minted with POST, and the redemption code is
  the row's **`id`** — there is no `code` field.

**Found — a suite design flaw:** `02-events.sh` cancelled the very event whose
codes `03-qr-payout.sh` needs, so the redemption scenario would have failed for
a reason the previous scenario caused. It now cancels a separate throwaway
event.

**Found — the harness was sending the wrong auth header entirely.** The backend
reads **`Access-Token`** (`utils/middleware/auth.go:17`); the harness sent
`Authorization: Bearer`. Every authenticated call would have come back 403 with
nothing to indicate why. The frontend gets this right at
`context/AppProvider.tsx:800`.

Related: `capture-token.sh` originally told a human to dig the token out of
Local Storage. Privy splits its session across storage and cookies with no
usefully-named key, so that was a dead end. The reliable route is the **Network
tab** — the app puts `Access-Token` on every backend request, so it can just be
copied from there.

**Not tested:** anything needing an authenticated user — W-9, merchant
onboarding, workflows. They need a captured Privy token, which needs a hand
login past the captcha.

---

## 2026-08-19 — TaxBandits W-9 adapter, sandbox

**Ran:** `cmd/w9probe` against the TaxBandits sandbox — auth, Business/Create,
RequestByUrl, Status, idempotency, and both TIN-match branches through two real
hosted-form submissions.

**Result:** the full W-9 lifecycle works. Four open questions settled.

**Found — a dropped rejection, silent and permanent.** A failed TIN match does
not arrive as a `FAILED` verdict. The vendor sets `W9Status: INVALID` and
**erases the `TINMatching` object entirely**. The adapter reported the resulting
emptiness, and `pollProviderFilings` gates rejection handling on
`status.TINMatch != ""` — so the rejection was never recorded, the filing kept
clearing future payouts, no corrected-form notice was sent, and the row polled
forever. Every test passed throughout, because the fake emitted a tidy
`rejected` the vendor never sends.

*Cause:* modelling a vendor from its documentation instead of one real call.
*Fix:* the adapter reports an invalid filing as a rejected match; the fake now
models the erasure.

**Found — `HostedFormURL` would mint a duplicate submission on every tap.**
`RequestByUrl` is **not** idempotent on `PayeeRef`: two calls produced two
`SubmissionId`s. But the `W9Url` is durable and reusable, so the fix was to
return the stored link and make no API call at all.

**Found — clock sync never worked.** `getservertime` is not an open endpoint; it
returns `AUTH-100002` without an assertion. The adapter called it
unauthenticated, so it 401'd every time, never set an offset, and retried on
every token mint.

**Also settled:** response field names match the docs exactly (`W9Status`,
`StatusTs`, nested `TINMatching`, the `Status` array); `StatusTs` is
`2006-01-02 15:04:05 -07:00`, which `time.RFC3339` rejects; auth is checked
before routing, so a 401 proves only the version segment is valid; `v1.7.3` and
`v2.0.0` both exist.

**Not tested:** anything through the payout code path. These were direct adapter
calls — escrow release, the sweeper and the tier modal have only ever run
against the fake.

---

## Template

```
## YYYY-MM-DD — what was under test

**Ran:**
**Result:**
**Found:**   the failure, then the cause. The cause is the reusable part.
**Not tested:**
```
