# reset-w9

Wipes an account's W-9 history so a tier walkthrough starts from zero. Clears
everything the tier logic reads: `payout_ledger` (the annual total), 
`w9_tier_notices` (which modals have fired and been acknowledged),
`w9_filings` (a cleared filing pays through every threshold), the bell-badge
read marks, and the fake provider's in-process state.

Does NOT clear the on-chain balance — transfers on the fork already happened.
The tier maths reads the ledger, not the chain, so that is cosmetic: the wallet
can read 800 while the modal correctly says "400 of 600".

```
./scripts/reset-w9/reset-w9.sh <wallet-address|did>
```
