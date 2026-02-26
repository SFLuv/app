
### Workflow Series Cards + Admin Workflow Claims + CSV Series ID (2026-02-26)
- Added `series_id` to supervisor CSV export rows in `backend/handlers/app_workflow.go` (`buildSupervisorWorkflowCSV` now includes `series_id` column/value).

- Added improver workflow-series unclaim backend flow:
  - New API route: `POST /improvers/workflow-series/unclaim`
  - New handler: `UnclaimImproverWorkflowSeries` in `backend/handlers/app_workflow.go`
  - New DB method: `UnclaimImproverWorkflowSeriesStep` in `backend/db/app_workflow.go`
  - Behavior:
    - Requires `series_id` + `step_order`
    - Releases the current improver’s recurring claims for that series step where assignments are still claimable (`locked`/`available`)
    - Returns `released_count` + `skipped_count`
    - Clears step-available notifications for released assignments

- Added admin workflow management backend flow:
  - New APIs:
    - `GET /admin/workflows` (paginated, searchable by workflow title and assigned improver email)
    - `GET /admin/workflow-series/{series_id}/claimants`
    - `POST /admin/workflow-series/{series_id}/revoke-claim`
  - New handlers in `backend/handlers/app_workflow.go`:
    - `GetAdminWorkflows`
    - `GetAdminWorkflowSeriesClaimants`
    - `RevokeAdminWorkflowSeriesImproverClaim`
  - New DB methods in `backend/db/app_workflow.go`:
    - `GetAdminWorkflows`
    - `GetWorkflowSeriesClaimants`
    - `AdminRevokeWorkflowSeriesImproverClaims`
  - New workflow/admin structs in `backend/structs/app_workflow.go` for list, claimants, and revoke payload/results.

- Improver panel UX updates (`frontend/app/improver/page.tsx`):
  - `My Workflows` now includes active recurring series cards (series+step based) when future workflows still exist.
  - Series cards support left/right arrows to cycle through workflows in the series.
  - Clicking a series card opens workflow details for the currently displayed workflow.
  - Workflow details modal now shows series navigation controls (left/right arrows) and an `Unclaim Series` action when opened from a series card.
  - Added unclaim action wiring to the new improver series unclaim API.

- Admin panel UX updates (`frontend/app/admin/page.tsx`):
  - Added a new `Workflows` tab with paginated workflow list backed by `GET /admin/workflows`.
  - Workflow cards are grouped by series with per-card arrows to cycle workflows in a series.
  - Workflow details modal from admin now supports series workflow navigation.
  - Added `Revoke Improver Claim` flow:
    - Opens modal
    - Loads claimant dropdown from `GET /admin/workflow-series/{series_id}/claimants`
    - Revokes selected improver via `POST /admin/workflow-series/{series_id}/revoke-claim`
    - Refreshes list/detail after revoke

- Frontend typing updates:
  - Added new admin/workflow-series API types to `frontend/types/workflow.ts`.

### Workflow API Unix Timestamp Normalization (2026-02-25)
- Removed backend `TO_TIMESTAMP(...)` usage from workflow-domain DB queries.
- Workflow API payloads now emit unix seconds for workflow timestamps instead of ISO date strings.
- Updated workflow backend structs in `backend/structs/app_workflow.go` so workflow timestamp fields are `int64` / `*int64` (start, created/updated, vote timestamps, step submission timestamps, absence timestamps, payout timestamps, etc.).
- Updated workflow DB logic in `backend/db/app_workflow.go` and related handler formatting in `backend/handlers/app_workflow.go` to use unix-second comparisons and explicit `time.Unix(...)` only when formatting for emails/CSV output.
- Updated workflow frontend types and displays to treat workflow timestamps as unix seconds and convert locally for display using `new Date(unix * 1000)` in:
  - proposer panel
  - improver panel
  - voter panel
  - supervisor panel
  - workflow details modal
- Result: storage and transport are unix seconds end-to-end for workflow data; timezone translation is now purely a frontend display concern.

### Step Not Possible Workflow Short-Circuit (2026-02-25)
- Added a per-step proposer option `allow_step_not_possible` so each workflow step can explicitly enable/disable a special improver action.
- Improver UX (`frontend/app/improver/page.tsx`):
  - Shows an optional **Step not possible** control at the top of enabled steps.
  - Requires a details text input when selected.
  - Disables all regular step work-item inputs while selected.
  - Completion submits `step_not_possible` + `step_not_possible_details` and skips item responses.
- Backend enforcement (`backend/db/app_workflow.go`):
  - Rejects `step_not_possible` if not enabled on the step.
  - Rejects missing details when `step_not_possible` is selected.
  - Rejects normal item responses when `step_not_possible` is selected.
  - On valid `step_not_possible`, records submission details and force-completes the full workflow with all step and supervisor/manager bounties zeroed so no payouts are attempted.
- Persistence/schema updates:
  - `workflow_steps.allow_step_not_possible` (bool, default `false`).
  - `workflow_step_submissions.step_not_possible` (bool, default `false`).
  - `workflow_step_submissions.step_not_possible_details` (text).
  - Added create-table definitions and `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` boot migrations in `backend/db/app.go`.
- Workflow details modal (`frontend/components/workflows/workflow-details-modal.tsx`) now renders a clear submission block when a step was marked not possible, including recorded details.

### Zero-Bounty Workflow Auto-Approval (2026-02-25)
- Updated workflow creation so proposals with **no step bounties and no supervisor bounty** are auto-approved **only when**:
  - proposer is an approved supervisor, and
  - proposer is assigned as the workflow supervisor on that proposal.
