# check_redeemer_roles

Chain diagnostic: reads `REDEEMER_ROLE()` from the SFLUV token contract and
scans RoleGranted/RoleRevoked logs from the contract's deployment block to
now, printing who currently holds the redeemer role. For diagnosing "why can't
this address redeem" against whatever RPC the env points at.

```
./scripts/check-redeemer-roles/check_redeemer_roles.sh
```
