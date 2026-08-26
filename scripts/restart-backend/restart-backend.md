# restart-backend

Rebuilds and restarts the dev backend from current source, reusing the env that
dev-up generated for it (`tmp/backend.dev.env`). Needed because dev-up starts
the backend once and never rebuilds it — a stack left running overnight serves
yesterday's code, so routes 404 and fields go missing while the source in front
of you clearly has them.

Parses the env file with python rather than `source` because `PRIVY_VKEY` is a
multi-line PEM key that `source` would truncate.

```
./scripts/restart-backend/restart-backend.sh
```
