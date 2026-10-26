# Branch scope — `sanchezo/merchant-mode-device-version`

Oct 23 2026 · app (backend) · **~0.1h active**

Hours are wall-clock time from this session's timestamps (about 11:57 AM to 12:03 PM PDT), not measured with the
`time-accounting` script.

## What this branch is

Niebla SF's iPad showed "ios · v1.0.3" under merchant mode devices after updating to 2.0.1; the admin Users
table showed 2.0.1 correctly. `merchant_mode_devices.app_version` was written only by `EnableMerchantModeDevice`.
The status check-in a till makes while in merchant mode only bumped `last_seen_at`, so a device updated in place
kept the version it was enabled on.

`GET /merchant-mode/status` now records the version from the `X-SFLUV-Client-Version` header the app already
sends on every request, via `AppDB.UpdateMerchantModeDeviceAppVersion`. It only writes when the version changed;
a failure is logged and leaves the old label rather than failing the poll.

| Item | Hours | Repo |
|---|---|---|
| Record app version on the merchant-mode status check-in | 0.1 | app |

Volume: 2 files (+ this scope file). 0 migrations, 0 routes.