- If the workflow is zero-bounty and those conditions are not met, it remains `pending` and follows normal voting.
- Backend changes:
  - In `backend/db/app_workflow.go` `CreateWorkflow`, removed the prior `total_bounty > 0` requirement.
  - Added guarded auto-approval path when computed `total_bounty == 0` and `supervisor_user_id == proposer_id` by calling `finalizeWorkflowApprovalTx(...)` before commit.
  - This sets workflow status/decision to approved immediately and bypasses the pending vote lifecycle.
  - Existing step-availability behavior remains intact.
  - Updated template normalization to allow zero-total-bounty workflow templates as well.
  - Added workflow template support for persisting supervisor assignment (`supervisor_user_id`) alongside `supervisor_bounty`.
- Frontend proposer updates:
  - In `frontend/app/proposer/page.tsx`, step bounty validation now allows `0` (still disallows negative values).
  - Step bounty input minimum changed from `1` to `0`.
  - Template save/apply now preserves and restores supervisor assignment (`supervisor_user_id`).
- Notification consistency:
  - In `backend/handlers/app_workflow.go`, `CreateWorkflow` now sends proposal outcome email when the workflow is immediately approved at creation.

### Workflow Boot Migration Cleanup (2026-02-25)
- Removed the workflow timestamp conversion migration block from `backend/db/app.go` that performed runtime `ALTER TABLE ... ALTER COLUMN ... TYPE BIGINT USING EXTRACT(EPOCH ...)` across workflow tables.
- Removed the related workflow timestamp default-reset `ALTER TABLE ... SET DEFAULT unix_now()` statements in the same block.
- Kept workflow table definitions as unix-epoch (`BIGINT`) in `CREATE TABLE IF NOT EXISTS` so fresh environments still initialize correctly without legacy conversion work at boot.

### Workflow Supervisor Role Migration + Panel (2026-02-25)
- Migrated frontend workflow UX off legacy "manager" language and behavior:
  - Removed all improver-panel manager claim/export/payout UI.
  - Improver panel now only handles improver step claiming/completion and improver unpaid-step payout retries.
  - Workflow board and my-workflows views no longer show or depend on manager assignment state.
- Removed legacy improver managed-workflow router endpoints (`/improvers/managed-workflows*`) so supervisor exports are the only supported workflow oversight export path.
- Added/finished supervisor role surfacing through app navigation and role flows:
  - Supervisor panel route (`/supervisor`) is now first-class and linked in dashboard nav for approved supervisors/admins.
  - Settings/admin flows include supervisor request + approval status handling.
- Supervisor panel capabilities completed:
  - Paginated workflow list with title search, status filtering, sorting (created/completed/start), and date filtering.
  - Export supports either explicit multi-select workflow IDs or date-based set selection.
  - Date filtering/export now uses local-date boundaries on frontend (`00:00:00.000` / `23:59:59.999`) converted to UTC RFC3339 for backend query consistency.
- Export pipeline behavior:
  - CSV includes one row per workflow step with required base columns and dynamic deduped item-response columns.
  - ZIP includes CSV + related photos converted to JPEG with normalized filenames:
    `{WORKFLOW_TITLE_TOKEN}_{UNIX}_{STEP_NUMBER}_{ITEM_TITLE_TOKEN}.jpeg`.
- Frontend workflow typing cleanup:
  - Removed manager-specific workflow/type fields from `frontend/types/workflow.ts` so client code uses supervisor-facing schema.

### Validation (Supervisor Migration Iteration)
- `cd backend && go test -vet=off ./...` (passed)
- `cd frontend && npx tsc --noEmit --pretty false` still reports existing unrelated repo-wide TS issues, but no errors from edited supervisor/improver/sidebar/workflow-type files.

### Workflow Credential Enforcement Hardening (2026-02-24)
- Enforced stronger backend validation for workflow role credentials at workflow creation:
  - `CreateWorkflow` now loads valid credential definitions once and validates manager + role credential lists against that set.
  - Role credentials are normalized (trim + dedupe), and each role must still resolve to at least one valid credential.
  - Insert-time FK races are translated into explicit `invalid workflow role credential` / `invalid workflow manager credential` errors.
- Added DB-level integrity enforcement for workflow role credentials:
  - Added migration to create FK `workflow_role_credentials_credential_type_fk` from `workflow_role_credentials.credential_type` to `credential_type_definitions.value`.
  - Added backfill to create missing credential definitions for any pre-existing workflow role credential rows before FK is added.
- Enforced credential-definition validity during claim flows:
  - `ClaimWorkflowStep` now rejects claims when a role has no credential requirements or references unknown credential types.
  - `ClaimWorkflowManager` now also rejects unknown credential types in manager role requirements.
  - Existing required-credential ownership checks remain in place and are still enforced server-side.
- Added safer credential-type deletion behavior:
  - `DeleteGlobalCredentialType` now returns `credential type is in use` on FK constraint failures.
  - Admin credential type delete handler maps `in use` errors to `400` responses.

### Validation (This Iteration)
- `gofmt -w backend/db/app_workflow.go backend/db/app.go backend/handlers/app_workflow.go`
- `cd backend && go test -vet=off ./handlers ./db ./router ./structs` (passed)

### Improver My Workflows Step Access Fix (2026-02-24)
- Fixed improver workflow-detail navigation so `My Workflows` opens directly to the improver's assigned step instead of always defaulting to step 1.
  - Added `detailInitialStepIndex` state in `frontend/app/improver/page.tsx`.
  - Added `getInitialStepIndexForMyWorkflow` helper to prioritize assigned actionable step pages (`available`, `in_progress`, fallback `locked`, then any assigned step).
  - Updated `openWorkflowDetails(...)` to accept an `initialStepIndex` option and pass it through to the modal.
  - Wired both card click and `View Details` button in `My Workflows` to open with that computed step index.
- Added `initialStepIndex={detailInitialStepIndex}` to `WorkflowDetailsModal` usage in improver page.
- Small UX consistency adjustment: assigned improvers now see the `Start Step` action when their step is `locked` or `available`, matching backend unlock/start behavior.

