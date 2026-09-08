# seed-merchant

Promotes a signed-in account to a merchant that owns an approved location, so
the merchant surfaces (mobile and web) can be tested. Exists because signing in
against the local stack's Privy app mints a DIFFERENT did than production —
so the merchant rows that came over in the prod clone belong to accounts the
simulator cannot log in as. This promotes the account you CAN log in as,
creating or approving a location for it directly in the local database.

```
./scripts/seed-merchant/seed-merchant.sh <did:privy:...>
```
