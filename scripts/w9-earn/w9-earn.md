# w9-earn

Pays an account real money the way a volunteer actually earns it: creates a
one-seat volunteer event with the given reward, mints its QR redemption code,
and redeems that code for the target account. The transfer, ledger row, and
W-9 tier notice are all produced by the system deciding what to do with a real
redemption — nothing is fabricated.

Run it repeatedly to walk somebody up the W-9 ladder. Against the default
thresholds (notice 400, warning 500, limit 600):

```
./scripts/reset-w9/reset-w9.sh <address|did>        # start from zero
./scripts/w9-earn/w9-earn.sh   <address|did> 400    # -> notice modal
./scripts/w9-earn/w9-earn.sh   <address|did> 100    # -> warning modal at 500
./scripts/w9-earn/w9-earn.sh   <address|did> 100    # -> crossing at 600, held
./scripts/w9-earn/w9-earn.sh   <address|did> 100    # -> refused, 409
```

The amounts land exactly on each line because `decidePayout` compares with
`>=`. Accepts a wallet address or a `did:privy:` id. Needs the dev stack
running.
