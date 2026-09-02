# Contributing to MinURL

This document is the canonical reference for development workflow, coding conventions, and contribution process. Both human developers and AI agents should follow these guidelines.

> For AI agent-specific project context (layer structure, domain model, architecture direction, migration strategy), see [AGENTS.md](AGENTS.md).

## Prerequisites

- Go 1.26.8+
- Docker (for integration tests and container builds)
- [`golangci-lint`](https://golangci-lint.run/) — install via `mise install` or follow the official docs
- [`prek`](https://prek.j178.dev/) — runs the git hooks and `make check`; install via `mise install`
- [`kiota`](https://learn.microsoft.com/en-us/openapi/kiota/) CLI — required only when regenerating the Go client

Install all tools at once with:

```bash
mise install
```

Then install the git hooks (the devcontainer's `post_create.sh` does this for you):

```bash
prek install --overwrite --prepare-hooks
```

## Development Setup

```bash
# Clone and enter the repo
git clone https://github.com/min0625/minurl.git
cd minurl

# Copy the example config
cp config.example.yaml config.yaml

# Run locally
go run ./cmd/minurl
```

## Make Targets Reference

| Target | Description |
|--------|-------------|
| `make fix` | `go mod tidy` + golangci-lint auto-fix, then `make lint` |
| `make lint` | `golangci-lint config verify` + `golangci-lint run` |
| `make test` | Race-enabled `go test ./...` |
| `make check` | Every prek hook over all files (tidy diff + lint + test included) |
| `make gen` | Regenerate OpenAPI docs **and** Kiota Go client |
| `make openapi` | Regenerate OpenAPI docs only |
| `make kiota` | Regenerate Kiota client (runs `openapi` first) |
| `make ci` | `check` + `gen` + `git diff --exit-code` |
| `make build` | Compile binary to `bin/minurl` |
| `make docker-build` | Build Docker image |
| `make docker-run` | Run Docker container |

`make check` runs every hook in `.pre-commit-config.yaml` over all tracked files — the same
gate `git commit` runs, but repo-wide. Run it before every commit. CI (`make ci`) additionally verifies generated files are in sync.

## Coding Conventions

- Follow idiomatic Go style; keep functions small.
- Prefer explicit, readable names over abbreviations.
- Return actionable errors with context.
- Keep public APIs minimal until requirements are clear.
- Do not introduce unrelated refactors in a PR.

## Testing

- Add or update tests for every behavior change.
- Prefer table-driven tests for handler and validation logic.
- Always run `make test && make lint` before submitting.

**PostgreSQL / MySQL integration tests** require Docker:

```bash
INTEGRATION_TEST=1 make test
```

## Mandatory: After Any API or Model Change

Whenever you modify **any** of the following, you **MUST** run `make gen` before finishing:

- `internal/service/model.go` (ShortURL struct fields)
- `internal/handler/short_url.go` (routes, operations, request/response types)
- Any new HTTP endpoint

`make gen` regenerates **both**:

- `docs/openapi/openapi.yaml` and `docs/openapi/openapi.json`
- `pkg/kiota/go/gen/client/` (Kiota Go client)

The CI `make ci` runs `git diff --exit-code` after `make gen` — out-of-date generated files will fail CI.

## Documentation Checklist

After any functional change, update **all** of the following that apply:

| File | When to update |
|------|----------------|
| `README.md` | API behavior, new fields, new endpoints |
| `docs/http/minurl.http` | New fields or scenarios — add example requests **manually** |
| `docs/openapi/` | Run `make gen` (auto) |
| `pkg/kiota/go/gen/` | Run `make gen` (auto) |
| `AGENTS.md` | Domain Model table, migration strategy, new behaviors |
| `CONTRIBUTING.md` | New mandatory workflows or conventions |

## Adding a New Storage Column

When adding a new column to `short_urls`:

1. Create migration files for **all three** backends, using the next unused version number
   (`000002` is already taken by `add_expire_time`; a duplicate version breaks golang-migrate at startup):
   - `internal/store/migrations/sqlite/000003_<name>.{up,down}.sql`
   - `internal/store/migrations/postgres/000003_<name>.{up,down}.sql`
   - `internal/store/migrations/mysql/000003_<name>.{up,down}.sql`
2. Update `CreateIfAbsent()` and `GetByID()` in:
   - `internal/store/sqlite.go` (SQLite)
   - `internal/store/postgres.go`
   - `internal/store/mysql.go`
3. Update `internal/testhelpers/storage.go` if the field needs special handling.
4. Run `make gen` to regenerate OpenAPI docs and the Go client.

## Branch and Commit Conventions

This project follows [Conventional Commits](https://www.conventionalcommits.org/).

### Branch naming

Format: `<type>/<short-description>` (lowercase kebab-case)

| Prefix | Use case |
|--------|----------|
| `feat/` | New feature or capability |
| `fix/` | Bug fix |
| `docs/` | Documentation-only changes |
| `chore/` | Maintenance, tooling, dependency updates |
| `refactor/` | Restructuring without behavior change |
| `test/` | Adding or improving tests |
| `ci/` | CI/CD pipeline changes |

### Commit message format

```
<type>(<optional scope>): <short description in present tense>

<optional body: explain WHY, not WHAT>
```

Examples:
```
feat(store): add MySQL storage backend
fix(service): return 404 for expired URLs on redirect
chore: upgrade golangci-lint to 2.11.2
docs: add MySQL deployment example
```

## Pull Request Process

1. Create a feature branch from `main`.
2. Make changes following the conventions above.
3. Run `make check` and `make gen`.
4. Ensure `git diff --exit-code` passes (same as CI checks).
5. Open a PR with a clear summary, list of changes, and testing steps.

For full PR creation steps (including MCP-based tooling), see [`.github/instructions/create-pull-request.instructions.md`](.github/instructions/create-pull-request.instructions.md).
