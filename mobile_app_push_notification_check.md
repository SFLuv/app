# Mobile App Push Notification Sync Contract

This document describes the expected mobile app behavior for SFLuv transaction push notifications.

## Backend Contract

The backend owns active transaction notification registrations through:

```http
PUT /ponder/push
```

Authenticated request body:

```json
{
  "token": "ExponentPushToken[...]",
  "addresses": ["0x..."],
  "preference_enabled": true,
  "device_registered": true
}
```

Fields:

- `token`: The current Expo push token for this app install/device.
- `addresses`: The complete desired set of wallet addresses for the state being synced.
- `preference_enabled`: Optional. The current device's in-app preference. Send this only when the user intentionally turns transaction push notifications on/off or changes this device's desired wallet list.
- `device_registered`: Optional. The OS/device registration state. Send this on app launch/resume after checking notification permissions and getting/validating the Expo token.
- `enabled`: Legacy optional alias. Prefer `preference_enabled` and `device_registered` because they keep in-app preference and device state distinct.

The backend stores these independently:

- `preference_enabled`: Whether this device should receive transaction push notifications.
- `device_registered`: Whether the current device/token is currently able to receive push notifications.
- `active`: Whether notifications should actually be sent. This is effectively `preference_enabled && device_registered`.

The backend stores push rows per `token + address`. `owner` is still recorded for authorization, but the preference is not a single owner-level setting.

## Sync Semantics

The backend treats preference updates and device registration updates differently:

- If `preference_enabled` is present, the request is an intentional in-app preference sync. The backend updates the stored preference for the token/address rows and treats `addresses` as the complete desired wallet list for that token.
- If `device_registered` is present without `preference_enabled`, the request is a device-state reconciliation. The backend updates only `device_registered` for existing rows on that token and recomputes `active`.
- Device-state reconciliation does not prune wallet addresses, does not create new wallet preferences, and does not turn `preference_enabled` back on.
- If a row has `preference_enabled: true` and later receives `device_registered: true`, the backend can restore it to `active: true` and recreate a missing Ponder hook.
- If a row has `preference_enabled: false`, receiving `device_registered: true` keeps it inactive. The user must explicitly toggle notifications back on inside the app.

The legacy `enabled` field still updates both preference and device state. Do not use it for routine app launch/resume checks.

## Device-Specific Preferences

Push preferences are stored per Expo token and wallet address, not as one global user preference. A user with multiple devices can keep transaction push notifications enabled on one device and disabled on another.

Mobile screens that display or reconcile the current device's push state should use the current Expo token:

```http
GET /ponder/push?token=<url-encoded Expo token>
```

An unfiltered `GET /ponder/push` can return records for every known device token owned by the user. The mobile app should not merge those rows into one global preference when deciding what the current device should display.

To inspect all known push rows for the authenticated user:

```http
GET /ponder/push
```

Push records include:

```json
{
  "id": 1,
  "owner": "did:privy:...",
  "token": "ExponentPushToken[...]",
  "address": "0x...",
  "type": "push",
  "active": true,
  "preference_enabled": true,
  "device_registered": true,
  "ponder_hook_id": 123
}
```

Use the top-level `token`, `address`, `active`, `preference_enabled`, and `device_registered` fields for mobile UI state. Any legacy `data` field should not be used as the source of truth for the token.

When the device is unavailable but the current device's in-app preference should be preserved, do not send `preference_enabled: false`. Send only the device state:

```json
{
  "token": "ExponentPushToken[...]",
  "addresses": [],
  "device_registered": false
}
```

When the user intentionally turns notifications off inside the app, send the preference change:

```json
{
  "token": "ExponentPushToken[...]",
  "addresses": [],
  "preference_enabled": false
}
```

When the app detects that the device is registered again, restore only the device state:

```json
{
  "token": "ExponentPushToken[...]",
  "addresses": [],
  "device_registered": true
}
```

This restores previously enabled rows but will not re-enable rows that the user turned off in the app.

To intentionally turn off one row by id, the backend also supports:

```http
DELETE /ponder/push?id=<push-row-id>
```

This marks that row `preference_enabled: false` and `active: false`; it does not mean the mobile app should treat all devices as disabled.

## Ponder Hook Lifecycle

The backend maintains Ponder hooks for transaction notification delivery:

