
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
