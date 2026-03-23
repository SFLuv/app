# Workflow Security Hardening (Current Scope)

## Scope
- Implement the pending security hardening for workflow payouts and submission-data access controls discussed earlier.

## Implemented
- Added payout in-flight locking at DB level to prevent duplicate sends under concurrency:
  - `workflow_steps.payout_in_progress`
  - `workflows.manager_payout_in_progress`
- Added DB claim methods for payout attempts:
  - `ClaimWorkflowStepPayoutAttempt(...)`
  - `ClaimWorkflowManagerPayoutAttempt(...)`
- Updated payout processor flow to claim lock **before** any transfer is attempted.
- Updated payout failure/success writes to clear in-progress flags safely.
- Updated payout retry requests to reject retries while a payout is in progress.
- Updated workflow completion transitions to reset payout in-progress flags.
- Updated workflow loading queries/structs to include in-progress payout fields (internal-only, not exposed in API JSON).
- Updated payout target selection to skip records already in payout progress.
- Added admin alert path for post-transfer DB state-update failures (manual intervention scenario, duplicate-send-safe).
- Submission data redaction and photo access restrictions remain enforced to relevant parties only.
- Added admin manual payout-lock resolution endpoint with persisted audit records:
  - `POST /admin/workflows/{workflow_id}/payout-lock-resolution`
  - request fields:
    - `target_type`: `step` or `supervisor`
    - `action`: `mark_paid_out` or `mark_failed`
    - `step_id`: required for `target_type=step`
    - `error_message`: optional for `mark_failed`
  - every action is written to `workflow_payout_admin_actions` with admin actor and timestamp.

## Endpoint Examples
- Mark a stuck step payout lock as paid out:
  - `{"target_type":"step","action":"mark_paid_out","step_id":"<step-id>"}`
- Mark a stuck step payout lock as failed:
  - `{"target_type":"step","action":"mark_failed","step_id":"<step-id>","error_message":"manual reconciliation required"}`
- Mark a stuck supervisor payout lock as paid out:
  - `{"target_type":"supervisor","action":"mark_paid_out"}`
- Mark a stuck supervisor payout lock as failed:
  - `{"target_type":"supervisor","action":"mark_failed","error_message":"transfer uncertain, unlocked for retry"}`

## Validation
- `GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` passes.

## Follow-Up
- Locked workflow edit proposals so recurrence, recurrence end date, and start date/time can no longer be changed during edit mode. The proposer UI now shows them as read-only, and the backend always reuses the existing series schedule when building an edit proposal.
- Investigated workflow edit behavior before improver claims. Root issue: approved edit proposals update `workflow_series.current_state_id` but do not retarget the existing unclaimed workflow instance or rebuild its copied roles/steps, so the live workflow stays on the old state while the series points at the new one. Secondary frontend footgun: template application clears `editProposalWorkflowId`, which would fall the shared form back to create mode and produce a genuinely new series if used during edit mode.

- Backend now explicitly rejects workflow edit proposal payloads that include `start_at`, `recurrence`, or `recurrence_end_at`, instead of silently ignoring them.
- Backend startup has been split so DB initialization/migrations now live at `backend/cmd/init/main.go`, while the normal server boot path lives at `backend/cmd/server/main.go` and assumes schema setup has already been handled.
- Applying a template during workflow edit mode now preserves `editProposalWorkflowId` and the locked schedule fields, so template use no longer falls the proposer form back into create mode.
- Restyled the proposer edit-mode notice to a neutral card treatment and shortened the copy.
- Moved the proposer workflow modal edit action to the top-right action area and moved the improver live camera capture button below the camera preview.
- Moved the proposer workflow modal `Save as Template` action into the same top action area as `Edit Workflow`.
- Repositioned the proposer workflow modal actions so they sit right-aligned above the roles section instead of beside the title.
- Restored the proposer modal `Edit Workflow` button to its filled styling while keeping the new placement above the roles section.

- Voter panel now supports admin force approval for workflow edit and deletion proposals, and edit/deletion proposal cards are clickable to open redacted detail previews in the workflow modal.
- Moved redeemer role sync, minter role sync, and Privy linked-email sync out of cmd/server startup and into cmd/init, so normal server boot no longer blocks on those startup-wide synchronization passes.
- Updated the voter active-workflow deletion proposal button so pending proposals now show `Workflow Deletion Proposed` or `Series Deletion Proposed` and remain disabled.
- Added proposer-side pending deletion proposal awareness so the deletion button now disables and switches to `Workflow Deletion Proposed` or `Series Deletion Proposed` once a matching deletion proposal already exists.
- Fixed proposer deletion-button matching so pending deletion proposals are detected by either matching series id or workflow id first, then the button label uses the actual pending proposal target type instead of relying only on the current inferred deletion target.
- Enforced vote-view notify-email redaction in the voter panel modal by stripping dropdown option `notify_emails` and preserving only `notify_email_count` for workflow, edit-proposal, and deletion-proposal detail views.
- Moved vote-view notify-email redaction into backend voter routes by adding a voter-specific workflow detail endpoint and explicit vote-view sanitization for voter workflow lists/details and edit proposal payloads.
- Smart wallet initialization now deploys code on the client during sign-in whenever an initialized smart wallet is still undeployed, and newly created smart wallets (signup/manual add) now require deployment to succeed before the wallet row is finalized.
