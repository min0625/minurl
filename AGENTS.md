# AGENTS

Guidance for AI coding agents working in this repository.

> For development workflow, coding conventions, testing requirements, and PR process,
> see [CONTRIBUTING.md](CONTRIBUTING.md).

## Purpose

MinURL is a Go short URL service. The core API is fully implemented — it supports creating, fetching, and redirecting short URLs backed by SQLite, PostgreSQL, or MySQL. The runtime provides a Cobra-based CLI entrypoint and HTTP server startup via `cmd/minurl/main.go`.

## Core Rules

- Keep changes small, focused, and easy to review.
- Preserve existing behavior unless a task explicitly requires behavior changes.
- Do not introduce unrelated refactors.
- Add tests when changing logic.
- Keep code and docs consistent with [CONTRIBUTING.md](CONTRIBUTING.md).

## Project Facts

- Go version: 1.26.8
- Main module: `github.com/min0625/minurl`
- Main entry point: `cmd/minurl/main.go`
- CLI subcommands: `openapi`, `version`, `healthcheck`
- Deployment configs:
  - `deploy/docker-compose/` — Docker Compose examples (`.example.yml`; copy and customize before use)
  - `deploy/kubernetes/` — Kubernetes manifest examples (`.example.yaml`; copy and customize before use)