### Validation (My Workflows Step Access Fix)
- `cd frontend && npx tsc --noEmit --pretty false | rg "app/improver/page.tsx|components/workflows/workflow-details-modal.tsx"` (no type errors)

### Step 1 Locked-on-Create Investigation + Fix (2026-02-24)
- Root cause found:
  - Proposer workflow creation defaulted `start_at` to **1 hour in the future**, so step 1 was initialized as `locked` by design in many new proposals.
- Fixes applied:
  - Updated proposer default start time to current local time (no +1 hour offset) in `frontend/app/proposer/page.tsx`.
  - Hardened approval finalization to unlock step 1 immediately when `start_at <= NOW()` at approval time, so workflows approved after their scheduled start do not stay stuck locked until a refresh sweep in `backend/db/app_workflow.go`.

### Validation (Step 1 Lock Fix)
- `gofmt -w backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./handlers ./db ./router ./structs` (passed)
- `cd frontend && npx tsc --noEmit --pretty false | rg "app/proposer/page.tsx"` (no type errors)

### Workflow Detail Fetch Availability Refresh (2026-02-24)
- Added a shared backend helper in `backend/handlers/app_workflow.go`:
  - `refreshWorkflowStartAvailabilityAndNotify(ctx)` now runs:
    - `RefreshWorkflowStartAvailability`
    - step-availability emails
    - series funding shortfall emails
- Wired detail endpoints to always refresh availability before returning workflow details:
  - `GET /workflows/{workflow_id}` (`GetWorkflow`)
  - `GET /proposers/workflows/{workflow_id}` (`GetProposerWorkflow`)
- If refresh fails, detail fetch returns `500` instead of serving stale state.

### Validation (Workflow Detail Fetch Refresh)
- `gofmt -w backend/handlers/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./handlers ./db ./router ./structs` (passed)

### Improver Modal Action-Only Step View (2026-02-24)
- Updated workflow details modal to support conditional hiding of default step details:
  - Added `hideDefaultStepDetails(workflow, step)` prop to `WorkflowDetailsModal`.
  - When hidden, the modal suppresses the default step detail/work-item/submission block and renders step actions directly.
- Wired improver modal behavior so assigned actionable steps show only completion UI:
  - In `frontend/app/improver/page.tsx`, `hideDefaultStepDetails` now returns `true` when:
    - `step.assigned_improver_id === user.id`
    - and `step.status` is `available` or `in_progress`
- Result: once an improver has an actionable step, opening the modal shows just the form/actions instead of detail block + form.

### Validation (Improver Modal Action-Only View)
- `cd frontend && npx tsc --noEmit --pretty false | rg "app/improver/page.tsx|components/workflows/workflow-details-modal.tsx"` (no type errors)

### Improver Unpaid Workflows SQL Fix (2026-02-25)
- Fixed Postgres error on unpaid workflow fetch:
  - `ERROR: for SELECT DISTINCT, ORDER BY expressions must appear in select list (SQLSTATE 42P10)`
- Updated `GetImproverUnpaidWorkflows` query in `backend/db/app_workflow.go`:
  - Removed `SELECT DISTINCT` + join pattern.
  - Switched step-match branch to `EXISTS (...)` against `workflow_steps`.
  - Kept manager unpaid branch unchanged.
  - Preserved ordering by `w.start_at`, `w.created_at`.
- Result: no duplicate expansion and no `DISTINCT/ORDER BY` conflict.

### Validation (Improver Unpaid Workflows SQL Fix)
- `gofmt -w backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./handlers ./db ./router ./structs` (passed)

### Workflow Timestamp Unix Normalization (2026-02-25)
- Migrated workflow-domain storage to unix epoch seconds (`BIGINT`) in DB schema/migrations:
  - `workflows`, `workflow_templates`, `workflow_steps`, `workflow_step_submissions`, `workflow_submission_photos`,
    `workflow_step_notifications`, `workflow_improver_absences`, `workflow_votes`,
    `workflow_deletion_proposals`, `workflow_deletion_votes`.
  - Added `unix_now()` SQL function and updated workflow timestamp defaults to `unix_now()`.
  - Added migration block that converts existing workflow timestamp/timestamptz columns to `BIGINT` via `EXTRACT(EPOCH ...)`.
- Updated workflow DB query layer (`backend/db/app_workflow.go`) to match unix storage:
  - Writes/updates use unix values (`unix_now()` / `.Unix()`).
  - Reads that hydrate API `time.Time` fields use `TO_TIMESTAMP(...)`.
  - Workflow start and vote/payout/absence timestamp comparisons are now unix-to-unix.
- Closed timezone submission gap in proposer workflow/template create flows:
  - `frontend/app/proposer/page.tsx` now converts `datetime-local` start values to RFC3339 UTC (`toISOString()`) before API submission.
  - Keeps local display formatting unchanged (`toLocaleString` / datetime-local input formatting).
- Hardened backend fallback parsing for naive datetime strings:
  - `parseWorkflowStartAt` in `backend/handlers/app_workflow.go` now interprets legacy no-timezone strings as UTC (not server local time).

