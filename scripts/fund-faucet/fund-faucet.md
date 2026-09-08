# fund-faucet

Tops the local faucet up with SFLUV from another holder on the fork — the same
thing dev-up does at boot, on demand, for when testing has drained it. An empty
faucet is the single most common reason a payout fails for a reason unrelated
to the code being tested.

anvil forks the real chain, so it impersonates a real holder at zero cost; the
transfer exists only on the local fork. Set `ALL_DONORS=1` to drain every
discoverable holder. If donors run dry entirely, use mint-to-faucet instead.

```
./scripts/fund-faucet/fund-faucet.sh
```
