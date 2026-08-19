#!/usr/bin/env bash
# W-9 round trip: threshold → hold → form → release.
#
# Runs against whichever provider is configured. With W9_PROVIDER=fake the whole
# loop is offline and automatic. With W9_PROVIDER=taxbandits it stops at the
# hosted form and prints the URL, because only a human can sign it — there is no
# W-9 simulator, verified 2026-08-19.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

step "W-9 round trip"

status="$(api GET /w9/status)"
if [[ "$(status)" != "200" ]]; then
  fail "GET /w9/status returned $(status) — is a token captured?"
  summary "W-9"; exit 1
fi

printf '%s' "$status" | jq '.' > "$RUN_DIR/w9-status-before.json" 2>/dev/null
expect_nonempty "$(printf '%s' "$status" | jq -r '.tax_year')" "status reports a tax year"

cleared="$(printf '%s' "$status" | jq -r '.cleared')"
earned="$(printf '%s' "$status" | jq -r '.earned_sfluv')"
threshold="$(printf '%s' "$status" | jq -r '.threshold_sfluv')"
tier="$(printf '%s' "$status" | jq -r '.tier // "none"')"
info "earned $earned of $threshold · cleared=$cleared · tier=$tier"

# The tier fields are what the mobile modal renders. Missing means an old
# backend, and the modal would show nothing at all.
for field in earned_base threshold_base tier_acknowledged blocked; do
  if printf '%s' "$status" | jq -e "has(\"$field\")" >/dev/null 2>&1; then
    pass "status carries $field"
  else
    fail "status is missing $field — the tier modal cannot render"
  fi
done

step "Requesting a form"
if [[ "$cleared" == "true" ]]; then
  # A cleared filing short-circuits EnsureW9Request, so no form URL exists and
  # /w9/start answers 503 "No tax form is available right now. Please try again
  # shortly." That message describes a transient outage; the actual situation is
  # "you already filed and need nothing". The UI hides the button for cleared
  # users, so nobody should normally see it — but the wording is worth fixing.
  #
  # To exercise the real form loop, use an account below the threshold, or clear
  # this user's filing with POST /admin/w9/{user_id}/clear.
  skip "already cleared for this tax year — nothing to request"
  info "to test the form loop, pick an unfiled account or reset this one"
  summary "W-9"; exit 0
fi

start="$(api POST /w9/start)"
if [[ "$(status)" == "200" ]]; then
  url="$(printf '%s' "$start" | jq -r '.form_url')"
  expect_nonempty "$url" "a hosted form URL was issued"
  printf '%s\n' "$url" > "$RUN_DIR/w9-form-url.txt"
  info "form: $url"

  case "$url" in
    *localhost*|*127.0.0.1*)
      # The fake provider serves a stub form from the backend, so the whole
      # loop closes without a human.
      info "fake provider — submitting the stub form"
      curl -s -o /dev/null -X POST "$url" --max-time 20
      info "waiting for the maintenance sweep to poll (runs about every 5 minutes)"
      after="$(api GET /w9/status)"
      printf '%s' "$after" | jq '.' > "$RUN_DIR/w9-status-after.json" 2>/dev/null
      info "filing status now: $(printf '%s' "$after" | jq -r '.filing_status')"
      ;;
    *)
      skip "real vendor — open the URL above and sign it, then re-run to see the release"
      ;;
  esac
elif [[ "$(status)" == "503" ]]; then
  fail "no provider configured (503) — set W9_PROVIDER"
else
  fail "POST /w9/start returned $(status)"
fi

step "Tier acknowledgement"
if [[ "$tier" != "none" && "$tier" != "null" ]]; then
  api POST "/w9/tier/$tier/ack" >/dev/null
  expect_status 200 "acknowledging tier $tier"
  re="$(api GET /w9/status)"
  ack="$(printf '%s' "$re" | jq -r '.tier_acknowledged')"
  if [[ "$tier" == "blocked" ]]; then
    # Tier 4 is deliberately re-armed: it is the only thing standing between a
    # blocked person and being paid, so dismissing it must not be permanent.
    info "blocked tier acknowledged=$ack (re-arms on the next refusal by design)"
  elif [[ "$ack" == "true" ]]; then
    pass "acknowledgement persisted"
  else
    fail "acknowledgement did not persist — the modal will reappear every poll"
  fi
else
  skip "no outstanding tier to acknowledge"
fi

summary "W-9"
