# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Project Overview

SFLUV is a local currency platform using a wrapped HONEY token on Berachain. This repo (`app`) is the main touchpoint for merchants, improvers (paid in the currency to do community projects), proposers (who suggest projects), voters, and issuers. It is a multi-role governance + workflow + commerce platform.

## Commands

### Backend (Go)
```bash
cd backend && go run ./cmd/init                     # Run DB init / migrations only
cd backend && go run ./cmd/server                   # Run the backend server
cd backend && go test -vet=off ./db ./handlers ./router ./structs # Run backend tests
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
Two credential types: `dpw_certified`, `sfluv_verifier`. Issuers grant/revoke credentials. Workflow roles can require specific credentials — improvers must hold them to claim steps.

### Recurring Workflows (Series)
Workflows with recurrence (`daily`/`weekly`/`monthly`) share a `series_id`. A new instance is blocked (`is_start_blocked = true`) until the prior one reaches `paid_out`.

### Merchant Mode
Merchant mode is a device-scoped, location-scoped safety mode for in-store merchant devices. It lets a merchant leave the mobile app open for payment confirmation without exposing send, map, contacts, improver, or other non-payment surfaces.

Backend support lives in:
- `backend/db/app_merchant_mode.go`
- `backend/handlers/app_merchant_mode.go`
- `backend/structs/app_merchant_mode.go`
- migrations in `backend/bootstrap/schema_migrations.go`

The schema adds:
- `merchant_mode_settings` — account-scoped 6-digit PIN hash and failure/lockout metadata.
- `merchant_mode_devices` — hashed device identifiers, location scope, selected wallet, display name, and current merchant-mode state.

Routes are registered in `backend/router/router.go` under authenticated user routes:
- `GET /merchant-mode/status`
- `GET /merchant-mode/devices`
- `PATCH /merchant-mode/devices/{device_id}`
- `POST /merchant-mode/pin`
- `POST /merchant-mode/enable`
- `POST /merchant-mode/disable`

Important behavior:
- PINs are 6 digits and stored only as bcrypt hashes.
- The first PIN can be created without an old PIN; resets require the current PIN.
- Disabling merchant mode from the device requires the PIN.
- Web settings can set/reset the PIN and toggle registered devices for merchant-owned locations.
- The backend intentionally treats merchant mode as UI-level protection, not a hard authorization boundary for every merchant action.

## Patterns to Follow

### Adding a New Backend Route
1. Add DB query function in `backend/db/`
2. Add handler in `backend/handlers/`
3. Register route in `backend/router/router.go` with appropriate role middleware
4. Add/update struct in `backend/structs/` if new request/response shape needed

### Updating Merchant Mode
- Keep mobile and web/backend changes aligned. Mobile consumes merchant mode through `AppBackend` routes, while web management is in `frontend/app/settings/page.tsx` under the merchant settings tab.
- Preserve device scoping: use the app-generated installation ID from the mobile client and store only the backend hash, not the raw device identifier.
- Preserve location scoping: owners manage devices attached to merchant locations they own.
- If changing the PIN flow, keep first-time PIN creation separate from PIN reset so new merchants are not blocked by a nonexistent old PIN.

### Adding a New Frontend Page
- Pages live in `frontend/app/<role>/page.tsx`
- Use `AppProvider` context for `user`, `authFetch`, role flags (`user.isProposer`, etc.)
- Auth-gate pages by checking role flags from context; redirect to `/settings` if not authorized
- Sidebar navigation is in `frontend/components/` — update it for new role pages

### Email Notifications
Mailgun is used for all transactional email. Styled HTML templates are constructed in Go handlers. Follow existing patterns in `backend/handlers/app.go` and `backend/handlers/app_workflow.go` for email template style.

## Additional Systems

### Affiliate System
Affiliates (`isAffiliate`) have a separate event/payout flow. `AffiliatScheduler` in `backend/handlers/affiliate_scheduler.go` runs recurring payouts. Routes under `/affiliates/*` are affiliate-guarded.

### W9 / Compliance
W9 submissions are created via `POST /w9/submit`. Eligibility and unwrap flows in `backend/handlers/w9.go` and `backend/handlers/unwrap.go`.

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
