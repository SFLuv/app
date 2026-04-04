# Pay Merchant And Tipping Improvements Checklist

Schema migration rule:
- `backend/db/app.go` remains the baseline `CreateTables()` snapshot.
- Any schema change after baseline must be added as a new ordered migration in `backend/bootstrap/schema_migrations.go`.
- Do not tuck new production schema changes directly into `CreateTables()` unless the whole baseline is intentionally being redefined.

Branch:
- `codex/pay-merchant-and-tipping-improvements`

Scope:
- improve merchant wallet selection in the web app
- add first-class merchant tipping wallet support in backend + web frontend
- prepare the shared merchant payload so pay flows can distinguish merchant payment destination vs tip destination
- keep mobile app changes out of this branch until backend/web changes settle

Non-goals:
- no QR payload changes
- no universal-link changes
- no mobile app changes in this branch
- no broader merchant branding/logo work

## Product Rules

- Any user can still have a `primary wallet`.
- Merchant payment wallets are location-owned, not user-owned.
- A merchant location can have multiple payment wallets.
- Each merchant location has at most one default payment wallet.
- A merchant tipping wallet is optional and location-owned.
- Only merchants can configure payment wallets and tipping wallets in the UX.
- The wallet panel should not be used to configure merchant payment or tipping wallets.
- A location tipping wallet must not match any configured payment wallet for that location.
- Tip money must stay distinguishable from base payment money.
  - Do not collapse payment + tip into one destination.
  - If the merchant has a tip wallet configured, payment and tip should remain separate transfers.

## Backend Plan

### 1. Store tipping wallet on locations

- Add `tipping_wallet_address` to `locations`.
- Keep it optional.
- Do not default it to the owner's primary wallet.

Files:
- `backend/structs/app_location.go`
- `backend/db/app_location.go`
- `backend/bootstrap/schema_migrations.go`

### 2. Add linked location payment wallets table

- Create `location_payment_wallets` to support multiple payment wallets per merchant location.
- Fields:
  - `id`
  - `location_id`
  - `wallet_address`
  - `is_default`
- Enforce one default payment wallet per location.
- Keep wallet addresses unique per location.

Files:
- `backend/db/app_location_wallet.go`
- `backend/bootstrap/schema_migrations.go`

### 3. Keep baseline schema clean and use migrations

- Keep `backend/db/app.go` as the baseline schema snapshot.
- Add location tipping/payment-wallet storage in a new ordered migration, not inline in `CreateTables()`.
- Current branch uses migration `1.2` for this.

Files:
- `backend/db/app.go`
- `backend/bootstrap/schema_migrations.go`

### 4. Add merchant-only location wallet settings endpoint

- Add authenticated route:
  - `PUT /locations/{id}/wallet-settings`
- Request body should support:
  - replacing the location payment-wallet list
  - setting the default payment wallet
  - setting or clearing the location tipping wallet
- Validation rules:
  - caller must own the location or be an admin
  - all payment wallets must be valid owned wallets
  - tipping wallet must be a valid owned wallet
  - tipping wallet must not match any payment wallet for the location
  - only merchants should reach this UX

Files:
- `backend/router/router.go`
- `backend/handlers/app_location.go`
- `backend/db/app_location_wallet.go`
- `backend/structs/app_location.go`

### 5. Expose payment/tip destinations in merchant/public location payloads

- Add `tip_to_address` to public location responses.
- `pay_to_address` should resolve from the location default payment wallet.
- If no location payment wallet exists, fall back to owner primary wallet and then legacy smart wallet `0`.
- `tip_to_address` should come only from `locations.tipping_wallet_address`.
- Do not silently fall back `tip_to_address` to `pay_to_address`, because that defeats accounting separation.

Files:
- `backend/structs/app_location.go`
- `backend/db/app_location.go`
- `frontend/types/location.ts`
- `frontend/context/LocationProvider.tsx` if field mapping is needed

### 6. Tighten wallet lookup metadata

- Review whether `/wallets/lookup/:address` should expose whether an address is:
  - a merchant location payment wallet
  - a merchant tipping wallet
  - tied to which merchant location
- This is useful for post-payment tipping prompts.

Files:
- `backend/db/app_wallet.go`
- `backend/structs/app_wallet.go`
- `backend/handlers/app_wallet.go`
- `frontend/components/wallets/send-crypto-modal.tsx`

## Web Frontend Plan

### 7. Keep primary-wallet controls in settings, not wallet cards

- Do not let users set primary or tipping wallets from the wallet panel.
- Merchant payment/tipping wallet controls should live in settings under merchant location management.
- Keep general primary-wallet selection in settings for all users.

Files:
- `frontend/app/settings/page.tsx`
- `frontend/app/wallets/page.tsx`

### 8. Add merchant location wallet settings UI

- For each merchant location in settings, allow:
  - viewing configured payment wallets
  - adding/removing payment wallets
  - selecting the default payment wallet
  - setting or clearing the tipping wallet
- Only show this surface to merchants.
- Prevent choosing a tipping wallet that matches any location payment wallet.

Files:
- `frontend/app/settings/page.tsx`
- `frontend/types/location.ts`

### 9. Keep user primary-wallet state unchanged

- Keep the existing user-level primary-wallet flow intact.
- Do not add user-level tipping-wallet state in frontend user context.

## Merchant Pay Flow Plan

### 10. Add merchant tip support to web pay/send flow

- When the recipient address matches a merchant location payment wallet and `tip_to_address` exists:
  - show a post-payment tip prompt
  - keep item total vs tip total separate
- Execution model:
  - base payment transfer -> matched/default location payment wallet
  - tip transfer -> location tipping wallet
- Treat this as two separate transfers, not one combined transfer.
- If `tip_to_address` is missing:
  - do not show tip UI

Files:
- `frontend/components/wallets/send-crypto-modal.tsx`
- any merchant pay entrypoints that prefill this modal

### 11. Merchant map/details consumption

- If merchant map/detail UI uses public location payloads, wire `tip_to_address` through those types now.
- No extra tip UX is required on the details screen immediately if the tip prompt is post-payment.

Files:
- `frontend/context/LocationProvider.tsx`
- `frontend/types/location.ts`
- `frontend/components/locations/location-modal.tsx` only if tip info is displayed

## Validation Rules To Enforce

- Primary wallet:
  - valid owned wallet
  - available to all users

- Location payment wallets:
  - valid owned wallets
  - merchant-only UX
  - multiple allowed
  - at most one default per location

- Location tipping wallet:
  - valid owned wallet
  - merchant-only UX
  - cannot equal any configured payment wallet for that location
  - can be unset

- Merchant pay payload:
  - `pay_to_address` may fall back to owner primary wallet, then legacy smart wallet `0`
  - `tip_to_address` must not fall back to `pay_to_address`

## Testing Checklist

### Backend

- Merchant can save multiple payment wallets for a location.
- Merchant can choose one default payment wallet per location.
- Merchant can set a location tipping wallet.
- Merchant cannot set a location tipping wallet equal to any location payment wallet.
- Non-merchant cannot access merchant location wallet settings.
- Public `/locations` payload returns:
  - `pay_to_address`
  - `tip_to_address` only when configured

### Web frontend

- Regular users only see primary-wallet settings.
- Merchants see location payment/tipping wallet settings in settings.
- Wallet panel does not expose merchant payment/tipping wallet actions.
- Send flow shows the tip follow-up only after a payment to a merchant payment wallet.

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
