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
| `OriginalURL` | `OriginalURL` | `original_url` | **Yes** | Absolute `http`/`https` URL with a host, no userinfo, no whitespace, control, invisible formatting or replacement characters (a literal `U+FFFD` means the sent bytes were not valid UTF-8); no length limit in the API (MySQL stores it in a TEXT column, which caps the value at 65,535 bytes; over that, `CreateIfAbsent` returns `ErrOriginalURLTooLong` and the handler answers 413) |
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

**`huma.ValidateStrictCasing` is load-bearing.** huma validates the exact JSON key, but
`encoding/json` then matches keys case-insensitively and keeps the last one, so without it
`{"id":"abc","ID":"bad*id"}` stores the unvalidated `ID`, and `"ORIGINAL_URL":""` slips an
empty URL past `minLength`. It is set in `init()` in `internal/handler/register.go`
(not in `Register`: tests build APIs in parallel, so a write there would be a data race).

`OriginalURL` has **no length limit, deliberately**. `minLength` is load-bearing though:
`required:"true"` only asserts the key is present, and the resolver returns nil for the
empty string, so dropping `minLength` lets an empty `original_url` through.

**An optional field's schema must accept the Go zero value.** `ShortURL.ID` uses `*`
rather than `+` in its pattern: `Create` already treats an empty ID as "generate one", and
a Go client without `omitzero` serializes an unset string as `""`, so `+` would 422 a
request that means the same as an omitted one. The `{id}` path param keeps `+` — an empty
path segment never reaches the pattern, because huma rejects it first with "required path
parameter is missing".

`ShortURL.ID` is deliberately **not** `nullable`. `ShortURL` is both the request body and
the response, so one schema describes both, and a nullable `id` would tell clients a
response may carry `"id": null`, which the server never sends. Input stays lenient anyway:
huma skips `null` for any non-required property whatever its schema says, so
`{"id": null}` still means "generate one" — the document just does not advertise it.
Declaring `null` for input only would take separate request and response types.

The `*`/`+` difference is why `ShortURL.ID` stays on tags instead of becoming a named type like
`OriginalURL`: one `Schema()` returns one schema, but the body needs `*` and the path
needs `+`. Only those two declarations remain — the get and redirect operations share one
`shortURLIDInput`, because their parameter is identical. `TestRegisterPublishesShortIDConstraints`
pins the body property and both published path params to `Base58Alphabet` and
`MaxShortURLIDLen`.

### Error responses

Three pieces in `internal/handler/register.go`:

1. **The service defines the errors** (`service.ErrShortURLNotFound`, `ErrShortURLIDConflict`, …).
2. **`errorResponses` gives each error its status and message, once**, for every operation.
3. **Every operation is registered through `register(api, operation{…}, handler)`**, never
   `huma.Register` directly. `operation.errs` lists the errors the operation returns; `register`
   publishes their statuses as `Operation.Errors` and converts the handler's errors through the
   same list. The handler returns service errors as they are.

**`register` takes its own `operation`, not a `huma.Operation`.** It builds the `huma.Operation`
itself, so fields that change which statuses huma returns cannot be set around it: `Errors` /
`Responses`, `MaxBodyBytes` / `BodyReadTimeout` (a negative value disables the 413 / 408 it
publishes), `SkipValidateBody` / `SkipValidateParams` / `RejectUnknownQueryParameters` (remove or
add a 422), `Middlewares` (write any status without passing through `toHTTPError`), `RequestBody`
and `Hidden`. Add a field to `operation` only once it is clear it changes none of the published
statuses, and copy it in `register` (`TestRegisterBuildsTheHumaOperation` pins that copy). Tags
are not a field yet: every operation is tagged `ShortURL`; add one with the first operation that
needs another tag.

A handler cannot return an error status the OpenAPI document does not list: every error it
returns goes through `toHTTPError`, so even a `huma.ErrorXXX` it builds itself is not in `errs`
and becomes a 500. Only `toHTTPError` returns a `huma.ErrorXXX` / `huma.NewError`. The success
status is `operation.defaultStatus`; outputs have no `Status` field, which huma would write as-is. To add a
failure: define the error in `service`, add it to `errorResponses` and to the operation's
`errs`, then add a case to `TestRegisterDeclaresEveryReachableErrorStatus`.

