# Runbook: Clone SFLuv Production Cloud SQL & Access Locally (Option A — Auth Proxy)

**Purpose:** Stand up an isolated, point-in-time copy of the SFLuv production
Cloud SQL (Postgres) instance and reach it from your laptop on `localhost:5432`,
with no public IP and no IP allowlisting. Safe for app-logic and read-only
debugging.

**Known environment (confirmed):**

| Item | Value |
|---|---|
| PROJECT_ID | `sfluv-app` |
| Prod instance | `sfluv-app-prod` |
| Region / zone | `us-central1` / `us-central1-c` |
| Engine | Postgres 17 |
| Prod tier | `db-custom-2-8192` (2 vCPU / 8 GB) |
| Prod primary IP | `34.56.216.71` (not needed — proxy is used instead) |
| PITR enabled | **Yes** → point-in-time clone is available |
| Databases on instance | `app`, `bot` (shared instance) |

**Scope:** This runbook covers the **database** isolation surface only. Before
pointing a full parallel stack (Go backend, Ponder) at the clone, also handle the
**chain** (fork the RPC) and **third-party APIs** (Mailgun/Privy kill switches).
See the companion `.env.fork` checklist.

> ⚠️ The clone contains **real production data (PII)** and **bills continuously**
> until deleted. Treat it as production-sensitive and tear it down when done.

---

## 0. Prerequisites (one-time)

### 0.1 Install the gcloud CLI
If you don't already have it:

```bash
# macOS (Homebrew)
brew install --cask google-cloud-sdk

# verify
gcloud version
```

### 0.2 Install the Cloud SQL Auth Proxy (v2)
```bash
# macOS (Homebrew)
brew install cloud-sql-proxy

# verify
cloud-sql-proxy --version
```

(If you prefer not to use Homebrew, download the binary directly from the Cloud
SQL Auth Proxy GitHub releases page for your OS/arch, `chmod +x`, and put it on
your `PATH`.)

### 0.3 Authenticate and select the project
```bash
# log in as yourself (skip if already logged in)
gcloud auth login

# set up Application Default Credentials (the proxy uses these)
gcloud auth application-default login

# point at the SFLuv project
gcloud config set project sfluv-app
```

### 0.4 Confirm permissions
Your account (or a service account) needs at minimum:
- `roles/cloudsql.admin` (to clone) — or `cloudsql.instances.clone` specifically
- `roles/cloudsql.client` (for the proxy to connect)

```bash
# sanity check: can you see the prod instance?
gcloud sql instances describe sfluv-app-prod \
  --format="value(name,state,region)"
# expect: sfluv-app-prod   RUNNABLE   us-central1
```

### 0.5 PITR status — already confirmed enabled
Point-in-time clone requires WAL retention on the source. For `sfluv-app-prod`
this is already **True**, so you can clone to a specific past moment. To re-verify
later:

```bash
gcloud sql instances describe sfluv-app-prod \
  --format="value(settings.backupConfiguration.pointInTimeRecoveryEnabled)"
# expect: True
```

---

## 1. Record the snapshot coordinates

Before cloning, capture the moment you care about so the DB state and the chain
state line up later.

```bash
# pick the instant (UTC, RFC3339). Examples:
#   the moment a bug fired, or just "now"
SNAPSHOT_TS='2026-06-24T17:42:00.000Z'

# OPTIONAL BUT RECOMMENDED for indexer debugging:
# record the Berachain (chain 80094) block Ponder had reached at SNAPSHOT_TS so
# you can later pin the RPC fork to a matching block. (Read from Ponder's sync
# tables, or Berascan for that timestamp.)
echo "Snapshot timestamp: $SNAPSHOT_TS"
```

Keep `SNAPSHOT_TS` (and the block number) in your notes / the debugging ticket.

> Note: SFLuv prod runs on **Berachain (chain ID 80094)**; the explorer is
> Berascan (`https://berascan.com`). Don't confuse it with the SFLuv **Polygon**
> community — that's a different chain and community.

---

## 2. Clone the instance

```bash
PROD_INSTANCE="sfluv-app-prod"
CLONE_INSTANCE="debug-clone-$(date +%Y%m%d-%H%M%S)"

# Point-in-time clone (PITR is enabled on sfluv-app-prod):
gcloud sql instances clone "$PROD_INSTANCE" "$CLONE_INSTANCE" \
  --point-in-time "$SNAPSHOT_TS"

# --- OR --- clone to the current moment (simplest first run):
# gcloud sql instances clone "$PROD_INSTANCE" "$CLONE_INSTANCE"
```

