# Pre-deploy test suite — implementation plan

Status: plan, not yet built. Written 2026-08-14.

## What this is for

A suite that can be run before any deploy, that does not need to be rewritten for
each change, and that covers the web app, the mobile app, and the marketing site.
It has two jobs:

1. **Scenarios.** Walk the paths real users walk — log in, send money, pay a shop,
   run a workflow, approve a location — and assert the outcome.
2. **A crawl.** Click everything else, semi-randomly, to catch the obviously broken
   thing nobody wrote a scenario for.

The suite is the gate. If it is red, we do not deploy.

## What already exists

`dev-up.sh` is most of a test harness and nothing here replaces it. It boots, in order:

- **anvil** forking Celo at chain id 42220, on `127.0.0.1:8545`, with production state
- **the engine and paymaster**, local, with the sponsor pointed at a local payer key
- **a throwaway faucet** at `tmp/faucet.key`, gas-funded, with the production token
  balance cloned onto it from the fork (`dev-up.sh:991`)
- **cloned databases** — app, bot, ponder — restored from production dumps
- **backend** on `:8080`, **ponder** on `:42069`, **frontend** on `:3000`,
  **webpage** from `:3002` up, **mobile** through Expo and the iOS simulator

Two more assets do most of the work in this plan:

**The prank table.** `prankForwardingMiddleware` (`backend/router/router.go:35`) swaps
the authenticated `userDid` for any other user's id. It is mounted only when
`IN_PRODUCTION` is not `true`, and it forwards only when a `pranks` row exists — a
table created solely by the dev CLI. This is how one login covers every role.

**47 Go test files.**

**`ADMIN_KEY`, for fixture setup only.** `dev-up.sh:1115` writes the dev backend's
value as `${DEV_ADMIN_KEY:-local-dev-admin-key}` — the default is a public
placeholder committed in `.dev.env.example:79`, not a secret and never a production
one. (`dev-up.sh` writes its own env to `tmp/backend.dev.env` and does not read
`backend/.env`, so nothing in that file reaches the dev stack.)

It must never be the thing under test. `withAdmin` injects `userDid` *after*
`prankForwardingMiddleware` has already run, so an `X-Admin-Key` request takes a
different route through the stack than a real admin user does. Use it to arrange
state quickly; run the admin scenarios themselves as a real admin through a prank,
or scenario 9 asserts the wrong path.

## Architecture

Five layers. The important line is between the layers a machine executes and the
layer a human or a model authors.

| Layer | Scope | Runtime | Cost per run |
|---|---|---|---|
| 1. Go tests | pure logic, DB queries, route table | `go test` | seconds |
| 2. API scenarios | backend flows without a browser | script + prank | seconds |
| 3. Web UI scenarios | the flows where the UI is the point | Playwright | minutes |
| 4. Mobile scenarios | merchant mode, wallet, map | Maestro on the simulator | minutes |
| 5. Crawl | everything else, semi-random | Playwright, seeded | minutes |
| — | authorship and triage | Claude, by hand | as needed |

Claude does not execute the suite. Claude writes the specs, and when one breaks,
opens the trace, drives that one flow by hand, finds what moved, and repairs it.
That division is what makes the suite affordable: the model is good at the work
that happens once, and a real runner is good at the work that happens every time.

### Why Playwright and not Claude driving the browser

Three reasons, in order of weight.

1. **Captcha.** `frontend/context/Providers.tsx:42` sets `captchaEnabled: true` with
   no env override. Claude is not permitted to solve captchas, so a cold Privy login
   by the model is impossible. The seeded session below is the answer, and it is
   also the cheaper answer.
2. **Cost.** One interaction costs the model three to five tool calls. A twenty-step
   scenario is several minutes. Eleven scenarios across three surfaces is hours per
   run, which means it never runs.
3. **Determinism and artifacts.** The model re-derives selectors on every pass, so
   the same scenario takes a different path each time — the opposite of a regression
   test. And a Playwright failure yields a trace, a video, a DOM snapshot and a
   network log; a model failure yields prose.

### Why there is no auth shim

An earlier draft proposed a `X-Test-User` header, gated on `TEST_AUTH_KEY` and
`IN_PRODUCTION`, to authenticate as any user from a script. It was rejected, and the
reasoning is recorded here so it does not get re-proposed.

It is not equivalent to prank forwarding. Prank forwarding requires an already
authenticated caller and only redirects them; it cannot let anyone in. A header shim
*creates* identity, which is a categorically stronger primitive — a universal
impersonation key over every route, in front of a system that moves USDC-backed
value.

