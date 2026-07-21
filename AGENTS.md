# AGENTS.md

## Repository

`github.com/justblue/mirage` — PostgreSQL schema migration tool and Go database toolkit. Go 1.26.2.

## Package Structure

```
mirage/                  # Root package: PostgreSQL toolkit (CRUD, transactions, notifications, locking, retry, UoW)
cmd/mirage/              # CLI binary entrypoint
internal/
  cli/                   # CLI command definitions (8 commands)
  checksum/              # Migration file checksum verification
  config/                # YAML config file loading (mirage.yaml / .mirage.yaml)
  dialect/               # SQL dialect interface
  dialect/postgres/      # PostgreSQL dialect implementation
  diff/                  # Schema diffing (old vs new state → events)
  generator/             # SQL migration file generation from events
  graph/                 # Dependency graph for topological ordering
  integration/           # Integration tests (build-tag gated, requires PostgreSQL)
  introspect/            # Live database introspection (pg catalog → schema types)
  runner/                # Migration execution engine
  scanner/               # Go source code scanner (reads struct tags, produces *schema.Package)
  schema/                # Unified schema types (Package, Table, Column, Enum, constraints) and runtime ORM
  validate/              # Schema validation rules
```

**Single type system**: All packages use `schema.Package` — scanner produces it, diff/generator/validate consume it. No more dual type systems.

## Build & Test

```bash
make build              # Builds CLI to bin/mirage.exe
make test               # go test -v -race ./...
make test-unit          # go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out ./...
make test-integration   # Runs PostgreSQL in podman, go test -tags=integration -race -count=1 ./...
make test-ci            # Runs lint + test-unit + test-integration sequentially
make lint               # golangci-lint run ./...
make install            # go install ./cmd/mirage
make clean              # rm -rf bin/
make release VERSION=vX.Y.Z  # Tag + push + GitHub release
go build ./...          # Quick build check (no binary output)
go test ./...           # Quick test (no race, no verbose)
go vet ./...            # Static analysis
```

## Testing

### Unit tests

Tests exist in: `internal/validate`, `internal/schema`, `internal/diff`, `internal/generator`, `internal/dialect/postgres`, `internal/graph`, `internal/config`, `internal/introspect`. Root package tests: `cache_test.go`, `errors_test.go`, `lock_test.go`, `repository_test.go`, `register_test.go`, `uow_test.go`.

```bash
make test-unit                                              # all unit tests with race + coverage
go test ./internal/validate/ -run TestValidate_CleanPackage  # single test
```

### Integration tests

Integration tests live in `internal/integration/` (8 test files) and require a running PostgreSQL instance. They are gated behind the `integration` build tag.

```bash
make test-integration          # starts PostgreSQL in podman, runs tests, tears down
go test -tags=integration -race -count=1 ./internal/integration/ -run TestRepository  # single test
```

Set `MIRAGE_TEST_DATABASE_URL` to bypass podman and use your own PostgreSQL instance.

## Architecture

- **No schema registration**: Table definitions derived lazily from struct `db:"..."` tags at first use, cached per type via `sync.Map` in `table_cache.go`.
- **State in DB**: Schema snapshots stored as JSONB in `schema_migrations` table (column `snapshot`). No `state.json` file.
- **Config file**: Optional `mirage.yaml` / `.mirage.yaml` searched from cwd up to filesystem root. All CLI flags fall back to config values when not explicitly set.
- **Root package `mirage`**: Exports `DB`, `Open()`, `OpenPool()`, `Repository[T]` (CRUD + cache + batch + retry + locking), `UnitOfWork`, `Register()`, raw SQL helpers, transactions, and LISTEN/NOTIFY.
- **Migration pipeline**: `scanner.Scan()` → `validate.Validate()` → `diff.Diff()` → `generator.Generate()` → SQL files on disk.
- **Live introspection**: `internal/introspect.FromLiveDatabase()` reads pg_catalog to reconstruct `schema.Package` from a running database, used for drift detection (`mirage status --check-drift`).

## Key Files

| File | Purpose |
|------|---------|
| `db.go` | DB struct, Open/Close, transaction management, Exec/Query, `RetryOptions`, `InTransactionWithRetry` |
| `repository.go` | `Repository[T]` — all CRUD, cache, batch, retry (WithRetry), locking (SelectByIDForUpdate, QueryForUpdate) |
| `table_cache.go` | Lazy per-type table definition cache (`cachedTable()`) |
| `lock.go` | `LockStrength`, `LockWaitMode`, `LockOption` types and constructors (ForUpdate, ForUpdateSkipLocked, etc.) |
| `cache.go` | `Cache` interface, `InMemoryCache`, `NewInMemoryCache()` |
| `uow.go` | Unit of Work — `UnitOfWork`, `Q(ctx)`, context-threaded transactions |
| `register.go` | `Registry` type, `Register()` for functions/views/triggers/etc. |
| `errors.go` | `IsErrNoRows`, `IsErrDuplicate`, `IsErrForeignKey`, `IsErrSerializationFailure`, `IsErrDeadlock`, `IsErrRetryable` |
| `listener.go` | LISTEN/NOTIFY support, `Listen()`, `Notify()` |
| `internal/schema/struct_table.go` | `ConvertStructToTable()`, `ConvertRowsToStruct()` |
| `internal/schema/constraints.go` | PrimaryKey, ForeignKey, UniqueConstraint, Index, CheckConstraint, Partition |
| `internal/schema/enum.go` | Enum, Package types, SQLName() methods |
| `internal/schema/naming.go` | Constraint name generation (PKName, FKName, UQName, IdxName, ChkName, Truncate) |
| `internal/runner/tracker.go` | `EnsureTracker()`, `SaveSnapshot()`, `LoadSnapshot()` |
| `internal/dialect/postgres/postgres.go` | All PostgreSQL SQL generation, `schema_migrations` DDL |
| `internal/introspect/introspect.go` | `FromLiveDatabase()` — reconstructs schema.Package from pg_catalog |
| `internal/introspect/typemap.go` | Reverse Postgres catalog type → schema.DataType mapping |
| `internal/config/config.go` | `Load()`, YAML config file discovery and parsing |
| `internal/cli/status.go` | `--check-drift` flag, `runDriftCheck()` for live vs snapshot comparison |

## Conventions

- Struct tag `db:"pk,type:bigserial"` defines schema — no annotations, no registration.
- Tests use table-driven pattern with `tt` subtests.
- Migration files: `V{version}_{description}.sql` with `-- +migrate Up` / `-- +migrate Down` markers.
- `.golangci.yml` is present — lint config scopes down style debt (exported doc comments) while keeping all bug-catching linters at full strength.
- Integration tests use the `integration` build tag and `MIRAGE_TEST_DATABASE_URL` env var.
