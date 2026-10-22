# Branch scope — `sanchezo/faucet-queue-fixes`

Oct 22 2026 · app (backend) · **~0.1h active** · based on `pjol/bot-smart-wallet-hotfix` at `5aa4df7`

Hours are wall-clock time from this session's timestamps (about 15:07 to 15:13 PDT), not measured with the
`time-accounting` script.

## What this branch is

Two double-payment fixes in the faucet Safe queue (`handlers/protocol_faucet.go`), found in review of round 4.

1. **An abandoned queued payout was still paid after its QR code was released.** When a caller's context expired
   while its request sat in the queue, `SendFromFaucet` returned a releasable error, so the redemption was undone, but the
   request stayed in the channel and the dispatcher paid it later. The volunteer could then redeem the same code again.
   There was also a read-then-return race against the dispatcher's `submitted.Store(true)`.
   **Fix:** each request has one atomic state, `queued` → `cancelled` (caller) or `queued` → `submitted`
   (dispatcher), decided by compare-and-swap, so exactly one side wins. The dispatcher drops cancelled requests
   (`takeFaucetBatch`) before building the operation.
2. **A send that failed ambiguously released every code in the batch.** Any `executeRaw` error was treated as "the
   engine refused it". **Fix:** `protocol.Client.rpc` now returns `*protocol.RejectedError` for definite refusals
   (a JSON-RPC error object, or a 4xx other than 408). `executeRaw` wraps any other `Send` failure (a timeout, a
   dropped connection, a 5xx, an undecodable response) in `uncertainSendError` carrying the signed operation hash.
   Those payouts stay non-releasable, and the hash is logged for tracing. Failures before `Send` are still releasable.

| Item | Hours | Repo |
|---|---|---|
| Both fixes. Verified with throwaway tests, not committed per the repo's testing rule: 60,000 racing cancel/take requests under `-race` gave 0 double payments; the same harness on the old logic released *and* paid 19,996 of 20,000 abandoned requests; engine responses classified as rejected (JSON-RPC error, 400, 429) or uncertain (408, 502, garbage, hang, connection refused) | 0.1 | app |

Volume: 2 files, +130 / −24. 0 migrations, 0 routes.

Known remaining edge: if a caller's context expires in the moments between the dispatcher taking its request and a
pre-send failure, the caller has already answered "submitted", so that code stays spent although nothing was sent.
It errs toward not paying twice.
