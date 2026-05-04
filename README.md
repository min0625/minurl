# minurl

A short URL service project implemented in Go.

## Project Status

Core short URL API is implemented and running:

- Entry point: `cmd/minurl/main.go`
- Runtime behavior:
	- Runs HTTP API server by default on `:8888`
	- Provides CLI subcommands: `openapi`, `version`
- Storage backend is SQLite only for now
- In SQLite mode, both short URL records and `id counter` are persisted
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
- `--id-seed`: deterministic seed for ID key derivation (uint32, empty means built-in default seed)
- `--storage-dsn`: storage DSN; `sqlite3://path` for SQLite (default `sqlite3://minurl.sqlite3`) or `postgres://...` for PostgreSQL

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

Example (env):

```bash
MINURL_HTTP_ADDR=:9090 MINURL_ID_SEED=12345 MINURL_STORAGE_DSN=sqlite3://minurl.sqlite3 go run ./cmd/minurl
```

PostgreSQL example (env):

```bash
MINURL_STORAGE_DSN="postgres://localhost:5432/minurl?sslmode=disable" go run ./cmd/minurl
```

PostgreSQL example (flags):

```bash
go run ./cmd/minurl --storage-dsn "postgres://localhost:5432/minurl?sslmode=disable"
```

Example (flags):

```bash
go run ./cmd/minurl --http-addr :9090 --id-seed 12345 --storage-path ./data/minurl.sqlite3
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
|   `-- minurl/
|       |-- main.go
|       `-- main_test.go
|-- docs/
|   `-- openapi/
|       |-- openapi.json
|       `-- openapi.yaml
|-- internal/
|   |-- handler/          # HTTP route handlers
|   |   |-- short_url.go
|   |   `-- short_url_test.go
|   |-- service/          # Business logic
|   |   |-- short_url.go
|   |   `-- short_url_test.go
|   `-- model/            # Domain types
|       `-- short_url.go
|-- go.mod
|-- Dockerfile
|-- Makefile
|-- LICENSE
```

## Next Suggested Milestones

1. ✅ Define URL entity and storage interface.
2. ✅ Add HTTP server and routing.
3. ✅ Implement create and get short URL endpoints.
4. ✅ Add tests and error handling.
5. Add redirect endpoint (`GET /{id}` → `302` to original URL).
6. Add custom alias support.
7. ✅ Add pluggable database backends (MySQL / PostgreSQL / DynamoDB).

## License

Apache License 2.0. See `LICENSE`.
