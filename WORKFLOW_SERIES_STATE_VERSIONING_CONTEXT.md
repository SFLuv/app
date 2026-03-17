# Workflow Series Regression Context And Next-Step Plan (No Implementation Yet)

## Current Incident (Observed)
- After the recent workflow-state update, recurring workflow cards that should be continuous (example: `3/13`, `3/14`, `3/15`, `3/16`) are splitting into separate groups/cards (`3/13` + `3/14` and `3/15` + `3/16`).
- In one affected sequence, `3/15` is submitted/completed, but `3/16` is still shown as `blocked`.
- Working hypothesis: recurrence/blocking/grouping logic is mixing workflow instance identity (`workflow.id`) and workflow state identity (`workflow_state_id`) in a way that breaks continuity.

## What Changed Before This Incident

### 1) Workflow data model and recurrence engine
- Added workflow state snapshots:
  - `workflow_states` table
  - `workflows.workflow_state_id`
  - `workflow_series.current_state_id`
- Added recurrence end support:
  - `recurrence_end_at` on workflow series/state/requests.
- Added recurring catch-up behavior for elapsed periods.
- Added `skipped`/`failed` status support in workflow model and filtering/export surfaces.

### 2) Workflow edit proposal system
- Added edit-proposal entities and voting flow:
  - proposal storage + vote tables
  - proposer endpoints to submit edit proposals
  - voter endpoints/UI to review and vote
  - finalize/apply state update path on approval
- Kept existing workflow instances as snapshots and applied edits to future generation path.

### 3) Proposer workflow builder UX and schema inputs
- Added drag reorder for steps/items/dropdown options (desktop and mobile handle flow).
- Added collapsible card behavior across top-level and nested workflow sections.
- Made step description optional.
- Updated template/create handling around recurrence + start-time behavior.

### 4) Improver panel behavior
- Updated tab/query-param syncing and browser history behavior.
- Updated “My Workflows” filtering/sorting behavior.
- Updated recurring series navigation behavior in modal contexts.
- Updated photo capture UX flow.

### 5) Wallet and payout hardening
- Added wallet-0 enforcement utilities in frontend app context (`ensurePrimarySmartWallet`).
- Updated improver join/settings/redeem flows to require/verify smart wallet index `0`.
- Updated payout/redeem address resolution to prefer smart wallet index `0`.
- Added primary rewards account default/sync improvements.

### 6) Email safety hardening
- Added centralized HTML escaping utility for email-injected values.
- Applied escaping to dynamic values passed into styled email content paths.

### 7) Faucet redemption flow hardening
- Refined login/signup/redeem sequencing and state gating.
- Added more explicit readiness checks before proceeding with redemption actions.

## Why The New Regression Is Plausible
- We introduced a new layer of identity (`workflow_state_id`) while preserving workflow instance identity (`workflow.id`) and series identity (`series_id`).
- If grouping or blocking logic now keys off state identity (or mixed identity) instead of series + ordered instances, the UI can split logically continuous recurring runs.
- If `blocked_by_workflow_id` is computed/checked against the wrong predecessor instance after state-version transitions, a later workflow can remain blocked even when the previous date was completed.

## Terminology Direction (Requested)
- Treat `workflow_state_id` conceptually as a **workflow state version**.
- Preferred naming going forward in code/docs/UI: `workflow_state_version` (or `workflow_state_version_id`).
- Keep DB compatibility during migration, but make code-level naming explicit to avoid confusing “workflow instance” vs “workflow state version”.

## Plan For Next Steps (Phased)

### Phase 0: Evidence capture and SQL audit (read-only)
- Build a per-series timeline report with:
  - `workflow.id`, `series_id`, `workflow_state_id`, `start_at`, `status`, `blocked_by_workflow_id`
- Identify all series where cards split unexpectedly despite continuous cadence.
- Identify blocked workflows whose predecessor is already `completed`/`paid_out`/`skipped`.

### Phase 1: Define canonical identity rules
- Canonical rules to enforce:
  - **Series continuity**: by `series_id` + date/order.
  - **Blocking predecessor**: by prior workflow instance (`workflow.id`) in series order.
  - **State template/version**: by `workflow_state_id` only for definition payload (roles/steps/etc), not continuity linkage.

### Phase 2: Serialization and versioning guardrails
- Add explicit per-series state version ordering (monotonic version number).
- Enforce one state-version transition at a time per series via transaction + row lock strategy.
- Ensure recurring generation always references the latest committed state version for newly generated instances.

### Phase 3: Retroactive data repair migration
- Backfill/repair script to:
  - Normalize contiguous recurring instances into the correct series grouping semantics.
  - Recompute predecessor links (`blocked_by_workflow_id`) by chronological order.
  - Preserve historical instance payload via existing state-version references.
- Dry-run mode first; apply only after validation output is reviewed.

### Phase 4: Query and UI alignment
- Backend list/query endpoints should group and order by series continuity, not split by state version.
- Frontend card grouping should remain series-based, with state version shown only as metadata if needed.

### Phase 5: Validation and rollout
- Run DB snapshot checks before/after migration.
- Validate known broken timeline (`3/13` -> `3/16`) end-to-end.
- Confirm blocked-state transitions resolve correctly after predecessor completion.
- Add targeted regression tests for:
  - recurring continuity with state-version changes
  - blocked predecessor checks
  - grouped card rendering for continuous series

## Explicit Non-Goals In This Document
- No code or migration is implemented here.
- This is a planning and context handoff artifact only.

## Immediate Implementation Checklist (When We Start)
- [ ] Produce audit query outputs for affected production series.
- [ ] Lock canonical identity rules above.
- [ ] Implement serialization/version-number migration.
- [ ] Implement blocking-chain repair migration.
- [ ] Patch grouping and blocking logic.
- [ ] Verify with targeted test cases and real affected series IDs.
