# AGENTS

Guidance for coding agents working in this repository.

## Purpose

This repository is a Go short URL service. The core API is fully implemented — it supports creating, fetching, and redirecting short URLs backed by SQLite or PostgreSQL. The runtime provides a Cobra-based CLI entrypoint and HTTP server startup via `cmd/minurl/main.go`.

## Core Rules

- Keep changes small, focused, and easy to review.
- Preserve existing behavior unless a task explicitly requires behavior changes.
- Do not introduce unrelated refactors.
- Add tests when changing logic.
- Keep code and docs consistent.

## Project Facts

- Go version: 1.26.2
- Main module: `github.com/min0625/minurl`
- Main entry point: `cmd/minurl/main.go`
- Docker output binary: `minurl`
- Deployment configs: `deploy/docker-compose/` — includes PostgreSQL and SQLite example configurations (`.example.yml` format; copy and customize before use)
- HTTP listen port: `:8888` (default)
- Storage backends: SQLite (`sqlite3://`) and PostgreSQL (`postgres://`), auto-detected from DSN scheme
- Log format: `text` (default) or `json`, controlled via `--log-format` / `MINURL_LOG_FORMAT`
- OpenTelemetry: opt-in tracing via `--otel-enabled`; supports `stdout` and `otlp` exporters
- Configuration precedence: CLI flags > env vars > config file > defaults

## Useful Commands

Use Make targets when possible:

- `make fix`
- `make lint`
- `make test`
- `make check`
- `make docker-build`
- `make docker-run`

Direct run:

- `go run ./cmd/minurl`

## Coding Conventions

- Follow idiomatic Go style and keep functions small.
- Prefer explicit, readable names over abbreviations.
- Return actionable errors with context.
- Keep public APIs minimal until requirements are clear.

## Testing Expectations

- Add or update tests for behavior changes.
- Prefer table-driven tests for handler and validation logic.
- Run `make test` and `make lint` before finalizing.

## Documentation Expectations

When changing functionality, update:

- `README.md` for user-facing behavior and run instructions
- API examples (when HTTP endpoints are added)
- Architecture notes (when new major components are introduced)

## Suggested Architecture Direction

As the short URL service is implemented, prefer this layering:

1. `cmd` or entry layer (startup and wiring)
2. `internal/handler` (HTTP handlers)
3. `internal/service` (business logic)
4. `internal/store` (persistence)
5. `internal/model` (domain types)

This is guidance, not a strict requirement, and can be adjusted per task.
