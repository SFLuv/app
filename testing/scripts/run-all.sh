#!/usr/bin/env bash
# Runs every headless scenario in order and writes one report.
#
# Order matters: events generate the codes the QR scenario redeems. Each script
# is independent enough to run alone, but the full sequence is the cheapest way
# to see the whole system at once.
cd "$(dirname "${BASH_SOURCE[0]}")"
export SFLUV_RUN_ID="${SFLUV_RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
source ./lib.sh

REPORT="$RUN_DIR/report.txt"
overall=0

./preflight.sh 2>&1 | tee "$RUN_DIR/preflight.log"
if [[ ${PIPESTATUS[0]} -ne 0 ]]; then
  c '1;31' "Preflight failed. Fix it before running scenarios — otherwise every result is suspect."
  exit 1
fi

# 06 and 07 were missing from this list, so the two scenarios that cover the
# W-9 threshold crossing and the merchant faucet bar — both of which decide
# whether somebody gets paid — were never part of a full run.
for s in 01-w9-round-trip.sh 02-events.sh 03-qr-payout.sh 04-merchant-onboarding.sh \
         05-workflow.sh 06-w9-threshold-crossing.sh 07-merchant-faucet-bar.sh; do
  [[ -x "./$s" ]] || continue
  printf '\n'
  c '1;35' "════════ $s ════════"
  ./"$s" 2>&1 | tee -a "$REPORT"
  [[ ${PIPESTATUS[0]} -ne 0 ]] && overall=1
done

printf '\n'
c '1;37' "Full report: $REPORT"
c '1;37' "Record anything notable in testing/artifacts/TESTING-LOG.md"
exit $overall
