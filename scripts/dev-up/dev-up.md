# dev-up

Boots the entire local dev stack — the lynchpin of the testing process. One
command brings up: an anvil fork of Celo (chain id 42220), local clones of the
production databases, the Go backend, the Next.js frontend, the public
marketing site, the Ponder indexer, and the Expo dev server with the iOS
simulator. After boot it drops into an interactive menu: open the simulator,
set an admin by email, set/clear user pranks (act as another user), tail logs,
quit.

Configuration comes from `.dev.env` at the repo root (auto-created from
`.dev.env.example`). Logs live in `tmp/logs/`. Ctrl-C stops everything.

```
./scripts/dev-up/dev-up.sh                  # boot everything, then the menu
./scripts/dev-up/dev-up.sh --no-mobile      # skip Expo and the simulator
./scripts/dev-up/dev-up.sh --skip-db-clone  # reuse already-cloned local DBs
./scripts/dev-up/dev-up.sh menu             # just the utilities menu, boots nothing
```

Other flags: `--no-frontend`, `--no-webpage`, `--no-backend`, `--no-ponder`,
`--mobile-branch <b>`.
