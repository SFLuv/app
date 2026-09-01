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

## The frontend is served from a production build

`npm run build` is kicked off first, before the chain, the databases and the
backend, and compiles alongside them; step 8 collects it and runs `next start`.
By then the build is usually finished, so the frontend is ready as soon as the
rest of the stack is.

This replaced `next dev`, which started sooner and then charged for it on every
first click — a cold `/map` compile was over two minutes, which is why the
script used to curl twenty routes on boot purely to warm them. That warm-up is
gone; a production build has already compiled every route.

`next start` has no HTTPS option (`--experimental-https` is dev-only), and the
whole stack depends on the frontend being at `https://localhost:3000` — Privy's
registered redirect URLs, the backend's `APP_BASE_URL`, every `wait_for` in the
script. So the production server listens on `:3100` and a small Node TLS proxy,
written to `tmp/` at boot, publishes it on `:3000` using the certificate
`next dev --experimental-https` generates in `frontend/certificates/`.

Two fallbacks, both landing on a working stack rather than a broken one:

- **No certificate yet** (fresh clone — the directory is gitignored): the dev
  server runs this once, which mints it. The next boot serves the build.
- **Build fails**: the dev server runs instead, and the failure is in
  `tmp/logs/frontend-build.log`.

Editing the frontend now needs a rebuild to show up. Run `--no-frontend` and
`cd frontend && npm run dev` by hand when working on it.

