# refresh-qr-windows

Pushes every cloned event's QR redemption window into the future. The local
databases are restored from production dumps, so every event's `qr_expires_at`
is already in the past and every redemption returns "code expired" — a failure
that looks like broken redemption code and is not.

Refuses to run against anything that is not localhost.

```
./scripts/refresh-qr-windows/refresh-qr-windows.sh
```
