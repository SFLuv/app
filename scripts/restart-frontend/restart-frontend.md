# restart-frontend

Restarts the dev frontend (Next.js on :3000) from current source with the same
env dev-up gave it. Needed because a dev server left running for days
accumulates hot-reload state until individual routes deadlock — a page that
compiled instantly yesterday never answers today, which looks like the app
being broken rather than the compiler being tired.

Mirrors dev-up.sh's FRONTEND_ENV; if that list changes, change this script to
match.

```
./scripts/restart-frontend/restart-frontend.sh
```
