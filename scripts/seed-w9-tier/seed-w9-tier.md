# seed-w9-tier

Puts an account at a named W-9 tier so its modal can be looked at. Two modes:

- `--real` (default): drives an actual redemption whose reward alone reaches
  the tier — money moves on the fork and the system writes the ledger row and
  tier notice itself. Slower, cannot express impossible states.
- `--fast`: writes the ledger row and tier notice directly. Instant, for when
  you only want to see the modal.

`--revert` deletes only the rows this script seeded (`w9-seed-%` keys); for a
full wipe use reset-w9. On-chain balances accumulate across `--real` runs.

```
./scripts/seed-w9-tier/seed-w9-tier.sh <address|did> notice_400
./scripts/seed-w9-tier/seed-w9-tier.sh --fast <address|did> blocked
./scripts/seed-w9-tier/seed-w9-tier.sh --revert <address|did>
```
