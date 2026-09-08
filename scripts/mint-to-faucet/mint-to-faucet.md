# mint-to-faucet

Sets the faucet's SFLUV balance directly by writing the token's storage slot on
the fork — unbounded and repeatable, unlike fund-faucet, which can only move
what holders on the fork actually have. Needed because event creation reserves
reward × seats against unallocated faucet balance and nothing gives it back, so
repeated testing steadily drains it.

Only legitimate because this is a local fork; nothing like it exists in
production.

```
./scripts/mint-to-faucet/mint-to-faucet.sh [whole-sfluv]
```
