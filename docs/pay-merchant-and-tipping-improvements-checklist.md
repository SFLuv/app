# Pay Merchant And Tipping Improvements Checklist

Branch:
- `codex/pay-merchant-and-tipping-improvements`

Scope:
- improve merchant wallet selection in the web app
- add first-class merchant tipping wallet support in backend + web frontend
- prepare the shared merchant payload so pay flows can distinguish merchant payment destination vs tip destination

Non-goals:
- no QR payload changes
- no universal-link changes
- no mobile app changes in this branch
- no broader merchant branding/logo work

## Product Rules

- Any user can set a `primary wallet`.
- Only merchant users can set a `tipping wallet`.
- A merchant tipping wallet is optional.
- A merchant tipping wallet must be different from the merchant primary wallet.
- Tip money must stay distinguishable from base payment money.
  - Do not collapse payment + tip into one destination.
  - If the merchant has a tip wallet configured, the future pay flow should treat payment and tip as separate transfers.

## Backend Plan

### 1. Add tipping wallet storage on users

- Add nullable `tipping_wallet_address` to `users`.
- Do not auto-backfill this from `primary_wallet_address`.
- Do not default it to the primary wallet.

Files:
- `backend/db/app_user.go`
- `backend/structs/app_user.go`
- database migration files if this repo uses explicit migrations outside these folders

### 2. Extend user read/write models

- Include `tipping_wallet_address` in:
  - user struct serialization
  - authed user response
  - frontend server types
- Keep naming parallel to `primary_wallet_address`.

Files:
- `backend/structs/app_user.go`
- `frontend/types/server.ts`
- `frontend/context/AppProvider.tsx`

### 3. Add tipping-wallet update endpoint

- Add authenticated route:
  - `PUT /users/tipping-wallet`
- Request body:
  - `{ "tipping_wallet_address": "0x..." }`
- Validation rules:
  - caller must be a merchant or admin acting as merchant if that pattern already exists
  - wallet must be a valid Ethereum address
  - wallet must belong to the authenticated user
  - wallet must not equal `primary_wallet_address`
- Decide whether empty string clears the tipping wallet.
  - recommended: allow clearing

Files:
- `backend/router/router.go`
- `backend/handlers/app_user.go`
- `backend/db/app_user.go`
- `backend/structs/app_user.go`

### 4. Expose tip destination in merchant/public location payloads

- Add `tip_to_address` to public location responses.
- Keep `pay_to_address` logic as-is:
  - `primary_wallet_address`
  - fallback smart wallet index `0`
- `tip_to_address` should come only from `users.tipping_wallet_address`.
- Do not silently fall back `tip_to_address` to `pay_to_address`, because that defeats accounting separation.

Files:
- `backend/structs/app_location.go`
- `backend/db/app_location.go`
- `frontend/types/location.ts`
- `frontend/context/LocationProvider.tsx` if field mapping is needed

### 5. Tighten wallet lookup metadata if needed

- Review whether `/wallets/lookup/:address` should expose whether an address is:
  - merchant primary wallet
  - merchant tipping wallet
- This is optional for v1, but useful if pay/send UI needs to label the destination role.

Files:
- `backend/db/app_wallet.go`
- `backend/structs/app_wallet.go`
- `backend/handlers/app_wallet.go`
- `frontend/components/wallets/send-crypto-modal.tsx`

## Web Frontend Plan

### 6. Add easy wallet-role actions on wallet cards

- On the main wallets page, add a per-wallet dropdown or action menu:
  - `Set as primary wallet`
  - `Set as tipping wallet`
- Show `Set as tipping wallet` only for merchant users.
- Hide or disable `Set as tipping wallet` if:
  - wallet is already the current primary wallet
  - wallet address is invalid/missing

Files:
- `frontend/app/wallets/page.tsx`

### 7. Show wallet-role badges

- On wallet cards, show badges:
  - `Primary Wallet`
  - `Tipping Wallet`
- A wallet should never show both at once.

Files:
- `frontend/app/wallets/page.tsx`

### 8. Keep settings page in sync

- Keep the existing primary wallet selector in settings.
- Add a merchant-only tipping wallet selector to settings as a secondary management surface.
- Apply the same validation:
  - tipping wallet cannot equal primary wallet
- If primary wallet changes to the currently selected tipping wallet:
  - block save, or
  - clear tip selection and require merchant to choose another tip wallet
- recommended: block save with explicit error until the conflict is resolved

Files:
- `frontend/app/settings/page.tsx`

### 9. Update frontend user state and helpers

- Store `tippingWalletAddress` in `useApp()` user state.
- Ensure `updateUser(...)` can refresh it cleanly after save actions.
- Keep the existing default-primary-wallet initialization logic unchanged.
- Do not add default tipping-wallet initialization.

Files:
- `frontend/context/AppProvider.tsx`
- `frontend/types/server.ts`

## Merchant Pay Flow Plan

### 10. Add merchant tip support to web pay/send flow

- When the recipient is a merchant and `tip_to_address` exists:
  - show a tip input
  - show item total vs tip total separately
- Execution model:
  - base payment transfer -> `pay_to_address`
  - tip transfer -> `tip_to_address`
- Treat this as two separate transfers, not one combined transfer.
- If `tip_to_address` is missing:
  - either hide tip UI
  - or show tip UI disabled with explanation
- recommended for first pass: hide tip UI unless merchant tip wallet exists

Files:
- `frontend/components/wallets/send-crypto-modal.tsx`
- any merchant pay entrypoints that prefill this modal

### 11. Merchant map/details consumption

- If merchant map/detail UI uses public location payloads, wire `tip_to_address` through those types now.
- No UX change required immediately if tip input is only added in send flow.

Files:
- `frontend/context/LocationProvider.tsx`
- `frontend/types/location.ts`
- `frontend/components/locations/location-modal.tsx` only if tip info is displayed

## Validation Rules To Enforce

- Primary wallet:
  - valid owned wallet
  - available to all users

- Tipping wallet:
  - valid owned wallet
  - merchant-only
  - cannot equal primary wallet
  - can be unset

- Merchant pay payload:
  - `pay_to_address` may fall back to smart wallet `0`
  - `tip_to_address` must not fall back to `pay_to_address`

## Testing Checklist

### Backend

- Merchant can set primary wallet.
- Merchant can set tipping wallet.
- Non-merchant cannot set tipping wallet.
- Merchant cannot set tipping wallet equal to primary wallet.
- Public `/locations` payload returns:
  - `pay_to_address`
  - `tip_to_address` only when configured

### Web frontend

- Regular users see primary-wallet controls only.
- Merchants see both primary-wallet and tipping-wallet controls.
- Wallet cards correctly badge primary vs tipping.
- Settings page and wallets page stay in sync after updates.

### Merchant pay flow

- Merchant with no tip wallet:
  - pay flow still works
  - no confusing tip UI
- Merchant with tip wallet:
  - pay amount and tip amount remain separate
  - transfers target different addresses

## Recommended Order

1. backend schema + structs + route
2. public location payload `tip_to_address`
3. frontend server/user/location types
4. wallets page role actions + badges
5. settings page merchant-only tipping selector
6. merchant pay/send flow tip UI
7. tests + QA

## Keep Out Of This Branch

- universal links
- faucet QR changes
- legacy Citizen Wallet compatibility work
- mobile send/pay UX changes
- merchant logo upload
