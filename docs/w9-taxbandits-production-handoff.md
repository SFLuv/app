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
| `W9_ENFORCEMENT` | `true` holds and refuses at the tiers; `false` computes and logs every decision but still pays everything. **Launching `false` for a week and reading the logs is the cautious path; flipping to `true` is one env change + restart.** (The old `enforce` / `shadow` strings still work as aliases.) Team's call. |
| `W9_TIER_NOTICE_SFLUV` / `W9_TIER_WARNING_SFLUV` / `W9_THRESHOLD_SFLUV` | Leave unset for the 400 / 500 / 600 defaults. **To keep W-9 fully dormant** (ship before the vendor is live): set both tier vars to `0` and the threshold to a very high number — see \"Launching before the vendor is live\" below. |
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

## Launching before the vendor is live

The whole system can ship before TaxBandits approval comes through, with W-9
**fully dormant** — everyone is paid, nobody sees a W-9 prompt, and you collect
W-9s manually in the meantime. Enforcement alone is not enough for this: the
tier modals in the app are driven by `/w9/status`, which does not consult
enforcement mode, so a volunteer who crosses 400 / 500 / 600 SFLUV would still
see "Fill out my W-9" — pointing at a form that is not wired yet. Disable the
tiers themselves:

    W9_ENFORCEMENT=false            # nobody withheld
    W9_TIER_NOTICE_SFLUV=0          # 0 disables this tier
    W9_TIER_WARNING_SFLUV=0         # 0 disables this tier
    W9_THRESHOLD_SFLUV=100000000    # so high nobody reaches the crossing
    # W9_PROVIDER left unset -> the provider is "disabled": every form call
    # returns ErrProviderDisabled, which is fine because no tier is ever
    # reached to surface a form button in the first place.

No W-9 UI appears anywhere, on mobile or web. When the vendor is live, flip to
the real values in one restart: real thresholds (unset them for 400/500/600),
`W9_ENFORCEMENT=true`, `W9_PROVIDER=taxbandits`, and the credential block above.

`W9_PROVIDER` is a real switch, not decoration: `taxbandits` / `track1099` /
`fake` select the adapter, and anything else (including unset) yields a disabled
provider whose every call errors. `fake` additionally mounts a local stub form —
never set it in production.

## Notes

- The receiver answers 200 within its deadline and does the work after —
  TaxBandits retries 9× over 24h on non-200s and expects an answer within ~5s.
- IP allowlisting from their go-live checklist is deliberately not
  implemented; the HMAC check is the authentication. If it is ever added, it
  must read `X-Forwarded-For` behind the proxy or it checks the proxy's own
  address.
- Rollback is env-only: `W9_ENFORCEMENT=false` keeps every payout flowing
  while leaving the decision log intact; `W9_PROVIDER=fake` exists for dev and
  must never be set in production.
- **IP whitelisting is enabled on the live account** (chosen at go-live), and
  the whitelisted address is the **backend VM's egress IP: `35.226.192.201`**
  (`sfluv-app-backend`, us-central1-c; verified with `curl ifconfig.me` from
  the box). It is deliberately NOT `104.154.240.202` — that is the separate
  `sfluv-app-load-balancer` VM that `api.sfluv.org` resolves to; inbound
  traffic enters there, but outbound API calls leave the backend as itself.
  Two consequences: leaked live credentials are useless from any other
  machine, and **if the backend VM migrates or its external IP changes, every
  API call starts returning 401 until the whitelist is updated in the
  TaxBandits console** — that outage looks like bad credentials, not a
  network change. Webhooks are unaffected (they arrive inbound through the
  load balancer and are authenticated by HMAC, not by any whitelist).
- **Before go-live, confirm `35.226.192.201` is a RESERVED STATIC address**
  (VPC network → IP addresses → Type column). GCP external IPs are ephemeral
  unless reserved, and an ephemeral one can change on a VM stop/start —
  silently breaking every API call. Promote in place if needed.
- **A migration off GCP (Railway is under consideration) must handle egress
  first.** Railway's default egress is a shared dynamic pool, which IP
  whitelisting cannot tolerate. Either enable Railway's static outbound IP
  feature and add the new IPs to the TaxBandits whitelist BEFORE cutover
  (the list holds 10, so both platforms can coexist during transition), or
  switch the account to Domain whitelisting — which requires adding their
  Reference-ID Referer header to every API call in the adapter. The webhook
  registration survives migration untouched as long as api.sfluv.org moves
  with the backend.