### Validation (Workflow Timestamp Unix Normalization)
- `gofmt -w backend/handlers/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)
- `cd frontend && npx tsc --noEmit` (fails due pre-existing unrelated TS errors outside workflow timestamp scope)

### Proposer Builder Add-Action Button Placement (2026-02-25)
- Updated workflow builder action placement in `frontend/app/proposer/page.tsx` so add-actions render at the bottom of their respective containers:
  - `Add Role` moved below the roles list.
  - `New Step` moved below the steps list.
  - `Add Work Item` moved below each step’s work-item list.
  - `Add Option` moved below each dropdown option list.
  - `Add Email` moved below each option’s notify-email list/input group.
- Behavior/handlers were unchanged; this is layout-only positioning.

### Validation (Proposer Builder Add-Action Placement)
- `rg -n "Add Role|New Step|Add Work Item|Add Option|Add Email" frontend/app/proposer/page.tsx` (verified updated placements)
- `cd frontend && npm run lint -- --file app/proposer/page.tsx` (blocked: project prompts for first-time ESLint setup)
- `cd frontend && npx tsc --noEmit` (fails due pre-existing unrelated TS errors in other files)

### Proposer Create Error Placement (2026-02-25)
- Added a second error banner at the bottom of the create-workflow form in `frontend/app/proposer/page.tsx`.
- This keeps proposal validation/submission errors visible near the `Submit Workflow Proposal` action, without removing the existing top-level banner.

### Workflow Supervisor Validation SQL Fix (2026-02-25)
- Fixed workflow creation failure:
  - `ERROR: FOR UPDATE cannot be applied to the nullable side of an outer join (SQLSTATE 0A000)`
- Root cause:
  - Supervisor validation in `CreateWorkflow` used `LEFT JOIN supervisors ... FOR UPDATE`, which attempts to lock the nullable side of an outer join in Postgres.
- Change made:
  - Replaced outer-join status read with a scalar subquery while locking only the `users` row:
    - File: `backend/db/app_workflow.go`
    - Query now selects `u.is_supervisor` and `COALESCE((SELECT s.status FROM supervisors s WHERE s.user_id = u.id), '')` from `users u ... FOR UPDATE`.
- Behavior preserved:
  - Still returns `workflow supervisor user not found` when user id is missing.
  - Still requires both `is_supervisor = true` and `supervisors.status = approved`.

### Validation (Workflow Supervisor Validation SQL Fix)
- `gofmt -w backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)

### Voter Panel Workflow-Votes Filtering (2026-02-25)
- Updated `frontend/app/voter/page.tsx` so approved proposals are not shown in the `Workflow Votes` tab.
- Added `workflowVotesList` derived state:
  - includes all workflows except `status === "approved"`.
- Updated workflow vote search filtering to use `workflowVotesList` instead of raw `workflows`.
- Result:
  - `Workflow Votes` tab no longer displays approved proposals.
  - Approved workflows remain available under `Active Workflows`.

### Validation (Voter Panel Workflow-Votes Filtering)
- `rg -n "workflowVotesList|status !== \"approved\"|filteredWorkflows" frontend/app/voter/page.tsx` (confirmed filter wiring)
- `cd frontend && npx tsc --noEmit --pretty false 2>&1 | rg "app/voter/page.tsx"` (no matches; command exits non-zero when no matches)

### Workflow Status + Assigned Improver Display Updates (2026-02-25)
- Updated frontend status labeling so settled workflow status `paid_out` renders as `Finalized`:
  - Added special-case mapping in `frontend/lib/status-labels.ts` (`paid_out` -> `Finalized`).
  - Updated explicit filter labels in:
    - `frontend/app/proposer/page.tsx`
    - `frontend/app/supervisor/page.tsx`
- Updated assigned improver display in workflow details:
  - Added backend step field `assigned_improver_name` to `WorkflowStep` payload.
  - Populated in `getWorkflowSteps` via join to `improvers` and formatted as `FirstName L.`.
  - Updated modal display in `frontend/components/workflows/workflow-details-modal.tsx` to show this name instead of raw improver id.

