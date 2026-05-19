# Merchant Mode Local Testing

This runbook is for testing merchant mode against a disposable local backend, a dev Privy app, and a development build on a physical iPhone.

## What This Proves

- The same merchant account can own approved locations.
- Merchant mode can be enabled per device and per merchant location.
- Merchant mode survives app restarts on that device.
- Exiting merchant mode requires the account-scoped 6 digit PIN.
- The map can still show realistic nearby merchant locations without using production data.

## Required Local Shape

- Backend repo: `/Users/sanchezoleary/Projects/SFLUV_MAIN`
- Mobile repo: `/Users/sanchezoleary/Projects/mobile-app-sanchezo/mobile`
- Local backend port: `8080`
- Test user email: `sanchez@oleary.com`
- Privy environment: the local dev Privy app ID is `cmlidct3t00pql50du730rr3o`. The backend, web app, and mobile app must all use this same app ID for local merchant-mode testing.

Your iPhone cannot call `localhost` on the Mac. Use the Mac LAN IP, for example:

```sh
EXPO_PUBLIC_APP_BACKEND_URL=http://192.168.1.166:8080
```

If iOS rejects local HTTP, use an HTTPS tunnel URL instead.

## One-Time Prep

1. Start the local backend once so schema migrations run.
2. Sign into the app against the local backend once as `sanchez@oleary.com`.
3. Confirm the local `users` table has either `contact_email = 'sanchez@oleary.com'` or a verified email row for that account.
4. Run the seed script.

The seed script is guarded. It refuses to run unless you explicitly confirm, and it refuses non-local DB targets unless overridden.

```sh
cd /Users/sanchezoleary/Projects/SFLUV_MAIN
CONFIRM_LOCAL_MERCHANT_MODE_SEED=YES \
TEST_USER_EMAIL=sanchez@oleary.com \
./scripts/seed_merchant_mode_local.sh
```

If the local user row does not have the email attached yet, pass the Privy DID directly:

```sh
CONFIRM_LOCAL_MERCHANT_MODE_SEED=YES \
TEST_USER_ID=did:privy:YOUR_LOCAL_USER_ID \
TEST_USER_EMAIL=sanchez@oleary.com \
./scripts/seed_merchant_mode_local.sh
```

## What The Seed Adds

- Promotes the test user to merchant.
- Accepts the current privacy policy locally so policy gating does not block testing.
- Uses the user's existing primary/smart wallet if one exists.
- Creates a fallback local-only wallet if no wallet exists yet.
- Adds two approved merchant locations owned by the test user:
  - `SFLuv Corner Market`
  - `SFLuv Community Cafe`
- Adds two fake approved public locations for map realism:
  - `Civic Center Books & Cafe`
  - `Little Saigon Hardware`
- Adds realistic location hours and default payment wallets.

The script only removes/replaces rows with `google_id` values prefixed by `local-merchant-mode-`.

## Mobile Dev Loop

In the mobile repo, make sure `.env` points to the local backend URL that the iPhone can reach:

```sh
cd /Users/sanchezoleary/Projects/mobile-app-sanchezo/mobile
npm run start:dev-client -- --lan
```

If LAN discovery is flaky:

```sh
npm run start:dev-client -- --tunnel
```

Use a development build, not a normal preview build, while testing against local backend changes. Preview and production builds bake their backend URL at build time.

## Test Cases

1. Sign in as `sanchez@oleary.com`.
2. Open Settings > Merchant.
3. Set a 6 digit merchant mode PIN.
4. Enable merchant mode for `SFLuv Corner Market`.
5. Restart the app and confirm merchant mode is still active on the iPhone.
6. Confirm the bottom tabs/actions are restricted to wallet/receive/history behavior.
7. Try the wrong PIN when exiting merchant mode and confirm it fails.
8. Exit with the correct PIN.
9. Enable merchant mode for `SFLuv Community Cafe` and confirm the location-specific device state updates.

## Known Local Limits

- The Mac web app is useful for setup and sanity checks, but it is not a second native device unless we add equivalent web-side merchant-mode device registration.
- For two native device states, use iPhone plus iOS Simulator or a second physical phone.
- Real incoming transaction notifications require the Ponder/push path to be running too. Merchant mode itself does not require push notifications.