- Enabling push for a token/address creates a Ponder hook when no hook is already known for that active push row.
- A device-state sync with `device_registered: true` can restore `active: true` only for rows whose `preference_enabled` is already true.
- If a restored active row has no valid `ponder_hook_id`, the backend creates a fresh Ponder hook and records it on that token/address row.
- Turning push off in app sets `preference_enabled: false`; OS/device unavailability sets `device_registered: false`.
- The backend unregisters a Ponder hook only when no active email notification or active push notification still depends on that address.
- When a Ponder hook is deleted, stale `ponder_hook_id` values are cleared from push rows so a later re-enable can create and store a new hook.

## Required Mobile Behavior

On app launch and app resume:

1. Check OS notification permission status.
2. If permission is granted, get the current Expo push token.
3. Fetch `GET /ponder/push?token=<url-encoded current Expo token>` to learn the backend's known `preference_enabled`, `device_registered`, and `active` state for this device.
4. If backend `device_registered` differs from the current device state, send `PUT /ponder/push` with `device_registered` only.
5. If backend `preference_enabled` is `false`, do not send `preference_enabled: true` just because the device is registered.
6. If permission is denied, revoked, or unavailable, send `PUT /ponder/push` with `device_registered: false` and an empty `addresses` array for the last known token when available.

When the user toggles transaction push notifications on:

1. Request notification permission if needed.
2. If granted, get the Expo push token.
3. Send `PUT /ponder/push` with `preference_enabled: true`, `device_registered: true`, and the full desired address list.
4. If not granted, send `preference_enabled: true` only if the user has just opted in, plus `device_registered: false`; this preserves the current device's preference while keeping delivery inactive.

When the user toggles transaction push notifications off:

1. Send `PUT /ponder/push` with the current token, `preference_enabled: false`, and `addresses: []`.
2. Do not send `preference_enabled: true` on later launch/resume unless the user explicitly turns notifications back on.

When the Expo token changes:

1. Send the new token with the current device preference and device registration state.
2. If the app still has the previous token and knows it should no longer receive pushes, send a `device_registered: false` sync for that previous token.
3. The backend will clear active registrations for the same token if it is reassigned to another owner.

When the wallet/address notification list changes:

1. Re-send the full desired address list with `preference_enabled: true`, not just the changed address.
2. The backend treats missing addresses as preference-disabled for that token.

When OS notification settings change outside the app:

1. On next launch/resume, compare the local OS permission state with backend `device_registered`.
2. Send `device_registered: false` if OS notifications are denied.
3. Send `device_registered: true` if OS notifications are granted and the token is valid.
4. Do not change `preference_enabled` during this device-state reconciliation.

When reconciling after a mismatch:

1. Fetch current rows with `GET /ponder/push?token=<url-encoded current Expo token>`.
2. Use `device_registered` only to update a discrepancy caused by OS permissions, token validity, receipt feedback, reinstall, or app resume.
3. Preserve the backend `preference_enabled` value locally unless the user changes it through an in-app control.
4. Update the local UI from `active`, not from OS permission alone.

## Deleted App Or Dead Token Detection

The mobile app cannot notify the backend after it has been deleted. The backend detects this indirectly:

1. Backend sends push through Expo.
2. Expo returns a push ticket.
3. Backend stores the ticket and checks the Expo receipt shortly afterward.
4. If the receipt returns `DeviceNotRegistered`, the backend marks the token's push rows `device_registered: false` and `active: false` while preserving `preference_enabled`.
5. The backend then removes any Ponder hook only if no active email or push notification still depends on that address.

Because the preference is preserved, a later app launch with valid OS permission and a valid token should reconcile with `device_registered: true`, not `preference_enabled: true`, unless the user explicitly toggles the in-app setting.

## Backend Lookup Notes

Device-specific fetches use the authenticated owner plus Expo token:

```http
GET /ponder/push?token=<url-encoded Expo token>
```

The database has an `owner, token` index for this lookup and a unique `token, address` index for the stored preference rows. This is the behavior the mobile client should lean on for current-device settings.

## Important Notes

- iOS/Android OS notification settings changes are not reliably visible to the backend until the app runs again.
- The mobile app should therefore check permissions whenever it becomes active.
- Do not assume a previously stored token still means notifications are allowed.
- Do not use `enabled: true` for routine launch/resume sync. That legacy field can change both preference and device state.
- Do not send partial address updates. Always send the complete desired set for the current token.
- Backend receipt checking currently defaults to a 30 second delay, configurable by `EXPO_PUSH_RECEIPT_DELAY_SECONDS`.