### Validation (Workflow Status + Assigned Improver Display)
- `gofmt -w backend/db/app_workflow.go backend/structs/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)
- `cd frontend && npx tsc --noEmit --pretty false 2>&1 | rg "workflow-details-modal.tsx|types/workflow.ts|lib/status-labels.ts|app/proposer/page.tsx|app/supervisor/page.tsx"` (no matches; command exits non-zero when no matches)

### Supervisor CSV Column Header Snake Case Enforcement (2026-02-25)
- Updated supervisor CSV export header generation in `backend/handlers/app_workflow.go` to enforce strict snake_case-style names:
  - lowercase letters + underscores only
  - non-letter characters (including digits/symbols/spaces) collapse to underscores
  - empty results fall back to `column`
- Dynamic workflow item columns now use sanitized headers like:
  - `<item_title>_dropdown`
  - `<item_title>_written`
  - `<item_title>_photos`
- Added duplicate-header disambiguation without digits:
  - repeated collisions append `_duplicate` segments (e.g., `_duplicate`, `_duplicate_duplicate`).
- Static CSV headers were already compliant and were left unchanged.

### Validation (Supervisor CSV Header Snake Case)
- `gofmt -w backend/handlers/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./handlers ./db ./router ./structs` (passed)

### Proposer Submit Button + Bottom Error Positioning (2026-02-25)
- Updated `frontend/app/proposer/page.tsx` create-workflow form:
  - Added a second `Submit Workflow Proposal` action row at the top of the form content (with draft bounty badge).
  - Reordered the bottom section so the bottom error banner renders above the bottom submit button row.
- Result:
  - Users can submit from both top and bottom of the form.
  - Bottom validation error now appears immediately above the bottom submit control.

### Optional Workflow/Template Descriptions (2026-02-25)
- Updated proposer workflow/template validation so descriptions are optional and only titles are required.
- Frontend (`frontend/app/proposer/page.tsx`):
  - `saveTemplate` now requires only template title.
  - `submitWorkflow` now requires only workflow title.
  - Updated error copy accordingly (`Template title is required.`, `Workflow title is required.`).
- Backend handler validation (`backend/handlers/app_workflow.go`):
  - `CreateProposerWorkflowTemplate` and `CreateDefaultWorkflowTemplate` now require only `template_title`.
  - `CreateWorkflow` now requires only `title`.
- Backend DB validation (`backend/db/app_workflow.go`):
  - Removed `template_description is required` guard in `CreateWorkflowTemplate`.
  - Empty `template_description` is now accepted and persisted.

### Validation (Optional Workflow/Template Descriptions)
- `gofmt -w backend/handlers/app_workflow.go backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./handlers ./db ./router ./structs` (passed)
- `rg -n "Template title is required|Workflow title is required|strings.TrimSpace\\(req.TemplateTitle\\) == \"\"|strings.TrimSpace\\(req.Title\\) == \"\"" frontend/app/proposer/page.tsx backend/handlers/app_workflow.go` (confirmed updated validation paths)

### Supervisor CSV Step Status Column (2026-02-25)
- Added a static `status` column to supervisor CSV exports in `backend/handlers/app_workflow.go`.
- Implemented normalized status mapping for each step row:
  - `pending`: step not yet submitted and not in-progress (`locked`/`available` or unknown)
  - `active`: step `in_progress` and not yet submitted
  - `failed`: step submission exists with `step_not_possible = true`
  - `complete`: step has been submitted, or step status is `completed` / `paid_out`
- `complete` is used regardless of payout status, per requirement.

### Validation (Supervisor CSV Step Status Column)
- `gofmt -w backend/handlers/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./handlers ./db ./router ./structs` (passed)

### Supervisor CSV Header Numeric Support (2026-02-25)
- Updated supervisor CSV header sanitizer in `backend/handlers/app_workflow.go` to allow digits in column titles.
- Rule is now: lowercase letters, digits, and underscores are allowed; all other characters collapse to underscores.

### CSV Photo Field Name Mapping (2026-02-25)
- Updated CSV exports so photo fields now prefer the actual downloadable archive file names (not raw photo IDs) when those files exist.
- Managed workflow CSV:
  - `DownloadManagedWorkflowCSV` now precomputes the same photo archive names used by the managed photo ZIP naming logic and passes a `photo_id -> archive_filename` map into CSV building.
  - `buildManagedWorkflowCSV` now writes those archive filenames in the `photo_ids` column when available.
- Supervisor workflow export ZIP + CSV:
  - Reworked export sequence so photo files are written first, and successful writes register `photo_id -> archive_filename`.
  - `buildSupervisorWorkflowCSV` now uses that mapping for photo columns; if a photo file does not exist in the ZIP, it falls back to the original ID/value.
- Added shared filename helper functions to keep naming consistent between ZIP entries and CSV references.

### Validation (CSV Photo Field Name Mapping)
- `gofmt -w backend/handlers/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./handlers ./db ./router ./structs` (passed)

### Supervisor Photo Filename Token Numeric Support (2026-02-25)
- Updated `sanitizeSupervisorExportToken` in `backend/handlers/app_workflow.go` to allow digits in formatted photo name tokens.
- Rule for workflow/item title tokens is now uppercase letters, digits, and underscores (other characters collapse to underscores).

### Recurring Series Generation + Persistent Series Claim Propagation (2026-02-26)
- Fixed recurring workflow continuity so series now create successor workflows automatically:
  - Added recurring successor generation logic in `backend/db/app_workflow.go`.
  - Successors are cloned from the prior workflow (roles, role credentials, steps, step items).
  - On approval, recurring workflows now automatically create the next occurrence (blocked until predecessor payout).
  - On payout finalization, newly unblocked successors now automatically seed their next occurrence.
  - Added continuity healing during `RefreshWorkflowStartAvailability` to backfill missing successors for active recurring series.
- Added persistent series-step claim mapping in DB bootstrap (`backend/db/app.go`):
  - New table: `workflow_series_step_claims(series_id, step_order, improver_id, created_at, updated_at)`.
  - Added index by improver.
  - Added backfill query to seed mappings from latest active recurring assignments.
- Claim flow now persists and propagates recurring claims:
  - `ClaimWorkflowStep` now upserts `workflow_series_step_claims` for recurring workflows.
  - Claim is propagated to matching future workflows in the series (same step order), with backend enforcement of:
    - no duplicate improver assignment within a workflow,
    - no assignment when user is supervisor on that workflow,
    - absence-period coverage checks.
- Future generated recurring workflows now apply stored series claims automatically at creation time.
- Revocation/unclaim now stop future auto-assignment:
  - `UnclaimImproverWorkflowSeriesStep` deletes matching claim mapping.
  - `AdminRevokeWorkflowSeriesImproverClaims` deletes claim mappings for the revoked improver in that series.

### Validation (Recurring Series + Claim Propagation)
- `gofmt -w backend/db/app.go backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)
- Full `./...` test run is sandbox-limited here because `backend/test` requires localhost Postgres connectivity.

### Recurrence Conn-Busy Fix (2026-02-26)
- Fixed `conn busy` failures in recurring continuity and recurring successor clone flows in `backend/db/app_workflow.go`.
- Root cause:
  - Queries/execs were being issued on the same transaction while row cursors were still open (nested row iteration + writes).
- Changes:
  - `ensureRecurringWorkflowContinuity` now loads seed workflow IDs into memory, closes rows, then processes successor generation.
  - `ensureRecurringWorkflowSuccessorTx` now loads:
    - role seeds first, then role credentials, then performs inserts
    - step seeds first, then step items, then performs inserts
  - This removes all write/query calls while an active cursor on the same tx is still open.