- HTTP listen port: `:8888` (default)
- Storage backends: SQLite (`sqlite3://`), PostgreSQL (`postgres://`), and MySQL (`mysql://`), auto-detected from DSN scheme
- Minimum database versions: PostgreSQL 9.5 (`ON CONFLICT`), MySQL **8.0** (the `utf8mb4_0900_as_cs`
  collation on `short_urls.id` — 5.7 fails to start). SQLite is embedded via `modernc.org/sqlite`,
  so its version is pinned by `go.mod`. See [README — Supported databases](README.md#supported-databases)
- MySQL DSN query params are sent as server session variables (`SET k = v`), so every driver-level
  flag — dangerous (`multiStatements`, `interpolateParams`, …) and harmless alike (`timeout`,
  `charset`, `maxAllowedPacket`, …) — is rejected by the server at connect time. `tls` is mapped
  explicitly and is the only driver setting a DSN can change; `parseTime` and `loc` are silently
  ignored (times are always parsed as UTC). This is MinURL's own vocabulary, **not** MySQL's
  official URI attributes (`ssl-mode`, `connect-timeout`, …) — see
  [README — MySQL DSN query parameters](README.md#mysql-dsn-query-parameters)
- `postgres://` and `sqlite3://` DSNs are **not** curated this way: the former goes to pgx
  untouched, the latter's query string is appended to the SQLite URI
- Log format: `text` (default) or `json`, controlled via `--log-format` / `MINURL_LOG_FORMAT`
- OpenTelemetry: opt-in tracing via `--otel-enabled`; supports `stdout` and `otlp` exporters
- Configuration precedence: CLI flags > env vars > config file > defaults

## Domain Model: ShortURL

Defined in `internal/service/model.go`:

| Field | Type | JSON key | Required | Notes |
|-------|------|----------|----------|-------|
| `ID` | `string` | `id` | No | Base58 ≤12 chars, case-sensitive; auto-generated when omitted, `null` or `""` |
| `OriginalURL` | `OriginalURL` | `original_url` | **Yes** | Absolute `http`/`https` URL with a host, no userinfo, no whitespace, control or invisible formatting characters; no length limit in the API (MySQL stores it in a TEXT column, which caps the value at 65,535 bytes; over that, `CreateIfAbsent` returns `ErrOriginalURLTooLong` and the handler answers 413) |
| `ExpireTime` | `*time.Time` | `expire_time` | No | RFC 3339 UTC; omit/null = permanent |
| `CreateTime` | `time.Time` | `create_time` | No | readOnly — set by server |

### Struct tag conventions

**`json` tags — default to `omitzero`** (Go 1.24+; the repo targets Go 1.26).

Use `omitempty` only on a slice/map field where a non-nil but zero-length value
should also be omitted, and add a comment saying so. `omitzero` omits a nil
slice/map but keeps `[]` / `{}`.

Rationale: `omitempty` has no effect at all on struct fields such as `time.Time`
(the `modernize` linter flags this), and `encoding/json/v2` redefines `omitempty`
in terms of the encoded JSON rather than the Go value, so its meaning shifts for
bools, numbers, pointers and interfaces. `omitzero` is defined on the Go value
and behaves identically in v1 and v2.

### Request validation

**Validation lives in the request schema, not in a second validator.** huma checks the
schema against the parsed JSON *before* it unmarshals into the struct, so it can tell an
absent key from a zero value, its errors carry `location` and `value`, and the rules reach
the generated OpenAPI. A struct-level validator sees none of that.

**A rule that belongs to a field belongs to its type.** `service.OriginalURL` is the
worked example: `huma.SchemaProvider` gives it `minLength`, and `huma.ResolverWithPath`
gives it the scheme/host/userinfo checks and the whitespace/control/invisible-character
screen JSON Schema cannot express.
huma calls the resolver for every field of that type and supplies the error location
itself — no hand-written `"body.original_url"` to go stale.

Two things such a resolver must respect: a value-typed field reaches it as the zero value
when the key is absent, so return early on empty rather than inventing an error for a
field nobody sent (a pointer field is skipped instead); and it runs on request input only,
which is why the redirect handler calls `IsValidOriginalURL` itself for stored rows.

`OriginalURL` has **no length limit, deliberately**. `minLength` is load-bearing though:
`required:"true"` only asserts the key is present, and the resolver returns nil for the
empty string, so dropping `minLength` lets an empty `original_url` through.

**An optional field's schema must accept the Go zero value.** `ShortURL.ID` uses `*`
rather than `+` in its pattern: `Create` already treats an empty ID as "generate one", and
a Go client without `omitzero` serializes an unset string as `""`, so `+` would 422 a
request that means the same as an omitted one. The `{id}` path param keeps `+` — an empty
path segment never reaches the pattern, because huma rejects it first with "required path
parameter is missing".

`ShortURL.ID` also carries `nullable:"true"`. huma accepts `null` for any non-required
property whatever its schema says, so without the tag the published document would claim
`type: "string"` while the server took `{"id": null}` — a spec-validating gateway would
reject a request the README documents as valid. `expire_time` already sets the precedent.

That difference is why `ShortURL.ID` stays on tags instead of becoming a named type like
`OriginalURL`: one `Schema()` returns one schema, but the body needs `*` and the path
needs `+`. Only those two declarations remain — the get and redirect operations share one
`shortURLIDInput`, because their parameter is identical. `TestRegisterPublishesShortIDConstraints`
pins the body property and both published path params to `Base58Alphabet` and
`MaxShortURLIDLen`.

**Expiry enforcement**: handled in `ShortURLService.Get()` in `internal/service/short_url.go`. The store layer returns raw rows; expiry is checked at the service layer.

## Layer Responsibilities

| Layer | Package | Key files |
|-------|---------|-----------|
| Entry | `cmd/minurl` | `main.go`, `server.go`, `service_factory.go`, `config.go` |
| Handler | `internal/handler` | `short_url.go`, `health.go` |
| Service | `internal/service` | `short_url.go`, `model.go`, `validation.go`, `id_generator.go`, `id_counter.go`, `interface.go`, `storage.go` |
| Store | `internal/store` | `sqlite.go` (SQLite), `postgres.go`, `mysql.go`, `migrations.go`, `pinger.go` |
| Test helpers | `internal/testhelpers` | In-memory fakes for unit tests |
| HTTP server | `internal/httpserver` | HTTP server lifecycle |
| Middleware | `internal/middleware` | Logging, recovery, decompression |
| Telemetry | `internal/telemetry` | OpenTelemetry initialization |

## Health Check Endpoints

Health endpoints are mounted directly on the chi router (not via Huma) and do **not** appear in the OpenAPI schema.

| Endpoint | Probe type | Checks |
|----------|-----------|--------|
| `GET /livez` | Liveness | HTTP server responds |
| `GET /readyz` | Readiness | `db.PingContext` |
| `GET /startupz` | Startup | Same as `/readyz` |

- Implemented via `github.com/alexliesenfeld/health` in `internal/handler/health.go`.
- `store.CloserPinger` interface (`internal/store/pinger.go`) is implemented by all three storage backends and passed through `service_factory → server → RegisterHealthHandlers`.
- The `minurl healthcheck` CLI subcommand (`cmd/minurl/command_healthcheck.go`) GETs `/livez` and exits 0/1 — used as Docker `HEALTHCHECK CMD` in the distroless container.

## Useful Commands

See [CONTRIBUTING.md — Make Targets Reference](CONTRIBUTING.md#make-targets-reference) for the full list.

Quick reference:

| Target | What it does |
|--------|-------------|
| `make check` | Every prek hook over all files (tidy diff + lint + test included) |
| `make gen` | regenerate OpenAPI docs and Kiota client |
| `make ci` | check + gen + `git diff --exit-code` |
| `make test` | race-enabled `go test ./...` |

`make lint` only reports issues new since `NEW_FROM_REV` (default `HEAD`) — see
[CONTRIBUTING.md — Make variables](CONTRIBUTING.md#make-variables) before concluding a clean run means clean code.

Direct run:

```bash
go run ./cmd/minurl
```

## MANDATORY: After Any API or Model Change

See [CONTRIBUTING.md — Mandatory: After Any API or Model Change](CONTRIBUTING.md#mandatory-after-any-api-or-model-change).

## Documentation Checklist

See [CONTRIBUTING.md — Documentation Checklist](CONTRIBUTING.md#documentation-checklist).

## Architecture Direction

Prefer this layering when adding new functionality:

1. `cmd` — startup and wiring only
2. `internal/handler` — HTTP route registration, request/response parsing
3. `internal/service` — business logic, validation, expiry
4. `internal/store` — persistence (SQLite / PostgreSQL / MySQL)

This is guidance, not a strict requirement.

## Database Migration Strategy

SQLite, PostgreSQL, and MySQL use [golang-migrate/migrate v4](https://github.com/golang-migrate/migrate) for versioned, in-process migrations:

- **Migration files**: `internal/store/migrations/sqlite/`, `internal/store/migrations/postgres/`, and `internal/store/migrations/mysql/`
- **Naming**: `000001_<name>.up.sql` / `000001_<name>.down.sql`
- **Embed**: `//go:embed` in each store file — no external files needed at runtime
- **Shared helper**: `runMigrations()` in `internal/store/migrations.go` — every backend calls it
- **Tracking**: golang-migrate creates a `schema_migrations` table in each database
- **Drivers**: `github.com/golang-migrate/migrate/v4/database/sqlite` (modernc, no cgo) for SQLite; `github.com/golang-migrate/migrate/v4/database/postgres` for PostgreSQL; `github.com/golang-migrate/migrate/v4/database/mysql` for MySQL
- **`m.Close()` is NOT called** after `Up()` — the database drivers wrap a caller-owned `*sql.DB`; calling Close() would close the shared connection

### Adding New Columns

See the full procedure in [CONTRIBUTING.md → Adding a New Storage Column](CONTRIBUTING.md#adding-a-new-storage-column).

Summary:
1. Add `000003_<name>.up.sql` + `down.sql` (the next unused version — `000002` is already taken) to **all three** migration directories.
2. Update `CreateIfAbsent()` and `GetByID()` in `sqlite.go`, `postgres.go`, and `mysql.go`.
3. Update `internal/testhelpers/storage.go` if needed.
4. Run `make gen`.
