# Contributing to MinURL

This document is the canonical reference for development workflow, coding conventions, and contribution process, for human developers and AI agents alike.

> [AGENTS.md](AGENTS.md) is written for AI agents: it is loaded into every agent session, so it
> holds only what an agent must see on every task and points here for the rest. Design notes
> that explain why the code is shaped the way it is live under [docs/design/](docs/design/).

## Prerequisites

- Go 1.26.8+
- Docker (for integration tests and container builds)
- [`golangci-lint`](https://golangci-lint.run/) — install via `mise install` or follow the official docs
- [`prek`](https://prek.j178.dev/) — runs the git hooks and `make check`; install via `mise install`
- [`kiota`](https://learn.microsoft.com/en-us/openapi/kiota/) CLI — required by `make gen`, `make kiota`, and `make ci`

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
| `make fix` | `go mod tidy`, golangci-lint auto-fix, `go mod tidy` again, then `make lint` |
| `make lint` | `golangci-lint config verify` + `golangci-lint run --new-from-rev=$(NEW_FROM_REV)` (aborts early if the rev is invalid) |
| `make test` | `go test -race -failfast ./...`, with `INTEGRATION_TEST` passed through to the tests |
| `make check-tidy` | `go mod tidy -diff` (fails when `go.mod`/`go.sum` are stale) |
| `make check` | Every prek hook over all files (tidy diff + lint + test included) |
| `make gen` | Regenerate OpenAPI docs **and** Kiota Go client |
| `make openapi` | Regenerate OpenAPI docs only |
| `make kiota` | Regenerate Kiota client (runs `openapi` first) |
| `make ci` | `check` + `gen` + `git diff --exit-code` |
| `make build` | Compile binary to `bin/minurl` |
| `make docker-build` | Build Docker image |
| `make docker-run` | Run Docker container |

`make check` runs every hook in `.pre-commit-config.yaml` over all tracked files — the same
gate `git commit` runs, but repo-wide. Run it before every commit; CI (`make ci`) additionally
verifies generated files are in sync.

### Make variables

| Variable | Default | Effect |
|----------|---------|--------|
| `NEW_FROM_REV` | `HEAD` | golangci-lint only reports issues **new since this revision**. The default reports uncommitted work; once committed, `make lint` goes quiet. |
| `INTEGRATION_TEST` | `0` | `1` also runs the PostgreSQL/MySQL integration tests (needs Docker). |
| `VERBOSE` | `0` | `1` adds `-v` to `go test` and `golangci-lint`. |

They pass through `make check` to the nested hooks, so reproduce the CI gate locally with the
same invocation CI uses:

```bash
make ci NEW_FROM_REV=origin/main INTEGRATION_TEST=1 VERBOSE=1
```

## Architecture

Prefer this layering when adding new functionality (guidance, not a strict requirement):

1. `cmd` — startup and wiring only
2. `internal/handler` — HTTP route registration, request/response parsing
3. `internal/service` — business logic, validation, expiry
4. `internal/store` — persistence (SQLite / PostgreSQL / MySQL)

Where each package lives: [README — Repository Structure](README.md#repository-structure).

## Coding Conventions

- Follow idiomatic Go style; keep functions small.
- Prefer explicit, readable names over abbreviations.
- Return actionable errors with context.
- Keep public APIs minimal until requirements are clear.
- Do not introduce unrelated refactors in a PR.
- Register every HTTP operation through `register(api, operation{…}, handler)` from
  `internal/handler/register.go`, never `huma.Register` directly, and have handlers return
  service errors as they are. `operation.errs` is what the OpenAPI document publishes, so a status
  set anywhere else goes undocumented. See
  [HTTP API design — Error responses](docs/design/http-api.md#error-responses).

### JSON struct tags

Default `json` tags to `omitzero` (Go 1.24+). Use `omitempty` only on a slice/map field where a
non-nil but zero-length value should also be omitted, and add a comment saying so. `omitzero`
omits a nil slice/map but keeps `[]` / `{}`.

Rationale: `omitempty` has no effect at all on struct fields such as `time.Time` (the
`modernize` linter flags this), and `encoding/json/v2` redefines `omitempty` in terms of the
encoded JSON rather than the Go value, so its meaning shifts for bools, numbers, pointers and
interfaces. `omitzero` is defined on the Go value and behaves identically in v1 and v2.

## Testing

- Add or update tests for every behavior change.
- Prefer table-driven tests for handler and validation logic.
- Unit tests use the in-memory fakes in `internal/testhelpers` — no database required.

**PostgreSQL / MySQL integration tests** require Docker:

```bash
INTEGRATION_TEST=1 make test
```

## Mandatory: After Any API or Model Change

Whenever you modify **any** of the following, you **MUST** run `make gen` before finishing:

- `internal/service/model.go` (ShortURL struct fields)
- `internal/handler/short_url.go` (routes, operations, request/response types)
- `internal/handler/register.go` (the published error statuses of every operation)
- Any new HTTP endpoint

`make gen` regenerates **both**:

- `docs/openapi/openapi.yaml` and `docs/openapi/openapi.json`
- `pkg/kiota/go/gen/client/` (Kiota Go client)

The CI `make ci` runs `git diff --exit-code` after `make gen` — out-of-date generated files will fail CI.

## Documentation Checklist

After any functional change, update **all** of the following that apply:

| File | When to update |
|------|----------------|
| `README.md` | API behavior, new fields, new endpoints, configuration options |
| `docs/http/minurl.http` | New fields or scenarios — add example requests **manually** |
| `docs/openapi/` | Run `make gen` (auto) |
| `pkg/kiota/go/gen/` | Run `make gen` (auto) |
| `docs/design/` | A change to how requests are validated or errors are returned |
| `AGENTS.md` | A rule an agent must see on every task, or a new section here or `docs/design/` note it should be pointed to (a row in its "Before You Start a Task" table) |
| `CONTRIBUTING.md` | New mandatory workflows or conventions |

## Adding an Error Status

Background: [HTTP API design — Error responses](docs/design/http-api.md#error-responses).

1. Define the error in `internal/service`. Do not implement `huma.StatusError` on it: HTTP
   status stays out of `service`.
2. Add it to `errorResponses` in `internal/handler/register.go`, with its status and message.
3. Add it to `errs` of every operation that returns it; an error an operation does not list
   becomes a 500.
4. Add a case to `TestRegisterDeclaresEveryReachableErrorStatus`. Every listed *error* needs a
   request that returns it, not just every status.
5. Run `make gen`.
6. Add the status to the [README — Error responses](README.md#error-responses) table, which
   lists every status each operation returns.

## Adding a New Setting

A setting has one name everywhere: its `configKeys` entry in `cmd/minurl/config.go` is the flag
name (`db-max-open-conns`), which is also the viper key, the config file key and, through the
`-` → `_` replacer, the env var (`MINURL_DB_MAX_OPEN_CONNS`). The config file is read once
(`readConfigFile`), and `checkConfigFileKeys` fails startup on an unknown key (even a null one;
a nested `db: {max-open-conns: …}` or dotted `db.max-open-conns` key is unknown), a key that is
not lowercase (viper would fold `ID-Seed` into `id-seed`), a list or mapping as a value, a key
given twice, or a merge key (`<<`).

1. Add a `configKeys` entry, a flag of the same name and a read in `loadAppConfig`. Do not give
   it a dotted name, which viper reads from a nested file key.
2. Keep its env var clear of the ones Kubernetes injects for every Service in the namespace
   (`<SVC>_SERVICE_HOST`, `<SVC>_SERVICE_PORT`, `<SVC>_PORT`, `<SVC>_PORT_<n>_TCP…`): no name
   ending in `-port`, `-service-host` or `-service-port`, or holding `-port-<n>-`, or a Service
   named `minurl-db` would set `MINURL_DB_PORT=tcp://…`, which outranks the config file. For the
   same reason an unknown `MINURL_*` env var is ignored, not rejected: the examples' Service
   `minurl` injects `MINURL_SERVICE_HOST`, `MINURL_PORT` and more.
3. Document it in the [README — Configuration](README.md#configuration) table and in
   `config.example.yaml`.

## Adding a New Storage Column

Migrations use [golang-migrate/migrate v4](https://github.com/golang-migrate/migrate), one
directory per backend under `internal/store/migrations/`, embedded with `//go:embed` in each
store file and applied on startup through the shared `runMigrations()` in
`internal/store/migrations.go`, which records them in a `schema_migrations` table. SQLite
migrates through golang-migrate's `database/sqlite` (modernc, no cgo), not `database/sqlite3`,
which needs cgo: the binary is built with `CGO_ENABLED=0`. `runMigrations()` does not
call `m.Close()` after `Up()`: the database drivers wrap a caller-owned `*sql.DB`, and closing
them would close the shared connection.

When adding a new column to `short_urls`:

1. Create migration files for **all three** backends, named like the existing ones:
   `internal/store/migrations/{sqlite,postgres,mysql}/<version>_<name>.{up,down}.sql`.
   `<version>` is one past the highest version across all three directories, zero-padded to six
   digits, and the same in each (e.g. after `000002_add_expire_time` comes `000003_<name>`; with
   the `migrate` CLI, pass `-seq`, since it defaults to timestamps). A duplicate version breaks
   golang-migrate at startup, a version at or below the one a database has already applied is
   never applied to that database (released or not), and only `.sql` files are embedded: a file
   that does not match `<digits>_<name>.{up,down}.sql` is silently skipped.
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
| `build/` | Build system or Dockerfile changes |
| `perf/` | Performance improvements |
| `style/` | Formatting or lint fixes (no logic change) |
| `revert/` | Reverting a previous commit |

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

1. Create a feature branch from `main` — never commit directly to `main`.
2. Make changes following the conventions above.
3. Run `make check` and `make gen`.
4. Ensure `git diff --exit-code` passes (same as CI checks).
5. Open a PR. The title follows the same Conventional Commits format as the commit
   message; the body uses:

   ```markdown
   ## Summary

   ## Changes

   -

   ## How to Test

   1.
   ```
6. Push follow-up commits to update the PR. If a rewrite is unavoidable, use
   `git push --force-with-lease`, never `--force`.
