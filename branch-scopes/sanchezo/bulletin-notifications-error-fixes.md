# Branch scope: `sanchezo/bulletin-notifications-error-fixes` (backend)

Oct 26 2026 · app (backend) + mobile-app · **~0.2h active** · backend based on `main` at `3a681e7`

Hours are wall-clock time from this session's timestamps (about 16:01–16:10 PDT), not from the `time-accounting`
script. They cover both repos. The mobile half lives on the mobile-app branch of the same name, which is built on
PJ's `pjol/internal-notifications` plus `sanchezo/server-reward-labels`.

## What this branch is (backend)

The two backend changes the mobile error fixes need. Both are additive, and older apps behave exactly as before.

1. **`admin_requested` on `/w9/status`.** It's true while an admin's W-9 request (the `admin_request` marker from
   #203) is outstanding, and false once the filing clears. Newer apps use it to word the prompt as "SFLuv has
   asked you for a W-9" instead of borrowing the threshold wording.
2. **`POST /redeem` answers `payout pending`** when a payout was submitted but not confirmed. The status is still
   500, so older apps show the same generic error as before. Newer apps show "sent, still confirming". The code
   stays spent, and the confirmation sweep (#202) settles the payout from the chain. A refused send still gives
   a plain 500 and hands the code back.

| Item | Hours | Repo |
|---|---|---|
| Both backend changes. Throwaway tests, not committed per the repo rule, against real Postgres 18 with the `cmd/init` schema. **Flag:** false before any request; true after an admin request (tier `notice_400`); false after the notices clear. **Redeem with a stub faucet:** unconfirmed → 500 `payout pending` with the code still spent; refused → plain 500 with the code handed back; normal → 200 | 0.1 | app |
| Mobile, on the mobile-app branch: the admin-request W-9 wording; the QR "pending" outcome; payment confirmation retrying failed checks instead of failing (7 polling cases pass, including the Oct 23 Niebla sequence); expo-updates with the fingerprint runtime and per-profile channels (expo-doctor: no new issues). Typecheck clean | 0.1 | mobile-app |

Volume (backend): 4 files, +28 / −1. 0 migrations, 0 routes.