- **An error the operation does not list is a 500**, even if `errorResponses` knows it: its real
  status is unpublished for that operation. Its log line carries `unlisted_error=true`.
- **Any 500 a handler returns carries no detail and is logged instead**, with the request's
  `method`, `path`, `request_id` and trace attributes. huma serializes an attached error into the
  body, and the store wraps driver errors that name tables and columns. A `context.Canceled` (the
  client went away) is logged at WARN, not ERROR, but is still a 500 in the access log and span.
  huma's own 500 for a body the client cut short never reaches `toHTTPError`: it carries the read
  error (`unexpected EOF`), which names nothing on the server side.
- **`register` panics at registration rather than publish a list it cannot vouch for**, so every
  test that builds an API fails: an error `errorResponses` lacks, and an input with a `RawBody`
  field (see below).
- **The published list must be exhaustive for what the server returns.** A non-empty
  `Operation.Errors` makes huma skip its `default` response; `register` adds `default` once
  `huma.Register` returns, to the `Responses` map it handed in — huma publishes that same map —
  so the Kiota client still decodes an `ErrorModel` for a status nobody lists (a gateway's 502, a
  status a huma upgrade adds). Its content is the map huma wrote for `500`, the status every
  operation lists, shared rather than copied. `default` is that safety net, not a substitute for
  listing.
- **Statuses returned before the handler runs** (`bodyReadErrors`: 400 / 408 / 413 / 415) cannot
  come from a service error. huma returns them while reading the body, and
  `middleware.RequestDecompress` returns them for a body with a `Content-Encoding` (400 / 408 /
  413 for gzip, 415 for any other encoding), both as `ErrorModel`.
  `register` adds them when the input has a `Body` field (promoted fields count, as they do for
  huma), since that is what makes huma limit, read and parse the body. A `RawBody` gets no size
  limit and is never parsed (no 413 / 415), so it panics until someone works out its statuses and
  adds reachability cases. `register` always adds 500, which keeps `Operation.Errors` non-empty so
  huma appends 422 and 500 itself.
- **The lists cannot prove the service still returns each error.** The reachability test covers
  that direction, so every listed *error*, not just every status, needs a request that returns
  it — `ErrOriginalURLTooLong` and the body limit are both 413. It drives `httpserver.BuildAPI`,
  so the middleware's statuses count, and reads the document `BuildOpenAPISpec` publishes.
- **HTTP status stays out of `service`.** Do not implement `huma.StatusError` on service errors:
  huma would use that status directly, bypassing the operation's list and the document.
- **Resolvers are outside the funnel.** huma answers with the status of any `StatusError` a
  resolver returns, never seeing `toHTTPError`. A resolver returns `*huma.ErrorDetail` (always
  422, as `OriginalURL.Resolve` does), never `huma.ErrorXXX`.

**Expiry enforcement**: handled in `ShortURLService.Get()` in `internal/service/short_url.go`. The store layer returns raw rows; expiry is checked at the service layer.

## Layer Responsibilities

| Layer | Package | Key files |
|-------|---------|-----------|
| Entry | `cmd/minurl` | `main.go`, `server.go`, `service_factory.go`, `config.go` |
| Handler | `internal/handler` | `register.go` (operation/error plumbing), `short_url.go` (routes), `health.go` |
| Service | `internal/service` | `short_url.go`, `model.go`, `validation.go`, `id_generator.go`, `id_counter.go`, `interface.go`, `storage.go` |
| Store | `internal/store` | `sqlite.go` (SQLite), `postgres.go`, `mysql.go`, `migrations.go`, `pinger.go` |
| Test helpers | `internal/testhelpers` | In-memory fakes for unit tests |
| HTTP server | `internal/httpserver` | HTTP server lifecycle |
| Middleware | `internal/middleware` | `logging.go`, `recovery.go`, `decompress.go`, `middleware.go` (`ResponseWriter`) |
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
