---
name: sfluv-test-harness
description: Drive the local SFLUV dev stack for testing — boot it with dev-up.sh, switch identity with pranks, control the anvil fork and faucet, and run or repair the Playwright e2e suite in e2e/. Use when writing, running, or triaging tests against the local stack, when you need to act as another user or role, or when a spec fails and you need the trace.
---

# SFLUV test harness

Everything here drives tooling that already exists. Nothing in this skill asks
you to add scripts to the repo.

Read `docs/test-suite-plan.md` for why the suite is shaped the way it is.

## Boot the stack

```bash
./dev-up.sh                    # everything, then the post-boot menu
./dev-up.sh --no-mobile        # skip Expo and the simulator
./dev-up.sh --skip-db-clone    # reuse the already-cloned local databases
```

Ctrl-C stops everything. Logs live in `tmp/logs/`.

| Service | Port | Log |
|---|---|---|
| anvil (Celo fork, chain id 42220) | 8545 | `tmp/logs/anvil.log` |
| engine | 3001 | `tmp/logs/` |
| backend | 8080 | `tmp/logs/` |
| ponder | 42069 | `tmp/logs/` |
| frontend (HTTPS, self-signed) | 3000 | `tmp/logs/` |
| webpage | 3002 and up | `tmp/logs/` |

The dev backend reads `tmp/backend.dev.env`, which `dev-up.sh` writes. It does
**not** read `backend/.env`.

## Switch identity — pranks

`prankForwardingMiddleware` swaps the authenticated user id for another user's,
so role guards and data lookups all behave as if the other user made the
request. It is mounted only when `IN_PRODUCTION` is not `true`, and forwards only
when a `pranks` row exists.

`./dev-up.sh menu` boots nothing and installs no shutdown trap (`dev-up.sh:649`),
so it is safe to run while a stack is up. It reads stdin and has no tty check, so
pipe it.

| Action | Piped input |
|---|---|
| Set prank | `2`, pranker id, `1`, prankee id, `1`, `q` |
| Clear pranks | `3`, `y`, `q` |
| Grant admin | `1`, email, `y`, `q` |

```bash
printf '2\n<pranker_did>\n1\n<prankee_did>\n1\nq\n' | ./dev-up.sh menu
printf '3\ny\nq\n' | ./dev-up.sh menu
```

Two rules keep this deterministic:

1. **Search by full user id, never by email.** `pick_user` matches
   `contact_email ILIKE '%'||q||'%' OR id = q`. A full did never matches an
   email, so exactly one row returns and the selection is always `1`. Emails
   genuinely collide — three dev accounts share `sanchez@oleary.com`.
2. **End with `q`.** The menu redraws after every action, and the option list
   changes once a prank exists.

The backend picks up a prank on the next request. No restart.

**Always clear pranks when you are done.** A leftover row changes what the
developer's own browser shows.

Find user ids:

```bash
psql -h localhost -p 5432 -U "$USER" -d app -tAc \
  "SELECT id, contact_email, is_admin FROM users WHERE contact_email ILIKE '%oleary%';"
```

From TypeScript, use `e2e/lib/harness.ts` instead of shelling out by hand — it
passes input over stdin rather than through a shell, so ids never need quoting.

## The chain clock — check this first when the app hangs

**Symptom:** the web app sits on its loading spinner forever after login. The
backend answers everything with 200s and looks perfectly healthy.

**Cause:** the anvil fork's `block.timestamp` has drifted from wall time. The
paymaster signs every UserOperation with a validity window taken from REAL
time, and the chain judges it against block time. Drift breaks **every**
account-abstraction operation with:

```
AA32 expired or not due
```

Wallet setup then never completes, so `AppProvider` never reaches
`authenticated` and the sidebar spins (`frontend/app/sidebar.tsx:44`).

**Two causes of drift.** anvil only advances time when blocks are mined, so an
idle fork falls behind by however long it sat. And `evm_revert` restores an old
snapshot *including its timestamp*, so any test that snapshots and reverts drags
the clock backwards.

```bash
testing/scripts/sync-chain-time.sh     # fixes it
testing/scripts/preflight.sh           # warns at >5 minutes out
```

Look in `tmp/logs/engine.log` for `AA32` before suspecting anything else.

## Chain and faucet

The fork is at `http://127.0.0.1:8545`.

```bash
# Isolate a scenario
cast rpc evm_snapshot --rpc-url http://127.0.0.1:8545      # returns an id
cast rpc evm_revert <id> --rpc-url http://127.0.0.1:8545

cast rpc anvil_setBalance <address> <wei> --rpc-url http://127.0.0.1:8545
cast rpc anvil_impersonateAccount <address> --rpc-url http://127.0.0.1:8545
```

The local faucet key is `tmp/faucet.key`. Its address is `FAUCET_ADDRESS` in
`tmp/backend.dev.env`. Fund and drain follow the impersonate-and-transfer pattern
already written at `dev-up.sh:991`.

The token has 6 decimals on Celo: `1000000` base units = 1 $SFLUV. Do not apply
an 18-decimal conversion.

## Admin calls

`ADMIN_KEY` in `tmp/backend.dev.env` defaults to the literal
`local-dev-admin-key` (`dev-up.sh:1115`), a placeholder committed in
`.dev.env.example:79`. Never a production secret.

```bash
curl -H "X-Admin-Key: local-dev-admin-key" http://localhost:8080/<admin-route>
```

