# AGENTS

Guidance for coding agents working in this repository.

## Purpose

This repository is a Go project intended to become a short URL service.
Current runtime code is minimal (`main.go` prints `Hello, World!`).

## Core Rules

- Keep changes small, focused, and easy to review.
- Preserve existing behavior unless a task explicitly requires behavior changes.
- Do not introduce unrelated refactors.
- Add tests when changing logic.
- Keep code and docs consistent.

## Project Facts

- Go version: 1.26.2
- Main module: `github.com/min0625/minurl`
- Main entry point: `main.go`
- Docker output binary: `hello-go`

## Useful Commands

Use Make targets when possible:

- `make fix`
- `make lint`
- `make test`
- `make check`
- `make docker-build`
- `make docker-run`

Direct run:

- `go run .`

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
