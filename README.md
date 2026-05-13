# minurl

A short URL service project implemented in Go.

## Project Status

Core short URL API is implemented and running:

- Entry point: `cmd/minurl/main.go`
- Runtime behavior:
	- Runs HTTP API server by default on `:8888`
	- Provides CLI subcommands: `openapi`, `version`, `healthcheck`
- Storage backend: SQLite (`sqlite3://`) or PostgreSQL (`postgres://`), selected via `--storage-dsn`
- Both short URL records and `id counter` are persisted in the configured backend
- Container build target binary: `minurl`

## Database migrations

SQLite and PostgreSQL use embedded `golang-migrate` migrations. New databases are migrated automatically on startup.

## API Documentation

API details are maintained in OpenAPI files under `docs/openapi/`:

- `docs/openapi/openapi.yaml`
- `docs/openapi/openapi.json`

Online viewer:
[https://min0625.github.io/openapi-viewer/?url=https://raw.githubusercontent.com/min0625/minurl/refs/heads/main/docs/openapi/openapi.yaml](https://min0625.github.io/openapi-viewer/?url=https://raw.githubusercontent.com/min0625/minurl/refs/heads/main/docs/openapi/openapi.yaml)

### API Endpoints

**Create a short URL**
(`id` is optional. If omitted, the server auto-generates one.)
(`expire_time` is optional. If omitted or null, the URL is permanent.)
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

> Returns `404 Not Found` if the short URL does not exist or has expired.

**Redirect to original URL**
```
GET /api/v1/urls/{id}:redirect

Response: 302 Found
Location: https://example.com/very/long/url
```

> Returns `404 Not Found` if the short URL does not exist or has expired.

## Health Check Endpoints

MinURL exposes three health check endpoints for use with container orchestration and monitoring tools. These endpoints are **not** part of the OpenAPI spec — they are infrastructure endpoints, not business API.

| Endpoint | Purpose | Checks |
|----------|---------|--------|
| `GET /livez` | Liveness — is the process alive? | HTTP server responds |
| `GET /readyz` | Readiness — can traffic be served? | DB `PingContext` |
| `GET /startupz` | Startup — has initialization completed? | Same as `/readyz` |

All endpoints return JSON (`{"status":"up"}` / `{"status":"down","details":{...}}`) with HTTP 200 or 503.

**`minurl healthcheck` CLI command** — for use as a Docker `HEALTHCHECK` in distroless containers (no `curl`/`wget` available):

```
minurl healthcheck [--addr http://localhost:8888]
```

Exits 0 if `/livez` returns 200, exits 1 otherwise.

## Short URL Expiry

Short URLs support an optional `expire_time` field (RFC 3339 / ISO 8601 UTC):

- **Omitted or `null`**: the URL is **permanent** and never expires.
- **Set to a future time**: the URL is valid until that moment.
- **Set to a past time** (or once the time has passed): the URL is treated as if it does not exist — both `GET` metadata and `:redirect` return `404 Not Found`.

Existing data in the database (rows without `expire_time`) are automatically treated as permanent.

## Short URL ID format

Auto-generated short IDs are Base58 strings using the alphabet:
`123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz`.

- Auto-generated IDs are 6–12 characters long.
- The first 6 characters encode a Feistel-permuted low 32-bit sequence.
- Longer IDs append an unpadded Base58 suffix derived from the upper 32 bits.
- This preserves compact 6-char IDs for the first 2^32 entries while extending capacity safely beyond 2^32 entries (up to the uint64 limit).

Custom `id` values are also validated against the same Base58 character rules and maximum length.

## HTTP Debug Requests

Reusable REST Client examples are available at:

- `docs/http/minurl.http`

This file is intended for local/manual API debugging (similar to a lightweight Postman collection), and includes:

- shared variables (e.g. base URL)
- create/get request flow
- a 404 not found example

## Tech Stack

- Language: Go 1.26.2
- Module: `github.com/min0625/minurl`
- Container: multi-stage Docker build + distroless runtime

## Local Development

### Run directly

```bash
go run ./cmd/minurl
```

### CLI commands (Cobra)

This project uses Cobra for command-line parsing.

```bash
go run ./cmd/minurl --help
go run ./cmd/minurl openapi --help
go run ./cmd/minurl version
```

Global options:

- `--config`: path to a configuration file (applies to all commands)
- `--http-addr`: HTTP listen address (default `:8888`)
- `--id-seed`: deterministic seed for ID key derivation (uint32, decimal or 0x hex; empty means built-in default seed)
- `--storage-dsn`: storage DSN; `sqlite3://path` for SQLite (default `sqlite3://minurl.sqlite3`) or `postgres://...` for PostgreSQL
- `--log-format`: log output format — `text` (default) or `json`
- `--otel-enabled`: enable OpenTelemetry tracing (default `false`)
- `--otel-service-name`: OpenTelemetry service name (default `minurl`)
- `--otel-exporter`: OpenTelemetry exporter — `stdout` (default) or `otlp`
- `--otel-endpoint`: OTLP collector endpoint (required when `--otel-exporter=otlp`)
- `--otel-insecure`: allow insecure OTLP connection (default `true`)
- `--db-max-open-conns`: max open DB connections, PostgreSQL only (default `25`, `0` = unlimited)
- `--db-max-idle-conns`: max idle DB connections in pool, PostgreSQL only (default `5`)
- `--db-conn-max-lifetime`: max connection lifetime, PostgreSQL only (default `30m`, `0` = no limit)
- `--db-conn-max-idle-time`: max connection idle time, PostgreSQL only (default `10m`, `0` = no limit)

Configuration precedence is:

1. CLI flags
2. Environment variables
3. Configuration file
4. Built-in defaults

### Configuration (flag / env / file)

This project uses Cobra + Viper to support unified configuration via CLI flags,
environment variables, and config file.

Environment variable names:

- `MINURL_HTTP_ADDR`
- `MINURL_ID_SEED`
- `MINURL_STORAGE_DSN`
- `MINURL_LOG_FORMAT`
- `MINURL_OTEL_ENABLED`
- `MINURL_OTEL_SERVICE_NAME`
- `MINURL_OTEL_EXPORTER`
- `MINURL_OTEL_ENDPOINT`
- `MINURL_OTEL_INSECURE`
- `MINURL_DB_MAX_OPEN_CONNS`
- `MINURL_DB_MAX_IDLE_CONNS`
- `MINURL_DB_CONN_MAX_LIFETIME`
- `MINURL_DB_CONN_MAX_IDLE_TIME`

Example (env):

```bash
MINURL_HTTP_ADDR=:9090 MINURL_ID_SEED=12345 MINURL_STORAGE_DSN=sqlite3://minurl.sqlite3 go run ./cmd/minurl
```

PostgreSQL example (env):

```bash
# Note: sslmode=disable is for local development only; use sslmode=require (or verify-full) in production.
MINURL_STORAGE_DSN="postgres://localhost:5432/minurl?sslmode=disable" go run ./cmd/minurl
```

PostgreSQL example (flags):

```bash
# Note: sslmode=disable is for local development only; use sslmode=require (or verify-full) in production.
go run ./cmd/minurl --storage-dsn "postgres://localhost:5432/minurl?sslmode=disable"
```

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

### DB connection pool configuration

Connection pool settings apply to the **PostgreSQL backend only**. SQLite always uses a single connection.

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--db-max-open-conns` | `MINURL_DB_MAX_OPEN_CONNS` | `25` | Max open connections. `0` = unlimited (not recommended). |
| `--db-max-idle-conns` | `MINURL_DB_MAX_IDLE_CONNS` | `5` | Max idle connections retained. `0` = none retained. |
| `--db-conn-max-lifetime` | `MINURL_DB_CONN_MAX_LIFETIME` | `30m` | Max connection lifetime. `0` = no limit. |
| `--db-conn-max-idle-time` | `MINURL_DB_CONN_MAX_IDLE_TIME` | `10m` | Max idle connection lifetime. `0` = no limit. |

**Tuning guidelines**:
- Typical production PostgreSQL: `max-open-conns=25`, `max-idle-conns=5`, `lifetime=30m`, `idle-time=10m`
- High-concurrency (many parallel requests): increase `max-open-conns` proportionally to your DB's `max_connections` and number of service instances
- Set `conn-max-lifetime` to avoid connections being closed by the DB server's idle timeout

Via config file:
```yaml
db-max-open-conns: 25
db-max-idle-conns: 5
db-conn-max-lifetime: "30m"
db-conn-max-idle-time: "10m"
```

Example (flags):

```bash
go run ./cmd/minurl --http-addr :9090 --id-seed 12345 --storage-dsn sqlite3://./data/minurl.sqlite3
```

Create a local config from the example:

```bash
cp config.example.yaml config.yaml
```

Then edit `config.yaml` as needed, for example:

```yaml
http-addr: ":9090"
storage-dsn: "sqlite3://./data/minurl.sqlite3"
id-seed: "12345"
log-format: "json"
otel-enabled: false
otel-service-name: "minurl"
otel-exporter: "stdout"
otel-endpoint: ""
otel-insecure: true
db-max-open-conns: 25
db-max-idle-conns: 5
db-conn-max-lifetime: "30m"
db-conn-max-idle-time: "10m"
```

Then run:

```bash
go run ./cmd/minurl --config config.yaml
```

Version metadata can be injected at build time via `ldflags`:

```bash
go run -ldflags "-X github.com/min0625/minurl/cmd/minurl.version=v1.0.0 -X github.com/min0625/minurl/cmd/minurl.commit=$(git rev-parse --short HEAD)" ./cmd/minurl version
```

In CI release pipelines, you can pass tag/commit like this:

```bash
mkdir -p bin
go build -ldflags "-s -w -X github.com/min0625/minurl/cmd/minurl.version=${GIT_TAG} -X github.com/min0625/minurl/cmd/minurl.commit=${GIT_COMMIT}" -o bin/minurl ./cmd/minurl
./bin/minurl version
```

Or use the make target:

```bash
make build
./bin/minurl version
```

### Build and run with Docker

```bash
make docker-build
make docker-run
```

`make docker-run` uses persistent volume defaults:

- port mapping: `8888:8888`
- volume mapping: `minurl-data:/data`
- SQLite path in container: `/data/minurl.sqlite3`

You can override volume mapping:

```bash
make docker-run DOCKER_VOLUME=/absolute/host/path:/data
```

You can also override port mapping:

```bash
make docker-run DOCKER_PORT=9090:8888
```

### Deployment with Docker Compose

Example Docker Compose configurations are available in `deploy/docker-compose/`:

- `docker-compose.postgres.example.yml` — PostgreSQL backend with nginx
- `docker-compose.sqlite.example.yml` — SQLite backend with nginx

#### Setup

1. Copy the example file for your chosen backend:

```bash
# For PostgreSQL:
cp deploy/docker-compose/docker-compose.postgres.example.yml deploy/docker-compose/docker-compose.postgres.yml

# For SQLite:
cp deploy/docker-compose/docker-compose.sqlite.example.yml deploy/docker-compose/docker-compose.sqlite.yml
```

2. Edit the copied file and customize environment variables:
   - **PostgreSQL**: Set `POSTGRES_PASSWORD`, `POSTGRES_USER`, and ensure the DSN in `MINURL_STORAGE_DSN` uses appropriate `sslmode` (see notes below)
   - **SQLite**: Adjust `MINURL_STORAGE_DSN` if needed

3. Start services:

```bash
# PostgreSQL:
docker-compose -f deploy/docker-compose/docker-compose.postgres.yml up

# SQLite:
docker-compose -f deploy/docker-compose/docker-compose.sqlite.yml up
```

#### Security Notes

- **PostgreSQL credentials**: The examples include default credentials (`minurl:minurl`) for local development. **In production**, replace with secure, randomly generated credentials. Consider using Docker secrets or an external secrets manager.
- **SSL/TLS for PostgreSQL**:
  - **Development**: `sslmode=disable` is acceptable for local-only setups.
  - **Production**: Always use `sslmode=require` or `sslmode=verify-full` to enforce encrypted connections.
  - The example file includes comments on how to configure this per environment.

### Deployment with Kubernetes

Example Kubernetes manifests are available in `deploy/kubernetes/`:

- `minurl-sqlite.example.yaml` — SQLite backend (single replica, PVC)
- `minurl-postgres.example.yaml` — PostgreSQL backend (multi-replica)

#### Setup

1. Build and push your image:

```bash
docker build -t <your-registry>/minurl:latest .
docker push <your-registry>/minurl:latest
```

2. Copy the example file for your chosen backend and update the image field:

```bash
# For PostgreSQL:
cp deploy/kubernetes/minurl-postgres.example.yaml deploy/kubernetes/minurl-postgres.yaml

# For SQLite:
cp deploy/kubernetes/minurl-sqlite.example.yaml deploy/kubernetes/minurl-sqlite.yaml
```

3. Apply to your cluster:

```bash
# PostgreSQL:
kubectl apply -f deploy/kubernetes/minurl-postgres.yaml

# SQLite:
kubectl apply -f deploy/kubernetes/minurl-sqlite.yaml
```

#### Notes

- **SQLite**: requires `replicas: 1` due to `ReadWriteOnce` PVC. For horizontal scaling, use PostgreSQL.
- **PostgreSQL**: the manifest does **not** include a PostgreSQL deployment. Use a managed database service or a separate Postgres StatefulSet. Create the `minurl-postgres` Secret with your DSN before applying (see USAGE comment in the manifest).
- **Secrets**: never commit real credentials. Use `kubectl create secret` or a secrets manager.

### Observability (OpenTelemetry)

The server supports OpenTelemetry distributed tracing. It is disabled by default.

Enable with stdout exporter (prints traces to stdout):

```bash
MINURL_OTEL_ENABLED=true go run ./cmd/minurl
```

Enable with OTLP exporter (e.g. sending to a local Jaeger collector):

```bash
MINURL_OTEL_ENABLED=true \
MINURL_OTEL_EXPORTER=otlp \
MINURL_OTEL_ENDPOINT=localhost:4317 \
MINURL_OTEL_INSECURE=true \
go run ./cmd/minurl
```

Or via flags:

```bash
go run ./cmd/minurl \
  --otel-enabled \
  --otel-exporter otlp \
  --otel-endpoint localhost:4317 \
  --otel-insecure
```

### Export OpenAPI docs

Generate OpenAPI files directly from the app contract (no server startup required):

```bash
go run ./cmd/minurl openapi
```

This writes:

- `docs/openapi/openapi.json`
- `docs/openapi/openapi.yaml`

Or use Make targets:

```bash
make openapi
```

By default:

- Image name: `minurl`
- Tag: current git tag (if exact tag exists) or short commit SHA
- Docker build injects metadata into binary with `LDFLAGS` in `Makefile`

## Quality and Checks

Run these commands during development:

```bash
make fix
make lint
make test
make check
```

What they do:

- `fix`: tidy modules and apply linter auto-fixes
- `lint`: run `golangci-lint`
- `test`: run race-enabled Go tests
- `check`: run tidy diff, lint, tests, and OpenAPI source consistency check

If `check` reports OpenAPI docs are out of date, run:

```bash
make openapi
```

then commit updates under `docs/openapi/`.

## Repository Structure

```text
.
|-- cmd/
|   |-- example-minurl-client/  # Example Kiota-generated Go client usage
|   |   `-- main.go
|   `-- minurl/                 # Main entry point and wiring
|       |-- main.go
|       |-- config.go           # Configuration loading (Viper)
|       |-- config_bind.go      # Flag/env binding helpers
|       |-- middleware.go       # HTTP middleware (logging, recovery, decompression)
|       |-- server.go           # HTTP server startup and routing
|       |-- service_factory.go  # Storage backend detection and service wiring
|       |-- telemetry.go        # OpenTelemetry initialization
|       |-- command_openapi.go  # `openapi` subcommand
|       `-- command_version.go  # `version` subcommand
|-- docs/
|   |-- http/
|   |   `-- minurl.http         # REST Client debug request examples
|   `-- openapi/
|       |-- openapi.json
|       `-- openapi.yaml
|-- internal/
|   |-- handler/                # HTTP route handlers
|   |   |-- short_url.go
|   |   `-- short_url_test.go
|   |-- service/                # Business logic
|   |   |-- short_url.go
|   |   |-- short_url_test.go
|   |   |-- id_generator.go
|   |   |-- id_generator_test.go
|   |   `-- id_counter.go
|   |-- store/                  # Persistence (SQLite and PostgreSQL)
|   |   |-- sqlite.go
|   |   |-- sqlite_test.go
|   |   |-- postgres.go
|   |   |-- postgres_test.go
|   |   |-- storage.go
|   |   `-- counter.go
|   |-- model/                  # Domain types
|   |   `-- short_url.go
|   `-- testhelpers/            # Shared test utilities
|       |-- helpers.go
|       |-- counter.go
|       |-- id_generator.go
|       `-- storage.go
|-- pkg/
|   `-- kiota/go/gen/           # Kiota-generated API client
|-- deploy/
|   |-- docker-compose/         # Docker Compose examples (PostgreSQL and SQLite)
|   `-- kubernetes/             # Kubernetes manifest examples (PostgreSQL and SQLite)
|-- go.mod
|-- Dockerfile
|-- Makefile
|-- config.example.yaml
`-- LICENSE
```

## Next Suggested Milestones

1. ✅ Define URL entity and storage interface.
2. ✅ Add HTTP server and routing.
3. ✅ Implement create and get short URL endpoints.
4. ✅ Add tests and error handling.
5. ✅ Add redirect endpoint (`GET /api/v1/urls/{id}:redirect` → `302` to original URL).
6. ✅ Add custom alias support (optional `id` field on create).
7. ✅ Add pluggable database backends (SQLite and PostgreSQL).
8. ✅ Add OpenTelemetry tracing (stdout and OTLP exporters).

## License

Apache License 2.0. See `LICENSE`.
