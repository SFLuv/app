# Backend Schema Versioning Pattern

This repo now uses an explicit schema-versioning pattern introduced by `pjol/production-upgrades`.

Rules:
- `backend/db/app.go` and the other `CreateTables()` helpers represent the baseline schema snapshot.
- New production schema changes must be added as ordered migrations in `backend/bootstrap/schema_migrations.go`.
- `backend/cmd/init/main.go` is responsible for baseline initialization plus running pending migrations.
- `backend/cmd/server/main.go` also runs pending migrations on startup.

What not to do:
- Do not add new production schema changes directly to `CreateTables()` unless the baseline schema itself is intentionally being redefined.
- Do not mix ad hoc one-off ALTER statements into unrelated startup paths when the change belongs in a numbered migration.

Current branch example:
- Merchant location tipping/payment-wallet storage is introduced in migration `1.2`.
- The baseline location table definition in `CreateTables()` stays clean.

Future rule of thumb:
- If the change affects a table or index that may already exist in production, it belongs in `backend/bootstrap/schema_migrations.go`.
