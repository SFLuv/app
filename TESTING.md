# Testing (W9 + Anvil Fork)

This document describes the local anvil-based test flow for W9 compliance and faucet QR redemptions.

## Prereqs
- Foundry installed (`anvil`, `cast`)
- Postgres running (local `app`, `bot`, `ponder` databases)

## Test Environment Files
- `/Users/sanchezoleary/Projects/SFLUV_Dev/app/backend/.env`
- `/Users/sanchezoleary/Projects/SFLUV_Dev/app/ponder/.env`
- `/Users/sanchezoleary/Projects/SFLUV_Dev/app/scripts/anvil.env`

Test scripts use plain `.env` files. `scripts/anvil.env` provides fork-only values used to override RPC/start block for anvil sessions.

## Start Anvil + Services (test mode)
```bash
/Users/sanchezoleary/Projects/SFLUV_Dev/app/scripts/start_anvil_test.sh
```

Starts:
- anvil fork (Berachain)
- backend (using `ENV_FILE=/Users/sanchezoleary/Projects/SFLUV_Dev/app/backend/.env`)
- ponder (using `/Users/sanchezoleary/Projects/SFLUV_Dev/app/ponder/.env` with anvil overrides from `scripts/anvil.env`)
- frontend

Logs:
- `/tmp/anvil.log`
- `/tmp/sfluv_backend.log`
- `/tmp/sfluv_ponder.log`
- `/tmp/sfluv_frontend.log`

## Create On-Chain Test Transfer + QR Code
```bash
/Users/sanchezoleary/Projects/SFLUV_Dev/app/scripts/w9_anvil_qr_test.sh
```

This script:
- Sends 200 SFLUV from the faucet address on the anvil fork to a new random wallet.
- Waits for `ponder.transfer_event` to index the transfer.
- Waits for `app.w9_wallet_earnings` to update.
- Creates a new event + code and prints a redeem URL.

Open the printed URL to confirm the **W9 Required** UI state (button + message).

## Submit W9 (mock)
```bash
/Users/sanchezoleary/Projects/SFLUV_Dev/app/scripts/w9_submit_latest.sh
```

This submits a W9 for the latest faucet transfer recipient (pulled from `ponder.transfer_event`).
Use this to test the **W9 Pending** UI state.

## Submit W9 Directly
Use the submit endpoint to create W9 submissions from either the app or the Wordpress form.

Endpoint:
```
POST /w9/submit
```

Accepted payloads:
- JSON: `{ "wallet_address": "...", "email": "...", "year": 2026 }`

## Approve + Verify Unblocked
1. Approve the pending submission in Admin → W9 tab.
2. Verify unblocked:

```bash
/Users/sanchezoleary/Projects/SFLUV_Dev/app/scripts/w9_verify_unblocked.sh
```

If successful, it prints `"Unblocked: OK"`.
