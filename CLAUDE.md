# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SFLUV is a local currency platform using a wrapped HONEY token on Berachain. This repo (`app`) is the main touchpoint for merchants, improvers (paid in the currency to do community projects), proposers (who suggest projects), voters, and issuers. It is a multi-role governance + workflow + commerce platform.

## Commands

### Backend (Go)
```bash
cd backend && go run ./cmd/init                     # Run DB init / migrations only
cd backend && go run ./cmd/server                   # Run the backend server
```
Backend env: `backend/.env` — requires `DB_USER`, `DB_PASSWORD`, `DB_URL` (for app, bot, and ponder DBs), `PRIVY_APP_ID`, `PRIVY_VKEY`, `RPC_URL`, `MAILGUN_API_KEY`, `MAILGUN_DOMAIN`, `PONDER_SERVER_BASE_URL`, `PONDER_KEY`.

### Frontend (Next.js)
```bash
npm run dev    # Dev server (Turbo)
npm run build  # Production build
npm run lint   # ESLint
npx tsc --noEmit  # Type-check (many pre-existing errors in unrelated files; focus on new/changed files only)
```
Frontend env: `frontend/.env` — public constants like `PRIVY_ID`, backend API URL, contract addresses.

### Ponder (Blockchain Indexer)
```bash
npm run dev   # Start dev indexer
```
**Ponder should not be changed** — it's a stable blockchain indexer that listens to ERC20 events and POSTs to `/ponder/callback`.

## Architecture

### Services
- **Backend** — Go 1.24, chi router, PostgreSQL (pgx), JWT auth via Privy
- **Frontend** — Next.js 15 / React 19, Tailwind, Radix UI, Privy for wallet/auth
- **Ponder** — TypeScript blockchain indexer (Berachain ERC20 events → DB + webhook)

### Three PostgreSQL Databases
1. **app** — Users, roles, proposers, improvers, workflows, votes, credentials, affiliates
2. **bot** — Faucet events, redemption codes, W9 submissions
3. **ponder** — Indexed blockchain transfers and approval events

### Auth Flow
Frontend uses Privy (`usePrivy`, `getAccessToken()`) to get a JWT Bearer token. All API calls go through `AppProvider.authFetch()`. Backend middleware validates the JWT and injects `userDid` into the request context. Role-based guards live in the handler layer.

### Backend Structure
```
backend/
  cmd/server/      — server startup and service wiring
  cmd/init/        — DB init / migration entrypoint
  bootstrap/       — shared startup helpers for env, DB pools, logger, wiring
  db/              — all DB query logic (app.go, app_workflow.go, bot.go, etc.)
  handlers/        — HTTP handlers grouped by role (app.go, app_workflow.go, bot.go)
  router/          — route definitions with role middleware guards
  structs/         — shared Go types (app_workflow.go is the big one)
  bot/             — background job service
```

### Frontend Structure
```
frontend/
  app/             — Next.js App Router pages (one folder per role/feature)
  context/         — AppProvider (auth state, user, authFetch), LocationProvider
  components/      — Reusable UI components
  types/           — TypeScript interfaces (workflow.ts, proposer.ts are key)
  lib/             — ABI exports, constants, wallet helpers
  hooks/           — Custom React hooks
```

### Key Pages by Role
- `/` — Merchant map (landing)
- `/settings` — Role request flows (proposer, improver, affiliate); merchant approval
- `/proposer` — Workflow builder, template library, active workflow management
- `/voter` — Workflow vote queue, deletion vote queue
- `/improver` — Workflow feed, step claim/start/complete
- `/your-opportunities` — Improver workflow opportunities dashboard
- `/issuer` — Credential grant/revoke
- `/admin` — Side-tab admin panel (users, proposers, improvers, issuers, templates)
- `/affiliates` — Affiliate dashboard and event management
- `/wallets` — Wallet connection and management
- `/contacts` — Contact CRUD
- `/calendar` — Workflow calendar view
- `/verify` — Email verification flow
- `/merchant-status` — W9 compliance status
- `/unwrap` — Token unwrapping UI
- `/map` — Full merchant location map

## Core Domain Concepts

### Workflow Lifecycle
`pending` → (voting) → `approved` → (start_at elapsed) → `in_progress` → (all steps done) → `completed` → `paid_out`

Special statuses: `rejected`, `expired` (pending > 14 days), `deleted`, `blocked` (series awaiting prior workflow payout).

### Workflow Steps
Sequential steps, each assigned to one improver role. Status: `locked` → `available` → `in_progress` → `completed` → `paid_out`. Steps unlock sequentially — completing step N makes step N+1 available.

### Voting System
- Quorum = 50% of eligible voters
- 24h countdown starts at quorum
- Early finalization if >50% of full voter body agrees before countdown
- Approval blocked if unallocated faucet balance < one week of workflow requirement
- Admin force-approve bypasses vote (uses `admin_approve` decision)

### Credential System
Credential types are **data, not constants** — they live in the database and are
added through the admin panel, so do not hardcode or assume them. Read the
current set from `GET /credentials/types` (twelve at the time of writing:
`sfluv_certified_volunteer`, `dpw_bufees_volunteer`, `sfluv_project_coordinator`,
several translator credentials, and so on).

Issuers grant and revoke them. A workflow role must require **at least one**, and
an improver must hold it to claim that role's steps.

### Recurring Workflows (Series)
Workflows with recurrence (`daily`/`weekly`/`monthly`) share a `series_id`. A new instance is blocked (`is_start_blocked = true`) until the prior one reaches `paid_out`.

## Patterns to Follow

