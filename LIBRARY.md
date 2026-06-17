# LIBRARY.md

Central work-in-progress ledger for concurrent implementation.
Use the agent-work-coordinator skill or `scripts/coordinator.py` to edit this file.

<!-- agent-work-coordinator-state
{
  "checkouts": {},
  "implementations": {
    "push-installation-id-backend": {
      "agent": "codex",
      "agent_uuid": "36f065ea-f4c0-4c98-897f-2c1705c87822",
      "bumped_files": [],
      "checked_out": [],
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
        },
        {
          "at": "2026-06-17T13:46:36Z",
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
            "AGENTS.md",
            "backend/structs/app_ponder.go",
            "backend/handlers/app_ponder.go",
            "backend/db/app_mobile_push_subscription.go",
            "backend/bootstrap/schema_migrations.go",
            "backend/db/app.go"
          ],
          "note": "released completed files: AGENTS.md, backend/structs/app_ponder.go, backend/handlers/app_ponder.go, backend/db/app_mobile_push_subscription.go, backend/bootstrap/schema_migrations.go, backend/db/app.go",
          "queued": []
        }
      ],
      "completed_files": [
        "AGENTS.md",
        "backend/structs/app_ponder.go",
        "backend/handlers/app_ponder.go",
        "backend/db/app_mobile_push_subscription.go",
        "backend/bootstrap/schema_migrations.go",
        "backend/db/app.go"
      ],
      "goal": "Add backend installation ID scoping for mobile push subscriptions",
      "id": "push-installation-id-backend",
      "last_checkin_at": "2026-06-17T13:46:36Z",
      "planned_files": [
        "AGENTS.md",
        "backend/structs/app_ponder.go",
        "backend/handlers/app_ponder.go",
        "backend/db/app_mobile_push_subscription.go",
        "backend/bootstrap/schema_migrations.go",
        "backend/db/app.go"
      ],
      "progress_note": "released completed files: AGENTS.md, backend/structs/app_ponder.go, backend/handlers/app_ponder.go, backend/db/app_mobile_push_subscription.go, backend/bootstrap/schema_migrations.go, backend/db/app.go",
      "queued": [],
      "started_at": "2026-06-17T13:41:56Z",
      "updated_at": "2026-06-17T13:46:36Z"
    }
  },
  "queues": {},
  "updated_at": "2026-06-17T13:46:36Z",
  "version": 1
}
agent-work-coordinator-state -->

## Active Implementation Briefs

### `push-installation-id-backend`

- Agent: codex [36f065ea-f4c0-4c98-897f-2c1705c87822]
- Started: 2026-06-17T13:41:56Z
- Last check-in: 2026-06-17T13:46:36Z
- Goal: Add backend installation ID scoping for mobile push subscriptions
- Progress: released completed files: AGENTS.md, backend/structs/app_ponder.go, backend/handlers/app_ponder.go, backend/db/app_mobile_push_subscription.go, backend/bootstrap/schema_migrations.go, backend/db/app.go
- Planned paths:
  - `AGENTS.md`
  - `backend/structs/app_ponder.go`
  - `backend/handlers/app_ponder.go`
  - `backend/db/app_mobile_push_subscription.go`
  - `backend/bootstrap/schema_migrations.go`
  - `backend/db/app.go`
- Completed paths:
  - `AGENTS.md`
  - `backend/structs/app_ponder.go`
  - `backend/handlers/app_ponder.go`
  - `backend/db/app_mobile_push_subscription.go`
  - `backend/bootstrap/schema_migrations.go`
  - `backend/db/app.go`
- Checked-out paths:
_None._
- Queued paths:
_None._
- Bumped paths:
_None._
- Recent check-ins:
  - 2026-06-17T13:41:56Z: checkout requested (`AGENTS.md, backend/structs/app_ponder.go, backend/handlers/app_ponder.go, backend/db/app_mobile_push_subscription.go, backend/bootstrap/schema_migrations.go, backend/db/app.go`)
  - 2026-06-17T13:44:39Z: implementation drafted; formatting and tests next (`backend/structs/app_ponder.go, backend/handlers/app_ponder.go, backend/db/app_mobile_push_subscription.go, backend/bootstrap/schema_migrations.go, backend/db/app.go`)
  - 2026-06-17T13:46:36Z: released completed files: AGENTS.md, backend/structs/app_ponder.go, backend/handlers/app_ponder.go, backend/db/app_mobile_push_subscription.go, backend/bootstrap/schema_migrations.go, backend/db/app.go (`AGENTS.md, backend/structs/app_ponder.go, backend/handlers/app_ponder.go, backend/db/app_mobile_push_subscription.go, backend/bootstrap/schema_migrations.go, backend/db/app.go`)

## File Checkouts

_No checked-out files._

## Queues

_No queued files._