The gate would also fail open. `isProduction()` is true only for the exact string
`"true"`, so a dropped or renamed env var on the VM silently re-enables the bypass,
with nothing logged and nothing crashed. And the two gates are not independent: both
are env vars set by the same mechanism at the same moment, unlike the prank table's
gate, which is a database write made by a human.

The suite does not need it. Real UI login through Playwright gives a real Privy
embedded wallet, which signs. That solves both authentication and signature with no
backend change at all.

## Ground rules

**Nothing new in `dev-up.sh`.** The menu functions read stdin and drive fine from a
pipe. Verified. See the recipes below. The script stays PJ's, the diff stays zero,
and we inherit whatever he adds later.

**No new shell tooling.** Everything wanted from a `testctl.sh` already exists as a
one-line `cast` call against the running anvil. The skill documents them instead.

**The sandbox is enforced in the Playwright config, not in a wrapper script:**

- a request block that drops every request whose host is not localhost, so a bad env
  physically cannot reach `api.sfluv.org`
- `ignoreHTTPSErrors`, because the frontend serves HTTPS locally from the
  self-signed pair in `frontend/certificates/`
- a preflight that fails closed unless the RPC is `127.0.0.1:8545`, the database host
  is local, and `IN_PRODUCTION` is not `true`

**Mail goes to a local sink.** No message leaves the machine during a run.

**State resets per scenario** with `evm_snapshot` / `evm_revert`, and per run with a
template-based database restore.

## Verified harness recipes

These are confirmed against the running stack, not assumed.

### Driving the dev menu from a pipe

`./dev-up.sh menu` boots nothing, installs no shutdown trap, and exits cleanly, so it
is safe to call while a stack is up (`dev-up.sh:649`). There is no tty check anywhere
in the script.

| Action | Piped input |
|---|---|
| Set prank | `2`, pranker did, `1`, prankee did, `1`, `q` |
| Clear pranks | `3`, `y`, `q` |
| Grant admin | `1`, email, `y`, `q` |

```bash
printf '2\n<pranker_did>\n1\n<prankee_did>\n1\nq\n' | ./dev-up.sh menu
```

Two rules make this deterministic:

- **Search by full user id, never by email.** `pick_user` matches
  `contact_email ILIKE '%'||q||'%' OR id = q`. A full did never matches an email, so
  exactly one row comes back and the selection index is always `1`.
- **End with `q`.** The menu redraws after every action, and the option list changes
  once a prank exists.

### Chain control

```bash
cast rpc evm_snapshot --rpc-url http://127.0.0.1:8545     # returns a snapshot id
cast rpc evm_revert <id> --rpc-url http://127.0.0.1:8545
```

Faucet fund and drain follow the impersonate-and-transfer pattern already written at
`dev-up.sh:991`, using `tmp/faucet.key`.

### What the prank round trip does and does not prove

Confirmed: the menu writes the table, and the two exact queries in
`LookupPrankeeUserID` return what the Go code expects. The middleware is mounted in
the running backend (`tmp/backend.dev.env` has `IN_PRODUCTION=false`).

Not yet confirmed: the forward itself. `prankForwardingMiddleware` returns early
unless `userDid` is already in context, which only a valid Privy token puts there.
The `X-Admin-Key` path does not substitute — it injects `userDid` inside `withAdmin`,
which runs after the router middleware, so the prank middleware has already passed.
Proving the forward is the first smoke spec's job.

## Phase 1 — harness skill and the seeded session

Nothing downstream can be written until two things exist: a known state to reset to,
and a browser that is already logged in.

**1.1 A repo skill** at `.claude/skills/sfluv-test-harness/`. Documentation of what
already exists, not new machinery: the menu recipes, the `cast` commands, the port
map, log locations under `tmp/logs/`, how to run one spec, and how to read a trace.
This is what makes model-side authorship and triage repeatable instead of
exploratory every time.

**1.2 Playwright**, with the config described under Ground rules, plus
`auth.setup.ts` — the one spec a human runs. It opens the app, waits for a hand
login through Privy, and writes `storageState`.

What persists is not the access token but the session: cookies and local storage,
from which the Privy SDK refreshes. The practical lifetime is therefore Privy's
refresh window, not one hour.

> **First task, before anything is built on it:** seed a session, come back the next
> day, and confirm a spec still runs. If the refresh does not survive, we learn it
> immediately rather than through flaky tests in a month. `@privy-io/react-auth` is
> on `^2.21.2`; the window has not been checked.

**1.3 One smoke spec.** Load the app from the saved state, assert a logged-in
landmark, set a prank, reload, and assert the page now shows the prankee. That closes
the one gap the round trip left open.

**Done means:** one command yields a clean chain and database, Playwright opens the
web app already authenticated as any named user, and a revert returns it to clean.

## Phase 2 — test ids

