# minurl

A short URL service project implemented in Go.

## Project Status

Current implementation provides a CLI + server foundation:

- Entry point: `cmd/minurl/main.go`
- Runtime behavior:
	- Runs HTTP API server by default
	- Provides CLI subcommands like `openapi` and `version`
- Container build target binary: `minurl`

This repository is prepared as the foundation for building a full short URL system.

## Planned Scope

The expected short URL system capabilities include:

- Create a short URL from a long URL
- Resolve a short code back to the original URL
- Optional custom alias support
- Basic validation and duplicate handling
- API-first design with clear HTTP endpoints

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

Version metadata can be injected at build time via `ldflags`:

```bash
go run -ldflags "-X github.com/min0625/minurl/cmd/minurl.version=v1.0.0 -X github.com/min0625/minurl/cmd/minurl.commit=$(git rev-parse --short HEAD)" ./cmd/minurl version
```

In CI release pipelines, you can pass tag/commit like this:

```bash
go build -ldflags "-s -w -X github.com/min0625/minurl/cmd/minurl.version=${GIT_TAG} -X github.com/min0625/minurl/cmd/minurl.commit=${GIT_COMMIT}" -o minurl ./cmd/minurl
./minurl version
```

### Build and run with Docker

```bash
make docker-build
make docker-run
```

### Export OpenAPI docs

Generate OpenAPI files directly from the app contract (no server startup required):

```bash
go run ./cmd/minurl openapi
```

This writes:

- `docs/openapi/openapi.json`
- `docs/openapi/openapi.yaml`

You can also generate each format separately:

```bash
go run ./cmd/minurl openapi --format=json
go run ./cmd/minurl openapi --format=yaml
```

`--format` accepts only `all`, `json`, or `yaml` and returns a friendly error for invalid values.

Or use Make targets:

```bash
make openapi
make openapi-json
make openapi-yaml
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
- `check`: run tidy diff, lint, and tests

## Repository Structure

```text
.
|-- cmd/
|   `-- minurl/
|       `-- main.go
|-- internal/
|   |-- handler/
|   |-- service/
|   `-- model/
|-- go.mod
|-- Dockerfile
|-- Makefile
|-- LICENSE
```

## Next Suggested Milestones

1. Define URL entity and storage interface.
2. Add HTTP server and routing.
3. Implement create and resolve endpoints.
4. Add input validation, tests, and error handling.
5. Add persistence (in-memory first, then database).

## License

Apache License 2.0. See `LICENSE`.
