# refresh-qr-windows

Pushes every cloned event's QR redemption window into the future. The local
databases are restored from production dumps, so every event's window
(`qr_live_at` → `expiration`, epoch seconds on `bot.events`) is already in the
past and every redemption returns "code expired" — a failure that looks like
broken redemption code and is not.

Reopens every non-cancelled event whose window has closed: opens an hour ago,
closes N days out (default 30). Side effect worth knowing: the volunteer panel
calls an event "upcoming" while its expiration is in the future, so refreshed
old events reappear in the upcoming list.

Refuses to run against anything that is not localhost. The DB role comes from
`backend/.env`.

```
./scripts/refresh-qr-windows/refresh-qr-windows.sh [days]
```
