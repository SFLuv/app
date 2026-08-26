# seed_merchant_mode_local

Seeds local merchant-mode test data directly in the app database: ensures the
target user exists and is a merchant, gives them a wallet, an approved location
with payment wallets, and merchant-mode settings/device rows — so the
device-scoped merchant-mode flows (PIN, device registration, lockdown) can be
tested without hand-building the rows.

Written for the June merchant-mode work; local databases only.

```
./scripts/seed-merchant-mode-local/seed_merchant_mode_local.sh
```
