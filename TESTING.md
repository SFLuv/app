# Testing

Testing here is **human-in-the-loop product use**. A person boots the local
stack, clicks through the product the way a user would, inspects what they see,
and reports what is wrong; an agent fixes it and the person looks again. That
loop — not an automated suite — is how this repo is tested.

There is deliberately no test harness, no e2e suite, and no expectation that
features come with unit tests. An automated-testing effort was tried, bloated
the repo, and was unwound; do not quietly rebuild it.

## How to test

1. Boot the stack: `./scripts/dev-up/dev-up.sh` (see `scripts/dev-up/dev-up.md`).
   It brings up an anvil fork, cloned databases, backend, frontend, indexer,
   and the iOS simulator, then drops into a utilities menu (admin, pranks,
   logs).
2. Use the product. When a flow needs setup that is boring to do by hand —
   money in an account, a W-9 history reset, a merchant to log in as — use a
   script from `scripts/`.
3. If something feels broken in a confusing way, run
   `./scripts/preflight/preflight.sh` first: it catches the classic local-stack
   traps (stale backend binary, drifted chain clock, empty faucet) that make
   working code look broken.

## The scripts folder

`scripts/` holds one subfolder per shortcut, each with the script and a short
markdown description of what it does. `scripts/MAINTENANCE.md` has the rules
for keeping it healthy — read it before adding or changing anything there.
