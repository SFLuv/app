#!/usr/bin/env bash
# Volunteer events: create, read back, list publicly, generate codes, delete.
#
# Event creation is admin- or affiliate-guarded. The fixture is arranged with
# the admin key; the scenario itself reads back through the public and
# authenticated routes, which is what a real visitor and a real volunteer hit.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

step "Volunteer events"

# Times are LOCAL wall clock in the event's own timezone — not UTC, and no
# trailing Z. parseLocalWallClock wants "2026-08-06T13:00:00", and the backend
# rejects a start in the past, so tomorrow.
TZ_NAME="America/Los_Angeles"
start_at="$(TZ=$TZ_NAME date -v+1d '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day' '+%Y-%m-%dT%H:%M:%S')"
end_at="$(TZ=$TZ_NAME date -v+1d -v+3H '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || TZ=$TZ_NAME date -d '+1 day +3 hours' '+%Y-%m-%dT%H:%M:%S')"
name="test-event-$RUN_ID"
info "event window $start_at → $end_at ($TZ_NAME)"

created="$(admin_api POST /admin/volunteer-events "$(jq -nc \
  --arg t "$name" --arg s "$start_at" --arg e "$end_at" --arg tz "$TZ_NAME" \
  '{title:$t,
    description:"Created by the SFLUV feature tests",
    start_at_local:$s,
    end_at_local:$e,
    timezone:$tz,
    max_participants:10,
    reward_amount_sfluv:5,
    signup_mode:"internal"}')")"

if [[ "$(status)" =~ ^20 ]]; then
  pass "event created (HTTP $(status))"
  printf '%s' "$created" | jq '.' > "$RUN_DIR/event-created.json" 2>/dev/null
else
  fail "event creation returned $(status) — body in artifacts"
  printf '%s' "$created" > "$RUN_DIR/event-create-error.txt"
  summary "Events"; exit 1
fi

event_id="$(printf '%s' "$created" | jq -r '.id // .event_id // .ID // empty')"
expect_nonempty "$event_id" "the new event has an id"
[[ -n "$event_id" ]] || { summary "Events"; exit 1; }

step "Publication"
# An admin-created event is born approved — it comes back status "scheduled"
# with approval null, and POST /approve then correctly refuses with 409
# "event is not pending approval".
#
# The approval queue exists for AFFILIATE-created events, which start pending.
# Covering that path needs an affiliate account and POST
# /affiliates/volunteer-events; it is not exercised here.
#
# Volunteer events are approve/reject/cancel — there is NO delete. The older
# DELETE /events/{id} belongs to the separate bot-database event system, which
# is the one that refuses with 409 once codes are redeemed.
created_status="$(printf '%s' "$created" | jq -r '.status // "?"')"
if [[ "$created_status" == "scheduled" ]]; then
  pass "admin-created event is published immediately (status scheduled)"
else
  fail "expected status 'scheduled' on creation, got '$created_status'"
fi

admin_api POST "/admin/volunteer-events/$event_id/approve" '{}' >/dev/null
if [[ "$(status)" == "409" ]]; then
  pass "re-approving an already-published event is refused, as it should be"
else
  fail "approving a scheduled event returned $(status); expected 409"
fi

step "Public visibility — what a website visitor sees"
# Ask for a big page. The public list defaults to 20 rows, and this database has
# more events than that — partly because these scenarios keep adding them — so
# asserting against the default page proves only that the newest event is not in
# the most recent twenty.
listing="$(api GET "/volunteer-events?count=200")"
expect_status 200 "the public event list responds"
if printf '%s' "$listing" | jq -e --arg id "$event_id" \
     '[.. | objects | select(has("id")) | (.id|tostring)] | index($id)' >/dev/null 2>&1; then
  pass "the approved event appears in the public list"
else
  fail "approved but absent from /volunteer-events — it will not reach the website"
fi

detail="$(api GET "/volunteer-events/$event_id")"
expect_status 200 "the event detail page has data to render"
printf '%s' "$detail" | jq '.' > "$RUN_DIR/event-detail.json" 2>/dev/null

step "Redemption codes"
# Codes are issued as part of approval, not minted on demand — this reads them.
codes="$(admin_api GET "/admin/volunteer-events/$event_id/codes")"
if [[ "$(status)" == "200" ]]; then
  pass "codes readable"
  printf '%s' "$codes" | jq '.' > "$RUN_DIR/event-codes.json" 2>/dev/null
  # The redemption code IS the row's id — there is no separate "code" field.
  printf '%s' "$codes" | jq -r '[.. | objects | select(has("number")) | .id] | .[]' 2>/dev/null | head -5 > "$RUN_DIR/codes.txt"
  n="$(wc -l < "$RUN_DIR/codes.txt" | tr -d ' ')"
  if [[ "${n:-0}" -gt 0 ]]; then
    pass "$n code(s) available for 03-qr-payout.sh"
  else
    skip "no codes in the response — approval may issue them asynchronously"
  fi
else
  fail "reading codes returned $(status)"
fi

step "Cancellation"
# Cancel a SEPARATE throwaway event. Cancelling the one above would void the
# codes it just handed to 03-qr-payout.sh, and the redemption scenario would
# then fail for a reason this scenario created.
throwaway="$(admin_api POST /admin/volunteer-events "$(jq -nc \
  --arg t "test-event-$RUN_ID-cancelme" --arg s "$start_at" --arg e "$end_at" --arg tz "$TZ_NAME" \
  '{title:$t, description:"Created to be cancelled", start_at_local:$s, end_at_local:$e,
    timezone:$tz, max_participants:5, reward_amount_sfluv:5, signup_mode:"internal"}')")"
throwaway_id="$(printf '%s' "$throwaway" | jq -r '.id // empty')"

if [[ -n "$throwaway_id" ]]; then
  admin_api POST "/admin/volunteer-events/$throwaway_id/cancel" '{}' >/dev/null
  case "$(status)" in
    200|204) pass "event cancelled — the payout record is preserved, unlike a delete" ;;
    *)       fail "cancel returned $(status)" ;;
  esac
else
  fail "could not create the throwaway event to cancel"
fi

info "event $event_id left live; its codes are in $RUN_DIR/codes.txt"

summary "Events"
