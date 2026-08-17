# Reno — Home Renovation Project Planner

A purpose-built web app for planning, budgeting, documenting, and tracking home
renovation projects.

**Stack:** Go + templ + htmx + Tailwind v4, with PocketBase embedded as the
backend (auth, SQLite, file storage) — one binary, no external services.

## Development

```sh
# frontend deps + CSS build (Tailwind v4)
bun install
bun run build        # or: bun run watch

# generate templ components (after editing *.templ)
go install github.com/a-h/templ/cmd/templ@latest
templ generate ./...

# run (dev)
go run . serve --http=127.0.0.1:8090 --dev

# build release binary
go build -o reno .
```

First run prints a one-time superuser install URL for the PocketBase admin UI
(`/_/`), or run `./reno superuser upsert EMAIL PASS`.

## Architecture

- `main.go` — boots PocketBase, registers routes + static files.
- `migrations/` — PocketBase collection schema (single migration). All
  collections are superuser-only at the API level; access goes through app
  handlers that enforce per-project roles (owner / editor / viewer).
- `internal/session/` — cookie session backed by PocketBase auth tokens.
- `internal/handlers/` — HTTP handlers (`*core.RequestEvent`), authorization,
  data queries.
- `internal/ui/` — templ components (server-rendered HTML).
- `public/` — built CSS + vendored JS (htmx, later SortableJS / Frappe Gantt).
- `src/app.css` — Tailwind entry.

## Data model

projects → project_members (role), rooms, phases, tasks (self-referencing tree:
task/subtask/checklist_item), vendors, materials, budget_items, photos
(stage: before/during/after/…, room), documents, notes, decisions.

Rooms are a first-class entity so future photo-visualization and 3D features
can attach to them without schema redesign.

## Demo data

```sh
./reno seed demo@reno.local demo1234   # add --dir=... to target an instance
```

## Testing

```sh
go test ./...        # role-based integration tests against an in-memory PB app
```

## Deployment

See [DEPLOYMENT.md](DEPLOYMENT.md) — single binary + reverse proxy + backups.

## Status

MVP complete (all milestones):

- [x] M1 — skeleton: auth, projects CRUD, project list, dashboard v1
- [x] M2 — tasks core (tree, inline edit, drag-drop reorder, kanban board)
- [x] M3 — materials table, vendors, budget rollups
- [x] M4 — photos, documents, rooms, notes/decisions
- [x] M5 — timeline (Frappe Gantt), search, members & roles, responsive pass
- [x] M6 — integration tests, seed data, deployment

Future extension points already modeled: `tasks.depends_on` (schedule
cascades), `photos.stage` + `room` (before/after comparison), `rooms`
(photo visualization / 3D).