This is copy-on-write: fast to create and it does **not** load the prod instance.
The clone carries **both** the `app` and `bot` databases on one instance, at a
single consistent transactional instant.

Wait for it to come up:
```bash
gcloud sql instances describe "$CLONE_INSTANCE" \
  --format="value(name,state)"
# wait until state = RUNNABLE
```

### 2.1 (Optional) Shrink the clone to save money
The clone inherits prod's tier (`db-custom-2-8192`). For debugging you usually
don't need that much. Because prod is a **custom** tier, patch to a smaller custom
size (not a shared-core `db-g1-small`):

```bash
# e.g. 1 vCPU / ~3.75 GB — adjust to taste
gcloud sql instances patch "$CLONE_INSTANCE" --tier=db-custom-1-3840
```
> Skip this if you need prod-like performance to reproduce the bug. You can patch
> back up at any time.

### 2.2 Get the instance connection name
The proxy identifies instances as `PROJECT:REGION:INSTANCE` — for SFLuv that's
`sfluv-app:us-central1:<clone>`.
```bash
CLONE_CONN="$(gcloud sql instances describe "$CLONE_INSTANCE" \
  --format='value(connectionName)')"
echo "$CLONE_CONN"
# -> sfluv-app:us-central1:debug-clone-20260624-104200
```

---

## 3. (If needed) Ensure a DB user/password you can log in with

The SFLuv app/Ponder connect with a Postgres user that already exists on the
clone. If you need a fresh password for local poking:

```bash
# reset (or set) the postgres superuser password on the clone
gcloud sql users set-password postgres \
  --instance="$CLONE_INSTANCE" \
  --password='CHOOSE_A_STRONG_TEMP_PASSWORD'

# or create a dedicated throwaway debug user
gcloud sql users create debug \
  --instance="$CLONE_INSTANCE" \
  --password='CHOOSE_A_STRONG_TEMP_PASSWORD'
```

> Whatever you set here goes into `DB_PASSWORD` in your local `.env` (section 6).

---

## 4. Start the Auth Proxy (the local tunnel)

In a dedicated terminal (it runs in the foreground):

```bash
cloud-sql-proxy "$CLONE_CONN" --port 5432
```

You should see it report listening on `127.0.0.1:5432`. Leave this running for the
duration of your session.

**Port already in use?** If you have a local Postgres on 5432, use another port:
```bash
cloud-sql-proxy "$CLONE_CONN" --port 5433
```

**Prefer a private/automatable setup?** Run the proxy with a service-account key
instead of your user ADC:
```bash
cloud-sql-proxy "$CLONE_CONN" --port 5432 \
  --credentials-file=/path/to/debug-sa-key.json
```

---

## 5. Connect and verify

In another terminal:

```bash
# password from step 3 if you set one
psql "host=127.0.0.1 port=5432 user=postgres dbname=app"

# quick checks
\l                 -- list databases: expect app, bot
\c app
\dt                -- list app tables
\c bot
\dt                -- list bot tables
SELECT now();      -- sanity
\q
```

If `psql` connects and you see the `app` and `bot` databases, the tunnel works.

---

## 6. Point your local app at the clone

In your local `.env`, repoint the database block to the proxy. Mapping to the
SFLuv `.env` variables:

```dotenv
IN_PRODUCTION=false
DB_TYPE=postgres
DB_BASE_URL=127.0.0.1:5432    # the Auth Proxy
APP_DB_NAME=app
BOT_DB_NAME=bot
DB_USER=postgres              # or your throwaway debug user
DB_PASSWORD=CHOOSE_A_STRONG_TEMP_PASSWORD
```

> ⚠️ This step only makes the **database** safe/isolated. Do **not** boot the full
> backend against the clone until you've also neutralized the chain and
> third-party side channels. For SFLuv specifically, before `go run`:
>
> - **Chain / keys:** point `RPC_URL`, `ENGINE_RPC_URL`, `ENGINE_WS_URL` at a
    >   pinned fork; swap `BOT_KEY` and `REDEEMER_ADMIN_KEY` for throwaway keys so no
    >   real Berachain tx can be signed/broadcast.
> - **Email/push:** set `NOTIFICATION_TEST_MODE=true` (disables Mailgun/Expo,
    >   writes notifications to disk).
