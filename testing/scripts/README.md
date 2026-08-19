# testing/scripts

Layer-2 feature tests: real HTTP against the local backend, no browser, seconds
per run. The full plan these came from — including the Playwright and Maestro
layers that are **not** built — is `docs/test-suite-plan.md`.

Read the `sfluv-testing` skill first. It explains prank forwarding, the token
capture, and what each scenario is actually proving.

## Order

```bash
./capture-token.sh                 # once per session; tokens last about an hour
export SFLUV_PRANKER_DID=did:privy:...

./preflight.sh                     # always. fails closed on anything non-local
./fund-faucet.sh                   # if the faucet is empty
./refresh-qr-windows.sh            # if cloned events have expired QR windows

./run-all.sh                       # everything, one report
```

## Environment

| Variable | Default | Needed for |
|---|---|---|
| `SFLUV_API` | `http://localhost:8080` | everything |
| `SFLUV_PRANKER_DID` | — | any scenario switching roles |
| `SFLUV_TOKEN_ADDRESS` | — | balance assertions, faucet funding |
| `SFLUV_FAUCET_ADDRESS` | — | faucet checks |
| `SFLUV_TEST_ADDRESS` | — | QR payout recipient |
| `SFLUV_PROPOSER_DID` / `SFLUV_IMPROVER_DID` | — | the full workflow chain |
| `DEV_ADMIN_KEY` | `local-dev-admin-key` | fixture setup |

## Safety

`require_local_stack` runs at the top of every scenario and **fails closed**: a
non-localhost API or RPC stops the run. These scripts create events, approve
merchants, redeem codes and move tokens — pointed at production they would do
all of that for real.

`X-Admin-Key` is for arranging fixtures only, never for testing admin behaviour:
`withAdmin` injects `userDid` after the prank middleware, so it exercises a
different path through the stack than a real admin does.

## Status

**None of these have been run yet.** They are written from the route table and
the handlers, so expect the first pass to be a debugging session — request
shapes are the most likely thing to be wrong. Fix them here and record the
cause in `../artifacts/TESTING-LOG.md`.
