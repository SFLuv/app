# LIBRARY.md

Central work-in-progress ledger for concurrent implementation.
Use the agent-work-coordinator skill or `scripts/coordinator.py` to edit this file.

<!-- agent-work-coordinator-state
{
  "checkouts": {
    "AGENTS.md": "push-installation-id-backend",
    "backend/bootstrap/schema_migrations.go": "push-installation-id-backend",
    "backend/db/app.go": "push-installation-id-backend",
    "backend/db/app_mobile_push_subscription.go": "push-installation-id-backend",
    "backend/handlers/app_ponder.go": "push-installation-id-backend",
    "backend/structs/app_ponder.go": "push-installation-id-backend"
  },
  "implementations": {
    "push-installation-id-backend": {
      "agent": "codex",
      "agent_uuid": "36f065ea-f4c0-4c98-897f-2c1705c87822",
      "bumped_files": [],
      "checked_out": [
        "AGENTS.md",
        "backend/structs/app_ponder.go",
        "backend/handlers/app_ponder.go",
        "backend/db/app_mobile_push_subscription.go",
        "backend/bootstrap/schema_migrations.go",
        "backend/db/app.go"
      ],
      "checkins": [
        {
          "at": "2026-06-17T13:41:56Z",
          "files": [
            "AGENTS.md",
            "backend/structs/app_ponder.go",
            "backend/handlers/app_ponder.go",
            "backend/db/app_mobile_push_subscription.go",
            "backend/bootstrap/schema_migrations.go",
            "backend/db/app.go"
          ],
          "note": "checkout requested"
        },
        {
          "at": "2026-06-17T13:44:39Z",
          "bumped": [],
          "checked_out": [
            "AGENTS.md",
            "backend/structs/app_ponder.go",
            "backend/handlers/app_ponder.go",
            "backend/db/app_mobile_push_subscription.go",
            "backend/bootstrap/schema_migrations.go",
            "backend/db/app.go"
          ],
          "files": [
            "backend/structs/app_ponder.go",
            "backend/handlers/app_ponder.go",
            "backend/db/app_mobile_push_subscription.go",
            "backend/bootstrap/schema_migrations.go",
            "backend/db/app.go"
          ],
          "note": "implementation drafted; formatting and tests next",
          "queued": [],
          "remote_state": "up-to-date"
        }
      ],
      "completed_files": [],
      "goal": "Add backend installation ID scoping for mobile push subscriptions",
      "id": "push-installation-id-backend",
      "last_checkin_at": "2026-06-17T13:44:39Z",
      "planned_files": [
        "AGENTS.md",
        "backend/structs/app_ponder.go",
        "backend/handlers/app_ponder.go",
        "backend/db/app_mobile_push_subscription.go",
        "backend/bootstrap/schema_migrations.go",
        "backend/db/app.go"
      ],
      "progress_note": "implementation drafted; formatting and tests next",
      "queued": [],
      "started_at": "2026-06-17T13:41:56Z",
      "updated_at": "2026-06-17T13:44:39Z"
    }
  },
  "queues": {},
  "updated_at": "2026-06-17T13:44:39Z",
  "version": 1
}
agent-work-coordinator-state -->

## Active Implementation Briefs

### `push-installation-id-backend`

- Agent: codex [36f065ea-f4c0-4c98-897f-2c1705c87822]
- Started: 2026-06-17T13:41:56Z
- Last check-in: 2026-06-17T13:44:39Z
- Goal: Add backend installation ID scoping for mobile push subscriptions
- Progress: implementation drafted; formatting and tests next
- Planned paths:
  - `AGENTS.md`
  - `backend/structs/app_ponder.go`
  - `backend/handlers/app_ponder.go`
  - `backend/db/app_mobile_push_subscription.go`
  - `backend/bootstrap/schema_migrations.go`
  - `backend/db/app.go`
- Completed paths:
_None._
- Checked-out paths:
  - `AGENTS.md`
  - `backend/structs/app_ponder.go`
  - `backend/handlers/app_ponder.go`
  - `backend/db/app_mobile_push_subscription.go`
  - `backend/bootstrap/schema_migrations.go`
  - `backend/db/app.go`
- Queued paths:
_None._
- Bumped paths:
_None._
- Recent check-ins:
  - 2026-06-17T13:41:56Z: checkout requested (`AGENTS.md, backend/structs/app_ponder.go, backend/handlers/app_ponder.go, backend/db/app_mobile_push_subscription.go, backend/bootstrap/schema_migrations.go, backend/db/app.go`)
  - 2026-06-17T13:44:39Z: implementation drafted; formatting and tests next (`backend/structs/app_ponder.go, backend/handlers/app_ponder.go, backend/db/app_mobile_push_subscription.go, backend/bootstrap/schema_migrations.go, backend/db/app.go`)

## File Checkouts

- `AGENTS.md` -> `push-installation-id-backend` by codex [36f065ea-f4c0-4c98-897f-2c1705c87822] (Add backend installation ID scoping for mobile push subscriptions)
- `backend/bootstrap/schema_migrations.go` -> `push-installation-id-backend` by codex [36f065ea-f4c0-4c98-897f-2c1705c87822] (Add backend installation ID scoping for mobile push subscriptions)
- `backend/db/app.go` -> `push-installation-id-backend` by codex [36f065ea-f4c0-4c98-897f-2c1705c87822] (Add backend installation ID scoping for mobile push subscriptions)
- `backend/db/app_mobile_push_subscription.go` -> `push-installation-id-backend` by codex [36f065ea-f4c0-4c98-897f-2c1705c87822] (Add backend installation ID scoping for mobile push subscriptions)
- `backend/handlers/app_ponder.go` -> `push-installation-id-backend` by codex [36f065ea-f4c0-4c98-897f-2c1705c87822] (Add backend installation ID scoping for mobile push subscriptions)
- `backend/structs/app_ponder.go` -> `push-installation-id-backend` by codex [36f065ea-f4c0-4c98-897f-2c1705c87822] (Add backend installation ID scoping for mobile push subscriptions)

## Queues

_No queued files._
