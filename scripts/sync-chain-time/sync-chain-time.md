# sync-chain-time

Drags the anvil fork's clock back to wall time. The paymaster signs every
UserOperation with a validity window taken from real time, and the chain judges
it against `block.timestamp` — once they drift apart (laptop sleep, a long
pause), every account-abstraction operation fails with "AA32 expired or not
due", which surfaces as the web app hanging on its loading spinner forever with
a perfectly healthy-looking backend.

No-op if the drift is under two minutes.

```
./scripts/sync-chain-time/sync-chain-time.sh
```