### Validation (Conn-Busy Fix)
- `gofmt -w backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)

### Series-Primary Workflow Model + Series-First UI (2026-02-26)
- Added first-class `workflow_series` records in `backend/db/app.go`:
  - New table: `workflow_series(id, proposer_id, title, description, recurrence, created_at, updated_at)`.
  - Added backfill from existing `workflows` rows.
  - Added FK `workflows.series_id -> workflow_series.id` (`workflows_series_fk`).
  - Added startup sync to keep legacy workflow mirror columns aligned with series metadata.
- Updated workflow creation to create series records first:
  - `CreateWorkflow` now inserts into `workflow_series` and then creates the initial workflow row under that series.
- Updated workflow metadata reads to source from series records:
  - `GetWorkflowByID` now resolves `title`, `description`, and `recurrence` from `workflow_series` (with legacy fallback).
  - Updated active/admin/supervisor list queries and start-availability refresh query to use series metadata.
  - Updated claim and deletion-proposal recurrence/title lookups to use series metadata.
- Updated recurring/claim recurrence predicates to series-level recurrence:
  - Claim propagation, continuity seed loading, improver absence targeting, and series unclaim now check recurrence through `workflow_series`.
- Frontend series-first workflow display updates:
  - `frontend/app/improver/page.tsx`:
    - `My Workflows` now groups all claimed workflows by `series_id` (including one-time series).
    - Removed split between “Active Workflow Series” and “Active Workflows”; now one series-first list.
    - Preserved arrow-based intra-series navigation and detail modal navigation.
    - Unclaim action now appears only when a recurring series claim exists.
  - `frontend/app/admin/page.tsx`:
    - Workflows tab remains grouped by series for all workflows (including one-time), with uniform series wording.
    - Removed one-time/single-workflow conditional UI branching.
    - Revoke claim action remains available from series cards/modal.

### Validation (Series-Primary + Series-First UI)
- `gofmt -w backend/db/app.go backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)
- `cd frontend && npx tsc --noEmit --pretty false 2>&1 | rg "app/improver/page.tsx|app/admin/page.tsx|types/workflow.ts"` (no matches; command exits non-zero when no matches)

### Fix: Refresh Availability SQL Missing Recurrence Alias (2026-02-26)
- Fixed runtime SQL error in `RefreshWorkflowStartAvailability`:
  - Error: `column u.recurrence does not exist (SQLSTATE 42703)`
  - Cause: `updated_workflows` CTE selected recurrence expression without alias, while outer query referenced `u.recurrence`.
  - Change in `backend/db/app_workflow.go`:
    - `COALESCE(NULLIF(TRIM(s.recurrence), ''), w.recurrence) AS recurrence`

### Validation (Refresh Availability Alias Fix)
- `gofmt -w backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)

### Series-Owned Workflow Definitions + Step Series Reference (2026-02-26)
- Shifted workflow definition ownership fully to `workflow_series` in backend schema and queries.
- `backend/db/app.go` changes:
  - Removed legacy workflow definition columns from `workflows` bootstrap path (`title`, `description`, `recurrence`).
  - Added explicit startup drops for those legacy columns to keep schema aligned.
  - Kept/used `workflow_series` as the canonical source for title/description/recurrence.
  - Updated legacy series backfill to only create missing `workflow_series` records (with safe defaults) without reading dropped workflow columns.
- `workflow_steps` now series-referenced:
  - Added `workflow_steps.series_id` with FK to `workflow_series`.
  - Removed workflow FK constraint from `workflow_steps.workflow_id` so step rows reference series structurally.
  - Added backfill/update of `workflow_steps.series_id` from `workflows.series_id` for existing rows.
  - Added/ensured `workflow_steps.series_id` index and NOT NULL enforcement.
  - Updated step inserts (initial create + recurring successor clone) to write `series_id` explicitly.
- `workflow_step_submissions` and photo/submission flows still reference concrete `workflow_id` for response sets (unchanged behavior).
- Updated all workflow metadata SQL reads in `backend/db/app_workflow.go` to source from `workflow_series` only:
  - Replaced all remaining `w.title / w.description / w.recurrence` usage.
  - Updated recurrence checks and title lookups used by claim flows, availability refresh, deletion proposal views, proposal-expiry notices, and proposer notifications.
- Supervisor behavior preserved:
  - Supervisor workflow list/search/export queries still return title/recurrence/status datasets, now sourced through `workflow_series`.
  - CSV and photo export code paths remain unchanged functionally.

### Validation (Series-Owned Definitions + Step Series Reference)
- `gofmt -w backend/db/app.go backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)

### Fix: workflow_steps series_id Bootstrap Order (2026-02-26)
- Resolved startup failure: `error creating workflow_steps table: column "series_id" does not exist`.
- Root cause:
  - Existing DBs can have `workflow_steps` without `series_id`.
  - Bootstrap attempted `CREATE INDEX ... ON workflow_steps(series_id)` in the initial create block before the later `ALTER TABLE ... ADD COLUMN IF NOT EXISTS series_id` step.
- Change:
  - Removed early `workflow_steps_series_idx` creation from the create block.
  - Index creation remains in the later alter block after `series_id` is added/backfilled.

### Validation (series_id Bootstrap Order)
- `gofmt -w backend/db/app.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)

### Fix: Recurring Successor Insert Column/Value Mismatch (2026-02-26)
- Resolved runtime SQL error during continuity refresh:
  - `error inserting recurring workflow successor: INSERT has more expressions than target columns (SQLSTATE 42601)`
- Root cause:
  - In `ensureRecurringWorkflowSuccessorTx`, the `INSERT INTO workflows (...) VALUES (...)` for successor creation had one extra `$13` after timestamp migration/refactor.
  - This produced 21 expressions for 20 target columns.
- Change:
  - Corrected value list alignment so vote/finalization fields map correctly:
    - `vote_quorum_reached_at = $13`
    - `vote_finalize_at = $13`
    - `vote_finalized_at = $13`
    - `vote_decision = 'approve'`
    - `approved_at = $13`
    - `approved_by_user_id = NULL`

### Validation (Recurring Successor Insert Fix)
- `gofmt -w backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)

### Fix: Recurring Successor manager_required Type Mismatch (2026-02-26)
- Resolved runtime SQL error during recurring successor insert:
  - `column "manager_required" is of type boolean but expression is of type integer (SQLSTATE 42804)`
- Root cause:
  - Successor `INSERT INTO workflows` values were still offset after prior column-removal refactor.
  - `manager_required` was receiving literal `0` instead of boolean placeholder.
- Change:
  - Corrected values ordering for recurring successor creation:
    - `budget_weekly_deducted = 0`
    - `budget_one_time_deducted = 0`
    - `manager_required = $10`
    - `manager_improver_id = $11`
    - `manager_bounty = $12`