There are currently **zero** `data-testid` in the frontend and **zero** `testID` in
the mobile app. Every spec and the whole crawl depend on stable handles, so this
comes before the scenarios.

Mechanical pass over interactive elements in both clients. Prefer role and accessible
name where one already exists and is unique; add an explicit id only where it is not.
This doubles as an accessibility improvement, since the crawl reads the accessibility
tree rather than pixels.

## Phase 3 — scenarios

Each scenario names the state it needs, which is what makes it runnable in any order
after a revert.

| # | Scenario | Layer | State needed |
|---|---|---|---|
| 1 | Log in | 3 | seeded session |
| 2 | Send SFLUV, user to user | 3 | two funded wallets, prank to switch |
| 3 | Pay a shop from the map | 3 | approved location with `pay_to_address` |
| 4 | Pay spawns the tip prompt | 3 | location with a tipping wallet |
| 5 | Workflow create, then step fill — faucet full | 2 + 3 | faucet funded |
| 6 | Same — faucet empty | 2 + 3 | faucet drained; expect a clean refusal |
| 7 | New location, valid Google place | 3 | Places API reachable |
| 8 | New location, no place yet | 3 | manual address path |
| 9 | Admin approves; location appears on the map | 3 | admin via prank |
| 10 | Merchant mode | 4 | PIN, device, `merchant-mode/today` |
| 11 | Second location gets a derived wallet | 2 | merchant with one existing location |

### Extensions

Grouped by what they protect.

**Around the new multi-location work**, which is the least proven code in the repo:

- Approve a second location while the RPC is unreachable. The handler returns 503 and
  rolls the approval back (`backend/handlers/app_admin.go`). Assert the location stays
  unpublished, then assert the retry succeeds.
- Assert the unique indexes from migration 1.40 bite: point shop B's tips at shop A's
  till and expect refusal.
- Re-run migration 1.40 and assert idempotency.

**Invariants**, which catch classes of bug rather than instances:

- **Money conservation.** After each scenario, sum wallet balances on the fork. The
  total must not move except by faucet payout.
- **Role guard matrix.** Every route crossed with every role, asserted 200 or 403.
  Cheap, mechanical, and the thing most likely to expose data by accident. Extends the
  existing route-walk pattern in `backend/router/router_test.go`.

**Failure paths people actually hit:**

- **Double submit.** Click every submit control twice, fast. Duplicated payments and
  duplicated workflows are the most expensive bugs and the easiest to trigger.
- **W9 blocks.** Manual sends and QR redeems are blocked; workflow payouts are
  believed not to be. A test settles it.
- **Client version gates.** Send an old `X-SFLUV-Client-Version` and assert the
  upgrade response.
- **Deep links and QR.** Redeem URLs and merchant QR codes, opened cold.
- **Email side effects.** Assert which mails a scenario sends, and that none send
  twice.

**Presentation:**

- **Visual baselines.** Key screens each run, light and dark, three widths, diffed
  against the last accepted set.

## Phase 4 — mobile

Maestro on the simulator. Flows are readable YAML, which makes them cheap to author
and cheap to repair. The nine screens under `tmp/mobile-app/mobile/src/screens/` are
the surface; merchant mode is the scenario that matters most.

The iOS Simulator tooling stays in use for authorship and triage, the same way the
browser tools do for web.

## Phase 5 — the crawl

Last, because it needs stable handles.

A seeded random walk per surface. The seed prints on start, so any failure replays
exactly.

1. Read the page as an accessibility tree.
2. Score controls; prefer never-clicked ones.
3. Refuse a destructive allow-list: delete, revoke, approve, send, submit.
4. After each click assert three passive things — no console error, no 5xx, a known
   landmark still renders.
5. Stop when coverage stops growing.

Output is a coverage report: which controls were reached, and which were never
reachable. The unreachable set is usually the real finding.

## Sequence

1. Phase 1.1 skill, 1.2 Playwright and seeded session, 1.3 smoke spec
2. Confirm session lifetime after a day
3. Phase 2 test ids
4. Phase 3 scenarios, API-layer ones first since they run in seconds
5. Phase 4 mobile
6. Phase 5 crawl

## Open questions

- **Privy session lifetime.** Unknown until measured. Decides whether the hand login
  is weekly or daily.
- **Privy test accounts.** If the dev app supports fixed-OTP test addresses, login
  could be automated later and the seeded session becomes a fallback rather than the
  mechanism.
- **Where the suite runs.** Locally before deploy is the stated need. Whether it also
  runs in CI depends on whether a runner can hold an anvil fork and three databases.
- **`TESTING.md` is stale** — it points at `SFLUV_Dev` paths that no longer describe
  the setup. It should be replaced by this document and the skill once they exist.
