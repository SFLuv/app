# preflight

Checks that the local stack is up, local, and in a state where testing means
something. Run it when something feels off before blaming the code: it verifies
each service answers (backend required; frontend, website, ponder reported),
that `jq`/`cast`/`curl` exist, that the **running backend is not older than the
newest backend commit** (the classic "debugging code that is not running"
trap), that the faucet actually holds tokens, and that the anvil clock has not
drifted from wall time (drift breaks all account abstraction with "AA32").

Read-only; changes nothing. Each failure names the script that fixes it.

```
./scripts/preflight/preflight.sh
```
