# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SFLUV is a local currency platform using a wrapped HONEY token on Berachain. This repo (`app`) is the main touchpoint for merchants, improvers (paid in the currency to do community projects), proposers (who suggest projects), voters, and issuers. It is a multi-role governance + workflow + commerce platform.

## Commands

### Backend (Go)
```bash
go run ./backend                                    # Run the backend server
go test -vet=off ./db ./handlers ./router ./structs # Run backend tests
```
Backend env: `backend/.env` — requires `DB_USER`, `DB_PASSWORD`, `DB_URL` (for app, bot, and ponder DBs), `PRIVY_APP_ID`, `PRIVY_VKEY`, `RPC_URL`, `MAILGUN_API_KEY`, `MAILGUN_DOMAIN`, `PONDER_SERVER_BASE_URL`, `PONDER_KEY`, `W9_WEBHOOK_SECRET`.

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
  main.go          — startup, DB init, service wiring
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
- `/settings` — Role request flows (proposer, improver, affiliate)
- `/proposer` — Workflow builder, template library, active workflow management
- `/voter` — Workflow vote queue, deletion vote queue
- `/improver` — Workflow feed, step claim/start/complete
- `/issuer` — Credential grant/revoke
- `/admin` — Side-tab admin panel (users, proposers, improvers, issuers, templates)

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

## Current Work Focus

See [IMPROVER_PANEL.md](IMPROVER_PANEL.md) for the active implementation context.

**Remaining items (as of last check-in):**
- Workflow step payout pipeline (faucet settlement → `paid_out` transitions)
- Improved attachment handling (direct upload/storage for required photos)
- Scheduled/background vote countdown finalization (currently finalizes lazily on endpoint hit)
