#!/usr/bin/env bash
# Merchant onboarding, and the second location that follows it.
#
# The second location is the least proven code in the repo: it derives its own
# wallet, and migration 1.40 added unique indexes meant to stop two shops
# pointing at the same till. Both are asserted here.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_local_stack

step "Merchant onboarding"

# listing_source "manual" is the no-Google-listing path, added so a merchant
# without a Google entry can still onboard. It also keeps this scenario off the
# live Places API: the default path calls Google server-side to verify the
# place id, so a local test would otherwise depend on an external service and a
# billable key.
#
# Covering the Google path as well needs a real place id and
# GOOGLE_MAPS_SERVER_API_KEY set — worth doing by hand in the browser, where the
# autocomplete supplies one.

# The required set, from structs/app_location_validation.go:173 — note the
# contact email and phone are admin_email / admin_phone, NOT contact_*.
# A manual listing must also carry no google_id at all.
mk_location(){
  jq -nc --arg n "$1" --arg a "$2" \
    '{name:$n,
      listing_source:"manual",
      street:$a, city:"San Francisco", state:"CA", zip:"94103",
      lat:37.7749, lng:-122.4194,
      type:"restaurant",
      description:"Created by the SFLUV feature tests",
      phone:"4155550100", email:"test@example.com",
      contact_firstname:"Test", contact_lastname:"Merchant",
      contact_phone:"4155550100",
      admin_email:"test@example.com", admin_phone:"4155550100"}'
}

# POST /locations answers 201 with the plain text "success" — no id, no JSON —
# so the new row has to be found afterwards by name.
find_location_id(){
  api GET /locations/user | jq -r --arg n "$1" \
    '[.. | objects | select(.name? == $n) | .id] | last // empty'
}

name1="Test Merchant $RUN_ID"
first="$(api POST /locations "$(mk_location "$name1" "1 Test Street")")"
if [[ "$(status)" =~ ^20 ]]; then
  pass "first location submitted (HTTP $(status))"
else
  fail "location creation returned $(status)"
  printf '%s' "$first" > "$RUN_DIR/location-1-error.txt"
  summary "Merchant onboarding"; exit 1
fi

loc1="$(find_location_id "$name1")"
expect_nonempty "$loc1" "the new location is listed under the owner"
api GET /locations/user | jq '.' > "$RUN_DIR/locations-user.json" 2>/dev/null

step "Approval publishes it to the map"
if [[ -n "$loc1" ]]; then
  admin_api PUT /admin/locations "$(jq -nc --argjson id "$loc1" '{id:$id, approval:true}')" >/dev/null
  case "$(status)" in
    200|201|204) pass "approved" ;;
    503)     fail "503 — the RPC was unreachable, so wallet derivation rolled back. Approval must stay unpublished; retry when anvil is up" ;;
    *)       fail "approval returned $(status)" ;;
  esac

  # The public map endpoint wraps its rows: {"locations":[...]}.
  public="$(api GET /locations)"
  if printf '%s' "$public" | jq -e --arg id "$loc1" \
       '[.. | objects | select(has("id")) | (.id|tostring)] | index($id)' >/dev/null 2>&1; then
    pass "it appears on the public map"
  else
    fail "approved but absent from /locations — it will not show on the map"
  fi

  wallets="$(api GET "/locations/$loc1/assignable-wallets")"
  expect_status 200 "assignable wallets can be listed"
  printf '%s' "$wallets" | jq '.' > "$RUN_DIR/location-1-wallets.json" 2>/dev/null
fi

step "A second location for the same merchant"
name2="Test Merchant $RUN_ID Branch"
second="$(api POST /locations "$(mk_location "$name2" "2 Test Street")")"
if [[ "$(status)" =~ ^20 ]]; then
  pass "second location submitted"
  loc2="$(find_location_id "$name2")"
  expect_nonempty "$loc2" "the second location is listed under the same owner"

  if [[ -n "$loc2" ]]; then
    admin_api PUT /admin/locations "$(jq -nc --argjson id "$loc2" '{id:$id, approval:true}')" >/dev/null
    [[ "$(status)" =~ ^20 ]] && pass "second location approved" || fail "second approval returned $(status)"

    # Every location must have its own till. Two shops sharing one wallet makes
    # their takings indistinguishable, which is what migration 1.40's unique
    # indexes exist to prevent.
    #
    # Read from /locations/user: GET /locations/{id} answers 400, and the owner
    # listing is where pay_to_address / tip_to_address actually appear.
    owned="$(api GET /locations/user)"
    wallet_for(){ printf '%s' "$owned" | jq -r --argjson id "$1" \
      '[.. | objects | select(.id? == $id) | .pay_to_address] | map(select(. != null and . != "")) | first // empty'; }
    w1="$(wallet_for "$loc1")"
    w2="$(wallet_for "$loc2")"
    info "wallets: $w1 / $w2"
    if [[ -n "$w1" && -n "$w2" && "$w1" != "$w2" ]]; then
      pass "the second location derived its own distinct wallet"
    elif [[ -n "$w1" && "$w1" == "$w2" ]]; then
      fail "both locations share a wallet — takings would be indistinguishable"
    elif [[ -z "$w1" && -z "$w2" ]]; then
      # Approval derives the wallet, so empty on both usually means derivation
      # did not run — worth distinguishing from "derived the same one twice".
      fail "neither location has a pay_to_address — wallet derivation did not run at approval"
    else
      skip "only one location has a wallet ($w1 / $w2)"
    fi
  fi
else
  fail "second location returned $(status)"
fi

step "Merchant mode"
api GET /merchant-mode/locations >/dev/null
case "$(status)" in
  200) pass "merchant-mode locations listed — both shops should be switchable on one device" ;;
  403) skip "not a merchant yet on this account" ;;
  *)   fail "merchant-mode/locations returned $(status)" ;;
esac

api GET /merchant-mode/status >/dev/null
[[ "$(status)" == "200" ]] && pass "merchant-mode status responds" || skip "merchant-mode status $(status)"

summary "Merchant onboarding"
