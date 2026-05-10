---
applyTo: "**"
---
# MinURL Project Instructions

Repository-specific rules and workflow guidance for AI agents working in this codebase.

## Project Overview

MinURL is a Go short URL service. The core API supports creating, fetching, and redirecting short URLs backed by SQLite or PostgreSQL.

## MANDATORY: After Any API or Model Change

Whenever you modify any of the following, you **MUST** run `make gen` before finishing:

- `internal/service/model.go` (ShortURL struct fields)
- `internal/handler/short_url.go` (routes, operations, request/response types)
- Any new HTTP endpoint

`make gen` regenerates **both**:
- `docs/openapi/openapi.yaml` and `docs/openapi/openapi.json`
- `pkg/kiota/go/gen/client/` (Kiota Go client)

The CI `make ci` runs `git diff --exit-code` after `make gen` — out-of-date generated files will fail CI.

## MANDATORY: Documentation Checklist

After any functional change, update **all** of the following that apply:

| File | When to update |
|------|---------------|
| `README.md` | API behavior, new fields, new endpoints |
| `docs/http/minurl.http` | New fields or scenarios — add example requests **manually** |
| `docs/openapi/` | Run `make gen` (auto) |
| `pkg/kiota/go/gen/` | Run `make gen` (auto) |
| `AGENTS.md` | Domain Model table, migration strategy, new behaviors |
| `.github/instructions/minurl.instructions.md` | New mandatory workflows or conventions |

## Domain Model: ShortURL

Defined in `internal/service/model.go`:

| Field | Type | JSON key | Required | Notes |
|-------|------|----------|----------|-------|
| `ID` | `string` | `id` | No | Base58 ≤12 chars; auto-generated if omitted |
| `OriginalURL` | `string` | `original_url` | **Yes** | Must be a valid URL |
| `ExpireTime` | `*time.Time` | `expire_time` | No | RFC 3339 UTC; omit/null = permanent |
| `CreateTime` | `time.Time` | `create_time` | No | readOnly — set by server |

**Expiry enforcement**: handled in `ShortURLService.Get()` in `internal/service/short_url.go`. The store layer returns raw rows; expiry is checked at the service layer.

## Storage Layer Conventions

- SQLite storage: `internal/store/storage.go` + `internal/store/sqlite.go`
- PostgreSQL storage: `internal/store/postgres.go`
- Test storage (in-memory): `internal/testhelpers/storage.go`

### Migration System

Migrations are managed by **[golang-migrate/migrate v4](https://github.com/golang-migrate/migrate)** with SQL files embedded in the binary via `//go:embed`.

- Migration files: `internal/store/migrations/sqlite/` and `internal/store/migrations/postgres/`
- Naming: `000001_<name>.up.sql` / `000001_<name>.down.sql`
- Shared helper: `runMigrations()` in `internal/store/migrations.go`
- `m.Close()` is **not** called — the DB drivers wrap a caller-owned `*sql.DB`; calling Close() would close the shared connection

### Adding New Columns

When adding a new column to `short_urls`:
1. Add `000002_<name>.up.sql` + `down.sql` to **both** `internal/store/migrations/sqlite/` and `internal/store/migrations/postgres/`
2. Update `CreateIfAbsent()` in both `internal/store/storage.go` and `internal/store/postgres.go`.
3. Update `GetByID()` in both files.
4. Update `internal/testhelpers/storage.go` if the field needs special handling.

## Make Targets Reference

| Target | What it does |
|--------|-------------|
| `make fix` | `go mod tidy` + golangci-lint auto-fix |
| `make lint` | golangci-lint check |
| `make test` | race-enabled `go test ./...` |
| `make check` | tidy diff + lint + test |
| `make gen` | regenerate OpenAPI docs and Kiota client |
| `make openapi` | regenerate OpenAPI docs only |
| `make kiota` | regenerate Kiota client only (runs openapi first) |
| `make ci` | check + gen + `git diff --exit-code` |
| `make build` | compile binary to `bin/minurl` |
| `make docker-build` | build Docker image |
| `make docker-run` | run Docker container |

## Layer Responsibilities

| Layer | Package | Responsibility |
|-------|---------|---------------|
| Entry | `cmd/minurl` | startup, wiring, CLI flags |
| Handler | `internal/handler` | HTTP route registration, request/response |
| Service | `internal/service` | business logic, validation, expiry |
| Store | `internal/store` | persistence (SQLite / PostgreSQL) |
| Test helpers | `internal/testhelpers` | in-memory fakes for unit tests |

## Testing Conventions

- Unit tests use in-memory fakes from `internal/testhelpers`.
- PostgreSQL integration tests require `INTEGRATION_TEST=1` and Docker.
- Always add tests when changing business logic.
- Prefer table-driven tests for validation and handler logic.
- Run `make test && make lint` before finalizing any change.
