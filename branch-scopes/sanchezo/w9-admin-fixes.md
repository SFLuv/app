# Branch scope — `sanchezo/w9-admin-fixes`

Nov 1 2026 · app (backend) · **0.4h active**

Hours are wall-clock time measured from the session transcript with the
`time-accounting` skill (`measure_sittings.py`, 30-minute sitting gap). The
sitting ran 07:15–07:54 MST (0.65h, measured at 07:54); this branch is the part
from 07:31, when the admin W-9 card was reported wrong, to the commit. Itemised
to the nearest 0.1h from the per-message `UserPromptSubmit` stamps; the items
sum to the measured figure. The 07:15–07:31 stretch (checking who had filed and
whether the webhook worked) produced no code and is not counted here.

Weakness: most of the 0.4h is diagnosis, done against production data that
Sanchez ran by hand in the Railway console; the code change itself is small.

---

# Round 1 — Nov 1

## Large features

### Admin W-9 card showed nothing for anyone — 0.2h · app
- **Cause:** the card URL-encodes the Privy DID (`did%3Aprivy%3A…`), and chi
  routes on the raw path, so `chi.URLParam` returned the encoded id. No user has
  it: status read "Not started / Earned 0" for everyone and "Send W-9 request"
  answered 404. Reproduced against chi v5.2.4.
- **Fix:** `chiParam` (handlers/w9.go) now decodes, as `userIdParam`
  (handlers/path_params.go) already does for the chat routes. Covers every
  handler reading params through it: W-9 status, request, resend, clear, and the
  merchant-mode PIN reset (same bug).

### Admin MCP W-9 report read a frozen table — 0.2h · app
- `w9_report` / `lookup_wallet` summed `w9_wallet_earnings`, which stopped
  updating when the payout ledger took over (~Sep 8 2026). Every total after
  that was low (one account: 595 reported, 685 real).
- Now reads `payout_ledger`, the table the gate counts, and keeps old-table rows
  only where the history import did not carry them (earlier years; 2026 wallets
  with no account), matched by the import's idempotency key so nothing counts
  twice. SFLuv's own addresses are excluded, as the import and gate do.
- `w9_required` is judged per person per year against the gate's own threshold
  (`handlers.W9ThresholdBase`, newly exported), not per wallet.
- `approved_at` is set only for filings that clear. A form completed and then
  rejected by TIN matching kept its `completed_at` and was reported approved;
  it now shows under `rejected_at`.
- Verified: the exact query run against a local DB copy in a rolled-back
  transaction; report total = ledger total + un-imported legacy total exactly,
  zero duplicate wallet/chain/year rows. `go build`, `go vet` clean.

## Smaller fixes

| Fix | Hours | Repo |
|---|---|---|
| — | | |

## Totals

| Area | Hours |
|---|---|
| Card encoding bug | 0.2 |
| Report repointed to the ledger | 0.2 |
| **Total (measured)** | **0.4** |

## Not in this branch

- `GetAffiliate` reads `r.PathValue("user_id")` undecoded, and the web event
  modal encodes the owner DID, so that lookup likely fails the same way.
- `app_organization.go` reads a `user_id` param undecoded; the web does not
  encode it today, so it works, but it is the same latent bug.
- `w9_required_at` for rows from the old table is approximate (the old table is
  cumulative per wallet, so the crossing time is that wallet's last pre-ledger
  payout). Ledger rows are exact.
