# AGENTS

Guidance for coding agents working in this repository.

## Purpose

This repository is a Go short URL service. The core API is fully implemented — it supports creating, fetching, and redirecting short URLs backed by SQLite or PostgreSQL. The runtime provides a Cobra-based CLI entrypoint and HTTP server startup via `cmd/minurl/main.go`.

## Core Rules

- Keep changes small, focused, and easy to review.
- Preserve existing behavior unless a task explicitly requires behavior changes.
- Do not introduce unrelated refactors.
- Add tests when changing logic.
- Keep code and docs consistent.

## Project Facts

- Go version: 1.26.2
- Main module: `github.com/min0625/minurl`
- Main entry point: `cmd/minurl/main.go`
- CLI subcommands: `openapi`, `version`, `healthcheck`
- Deployment configs:
  - `deploy/docker-compose/` — Docker Compose examples (`.example.yml`; copy and customize before use)
  - `deploy/kubernetes/` — Kubernetes manifest examples (`.example.yaml`; copy and customize before use)
- HTTP listen port: `:8888` (default)
- Storage backends: SQLite (`sqlite3://`), PostgreSQL (`postgres://`), and MySQL (`mysql://`), auto-detected from DSN scheme
- Log format: `text` (default) or `json`, controlled via `--log-format` / `MINURL_LOG_FORMAT`
- OpenTelemetry: opt-in tracing via `--otel-enabled`; supports `stdout` and `otlp` exporters
- Configuration precedence: CLI flags > env vars > config file > defaults

## Domain Model: ShortURL

The `ShortURL` struct (defined in `internal/service/model.go`) has these fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | No | Short URL identifier (Base58, ≤12 chars). Auto-generated if omitted. |
| `original_url` | string | **Yes** | The destination URL. |
| `expire_time` | *time.Time | No | Expiry in RFC 3339 UTC. Omit or null = permanent. |
| `create_time` | time.Time | No (readOnly) | Set by server on creation. |

**Expiry behavior**: `Get()` and `:redirect` return `not found` (false) for expired URLs. The store layer always returns the raw row; expiry is enforced in `internal/service/short_url.go`.

## Health Check Endpoints

Health endpoints are mounted directly on the chi router (not via Huma) and do **not** appear in the OpenAPI schema.

| Endpoint | Probe type | Checks |
|----------|-----------|--------|
| `GET /livez` | Liveness | HTTP server responds |
| `GET /readyz` | Readiness | `db.PingContext` |
| `GET /startupz` | Startup | Same as `/readyz` |

- Implemented via `github.com/alexliesenfeld/health` in `internal/handler/health.go`.
- `store.CloserPinger` interface (`internal/store/pinger.go`) is implemented by both SQLite and Postgres backends and passed through `service_factory → server → RegisterHealthHandlers`.
- The `minurl healthcheck` CLI subcommand (`cmd/minurl/command_healthcheck.go`) GETs `/livez` and exits 0/1 — used as Docker `HEALTHCHECK CMD` in the distroless container.

## Useful Commands

Use Make targets when possible:

- `make fix`
- `make lint`
- `make test`
- `make check`
- `make gen` — regenerate OpenAPI docs (`docs/openapi/`) **and** Kiota Go client (`pkg/kiota/go/gen/client/`) from the live server contract
- `make docker-build`
- `make docker-run`

Direct run:

- `go run ./cmd/minurl`

**IMPORTANT**: After any change to `internal/service/model.go`, HTTP handlers, or API routes, you MUST run `make gen` to keep `docs/openapi/` and `pkg/kiota/go/gen/` in sync. The CI `make ci` target enforces this with `git diff --exit-code`.

## Coding Conventions

- Follow idiomatic Go style and keep functions small.
- Prefer explicit, readable names over abbreviations.
- Return actionable errors with context.
- Keep public APIs minimal until requirements are clear.

## Testing Expectations

- Add or update tests for behavior changes.
- Prefer table-driven tests for handler and validation logic.
- Run `make test` and `make lint` before finalizing.

## Documentation Expectations

When changing functionality, update **all** of the following that apply:

- `README.md` — user-facing behavior and API examples
- `docs/http/minurl.http` — REST Client debug examples (add new field/scenario manually)
- `docs/openapi/openapi.yaml` and `docs/openapi/openapi.json` — run `make openapi` or `make gen`
- `pkg/kiota/go/gen/` — run `make kiota` or `make gen`
- `AGENTS.md` — update Domain Model table and any changed behaviors
- Architecture notes (when new major components are introduced)

## Suggested Architecture Direction

As the short URL service is implemented, prefer this layering:

1. `cmd` or entry layer (startup and wiring)
2. `internal/handler` (HTTP handlers)
3. `internal/service` (business logic)
4. `internal/store` (persistence)
5. `internal/model` (domain types)

This is guidance, not a strict requirement, and can be adjusted per task.

## Database Migration Strategy

SQLite, PostgreSQL, and MySQL use [golang-migrate/migrate v4](https://github.com/golang-migrate/migrate) for versioned, in-process migrations:

- **Migration files**: `internal/store/migrations/sqlite/`, `internal/store/migrations/postgres/`, and `internal/store/migrations/mysql/`
- **Format**: `000001_<name>.up.sql` / `000001_<name>.down.sql`
- **Embed**: `//go:embed` in each store file — no external files needed at runtime
- **Tracking**: golang-migrate creates a `schema_migrations` table in each database
- **Drivers**: `github.com/golang-migrate/migrate/v4/database/sqlite` (modernc, no cgo) for SQLite; `github.com/golang-migrate/migrate/v4/database/postgres` for PostgreSQL; `github.com/golang-migrate/migrate/v4/database/mysql` for MySQL
- **`m.Close()` is NOT called** after `Up()` — the database drivers wrap a caller-owned `*sql.DB` and calling Close() would close the shared connection

When adding new columns:
1. Add a new `000002_<name>.up.sql` + `down.sql` pair under **all three** `migrations/sqlite/`, `migrations/postgres/`, and `migrations/mysql/`
2. Update `CreateIfAbsent()` and `GetByID()` in `internal/store/storage.go` (SQLite), `internal/store/postgres.go`, and `internal/store/mysql.go`
3. Update `internal/testhelpers/storage.go` if the field needs special handling
