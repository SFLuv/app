# Improver Workflows Fetch Time Reduction

## Problem

The improver page's active workflow fetch was slow because the top-level `/improvers/workflows` request was doing full workflow hydration for every candidate workflow.

Before this change, the backend queried up to 500 workflow IDs, then called `GetWorkflowByID` once per workflow. That loaded roles, steps, work items, submissions, and other detail-only data even though the first render only needed enough information to draw workflow cards.

## Fix Summary

The endpoint now returns lightweight card metadata for the improver feed and leaves full workflow loading to the details modal.

The top-level response includes enough data to render:

- Workflow card title, description, status, start time, recurrence, bounty metadata, and series metadata.
- Whether the user has claimed work on the workflow.
- Whether the user's claimed work is currently active.
- A small `assigned_steps` summary for claimed workflow cards and series grouping.
- A small `claimable_step` summary for workflow board cards.
- Pagination metadata: `total`, `page`, and `count`.

When a user opens a workflow details modal, the frontend now fetches the full workflow from `/workflows/{workflow_id}` instead of relying on the top-level improver feed to already contain full details.

## Backend Changes

Files changed:

- `backend/db/app_workflow.go`
- `backend/handlers/app_workflow.go`
- `backend/structs/app_workflow.go`
- `backend/db/app.go`

### Data Shape

Added lightweight response structs:

- `ImproverWorkflowListItem`
- `ImproverWorkflowStepSummary`

The `ImproverWorkflowFeed` response now contains:

```go
type ImproverWorkflowFeed struct {
    ActiveCredentials []string
    Workflows         []*ImproverWorkflowListItem
    Total             int
    Page              int
    Count             int
}
```

### Query Strategy

`GetImproverWorkflows` now performs summary selection in SQL instead of looping over workflow IDs and hydrating each one.

The query now builds narrower candidate sets before joining workflow metadata:

- `assigned_workflow_ids`: starts from `workflow_steps.assigned_improver_id = $user`.
- `claimable_workflow_ids`: starts from unassigned, claimable `workflow_steps`.
- `manager_workflow_ids`: starts from workflows where the user is the assigned manager.
- `manager_eligible_workflow_ids`: starts from workflows with an open manager role the user is credentialed for.

The default endpoint behavior still returns the combined feed needed by the current improver page, but the endpoint now also supports narrower fetch scopes:

- `scope=assigned`
- `scope=claimed`
- `scope=mine`
- `scope=board`
- `scope=claimable`

The assigned/claimed/mine scopes only use the assigned workflow candidate path. That means fetching an improver's own claimed workflows is driven by the assigned-improver index rather than by scanning active workflows and checking each one for relevance.

The query still computes relevance metadata server-side:

- Whether the user is assigned to at least one step.
- Whether the user has an active assigned step.
- Whether the user can claim at least one available or unlockable step based on credentials and absence coverage.
- Whether the user is the manager, or is eligible for an unclaimed manager role.

The response orders more urgent workflows first:

1. Active claimed steps.
2. Claimed workflows.
3. Claimable workflows.
4. Manager workflows.
5. Other eligible manager workflows.

## Frontend Changes

Files changed:

- `frontend/app/improver/page.tsx`
- `frontend/types/workflow.ts`

The improver page now stores the top-level feed as `ImproverWorkflowListItem[]` instead of full `Workflow[]`.

Card and grouping logic now uses summary fields:

- `assigned_steps`
- `has_claimed_step`
- `has_active_claimed_step`
- `claimable_step`

The details modal path still uses the full `Workflow` type. Opening a workflow from the board or "My Workflows" now lazy-loads full details through `/workflows/{workflow_id}`.

## Indexes Added

Added indexes to support the new summary query:

- `workflows_active_status_start_created_idx`
- `workflows_manager_improver_status_idx`
- `workflows_open_manager_role_status_idx`
- `workflow_steps_assigned_improver_status_workflow_idx`
- `workflow_steps_assigned_improver_workflow_idx`
- `workflow_steps_claimable_workflow_status_role_idx`
- `workflow_steps_claimable_status_role_workflow_idx`
- `workflow_improver_absences_improver_series_step_idx`

These support active workflow filtering, assigned-step lookups by improver ID, claimable-step lookups, manager workflow lookups, and absence coverage checks.

## Expected Impact

The improver page no longer performs one full workflow load per card on initial fetch. The initial request should be substantially faster and lighter, especially for users with many active or recurring workflows.

For own-workflow fetches, callers can use `scope=assigned`, `scope=claimed`, or `scope=mine` so the candidate set is derived from indexed assigned step rows.

Full detail data is fetched only when it is actually needed, such as when the user opens a workflow details modal.

## Verification

Backend focused checks passed with a writable Go cache:

```sh
GOCACHE=/tmp/sfluv-go-build-cache go test -vet=off ./handlers ./db ./structs
```

Whitespace check passed:

```sh
git diff --check
```

Frontend typecheck was run:

```sh
npx tsc --noEmit --pretty false
```

It still reports pre-existing unrelated settings and merchant type errors, but no improver workflow errors remained after this change.
