# AGENTS

Guidance for AI coding agents working in this repository. It is loaded into every session, so it
keeps only what applies to most tasks. [CONTRIBUTING.md](CONTRIBUTING.md) is the canonical
workflow and conventions for human developers and agents alike; the table below says which part
of it to read before which task.

## Purpose

MinURL is a Go short URL service: it creates, fetches, and redirects short URLs backed by SQLite, PostgreSQL, or MySQL. The runtime provides a Cobra-based CLI entrypoint and HTTP server startup via `cmd/minurl/main.go`.

## Core Rules

- Keep changes small, focused, and easy to review.
- Preserve existing behavior unless a task explicitly requires behavior changes.
- Do not introduce unrelated refactors.
- Add tests when changing logic.
- Keep code and docs consistent with [CONTRIBUTING.md](CONTRIBUTING.md).

## Before You Start a Task

| Task | Read first |
|------|------------|
| Change `internal/handler`, `internal/middleware`, `internal/httpserver`, a request or response type, a validation rule, or an error status | [HTTP API design](docs/design/http-api.md) |
| Add an error status | [CONTRIBUTING — Adding an Error Status](CONTRIBUTING.md#adding-an-error-status) |
| Change the API or the model | [CONTRIBUTING — Mandatory: After Any API or Model Change](CONTRIBUTING.md#mandatory-after-any-api-or-model-change) |
| Add a configuration setting | [CONTRIBUTING — Adding a New Setting](CONTRIBUTING.md#adding-a-new-setting) |
| Add a storage column or migration | [CONTRIBUTING — Adding a New Storage Column](CONTRIBUTING.md#adding-a-new-storage-column) |
| Add or change a `json` struct tag | [CONTRIBUTING — JSON struct tags](CONTRIBUTING.md#json-struct-tags) |
| Add a package or decide where code goes | [CONTRIBUTING — Architecture](CONTRIBUTING.md#architecture) |
| Finish any functional change | [CONTRIBUTING — Documentation Checklist](CONTRIBUTING.md#documentation-checklist) |
| Commit, name a branch, or open a PR | [CONTRIBUTING — Branch and Commit Conventions](CONTRIBUTING.md#branch-and-commit-conventions) and [Pull Request Process](CONTRIBUTING.md#pull-request-process) |

## Rules That Hold on Every Task

- Register every HTTP operation through `register(api, operation{…}, handler)` in
  `internal/handler/register.go`, never `huma.Register` directly; handlers return service errors
  as they are. HTTP status stays out of `internal/service`: never implement `huma.StatusError` on
  a service error.
- Validation lives in the request schema and on the field's type (`service.OriginalURL`), not in
  a second validator. A resolver returns `*huma.ErrorDetail`, never `huma.ErrorXXX`.
- `huma.ValidateStrictCasing` (set in `init()` in `register.go`) is load-bearing: without it a
  differently cased JSON key bypasses validation.
- Expiry, and the validity of a stored `original_url`, are checked in `ShortURLService.Get()`, not
  in the store.
- `HEAD` is answered wherever `GET` is, through chi's `middleware.GetHead` in
  `httpserver.NewRouter`; the OpenAPI document lists only the GET.
- MySQL DSN query params are sent as server session variables, so a driver flag
  (`multiStatements`, `timeout`, …) is rejected at connect time; only `tls` is mapped, and
  `parseTime` / `loc` are ignored. This is MinURL's own vocabulary, not MySQL's URI attributes —
  see [README — MySQL DSN query parameters](README.md#mysql-dsn-query-parameters). A
  `postgres://` DSN goes to pgx untouched; a `sqlite3://` query string is appended to the SQLite URI.
- Minimum database versions: PostgreSQL 9.5, MySQL 8.0 (the `utf8mb4_0900_as_cs` collation on
  `short_urls.id` — 5.7 fails to start). See [README — Supported databases](README.md#supported-databases).

## Commands

Full list: [CONTRIBUTING.md — Make Targets Reference](CONTRIBUTING.md#make-targets-reference).

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