### Validation (manager_required Mismatch Fix)
- `gofmt -w backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)

### Recurrence Window + Upcoming Status Display (2026-02-26)
- Updated recurring generation behavior to enforce a single-future-window model per series.
- Backend (`backend/db/app_workflow.go`):
  - `ensureRecurringWorkflowSuccessorTx` now only generates successors when seed workflow status is `completed` or `paid_out`.
  - Added guard to prevent creating a successor when there is already any non-deleted future workflow (`start_at > now`) in the same series.
  - `ensureRecurringWorkflowContinuity` now seeds from the latest workflow per series and only proceeds when that latest workflow is `completed`/`paid_out` (recurring only).
  - Successor generation now runs at workflow completion time in submit flow:
    - step-not-possible completion path
    - normal completion path when all steps are complete
  - Removed approval-time/payout-time chaining behavior that could create extra future items.
- Resulting behavior:
  - once a workflow is completed, only the immediate next workflow is generated
  - at most one workflow per series with a future `start_at` exists at any time
- Frontend status rendering:
  - Added `frontend/lib/workflow-status.ts` with derived display status logic.
  - Workflows with `status in {approved, blocked}` and `start_at > now` display as `Upcoming`.
  - Applied to workflow card/detail displays in:
    - `frontend/app/improver/page.tsx`
    - `frontend/app/voter/page.tsx`
    - `frontend/app/proposer/page.tsx`
    - `frontend/app/admin/page.tsx`
    - `frontend/app/supervisor/page.tsx`
    - `frontend/components/workflows/workflow-details-modal.tsx`

### Validation (Recurrence Window + Upcoming Status)
- `gofmt -w backend/db/app_workflow.go`
- `cd backend && GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` (passed)
- `cd frontend && npx tsc --noEmit --pretty false 2>&1 | rg "app/improver/page.tsx|app/voter/page.tsx|app/proposer/page.tsx|app/admin/page.tsx|app/supervisor/page.tsx|components/workflows/workflow-details-modal.tsx|lib/workflow-status.ts"` (no matches for changed files)

### Improver Unclaim UX + Mobile Polish (2026-02-26)
- Updated improver workflow unclaim behavior/UI:
  - Removed `Unclaim Series` from workflow cards in `My Workflows`.
  - Added unclaim action only in the workflow details modal, positioned in the modal top-right via a new top-action slot.
  - Styled unclaim action as low-emphasis (`ghost`, small text) so it is present but not visually dominant.
  - Added a second confirmation modal (`Unclaim Series?`) before submitting unclaim requests.
- Workflow details modal enhancements for mobile:
  - Added optional `renderTopRightActions` prop in `frontend/components/workflows/workflow-details-modal.tsx`.
  - Header now uses responsive stacked layout on mobile and side-by-side on larger screens.
  - Adjusted dialog sizing/padding (`w-[96vw]`, `max-h-[92vh]`, `p-4 sm:p-6`) for better small-screen readability.
- My Workflows card mobile polish in `frontend/app/improver/page.tsx`:
  - Removed destructive unclaim button row from cards.
  - Kept card action row compact with `View Details` + compact arrow controls only.
  - Arrow controls now use icon-only square buttons and responsive alignment for mobile.

### Validation (Improver Unclaim UX + Mobile)
- Type checks in changed files showed no matches when filtered to the edited files.
- Full frontend `tsc` still has unrelated pre-existing errors in other modules (merchant/wallet/etc.), unchanged by this work.

### Fix: Start Button/Form Overlap in Improver Step UI (2026-02-26)
- Resolved issue where `Start Step` could show at the same time as editable step inputs.
- Cause:
  - start button rendered for step status `available` and `locked`.
  - form inputs rendered for `in_progress` and `available`.
  - overlap occurred on `available`.
- Change in `frontend/app/improver/page.tsx`:
  - `Start Step` now renders only when `step.status === 'locked'`.
  - once a step is `available` (form-ready), start button no longer appears.

### Improver My-Workflows Active-Series Filter (2026-02-26)
- Added a default-on checkbox in improver `My Workflows` view:
  - `Show only active workflow series`
- Active series definition implemented as requested in context of claimed work:
  - series is shown when at least one workflow in that series has a step assigned to the current improver with status `available` or `in_progress`.
- Behavior:
  - checkbox ON by default: only active series are shown.
  - checkbox OFF: all claimed series are shown.
- Also updated empty-state text to reflect when active-only filtering hides all results.

### Improver Series Card Copy/Indicator Update (2026-02-26)
- Updated `My Workflows` series cards in `frontend/app/improver/page.tsx`:
  - Removed middle-card text: `Step status: ...`
  - Removed middle-card text: `Series workflow x of y`
  - Added a compact bottom-right indicator bubble with icon + index format `x/y`.
- Kept existing right-side workflow status badge unchanged.

### Improver Series Card Index Styling Tweak (2026-02-26)
- Updated `frontend/app/improver/page.tsx`:
  - Changed the bottom-right series index display from a badge bubble with icon to plain text `x/y`.

### Improver Series Card Navigation + Status Label Update (2026-02-26)
- Updated `frontend/app/improver/page.tsx`:
  - Card status bubble now shows `Available` when display status would otherwise be `Approved`.
  - Series card navigation no longer loops from ends; indices clamp to first/last item.
  - Left/right tab buttons are disabled at bounds (grayed out) instead of wrapping.
  - Series tab buttons are hidden for one-time series.
  - Series cards now default to newest workflow in the series, so tabbing reveals prior submissions.
- Updated `backend/db/app_workflow.go`:
  - Improver workflow feed now includes statuses `completed`, `paid_out`, and `blocked` in addition to `approved`/`in_progress`, enabling prior submissions to appear in series tabbing.

### Workflow Modal Submission Rendering Alignment (2026-02-26)
- Updated shared modal rendering in `frontend/components/workflows/workflow-details-modal.tsx`:
  - Removed the separate post-list `Submitted Step Details` block.
  - Submission timestamp and step-not-possible details now render at step header level.
  - Item responses now render inline inside each corresponding work-item card under `Submitted Response`.
  - If an item has no submitted response, the item card explicitly shows that no response was submitted for that item.

### Supervisor Default Date Field + Title Format Update (2026-02-26)
- Updated `frontend/app/supervisor/page.tsx`:
  - Changed default `Date Field` filter selection from `Date Created` to `Start Time` (`dateField` now defaults to `start_at`).
  - Updated supervisor list title rendering to: `{SERIES TITLE} - {MM/DD/YYYY}` using local date derived from `start_at`.

### Improver Modal/Series Card Layout Tweaks (2026-02-26)
- Updated `frontend/components/workflows/workflow-details-modal.tsx`:
  - Moved top-right workflow action rendering into the title row so `Unclaim series` appears beside the modal title.
- Updated `frontend/app/improver/page.tsx`:
  - Series card action row now keeps `View Details` and tab arrows inline on mobile and web.
  - Desktop (`sm+`): `x/y` indicator now appears inline on the same row as actions.
  - Mobile: `x/y` indicator remains below the action row.

### Start Step Visibility Eligibility Guard (2026-02-26)
- Updated `frontend/app/improver/page.tsx`:
  - `Start Step` button for locked assigned steps now only renders when the step is actually eligible to start.
  - Eligibility mirrors backend start transition rules:
    - step 1: workflow start time has elapsed
    - step N>1: previous step status is `completed` or `paid_out`

### Supervisor Loading Spinner Style Alignment (2026-02-26)
- Updated `frontend/app/supervisor/page.tsx`:
  - Replaced the top-level `Loader2` spinner in the supervisor page loading state with the same circular border spinner style used in improver/proposer/voter panels.
  - Updated the loading container from `py-24` to `min-h-[70vh]` so spinner placement matches the vertical centering used in other panels.

### Workflow Board Claim Label Cleanup (2026-02-26)
- Updated `frontend/app/improver/page.tsx` workflow board cards:
  - Removed `Claim available in modal` badge.
  - Removed `Claims available in workflow details modal` inline card text.

### Complete Step Query Fix: Missing workflows.title (2026-02-26)
- Fixed completion-path SQL in `backend/db/app_workflow.go` (`CompleteWorkflowStep`):
  - Removed stale `workflows.title` selection after series-schema migration.
  - Workflow title is now resolved from `workflow_series.title` via subquery while locking the workflow row.
- This addresses runtime error:
  - `ERROR: column "title" does not exist (SQLSTATE 42703)` during step completion.

### Improver Absence Management Expansion (2026-02-26)
- Backend routes in `backend/router/router.go`:
  - Added `PUT /improvers/workflows/absence-periods/{absence_id}`.
  - Added `DELETE /improvers/workflows/absence-periods/{absence_id}`.
- Backend handlers in `backend/handlers/app_workflow.go`:
  - Added `UpdateImproverAbsencePeriod`.
  - Added `DeleteImproverAbsencePeriod`.
  - Absence date parsing now supports date-only values (`YYYY-MM-DD`) via `parseAbsenceBoundary`.
  - For date-only end values, end boundary is normalized as next-day start (exclusive) so same-day ranges are valid.
- Backend DB logic in `backend/db/app_workflow.go`:
  - Added absence update/delete DB methods.
  - Added guard that blocks edit/delete when another improver has already claimed work in that absence period.
  - Added reusable absence helpers for overlap checks and assignment release.
  - On edit/delete, claim propagation is reapplied (when claim mapping exists) so claims can be restored where absence no longer applies.
- Shared workflow structs/types:
  - Added update/delete absence request/result structs in `backend/structs/app_workflow.go`.
  - Added corresponding frontend types in `frontend/types/workflow.ts`.
- Frontend improver absence UI in `frontend/app/improver/page.tsx`:
  - Absence inputs are now date-only (`type="date"`), no time required.
  - Added coverage target mode:
    - `One workflow series step` (existing behavior)
    - `All active workflow serieses` (bulk create by iterating active recurring claims)
  - Added absence period `Edit` and `Delete` actions with backend enforcement.
  - Edit mode provides inline date fields + save/cancel actions.
  - Absence period display now shows date values (not datetime timestamps).

### Primary Rewards Account Selection for Improver/Supervisor (2026-02-26)
- Added role-level primary rewards account persistence:
  - `improvers.primary_rewards_account`
  - `supervisors.primary_rewards_account`
- DB bootstrap in `backend/db/app.go` now:
  - Creates both columns on fresh tables.
  - Adds both columns for existing tables if missing.
  - Backfills empty values from Smart Wallet 1 (`wallets.is_eoa = false`, `smart_index = 0`) when available.
- Backend role models now include `primary_rewards_account`:
  - `backend/structs/app_workflow.go` (`Improver`, `Supervisor`).
- Added backend validation + update APIs:
  - `PUT /improvers/primary-rewards-account`
  - `PUT /supervisors/primary-rewards-account`
  - Both enforce valid Ethereum address format and approved role status.
- Added defaults during role lifecycle:
  - On improver/supervisor request upsert and admin approval transitions, empty primary rewards account is defaulted from Smart Wallet 1 when available.
- Workflow payout selection now respects role-specific primary rewards account preference:
  - Step/improver payouts prefer improver primary account.
  - Supervisor/manager payouts prefer supervisor primary account.
  - Falls back to existing wallet selection logic if no role primary account is set.
- Settings UI (`frontend/app/settings/page.tsx`) updated for approved users:
  - Improver tab and Supervisor tab now each include `Primary Rewards Account` controls.
  - Dropdown shows user wallet names with short addresses (name-first display).
  - Includes `Custom account` option with manual address input.
  - Frontend Ethereum address validation enforced before save.
  - Save actions call new backend endpoints and update local role state on success.
