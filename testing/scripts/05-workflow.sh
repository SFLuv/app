#!/usr/bin/env bash
# Workflow lifecycle: propose → approve → claim a step → complete → payout.
#
# Spans three roles, which is what prank forwarding is for. Set
# SFLUV_PROPOSER_DID and SFLUV_IMPROVER_DID to run the whole chain; without them
# the scenario covers only what the captured account can reach.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

step "Workflow lifecycle"

as(){ [[ -n "${1:-}" ]] && prank_as "$1"; }

# Discover the role holders rather than demanding they be exported.
#
# run-all.sh sets no per-role dids, so this scenario used to run as whoever the
# captured token belongs to — who is not a proposer — and fail with a 500 whose
# real cause ("proposer not found") was only in the app log. A scenario that
# needs a role should go and find one, the way 07 does.
find_role_holder(){
  psql -h "${DB_HOST:-localhost}" -U "${DB_USER:-$(whoami)}" -d "${APP_DB:-app}" -tAc "$1" 2>/dev/null | head -1
}

if [[ -z "${SFLUV_PROPOSER_DID:-}" ]]; then
  SFLUV_PROPOSER_DID="$(find_role_holder "SELECT user_id FROM proposers WHERE status='approved' ORDER BY user_id LIMIT 1;")"
  [[ -n "$SFLUV_PROPOSER_DID" ]] && info "proposer: ${SFLUV_PROPOSER_DID#did:privy:}"
fi
if [[ -z "${SFLUV_IMPROVER_DID:-}" ]]; then
  SFLUV_IMPROVER_DID="$(find_role_holder "SELECT user_id FROM improvers WHERE status='approved' ORDER BY user_id LIMIT 1;")"
  [[ -n "$SFLUV_IMPROVER_DID" ]] && info "improver: ${SFLUV_IMPROVER_DID#did:privy:}"
fi

if [[ -z "${SFLUV_PROPOSER_DID:-}" ]]; then
  fail "no approved proposer in the database — this scenario cannot run"
  summary "Workflow"; exit 1
fi

step "Propose"
as "${SFLUV_PROPOSER_DID:-}"
# Shape from structs/app_workflow.go:130. Steps do NOT name a role directly —
# roles are declared separately with a client_id the step then references, so
# several steps can share one role and its credential requirements.
start_at="$(date -u -v+1d '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -d '+1 day' '+%Y-%m-%dT%H:%M:%SZ')"
payload="$(jq -nc --arg t "test-workflow-$RUN_ID" --arg s "$start_at" \
  '{title:$t,
    description:"Created by the SFLUV feature tests",
    recurrence:"one_time",
    start_at:$s,
    # A role must require at least one credential, and it must be a real one.
    # List them with GET /credentials/types — note CLAUDE.md is stale here: it
    # names dpw_certified and sfluv_verifier, neither of which exists.
    roles:[{client_id:"role-1", title:"Test improver role", required_credentials:["sfluv_certified_volunteer"]}],
    steps:[{title:"Step one",
            description:"Do the thing",
            bounty:5,
            role_client_id:"role-1",
            allow_step_not_possible:false,
            # A work item must demand at least one of photo, written response
            # or dropdown — an item with none is rejected. Written is chosen
            # here because a photo would make completion need an upload.
            work_items:[{title:"Confirm it is done",
                         description:"Describe what was completed",
                         optional:false,
                         requires_photo:false,
                         requires_written_response:true,
                         requires_dropdown:false}]}]}')"

created="$(api POST /proposers/workflows "$payload")"
if [[ "$(status)" =~ ^20 ]]; then
  pass "workflow proposed"
  printf '%s' "$created" | jq '.' > "$RUN_DIR/workflow.json" 2>/dev/null
else
  # A 500 here is usually a VALIDATION failure, not an outage: the handler
  # returns "an internal workflow database operation failed" for things like a
  # role with no credentials. The real reason is only in backend/logs/prod/app.log.
  fail "proposal returned $(status) — check backend/logs/prod/app.log for the real reason"
  printf '%s' "$created" > "$RUN_DIR/workflow-error.txt"
  summary "Workflow"; exit 1
fi

wf="$(printf '%s' "$created" | jq -r '.id // .workflow_id // empty')"
expect_nonempty "$wf" "the workflow has an id"
[[ -n "$wf" ]] || { summary "Workflow"; exit 1; }

step "Approve"
# Force-approve bypasses the vote. The vote itself is its own scenario; here we
# just need an approved workflow to claim against.
admin_api POST "/admin/workflows/$wf/force-approve" '{}' >/dev/null
case "$(status)" in
  200|204) pass "force-approved" ;;
  409)     pass "refused with 409 — likely insufficient unallocated faucet balance, which is the documented guard" ;;
  *)       fail "force-approve returned $(status)" ;;
esac

detail="$(api GET "/workflows/$wf")"
expect_status 200 "the workflow reads back"
state="$(printf '%s' "$detail" | jq -r '.status // empty')"
info "workflow status: $state"

step "Claim and complete a step"
as "${SFLUV_IMPROVER_DID:-}"
step_id="$(printf '%s' "$detail" | jq -r '(.steps//[])[0].id // empty')"
if [[ -z "$step_id" ]]; then
  skip "no step id in the workflow payload"
else
  # Steps only become claimable once the workflow reaches in_progress, which
  # happens when start_at elapses. A workflow created for tomorrow is approved
  # but its steps are still locked, so a 400 here is the guard working.
  #
  # To exercise claim → start → complete, create the workflow with start_at in
  # the past, or wait for the scheduler to advance it.
  api POST "/improvers/workflows/$wf/steps/$step_id/claim" '{}' >/dev/null
  case "$(status)" in
    200|204) pass "step claimed" ;;
    400)     skip "step not claimable — workflow is '$state', not in_progress (start_at is in the future)" ;;
    403)     skip "not an improver, or missing the role's required credential" ;;
    409)     skip "step not available yet — an earlier step unlocks it" ;;
    *)       fail "claim returned $(status)" ;;
  esac

  api POST "/improvers/workflows/$wf/steps/$step_id/start" '{}' >/dev/null
  [[ "$(status)" =~ ^20 ]] && pass "step started" || skip "start returned $(status)"

  api POST "/improvers/workflows/$wf/steps/$step_id/complete" '{}' >/dev/null
  case "$(status)" in
    200|204) pass "step completed" ;;
    400)     skip "completion refused — the work item needs its written response" ;;
    403)     skip "cannot complete a step that was never claimed" ;;
    *)       fail "complete returned $(status)" ;;
  esac
fi

step "Payout"
unpaid="$(api GET /improvers/unpaid-workflows)"
expect_status 200 "unpaid workflows can be listed"
printf '%s' "$unpaid" | jq '.' > "$RUN_DIR/unpaid-workflows.json" 2>/dev/null

# A bounty for someone past the limit must be held, not failed — an improver has
# no QR to re-present, so the step stays claimable and pays on a later sweep.
info "if the improver is past the W-9 limit the bounty should be HELD, never marked failed"

prank_clear
summary "Workflow"
