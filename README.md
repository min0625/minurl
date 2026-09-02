# minurl

A short URL service project implemented in Go.

## Table of Contents

- [Quick Start](#quick-start)
- [Project Status](#project-status)
- [Database migrations](#database-migrations)
- [API Documentation](#api-documentation)
  - [API Endpoints](#api-endpoints)
- [Health Check Endpoints](#health-check-endpoints)
- [Short URL Expiry](#short-url-expiry)
- [Short URL ID Format](#short-url-id-format)
- [HTTP Debug Requests](#http-debug-requests)
- [Tech Stack](#tech-stack)
- [Local Development](#local-development)
  - [Run directly](#run-directly)
  - [CLI commands (Cobra)](#cli-commands-cobra)
  - [Configuration (flag / env / file)](#configuration-flag--env--file)
  - [Storage DSN and SSL configuration](#storage-dsn-and-ssl-configuration)
  - [DB connection pool configuration](#db-connection-pool-configuration)
  - [Build version metadata](#build-version-metadata)
  - [Build and run with Docker](#build-and-run-with-docker)
  - [Deployment with Docker Compose](#deployment-with-docker-compose)
  - [Deployment with Kubernetes](#deployment-with-kubernetes)
  - [Observability (OpenTelemetry)](#observability-opentelemetry)
  - [Export OpenAPI docs](#export-openapi-docs)
- [Contributing](#contributing)
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

See [Local Development](#local-development) for PostgreSQL/MySQL setup, configuration options, and Docker/Kubernetes deployment.

## Project Status

Core short URL API is implemented and running:

- Entry point: `cmd/minurl/main.go`
- Runtime behavior:
	- Runs HTTP API server by default on `:8888`
	- Provides CLI subcommands: `openapi`, `version`, `healthcheck`
- Storage backend: SQLite (`sqlite3://`), PostgreSQL (`postgres://`), or MySQL (`mysql://`), selected via `--storage-dsn`
- Both short URL records and `id counter` are persisted in the configured backend
- Container build target binary: `minurl`

## Database migrations

SQLite, PostgreSQL, and MySQL use embedded `golang-migrate` migrations. New databases are migrated automatically on startup.

## API Documentation

API details are maintained in OpenAPI files under `docs/openapi/`:

- `docs/openapi/openapi.yaml`
- `docs/openapi/openapi.json`

Online viewer: [OpenAPI Docs](https://redocly.github.io/redoc/3.x/shorturl?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmin0625%2Fminurl%2Frefs%2Fheads%2Fmain%2Fdocs%2Fopenapi%2Fopenapi.yaml&nocors)

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

## Short URL ID Format

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

- Language: Go 1.26.8
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

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--config` | — | (none) | Path to a configuration file (applies to all commands) |
| `--http-addr` | `MINURL_HTTP_ADDR` | `:8888` | HTTP listen address |
| `--id-seed` | `MINURL_ID_SEED` | (built-in default seed) | Deterministic seed for ID key derivation (uint32, decimal or 0x hex) |
| `--storage-dsn` | `MINURL_STORAGE_DSN` | `sqlite3://minurl.sqlite3` | Storage DSN — `sqlite3://path` for SQLite, `postgres://...` for PostgreSQL, or `mysql://...` for MySQL |
| `--log-format` | `MINURL_LOG_FORMAT` | `text` | Log output format — `text` or `json` |
| `--otel-enabled` | `MINURL_OTEL_ENABLED` | `false` | Enable OpenTelemetry tracing |
| `--otel-service-name` | `MINURL_OTEL_SERVICE_NAME` | `minurl` | OpenTelemetry service name |
| `--otel-exporter` | `MINURL_OTEL_EXPORTER` | `stdout` | OpenTelemetry exporter — `stdout` or `otlp` |
| `--otel-endpoint` | `MINURL_OTEL_ENDPOINT` | (empty) | OTLP collector endpoint (required when `--otel-exporter=otlp`) |
| `--otel-insecure` | `MINURL_OTEL_INSECURE` | `true` | Allow insecure OTLP connection |

DB connection pool flags (`--db-max-open-conns`, `--db-max-idle-conns`, `--db-conn-max-lifetime`, `--db-conn-max-idle-time`) apply to the **PostgreSQL and MySQL backends** and are covered separately in [DB connection pool configuration](#db-connection-pool-configuration) below.

Configuration precedence is:

1. CLI flags
2. Environment variables
3. Configuration file
4. Built-in defaults

### Configuration (flag / env / file)

This project uses Cobra + Viper to support unified configuration via CLI flags,
environment variables, and config file. See the flag/env var table above for
the full list of names and defaults.

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

```bash
# Production
MINURL_STORAGE_DSN="mysql://user:password@db.example.com:3306/minurl?tls=true"

# Local development only
MINURL_STORAGE_DSN="mysql://user:password@localhost:3306/minurl"
```

### DB connection pool configuration

Connection pool settings apply to the **PostgreSQL and MySQL backends**. SQLite always uses a single connection.

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

### Build version metadata

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
- `docker-compose.mysql.example.yml` — MySQL backend with nginx
- `docker-compose.sqlite.example.yml` — SQLite backend with nginx

#### Setup

1. Copy the example file for your chosen backend:

```bash
# For PostgreSQL:
cp deploy/docker-compose/docker-compose.postgres.example.yml deploy/docker-compose/docker-compose.postgres.yml

# For MySQL:
cp deploy/docker-compose/docker-compose.mysql.example.yml deploy/docker-compose/docker-compose.mysql.yml

# For SQLite:
cp deploy/docker-compose/docker-compose.sqlite.example.yml deploy/docker-compose/docker-compose.sqlite.yml
```

2. Edit the copied file and customize environment variables:
   - **PostgreSQL**: Set `POSTGRES_PASSWORD`, `POSTGRES_USER`, and ensure the DSN in `MINURL_STORAGE_DSN` uses appropriate `sslmode` (see notes below)
   - **MySQL**: Set `MYSQL_PASSWORD`, `MYSQL_USER`, `MYSQL_ROOT_PASSWORD`, and ensure the DSN in `MINURL_STORAGE_DSN` uses an appropriate `tls` setting (see notes below)
   - **SQLite**: Adjust `MINURL_STORAGE_DSN` if needed

3. Start services:

```bash
# PostgreSQL:
docker-compose -f deploy/docker-compose/docker-compose.postgres.yml up

# MySQL:
docker-compose -f deploy/docker-compose/docker-compose.mysql.yml up

# SQLite:
docker-compose -f deploy/docker-compose/docker-compose.sqlite.yml up
```

#### Security Notes

- **PostgreSQL credentials**: The examples include default credentials (`minurl:minurl`) for local development. **In production**, replace with secure, randomly generated credentials. Consider using Docker secrets or an external secrets manager.
- **SSL/TLS for PostgreSQL**:
  - **Development**: `sslmode=disable` is acceptable for local-only setups.
  - **Production**: Always use `sslmode=require` or `sslmode=verify-full` to enforce encrypted connections.
  - The example file includes comments on how to configure this per environment.
- **MySQL credentials**: The example includes default credentials (`minurl:minurl`, plus a `rootpassword` root password) for local development. **In production**, replace with secure, randomly generated credentials. Consider using Docker secrets or an external secrets manager.
- **TLS for MySQL**:
  - **Development**: an omitted or `false` `tls` value is acceptable for local-only setups.
  - **Production**: Always use `tls=true` to enforce encrypted connections with system CA verification.
    A private CA requires registering a named TLS config in code via `mysql.RegisterTLSConfig`; minurl does not
    do this today, so a DSN using an unregistered name (for example `tls=custom`) is rejected at startup.
  - The example file includes comments on how to configure this per environment.

### Deployment with Kubernetes

Example Kubernetes manifests are available in `deploy/kubernetes/`:

- `minurl-sqlite.example.yaml` — SQLite backend (single replica, PVC)
- `minurl-postgres.example.yaml` — PostgreSQL backend (multi-replica)
- `minurl-mysql.example.yaml` — MySQL backend (multi-replica)

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

# For MySQL:
cp deploy/kubernetes/minurl-mysql.example.yaml deploy/kubernetes/minurl-mysql.yaml

# For SQLite:
cp deploy/kubernetes/minurl-sqlite.example.yaml deploy/kubernetes/minurl-sqlite.yaml
```

3. Apply to your cluster:

```bash
# PostgreSQL:
kubectl apply -f deploy/kubernetes/minurl-postgres.yaml

# MySQL:
kubectl apply -f deploy/kubernetes/minurl-mysql.yaml

# SQLite:
kubectl apply -f deploy/kubernetes/minurl-sqlite.yaml
```

#### Notes

- **SQLite**: requires `replicas: 1` due to `ReadWriteOnce` PVC. For horizontal scaling, use PostgreSQL or MySQL.
- **PostgreSQL**: the manifest does **not** include a PostgreSQL deployment. Use a managed database service or a separate Postgres StatefulSet. Create the `minurl-postgres` Secret with your DSN before applying (see USAGE comment in the manifest).
- **MySQL**: the manifest does **not** include a MySQL deployment. Use a managed database service or a separate MySQL StatefulSet. Create the `minurl-mysql` Secret with your DSN before applying (see USAGE comment in the manifest).
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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, make targets, coding conventions, testing, and PR process.

Quick reference:

```bash
make fix      # tidy + lint auto-fix, then lint (fails on what --fix could not repair)
make check    # all prek hooks (tidy diff + lint + test included)
make gen      # regenerate OpenAPI docs and Kiota Go client
make ci       # full CI check (same as CI pipeline)
```

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
