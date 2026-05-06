# minurl

A short URL service project implemented in Go.

## Project Status

Core short URL API is implemented and running:

- Entry point: `cmd/minurl/main.go`
- Runtime behavior:
	- Runs HTTP API server by default on `:8888`
	- Provides CLI subcommands: `openapi`, `version`
- Storage backend: SQLite (`sqlite3://`) or PostgreSQL (`postgres://`), selected via `--storage-dsn`
- Both short URL records and `id counter` are persisted in the configured backend
- Container build target binary: `minurl`

## API Documentation

API details are maintained in OpenAPI files under `docs/openapi/`:

- `docs/openapi/openapi.yaml`
- `docs/openapi/openapi.json`

Online viewer:
[https://min0625.github.io/openapi-viewer/?url=https://raw.githubusercontent.com/min0625/minurl/refs/heads/main/docs/openapi/openapi.yaml](https://min0625.github.io/openapi-viewer/?url=https://raw.githubusercontent.com/min0625/minurl/refs/heads/main/docs/openapi/openapi.yaml)

### API Endpoints

**Create a short URL**
(`id` is optional. If omitted, the server auto-generates one.)
```
POST /api/v1/urls
Content-Type: application/json

{
  "original_url": "https://example.com/very/long/url",
  "id": "myshort"
}

Response: 200 OK
{
  "id": "myshort",
  "original_url": "https://example.com/very/long/url",
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
  "create_time": "2026-04-28T16:00:00Z"
}
```

**Redirect to original URL**
```
GET /api/v1/urls/{id}:redirect

Response: 302 Found
Location: https://example.com/very/long/url
```

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
|   `-- docker-compose/         # Deployment examples (PostgreSQL and SQLite)
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