> - **Config service:** set `CLIENT_CONFIG_LOCAL_ONLY=true` and pin
    >   `CLIENT_CONFIG_FALLBACK_PATH=community-config.json` so boot doesn't depend on
    >   the live Citizen Wallet config (and can't drift / hit the 403 per-community
    >   files).
> - **Privy:** use a separate Privy app, not the prod `PRIVY_APP_ID`/secret.
> - **Ponder:** repoint `PONDER_CALLBACK_URL` at the *parallel* backend, and the
    >   parallel Ponder's DB → the clone, RPC → the pinned fork.
>
> See the `.env.fork` checklist for the full KEEP / REPOINT / SANDBOX / KILL-SWITCH
> treatment.

---

## 7. Teardown (do not skip — this stops billing)

When you're done:

```bash
# 1. Stop the proxy: Ctrl-C in its terminal.

# 2. Delete the clone instance (this is the part that stops the bill).
gcloud sql instances delete "$CLONE_INSTANCE"
# confirm when prompted, or add --quiet to skip the prompt
```

Verify it's gone:
```bash
gcloud sql instances list --filter="name:debug-clone-*"
# should not list your clone
```

### 7.1 Clean up any leftovers
```bash
# if you exported to a bucket anywhere, remove those dumps too (PII!)
# gsutil rm gs://your-bucket/dump.sql.gz

# revoke the temp ADC if you logged in just for this (optional)
# gcloud auth application-default revoke
```

---

## Quick reference (copy/paste skeleton — SFLuv values baked in)

```bash
# --- vars ---
gcloud config set project sfluv-app
PROD_INSTANCE="sfluv-app-prod"
CLONE_INSTANCE="debug-clone-$(date +%Y%m%d-%H%M%S)"
SNAPSHOT_TS='2026-06-24T17:42:00.000Z'   # edit to the moment you want

# --- clone (PITR enabled) ---
gcloud sql instances clone "$PROD_INSTANCE" "$CLONE_INSTANCE" \
  --point-in-time "$SNAPSHOT_TS"

# --- connection name ---
CLONE_CONN="$(gcloud sql instances describe "$CLONE_INSTANCE" \
  --format='value(connectionName)')"
echo "$CLONE_CONN"   # sfluv-app:us-central1:<clone>

# --- (optional) shrink + temp password ---
gcloud sql instances patch "$CLONE_INSTANCE" --tier=db-custom-1-3840
gcloud sql users set-password postgres --instance="$CLONE_INSTANCE" \
  --password='TEMP_PW'

# --- tunnel (foreground) ---
cloud-sql-proxy "$CLONE_CONN" --port 5432

# --- connect (other terminal) ---
psql "host=127.0.0.1 port=5432 user=postgres dbname=app"

# --- TEARDOWN ---
gcloud sql instances delete "$CLONE_INSTANCE" --quiet
```

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `clone` fails citing point-in-time | timestamp outside WAL retention window | Choose a more recent `SNAPSHOT_TS`, or clone to now (omit the flag) |
| Proxy: permission denied | ADC missing or wrong project, or no `cloudsql.client` | Re-run `gcloud auth application-default login`; `gcloud config set project sfluv-app`; check IAM role |
| `psql` connection refused | Proxy not running, or wrong port | Confirm proxy terminal shows listening on 127.0.0.1; match `--port` |
| Port 5432 in use | local Postgres running | Use `--port 5433` and update `.env` `DB_BASE_URL` |
| Auth fails to Postgres | user/password mismatch on clone | Set a temp password via `gcloud sql users set-password` |
| Clone underpowered after shrink | patched tier too small | `gcloud sql instances patch "$CLONE_INSTANCE" --tier=db-custom-2-8192` (back to prod size) |

---

## Notes & cautions

- **Billing:** the clone is a full managed instance and bills until deleted. Set a
  calendar reminder or add the delete to your end-of-session ritual.
- **PII:** the clone inherits all SFLuv production data sensitivity. Don't share
  access broadly; scrub if a teammate without prod-data access needs it.
- **Consistency win:** because `app` and `bot` share `sfluv-app-prod`, the clone
  gives you both at the same transactional instant — no cross-DB coordination.
- **Chain context:** prod is Berachain (chain 80094), explorer Berascan. Pin any
  RPC fork to the block matching `SNAPSHOT_TS`, and don't accidentally wire the
  parallel env to the SFLuv Polygon community.
- **Next step for a full parallel env:** fork the RPC at the matching block and
  flip the third-party kill switches (section 6 warning) before booting the Go
  backend or a parallel Ponder against this clone.