### Adding a New Backend Route
1. Add DB query function in `backend/db/`
2. Add handler in `backend/handlers/`
3. Register route in `backend/router/router.go` with appropriate role middleware
4. Add/update struct in `backend/structs/` if new request/response shape needed

### Adding a New Frontend Page
- Pages live in `frontend/app/<role>/page.tsx`
- Use `AppProvider` context for `user`, `authFetch`, role flags (`user.isProposer`, etc.)
- Auth-gate pages by checking role flags from context; redirect to `/settings` if not authorized
- Sidebar navigation is in `frontend/components/` — update it for new role pages

### Email Notifications
Mailgun is used for all transactional email. Styled HTML templates are constructed in Go handlers. Follow existing patterns in `backend/handlers/app.go` and `backend/handlers/app_workflow.go` for email template style.

### Branch Scope Documents
Every branch records its own work in `branch-scopes/<branch-name>.md`, and that file must be up to date
**before the branch is merged**. `branch-scopes/pjol/volunteer-panel.md` is the worked example.

What it is for: the branch is the unit of work people ask about later — what shipped in it, why a decision
was made, and how long it took. Commit messages scatter that across dozens of entries and a PR description
disappears once it is merged, so it lives in the repo alongside the code it describes.

Each file carries:
- A header with the date range, the repos touched, and total active hours
- How those hours were measured, stated plainly, and any weakness in the measurement named
- Features grouped large-to-small, each with its own hours (**to the nearest 0.1h**) and repo
- A table of smaller fixes, same rule
- A totals table and a volume line (files, insertions/deletions, migrations, new routes)

**Hours are measured wall-clock time, not an estimate of how long the work would have taken by hand.**
That distinction is the one this convention has got wrong most expensively — see the correction at the
top of `branch-scopes/pjol/merchant-onboarding-revamp.md`, where work that measured 2.5h was written up
as 11.5h, a 4.7x inflation, because the figures came from diff size and never from a clock.

**Before writing any hours, pull and follow the `time-accounting` skill.** It lives in the public repo
<https://github.com/pjol/SKILLS> (browsable at
<https://github.com/pjol/SKILLS/tree/main/time-accounting>), so no checkout or credentials are needed:

```sh
BASE=https://raw.githubusercontent.com/pjol/SKILLS/main/time-accounting
curl -fsSL $BASE/SKILL.md                                    # the method — read this first
curl -fsSL $BASE/scripts/measure_sittings.py -o /tmp/ms.py
python3 /tmp/ms.py --project .                               # the measurement
```

Or clone it whole with `git clone https://github.com/pjol/SKILLS.git`. The script clusters
session-transcript timestamps into sittings on a 30-minute gap and prints the measured total; the skill
carries the full method, including what to do when a figure has already been published wrong.

Commit clustering is **not** the method — it measures when work landed, not when it was done, and a
branch committed days after it was written clusters into the length of the commit session. If the clock
was not consulted at all, report the volume and say the time was not measured rather than reporting hours.

The measured figures live here, in `branch-scopes/<branch-name>.md`, in the shape described above — the
skill supplies the number, this folder is where it is recorded. Nothing about time reporting is stored
outside this folder.

Itemised hours should add up to the measured figure. Rounding every item up to a common floor inflates the
total — the first draft of round 2 in the example file floored at 0.5h and came out 35% over what the
commit history showed.

Work spanning more than one sitting is appended as a new `# Round N` section rather than folded into the
existing numbers, so an earlier estimate is never silently restated.

## Testing

Testing is **human-in-the-loop**: boot the stack with `./scripts/dev-up/dev-up.sh`, use the product, fix what the human reports. There is no automated test suite and none should be added — no unit-test scaffolding, no e2e harness. See `TESTING.md` for the process and `scripts/MAINTENANCE.md` for the convenience-script rules. A few pre-existing `go test` files remain in the backend; they are not part of the process and are not a gate.

## Additional Systems

### Affiliate System
Affiliates (`isAffiliate`) have a separate event/payout flow. `AffiliatScheduler` in `backend/handlers/affiliate_scheduler.go` runs recurring payouts. Routes under `/affiliates/*` are affiliate-guarded.

### W9 / Compliance
Every payout runs through one choke point (`decidePayout` in `backend/handlers/payout.go`) that compares a person's annual earnings against escalating tiers (defaults 400/500/600 SFLUV): notice, warning, escrow of the crossing payment, then refusal of anything further until a W-9 clears. Filings go through a vendor adapter (`backend/w9provider/`, TaxBandits) — the app mints a hosted form URL, the vendor calls back on `POST /w9/webhook/taxbandits` (HMAC-verified), and a sweeper polls as a fallback. Escrowed money releases automatically when the filing clears. There is no `POST /w9/submit`; the old self-hosted form is gone.

### Account Abstraction
Frontend uses Permissionless SDK (`frontend/lib/paymaster/`) for smart accounts and transaction batching via a bundler client.

### Background Services
`BotService` (faucet events, QR code redemptions) and `AffiliatScheduler` are initialized in `main.go` and run as goroutines. Logging via `backend/logger/logger.go`.

### Middleware Guards
In `router.go`: `withAuth()`, `withAdmin()`, `withProposer()`, `withImprover()`, `withVoter()`, `withIssuer()`, `withAffiliate()`. Admin users bypass all role checks. Admin endpoints also accept an `X-Admin-Key` header for scripted calls.

## Remaining Work Items
- Workflow step payout pipeline (faucet settlement → `paid_out` transitions)
- Improved attachment handling (direct upload/storage for required photos)
- Scheduled/background vote countdown finalization (currently finalizes lazily on endpoint hit)
