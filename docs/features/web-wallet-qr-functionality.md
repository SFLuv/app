# Web Wallet QR Functionality

## Feature Goal
Enable users in the web wallet view to scan QR codes and route to the correct flow:
- Payment QR -> prefill/open send flow
- Redemption QR -> execute redemption flow

## Scope
In scope:
- QR scanning entry point in wallet UX
- QR payload classification and normalization
- Routing to send/redeem behavior based on payload type
- Error handling for unsupported or invalid QR payloads

Out of scope for initial slice:
- Redesigning admin/event QR generation UX
- Non-wallet pages unrelated to send/redeem behavior

## Current Baseline (Codebase Snapshot: March 5, 2026)

### Wallet UX
- `frontend/app/wallets/page.tsx`
  - Lists connected wallets and routes to wallet detail pages.
- `frontend/app/wallets/[address]/page.tsx`
  - Quick actions are `Send`, `Receive`, and optionally `Unwrap SFLUV`.
  - No QR scan action currently.
- `frontend/components/wallets/receive-crypto-modal.tsx`
  - Generates QR codes for:
    - CitizenWallet receive link (`generateReceiveLink`)
    - External wallet address
  - Supports optional `tipTo` param in CitizenWallet link.
- `frontend/components/wallets/send-crypto-modal.tsx`
  - Manual recipient entry only (`ContactOrAddressInput`).
  - No scanner integration and no prefill-from-QR API.

### Redeem Flow
- `frontend/components/events/qr-code-card.tsx`
- `frontend/components/events/affiliate-qr-code-card.tsx`
  - Event QR uses `buildEventRedeemQrValue(code)` from `frontend/lib/redeem-link.ts`.
- `frontend/middleware.ts`
  - Detects `page=redeem` and redirects to `/faucet/redeem`.
  - Normalizes the `code` query value.
- `frontend/app/faucet/redeem/page.tsx`
  - Reads `code` and signature params (`sigAuthAccount`, etc.), then POSTs to backend `/redeem`.
- `backend/handlers/bot.go` (`Redeem`)
  - Normalizes code server-side.
  - Validates code state (expired/redeemed/user already redeemed) via `backend/db/faucet_bot.go`.
  - Applies W9 compliance checks and sends token transfer.

### Relevant Wallet/Role Context
- `backend/db/app_wallet.go`
  - Wallet persistence + flags (`is_redeemer`, `is_minter`, `last_unwrap_at`).
- `backend/handlers/redeemer.go`
  - Auto-grants redeemer role for eligible merchant/admin smart wallets.

## Gaps To Close
- No dedicated QR scanner component in wallet flow.
- No centralized QR parser/router for wallet scan payloads.
- Send flow cannot be programmatically seeded from scanned data.
- Redeem behavior currently expects parameters commonly present in CitizenWallet webview signatures.

## Target Behavior
1. User taps `Scan QR` from wallet detail view.
2. App opens camera scanner in web wallet UI.
3. Parsed QR payload is classified as one of:
   - `payment_request`
   - `redeem_code`
   - `unsupported`
4. App routes accordingly:
   - `payment_request` -> open send modal with recipient/amount/memo prefilled where available
   - `redeem_code` -> route into redeem path with normalized code
   - `unsupported` -> show clear error/toast

## QR Type Matrix (Initial)
- Redeem code:
  - Raw UUID
  - UUID with suffix artifacts (example: trailing `26`)
  - URLs containing `code=...` and/or `page=redeem`
  - CitizenWallet plugin deep links containing redeem target URL
- Payment request:
  - CitizenWallet receive links (`generateReceiveLink` format)
  - Plain `0x...` address payloads
  - `sfluv:` URI payloads (with optional `amount`/`message`)

## Implementation Plan
- [x] Discovery and baseline mapping complete
- [ ] Define QR payload parser contract (`type`, normalized fields, errors)
- [ ] Implement shared QR parser utility with unit tests
- [ ] Add scan entry point/button in `frontend/app/wallets/[address]/page.tsx`
- [ ] Implement scanner modal/component and camera permission handling
- [ ] Add send modal prefill support (recipient/amount/memo)
- [ ] Wire payment QR -> send prefill flow
- [ ] Wire redeem QR -> redeem route flow with normalized code
- [ ] Add unsupported/invalid QR UX (toast + retry path)
- [ ] Add end-to-end/manual test checklist for desktop/mobile browsers and webview

## Open Questions
1. Should redeem scans be supported only in CitizenWallet webview, or also in standard browser sessions?
2. For payment QR payloads with token/chain mismatch, do we reject or show a conversion prompt?
3. Should scan live in wallet detail only, or also in wallets list and send modal?
4. Do we need analytics events for `scan_started`, `scan_success`, `scan_type`, and `scan_error`?

## Test Strategy (Initial)
- Unit:
  - QR parsing and normalization across known payload variants
  - Redeem code normalization parity with backend behavior
- Integration:
  - Payment scan opens send modal with correct prefill
  - Redeem scan routes to redeem page with normalized query
- Manual:
  - iOS Safari, Android Chrome, and CitizenWallet webview camera flow
  - Permission denied, invalid QR, malformed QR, and repeated scans

## Change Log
- 2026-03-05: Created feature tracker with current codebase context and implementation checklist.
