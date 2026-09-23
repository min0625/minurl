# minurl

A short URL service written in Go. It creates, fetches, and redirects short URLs, backed by
SQLite, PostgreSQL, or MySQL.

## Table of Contents

- [Quick Start](#quick-start)
- [Overview](#overview)
- [API](#api)
  - [Endpoints](#endpoints)
  - [Create request fields](#create-request-fields)
  - [Short URL expiry](#short-url-expiry)
  - [Short URL ID format](#short-url-id-format)
  - [Error responses](#error-responses)
- [Health Check Endpoints](#health-check-endpoints)
- [Configuration](#configuration)
- [Storage](#storage)
  - [Supported databases](#supported-databases)
  - [Storage DSN and SSL configuration](#storage-dsn-and-ssl-configuration)
  - [MySQL DSN query parameters](#mysql-dsn-query-parameters)
  - [MySQL `original_url` length limit](#mysql-original_url-length-limit)
  - [DB connection pool configuration](#db-connection-pool-configuration)
  - [Database migrations](#database-migrations)
- [Deployment](#deployment)
  - [Docker](#docker)
  - [Docker Compose](#docker-compose)
  - [Kubernetes](#kubernetes)
- [Observability (OpenTelemetry)](#observability-opentelemetry)
- [Development](#development)
- [Repository Structure](#repository-structure)
- [License](#license)

## Quick Start

```bash
git clone https://github.com/min0625/minurl.git
cd minurl
go run ./cmd/minurl
```

This starts the HTTP API on `:8888` using a local SQLite database (`sqlite3://minurl.sqlite3`) — no extra setup required. Then, in another terminal:

```bash
# Create a short URL
curl -X POST http://localhost:8888/api/v1/urls \
  -H "Content-Type: application/json" \
  -d '{"original_url": "https://github.com/min0625"}'

# Use the returned "id" to redirect to the original URL (replace <id> with the real value)
curl -i "http://localhost:8888/api/v1/urls/<id>:redirect"
```

## Overview

- Go 1.26.8, module `github.com/min0625/minurl`, entry point `cmd/minurl/main.go`
- Runs the HTTP API on `:8888` by default; CLI subcommands `openapi`, `version`, `healthcheck` (Cobra)
- Storage backend selected by the `--storage-dsn` scheme: `sqlite3://`, `postgres://`, or `mysql://`.
  Both short URL records and the ID counter are persisted there
- Container: multi-stage Docker build + distroless runtime, binary `minurl`

## API

The OpenAPI document is the reference: `docs/openapi/openapi.yaml` and `docs/openapi/openapi.json`
([online viewer](https://redocly.github.io/redoc/3.x/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmin0625%2Fminurl%2Frefs%2Fheads%2Fmain%2Fdocs%2Fopenapi%2Fopenapi.yaml&nocors)).

### Endpoints

**Create a short URL**

```
POST /api/v1/urls
Content-Type: application/json

{
  "original_url": "https://example.com/very/long/url",
  "id": "myshort",
  "expire_time": "2099-01-01T00:00:00Z"
}

Response: 200 OK
{
  "id": "myshort",
  "original_url": "https://example.com/very/long/url",
  "expire_time": "2099-01-01T00:00:00Z",
  "create_time": "2026-04-28T16:00:00Z"
}
```

**Get short URL metadata**

```
GET /api/v1/urls/{id}

Response: 200 OK
{
  "id": "myshort",
  "original_url": "https://example.com/very/long/url",
  "expire_time": "2099-01-01T00:00:00Z",
  "create_time": "2026-04-28T16:00:00Z"
}
```

**Redirect to original URL**

```
GET /api/v1/urls/{id}:redirect

Response: 302 Found
Location: https://example.com/very/long/url
```

Both `GET` endpoints:

- return `404 Not Found` if the short URL does not exist or has expired;
- apply the same `original_url` rules to stored data, so a short URL whose target does not
  satisfy them — a row written before those rules existed, or straight to the database —
  returns `404 Not Found` too. Each such request logs a `WARN`
  `stored original URL is not a valid http(s) URL` with the row's `id`;
- also answer `HEAD`, with the status and headers of the `GET` (`Location` included) and no
  body, so `curl -I` and link checkers can check a short URL. The OpenAPI document lists only
  the `GET`.

### Create request fields

Keys are case-sensitive: `Original_URL` or `ID` is an unexpected property and returns
`422 Unprocessable Entity`, even alongside the correctly spelled key.

**`original_url`** (required) must be an absolute `http`/`https` URL with a host, without
embedded credentials, and without whitespace, control or invisible formatting characters
(`U+200B`, `U+202E`, `U+FEFF`, …) — all of those have to be percent-encoded. Sent raw, they
either break the `Location` header or hide and reorder what the target reads as. A literal
replacement character (`U+FFFD`) is rejected for a different reason: it is what an invalid
UTF-8 byte decodes to, so storing it would serve a `Location` other than the one sent.
Percent-encode it (`%EF%BF%BD`) to store one deliberately.

- A URL that breaks any of these rules — `javascript:`, `data:`, `file:`, `ftp:`,
  `//example.com`, `https://user@example.com/`, a literal space — returns `422`.
- The allowlist covers the URL scheme and embedded credentials only. It does not restrict
  which host a short URL may point at: private and loopback addresses, cloud metadata
  endpoints and internationalized domains are all accepted. MinURL never fetches the URL
  itself, so this is a redirect target, not a server-side request.
- It has no length limit of its own; the request body must stay under 1 MiB. On MySQL a URL
  over 65,535 bytes returns `413` (see [MySQL `original_url` length limit](#mysql-original_url-length-limit)).

**`id`** (optional) — omit it or send `""`, and the server auto-generates one. `null` is
accepted the same way, but the OpenAPI document declares `id` as a plain string: the same
schema describes responses, which always carry one. See [Short URL ID format](#short-url-id-format).

**`expire_time`** (optional) — see [Short URL expiry](#short-url-expiry).

### Short URL expiry

`expire_time` is RFC 3339 / ISO 8601 UTC:

- **Omitted or `null`**: the URL is **permanent** and never expires.
- **Set to a future time**: the URL is valid until that moment.
- **Set to a past time** (or once the time has passed): the URL is treated as if it does not exist — both `GET` metadata and `:redirect` return `404 Not Found`.

Existing rows without `expire_time` are treated as permanent.

### Short URL ID format

IDs are Base58 strings using the alphabet
`123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz`.

- Auto-generated IDs are 6–12 characters long.
- The first 6 characters encode a Feistel-permuted low 32-bit sequence.
- Longer IDs append an unpadded Base58 suffix derived from the upper 32 bits.
- This preserves compact 6-char IDs for the first 2^32 entries while extending capacity safely beyond 2^32 entries (up to the uint64 limit).

Custom `id` values are validated against the same alphabet: up to 12 characters, case-sensitive, and
anything else (including `0`, `O`, `I`, `l`) returns `422 Unprocessable Entity`. The same rule
applies to the `{id}` path parameter, so a malformed ID is a `422`, not a `404`.

### Error responses

Errors use the `ErrorModel` body (`application/problem+json`), and the OpenAPI document lists
every status each of these operations returns:

| Status | Endpoints | When |
|--------|-----------|------|
| `400 Bad Request` | create | The body is missing, is not valid JSON, or is a corrupt gzip stream |
| `404 Not Found` | get, redirect | The short URL does not exist, has expired, or its stored target breaks the `original_url` rules |
| `408 Request Timeout` | create | The body, gzip or not, was not received within 5 seconds |
| `409 Conflict` | create | The requested `id` is already taken |
| `413 Request Entity Too Large` | create | The body is 1 MiB or larger, as sent or after gzip decompression, or `original_url` is too long for the storage backend |
| `415 Unsupported Media Type` | create | The `Content-Type` is not JSON, or the `Content-Encoding` is not `gzip` |
| `422 Unprocessable Entity` | all | The body or `{id}` parses but breaks a schema rule |
| `500 Internal Server Error` | all | The server failed, e.g. the database is unreachable |

- A request that fails validation is a `422`; `400` is reserved for a body that cannot be
  parsed at all.
- A `404`, a `409`, a `413` for a URL too long for storage and a `500` carry `detail` alone,
  with no `errors`. A `500` says only `Internal Server Error`; the cause is written to the
  server log, a server panic included. One `500` differs: a body the client cut short carries
  the read error in `errors`.
- A request that matches no operation is answered by the router rather than the API, still as
  an `ErrorModel`: another method on one of these paths gets a `405` with an `Allow` header,
  an unrouted path a `404`.
- The document also declares a `default` `ErrorModel` response, so a generated client decodes
  any other status, e.g. from a proxy, the same way.
- Only a request that `net/http` refuses before routing gets no `ErrorModel`, but `text/plain`
  or no body, e.g. request headers over 1 MiB (`431`) or a malformed request line (`400`).

## Health Check Endpoints

These infrastructure endpoints are for container orchestration and monitoring, and are **not**
part of the OpenAPI spec.

| Endpoint | Purpose | Checks |
|----------|---------|--------|
| `GET /livez` | Liveness — is the process alive? | HTTP server responds |
| `GET /readyz` | Readiness — can traffic be served? | DB `PingContext` |
| `GET /startupz` | Startup — has initialization completed? | Same as `/readyz` |

All endpoints return JSON with HTTP 200 when up and 503 when down. `/livez` returns
`{"status":"up"}`; `/readyz` and `/startupz` add the database check under `details` either way
(`{"status":"up","details":{"database":{...}}}`).

**`minurl healthcheck`** — for use as a Docker `HEALTHCHECK` in distroless containers (no `curl`/`wget` available).
Exits 0 if `/livez` returns 200, exits 1 otherwise:

```
minurl healthcheck [--addr http://localhost:8888]
```

## Configuration

Every option can be set by CLI flag, environment variable, or config file (Cobra + Viper).
Precedence: **CLI flags > environment variables > config file > built-in defaults**.
A value that does not parse fails startup rather than falling back to `0` or `false`, so
`MINURL_DB_MAX_OPEN_CONNS=abc` or `MINURL_OTEL_ENABLED=yes` is an error. Booleans take
`true` / `false` (or `1` / `0`). Integers (`--id-seed`, `--db-max-*-conns`) accept
decimal `25`, hex `0x19`, binary `0b11001`, `_` separators (`1_000`), and octal with `0o` or a
**leading `0`: `010` is 8, not 10**. `08`, `25.9` and `1e3` are errors. Surrounding
whitespace in an env var, such as the trailing newline of a Kubernetes Secret, is ignored.

In the config file, an unquoted number is decoded by YAML first. It agrees on `010`, `0x19`,
`0b11001` and `1_000`, but reads anything else that looks like a number as a float: `08` is 8,
`25.0` is 25 and `1e3` is 1000 there, while `25.9` is still an error. Quote the value (`"010"`)
to get the env var rules exactly.

The config file must be YAML (`.yaml` or `.yml`). Its keys are the flag names
without `--`, as in `config.example.yaml` (`db-max-open-conns: 25`), and each takes a single
value. An unknown key (including a nested `db:` / `  max-open-conns: 25` or a dotted
`db.max-open-conns`), a key that is not lowercase (`HTTP-Addr`), a list or mapping as a value, or
a key given twice fails startup, and the error names the line and the key
(`line 3: unknown key "idseed"`), so a typo cannot silently leave the default in place. A known key left empty (`db-max-open-conns:`) sets nothing. YAML
anchors and aliases (`&name` / `*name`) work; merge keys (`<<`) do not.

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--config` | — | (none) | Path to a YAML configuration file (`.yaml` or `.yml`); read by the server only, not by `openapi`, `version` or `healthcheck` |
| `--http-addr` | `MINURL_HTTP_ADDR` | `:8888` | HTTP listen address |
| `--id-seed` | `MINURL_ID_SEED` | (built-in default seed) | Deterministic seed for ID key derivation (uint32 integer, e.g. `12345` or `0x3039`) |
| `--storage-dsn` | `MINURL_STORAGE_DSN` | `sqlite3://minurl.sqlite3` | Storage DSN — see [Storage DSN and SSL configuration](#storage-dsn-and-ssl-configuration) |
| `--log-format` | `MINURL_LOG_FORMAT` | `text` | Log output format — `text` or `json` |
| `--otel-enabled` | `MINURL_OTEL_ENABLED` | `false` | Enable OpenTelemetry tracing |
| `--otel-service-name` | `MINURL_OTEL_SERVICE_NAME` | `minurl` | OpenTelemetry service name |
| `--otel-exporter` | `MINURL_OTEL_EXPORTER` | `stdout` | OpenTelemetry exporter — `stdout` or `otlp` |
| `--otel-endpoint` | `MINURL_OTEL_ENDPOINT` | (empty) | OTLP collector endpoint (required when `--otel-exporter=otlp`) |
| `--otel-insecure` | `MINURL_OTEL_INSECURE` | `true` | Allow insecure OTLP connection |
| `--db-*` | `MINURL_DB_*` | | Connection pool — see [DB connection pool configuration](#db-connection-pool-configuration) |

```bash
# Flags
go run ./cmd/minurl --http-addr :9090 --id-seed 12345 --storage-dsn sqlite3://./data/minurl.sqlite3

# Env
MINURL_HTTP_ADDR=:9090 MINURL_ID_SEED=12345 MINURL_STORAGE_DSN=sqlite3://minurl.sqlite3 go run ./cmd/minurl

# Config file: keys are the flag names without `--`; config.example.yaml documents every one
cp config.example.yaml config.yaml
go run ./cmd/minurl --config config.yaml
```

Run `go run ./cmd/minurl --help` (or `<subcommand> --help`) for the full CLI reference.

## Storage

### Supported databases

| Backend | Minimum | Verified | Notes |
|---------|---------|----------|-------|
| SQLite | — | 3.53.0 | Embedded via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) — no external server, and the version is pinned by `go.mod`. The schema uses UPSERT (`ON CONFLICT ... DO NOTHING`), which needs SQLite 3.24+; the embedded build is well past that. |
| PostgreSQL | 9.5 | 11, 13, 17 | 9.5 is the floor for `ON CONFLICT ... DO NOTHING`. |
| MySQL | **8.0** | 8.0, 8.4 | The `short_urls.id` column uses the `utf8mb4_0900_as_cs` collation so that IDs are case-sensitive. **MySQL 5.7 fails to start** with `Error 1273 (HY000): Unknown collation: 'utf8mb4_0900_as_cs'`. |
| MariaDB | — | 11.4 | Not officially supported and not covered by CI, but 11.4 runs the full API correctly, including case-sensitive IDs — it accepts `utf8mb4_0900_as_cs` as an alias. Use at your own risk. |

CI runs the integration suite against `postgres:17-alpine` and `mysql:8.4`; the other
versions in the "Verified" column were checked by hand.

### Storage DSN and SSL configuration

MinURL auto-detects the storage backend from the DSN scheme.

**SQLite** (development and small deployments):

```
sqlite3://minurl.sqlite3              relative path
sqlite3://var/data/minurl.sqlite3     relative subdirectory
sqlite3:///absolute/path/minurl.db    absolute path (three slashes)
```

**PostgreSQL** — choose `sslmode` appropriate for your environment:

| sslmode | When to use |
|---------|------------|
| `disable` | Local development / loopback only. **Never use in production.** |
| `require` | SSL required, server certificate **not** verified. Protects against passive eavesdropping only; does not prevent MITM attacks. |
| `verify-ca` | SSL required, CA signature verified. Acceptable for internal networks with a private CA. |
| `verify-full` | SSL required, CA + hostname verified. **Recommended for production.** |

> **Warning**: When the server detects `sslmode=disable` in the PostgreSQL DSN, it logs a warning at startup. Do not ignore this warning in production.

```bash
# Production
MINURL_STORAGE_DSN="postgres://user:password@db.example.com:5432/minurl?sslmode=verify-full"

# Staging / private network with known CA
MINURL_STORAGE_DSN="postgres://user:password@db.example.com:5432/minurl?sslmode=require"

# Local development only
MINURL_STORAGE_DSN="postgres://user:password@localhost:5432/minurl?sslmode=disable"
```

**MySQL** — use the `tls` query parameter to control encryption:

| tls | When to use |
|-----|-------------|
| omitted / `false` | Local development / loopback only. **Never use in production.** |
| `skip-verify` | TLS required, server certificate **not** verified. |
| `true` | TLS required with system CA verification. **Recommended for production.** |

A private CA requires registering a named TLS config in code via `mysql.RegisterTLSConfig`;
minurl does not do this today, so a DSN using an unregistered name (for example `tls=custom`)
is rejected at startup.

```bash
# Production
MINURL_STORAGE_DSN="mysql://user:password@db.example.com:3306/minurl?tls=true"

# Local development only
MINURL_STORAGE_DSN="mysql://user:password@localhost:3306/minurl"
```

### MySQL DSN query parameters

Query parameters in a `mysql://` DSN are sent to the server as session variables
(`SET name = value`), so `?sql_mode=...` works as expected. Driver-level connection flags are
**not** accepted — neither the dangerous ones (`multiStatements`, `interpolateParams`,
`charset`, `allowCleartextPasswords`, …) nor the harmless ones (`timeout`, `readTimeout`,
`maxAllowedPacket`, …): the server rejects them with `Unknown system variable` and the
process refuses to start. Two parameters are handled by the service instead:

- `tls` is a real connection setting and is mapped explicitly (see
  [Storage DSN and SSL configuration](#storage-dsn-and-ssl-configuration)).
- `parseTime` and `loc` are **ignored without error**: times are always parsed as
  `time.Time` in UTC, and a caller-supplied value would break that.

This is not the attribute vocabulary of [MySQL's own URI-like connection strings](https://dev.mysql.com/doc/refman/8.0/en/connecting-using-uri-or-key-value-pairs.html),
which reserve the query string for connection attributes (`ssl-mode`, `connect-timeout`,
`compression`, …) and do not allow server variables there at all. MinURL borrows the shape
of that URI, not its attributes: write `?tls=true`, not `?ssl-mode=REQUIRED` — the latter is
sent as a session variable and fails the connection (a hyphenated name surfaces as a SQL
syntax error rather than `Unknown system variable`).

The rule above is MySQL-only. A `postgres://` DSN is handed to
[pgx](https://github.com/jackc/pgx) untouched, so every libpq parameter (`sslmode`,
`application_name`, `options`, …) takes effect and an unknown one fails the connection. A
`sqlite3://` DSN's query string is appended to the SQLite URI, so driver parameters such as
`_pragma=` and `mode=` take effect — including `?mode=memory`, which starts cleanly and then
loses every short URL on restart.

### MySQL `original_url` length limit

`original_url` is stored in a MySQL `TEXT` column, which holds 65,535 bytes. A longer URL
is rejected with `413 Request Entity Too Large` before the insert, rather than being
silently truncated — MySQL only raises an error for an over-length value when the server
runs in strict SQL mode. SQLite and PostgreSQL have no such column limit, so the same
request succeeds there.

The request body itself must stay under 1 MiB on every backend, so that is the practical
ceiling for a URL — at 1 MiB the same `413` comes from the HTTP layer instead of the store.

### DB connection pool configuration

Connection pool settings apply to the **PostgreSQL and MySQL backends**. SQLite always uses a single connection.

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--db-max-open-conns` | `MINURL_DB_MAX_OPEN_CONNS` | `25` | Max open connections. `0` = unlimited (not recommended). |
| `--db-max-idle-conns` | `MINURL_DB_MAX_IDLE_CONNS` | `5` | Max idle connections retained. `0` = none retained. |
| `--db-conn-max-lifetime` | `MINURL_DB_CONN_MAX_LIFETIME` | `30m` | Max connection lifetime. `0` = no limit. |
| `--db-conn-max-idle-time` | `MINURL_DB_CONN_MAX_IDLE_TIME` | `10m` | Max idle connection lifetime. `0` = no limit. |

**Tuning guidelines**:
- Typical production PostgreSQL: the defaults above
- High-concurrency (many parallel requests): increase `max-open-conns` proportionally to your DB's `max_connections` and number of service instances
- Set `conn-max-lifetime` to avoid connections being closed by the DB server's idle timeout

### Database migrations

All three backends use embedded `golang-migrate` migrations; a database is migrated
automatically on startup.

## Deployment

### Docker

```bash
make docker-build
make docker-run
```

- Image name: `minurl`
- Tag: current git tag with any leading `v` stripped (if an exact tag exists), else
  the short commit SHA — tag `v1.2.3` builds `minurl:1.2.3`
- The build injects version metadata into the binary via `LDFLAGS` in `Makefile`

`make docker-run` maps port `8888:8888` and volume `minurl-data:/data`, with SQLite at
`/data/minurl.sqlite3` in the container. Override either:

```bash
make docker-run DOCKER_VOLUME=/absolute/host/path:/data
make docker-run DOCKER_PORT=9090:8888
```

### Docker Compose

`deploy/docker-compose/` has one example per backend, each with nginx in front:
`docker-compose.<backend>.example.yml`, where `<backend>` is `postgres`, `mysql`, or `sqlite`.

1. Copy the example for your backend:

   ```bash
   cp deploy/docker-compose/docker-compose.postgres.example.yml deploy/docker-compose/docker-compose.postgres.yml
   ```

2. Edit the copy:
   - **PostgreSQL**: set `POSTGRES_PASSWORD`, `POSTGRES_USER`, and the `sslmode` in `MINURL_STORAGE_DSN`
   - **MySQL**: set `MYSQL_PASSWORD`, `MYSQL_USER`, `MYSQL_ROOT_PASSWORD`, and the `tls` setting in `MINURL_STORAGE_DSN`
   - **SQLite**: adjust `MINURL_STORAGE_DSN` if needed

3. Start it:

   ```bash
   docker-compose -f deploy/docker-compose/docker-compose.postgres.yml up
   ```

**Security notes**

- **Credentials**: the PostgreSQL and MySQL examples ship default credentials (`minurl:minurl`,
  plus a `rootpassword` MySQL root password) for local development. **In production**, replace
  them with secure, randomly generated credentials — consider Docker secrets or an external
  secrets manager.
- **Encryption**: `sslmode=disable` (PostgreSQL) or an omitted / `false` `tls` (MySQL) is
  acceptable for local-only setups. In production, use `sslmode=require` / `verify-full` or
  `tls=true` (see [Storage DSN and SSL configuration](#storage-dsn-and-ssl-configuration)).
  Each example file comments on how to configure this per environment.

### Kubernetes

`deploy/kubernetes/` has one manifest per backend: `minurl-<backend>.example.yaml`.

1. Build and push your image:

   ```bash
   docker build -t <your-registry>/minurl:latest .
   docker push <your-registry>/minurl:latest
   ```

2. Copy the manifest for your backend and update its `image` field:

   ```bash
   cp deploy/kubernetes/minurl-postgres.example.yaml deploy/kubernetes/minurl-postgres.yaml
   ```

3. Apply it:

   ```bash
   kubectl apply -f deploy/kubernetes/minurl-postgres.yaml
   ```

Notes:

- **SQLite**: single replica with a PVC — `replicas: 1` is required by the `ReadWriteOnce`
  PVC. For horizontal scaling, use PostgreSQL or MySQL.
- **PostgreSQL / MySQL**: multi-replica. The manifest does **not** include the database
  itself; use a managed database service or a separate StatefulSet. The manifest carries its
  own `minurl-postgres` / `minurl-mysql` Secret: set `stringData.dsn` in your copy before
  applying (see the USAGE comment in the manifest).
- **Secrets**: never commit real credentials. Use `kubectl create secret` or a secrets manager.

## Observability (OpenTelemetry)

Tracing is disabled by default. Enable it with the stdout exporter (prints traces to stdout):

```bash
MINURL_OTEL_ENABLED=true go run ./cmd/minurl
```

Or with the OTLP exporter (e.g. sending to a local Jaeger collector):

```bash
MINURL_OTEL_ENABLED=true \
MINURL_OTEL_EXPORTER=otlp \
MINURL_OTEL_ENDPOINT=localhost:4317 \
MINURL_OTEL_INSECURE=true \
go run ./cmd/minurl
```

The same via flags: `--otel-enabled --otel-exporter otlp --otel-endpoint localhost:4317 --otel-insecure`.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, make targets, coding conventions, testing,
and the PR process. Quick reference:

```bash
make fix      # tidy + lint auto-fix, then lint (fails on what --fix could not repair)
make check    # all prek hooks (tidy diff + lint + test included)
make gen      # regenerate OpenAPI docs and Kiota Go client
make ci       # full CI check (same as CI pipeline)
```

**Export OpenAPI docs** — generated from the app contract, no server startup required. Both
write `openapi.json` and `openapi.yaml` (`make openapi` does the first):

```bash
go run ./cmd/minurl openapi          # writes to docs/openapi by default
go run ./cmd/minurl openapi --out /tmp/spec
```

**Build version metadata** — injected via `ldflags` (`make build` does this into `bin/minurl`):

```bash
go build -ldflags "-s -w -X main.version=${GIT_TAG#v} -X main.commit=${GIT_COMMIT}" -o bin/minurl ./cmd/minurl
./bin/minurl version
```

**HTTP debug requests** — `docs/http/minurl.http` holds REST Client examples (a lightweight
Postman collection) sharing a base-URL variable: create / get / redirect flows, `HEAD`,
expiry, and the `404` / `405` / `422` error cases.

## Repository Structure

```text
.
├── cmd/
│   ├── minurl/                    # Main entry point and wiring
│   │   ├── main.go
│   │   ├── config.go              # Configuration loading (Viper)
│   │   ├── config_bind.go         # Flag/env binding helpers
│   │   ├── server.go              # HTTP server startup and routing
│   │   ├── service_factory.go     # Storage backend detection and service wiring
│   │   ├── command_healthcheck.go
│   │   ├── command_openapi.go
│   │   └── command_version.go
│   └── minurl-client-example/     # Example Kiota-generated Go client usage
│       └── main.go
├── docs/
│   ├── http/
│   │   └── minurl.http            # REST Client debug request examples
│   └── openapi/
│       ├── openapi.json
│       └── openapi.yaml
├── internal/
│   ├── handler/                   # HTTP route handlers
│   ├── httpserver/                # HTTP server lifecycle
│   ├── middleware/                # HTTP middleware (logging, recovery, decompression)
│   ├── service/                   # Business logic
│   ├── store/                     # Persistence (SQLite, PostgreSQL, MySQL)
│   │   └── migrations/            # Embedded SQL migration files
│   ├── telemetry/                 # OpenTelemetry initialization
│   └── testhelpers/               # Shared test utilities
├── pkg/
│   └── kiota/go/gen/              # Kiota-generated API client
├── deploy/
│   ├── docker-compose/            # Docker Compose examples (SQLite, PostgreSQL, MySQL)
│   └── kubernetes/                # Kubernetes manifests (SQLite, PostgreSQL, MySQL)
├── go.mod
├── Dockerfile
├── Makefile
├── config.example.yaml
└── LICENSE
```

## License

Apache License 2.0. See `LICENSE`.
