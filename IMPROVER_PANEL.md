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
