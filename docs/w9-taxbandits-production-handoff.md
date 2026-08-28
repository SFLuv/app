# W-9 / TaxBandits: production go-live handoff

What the production backend needs to run the W-9 system against the real
TaxBandits API, where each value comes from, and the order that works. The
sandbox integration has been verified end to end (form minted → signed →
webhook received and HMAC-verified → escrow released on chain); production is
the same wiring with a different credential set and no tunnel.

**No secret values live in this file or anywhere in git.** Values are handed
over out-of-band (1Password). This file is the map, not the treasure.

## The environment variables

| Variable | Value / where it comes from |
|---|---|
| `W9_PROVIDER` | `taxbandits` |
| `W9_PROVIDER_ENV` | `production` — selects the live host pair. Do **not** set `W9_PROVIDER_BASE_URL`; the adapter derives it, and overriding it is how the sandbox once ended up calling our own router. |
| `W9_PROVIDER_CLIENT_ID` | TaxBandits **production** console → Settings → API Credentials |
| `W9_PROVIDER_CLIENT_SECRET` | Same page. Also the webhook HMAC key — the callback receiver verifies `base64(HMAC-SHA256(ClientId + "\n" + Timestamp, ClientSecret))`, so a wrong secret means every webhook is dropped with a 401. |
| `W9_PROVIDER_USER_TOKEN` | Same page |
| `W9_PROVIDER_BUSINESS_ID` | The payer business GUID in the **production** console (create the SFLuv business there if it does not exist). A missing value is a hard error on the first form request, not a silent disable. |
| `W9_PROVIDER_WEBHOOK_REF` | Minted when the webhook is registered in the production console — **step 3 below, after deploy**. Routes callbacks to exactly one registered URL; without it every registered URL receives every event, including sandbox tunnels. |
| `W9_PROVIDER_API_VERSION` | Leave unset (defaults to `v1.7.3`) |
| `W9_RETURN_URL` | Leave unset **if** `PUBLIC_BACKEND_URL` is correct — it defaults to `${PUBLIC_BACKEND_URL}/w9/complete`, a page the backend serves itself that hands people back to the app. |
| `PUBLIC_BACKEND_URL` | The backend's public https origin (e.g. `https://api.sfluv.org`). The return page, photo URLs and callback registration all build from it. |
| `W9_ENFORCEMENT` | `shadow` computes and logs every decision but still pays everything; `enforce` holds and refuses at the tiers. **Launching in `shadow` for a week and reading the logs is the cautious path; flipping to `enforce` is one env change + restart.** Team's call. |
| `W9_TIER_NOTICE_SFLUV` / `W9_TIER_WARNING_SFLUV` / `W9_THRESHOLD_SFLUV` | Leave unset for the 400 / 500 / 600 defaults |
| `W9_ESCROW_WINDOW_DAYS` | Leave unset (7). Urgency copy only — escrow holds until the filing clears, it does not expire. |

## The order that works

1. **Get production API access.** TaxBandits gates live keys behind a Go-Live
   review (business details, sandbox usage). Request it from the console;
   sandbox credentials are rejected by the production hosts and vice versa.
2. **Deploy the backend first.** The webhook receiver
   (`POST /w9/webhook/taxbandits`) and the return page (`GET /w9/complete`)
   must be live on public https **before** step 3, because the console fires a
   sample POST at the URL and refuses to save the webhook unless it answers
   200.
3. **Register the webhook** in the production console (Form W-9 status
   change → the deployed URL). Copy the WebhookRef it mints into the vault,
   set `W9_PROVIDER_WEBHOOK_REF`, restart.
4. **Verify** with one real round trip: mint a form from the app, sign it,
   and watch the backend log the verified callback and the filing flip to
   `completed`. The sweeper covers a missed callback at a slow cadence, so a
   quiet webhook shows up as filings clearing minutes late rather than never.

## Notes

- The receiver answers 200 within its deadline and does the work after —
  TaxBandits retries 9× over 24h on non-200s and expects an answer within ~5s.
- IP allowlisting from their go-live checklist is deliberately not
  implemented; the HMAC check is the authentication. If it is ever added, it
  must read `X-Forwarded-For` behind the proxy or it checks the proxy's own
  address.
- Rollback is env-only: `W9_ENFORCEMENT=shadow` keeps every payout flowing
  while leaving the decision log intact; `W9_PROVIDER=fake` exists for dev and
  must never be set in production.