**Fixture setup only — never the thing under test.** `withAdmin` injects
`userDid` *after* `prankForwardingMiddleware` has run, so an `X-Admin-Key`
request takes a different path through the stack than a real admin user. Run
admin scenarios as a real admin through a prank.

## Ranking what you find

**A bug found by calling the API directly ranks far below one found through the
UI.** The only exception is a genuine, dangerous security hole.

The client usually validates before it ever reaches the backend, so an API-only
failure often describes a state no real user can occupy. Check whether the UI
can produce it before writing it up.

Worked example: workflow creation answers **500** for "workflow role requires at
least one credential", because the handler classifies DB errors by substring and
matches `required`, not `requires`. Low severity in practice —
`app/proposer/page.tsx:1304` refuses to submit without one, so only a direct API
caller gets there.

## API-level scenarios

`testing/scripts/` holds shell scenarios that drive the backend without a
browser: W-9 (including a real threshold crossing), events, QR redemption,
merchant onboarding and multi-location, workflows. Seconds per run, no test ids
needed. They found the blocked-retry payout bug (`dae984b`).

Two things they rely on that are easy to get wrong:

- **The auth header is `Access-Token`, not `Authorization: Bearer`**
  (`utils/middleware/auth.go:17`). A bearer header authenticates nothing and
  returns a bare 403.
- **The token is not in Local Storage.** Privy splits its session across storage
  and cookies with no usefully-named key. Copy `Access-Token` from any backend
  request in the **Network** tab.

`testing/scripts/restart-backend.sh` exists because `dev-up` never rebuilds the
backend, so a stack left up overnight serves yesterday's code — routes 404 and
fields go missing while the source clearly has them. It parses the env file in
python rather than sourcing it, because `PRIVY_VKEY` is a multi-line PEM and
`set -a; source` truncates it at the first newline, after which **every**
authenticated request 403s with nothing logged.

## The e2e suite

Lives in `e2e/`, deliberately outside `frontend/` — `dev-up.sh`'s `ensure_deps`
compares `node_modules` against `package.json`, so Playwright deps there would
force a reinstall on every boot.

```bash
cd e2e
npm install                 # first time only
npm run auth                # HUMAN STEP: opens a browser, log in through Privy
npm test                    # run the suite
npm run smoke               # just the smoke spec
npm run report              # open the last HTML report
```

`npm run auth` is a human step and cannot be automated: Privy runs with
`captchaEnabled: true` (`frontend/context/Providers.tsx:42`).

**Never poll `context.storageState()` in a wait loop.** Playwright reads
localStorage by opening a transient page per origin, so a once-a-second poll
spawns and closes a tab every second — the Privy window visibly flickers, and it
can steal focus from the login being waited on. Wait on a positive UI landmark
("Contacts" is visible, which only an authenticated user has), then snapshot
storage once. The saved session
lives in `e2e/.auth/` and is git-ignored — it is a live credential.

It saves two files. `user.json` is the Playwright storage state. `session.json`
records which account the session belongs to, because specs cannot know that in
advance and must key off the user id.

### Safety, and why runs refuse to start

Two independent layers, both fail-closed:

- **`e2e/global-setup.ts`** refuses to run unless the base URL, RPC, database and
  backend are all local, and `IN_PRODUCTION` is not `true`.
- **`e2e/lib/test.ts`** aborts every request to a host outside a small allow-list.
  `api.sfluv.org` and `app.sfluv.org` are not on it and must never be added.

Every spec imports `test` from `../lib/test`. The single exception is
`tests/auth.setup.ts`, which needs the captcha and OAuth hosts a real login uses.

### Writing specs

- Serial by design: `workers: 1`. The pranks table has a primary key on the
  pranker, so one seeded session acts as one user at a time.
- Prefer role and accessible name. There are currently **no** `data-testid` in
  the frontend and **no** `testID` in the mobile app; adding them is a planned
  phase, not something to do ad hoc mid-spec.
- Clear pranks in `afterEach`, including on failure.

Useful landmarks in the web app:

| Meaning | Locator |
|---|---|
| Authenticated | `getByRole("button", { name: "Contacts" })` visible |
| Not authenticated | `getByRole("button", { name: "Connect" })` visible |
| Admin | `getByRole("button", { name: "Admin Panel" })` visible |

Sidebar nav entries are **buttons, not links**. They render as `<Button>` with an
`onClick` that calls `navigateTo` (`dashboard/sidebar.tsx:280`), so asking for a
link role matches nothing.

Do not use "Connect is hidden" alone to detect login. While `AppProvider`
resolves, the sidebar renders a spinner and no header at all
(`frontend/app/sidebar.tsx:44`), so Connect is absent before login too.

### Triage a failure

```bash
cd e2e
npm run report                              # HTML report, last run
npx playwright show-trace test-results/<dir>/trace.zip
```

The trace has the DOM at each step, the network log, and console output. Read it
before re-running anything. If a failure looks unexplained, check the run output
for a `network sandbox blocked` warning first — that is the sandbox, not a bug in
the app.

To drive a single flow by hand while repairing a spec, use the browser tools
against `https://localhost:3000` directly. That is what model-driven browsing is
for here: authorship and triage, never execution.

## Go tests

```bash
cd backend && go test -vet=off ./db ./handlers ./router ./structs
```

`backend/test/` needs `backend/.env` present and clones its own databases.
