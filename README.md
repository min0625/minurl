# minurl

A short URL service project implemented in Go.

## Project Status

Current implementation is a minimal bootstrap app:

- Entry point: `main.go`
- Current behavior: prints `Hello, World!`
- Container build target binary: `hello-go`

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
go run .
```

### Build and run with Docker

```bash
make docker-build
make docker-run
```

By default:

- Image name: `hello-go`
- Tag: current git tag (if exact tag exists) or short commit SHA

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
|-- main.go
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

BSD-style license. See `LICENSE`.
