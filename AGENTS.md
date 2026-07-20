# AGENTS.md

## Repository

`github.com/justblue/mirage` — PostgreSQL schema migration tool and Go database toolkit. Go 1.26.2.

## Package Structure

```
mirage/                  # Root package: PostgreSQL toolkit (CRUD, transactions, notifications)
cmd/mirage/              # CLI binary entrypoint
internal/
  cli/                   # CLI command definitions (8 commands)
  dialect/               # SQL dialect interface
  dialect/postgres/      # PostgreSQL dialect implementation
  diff/                  # Schema diffing (old vs new state → events)
  generator/             # SQL migration file generation from events
  graph/                 # Dependency graph for topological ordering
  runner/                # Migration execution engine
  scanner/               # Go source code scanner (reads struct tags, produces *schema.Package)
  schema/                # Unified schema types (Package, Table, Column, Enum, constraints) and runtime ORM
  validate/              # Schema validation rules
```

**Single type system**: All packages use `schema.Package` — scanner produces it, diff/generator/validate consume it. No more dual type systems.

## Build & Test

```bash
make build          # Builds CLI to bin/mirage.exe
make test           # go test -v -race ./...
make lint           # golangci-lint run ./...
make install        # go install ./cmd/mirage
go build ./...      # Quick build check (no binary output)
go test ./...       # Quick test (no race, no verbose)
go vet ./...        # Static analysis
```

## Testing

Tests exist in: `internal/validate`, `internal/schema`, `internal/diff`, `internal/generator`, `internal/dialect/postgres`, `internal/graph`. No integration tests (no DB required for tests). Tests use in-memory `schema.Package` structs directly — no fixtures or DB setup.

Run a single test:
```bash
go test ./internal/validate/ -run TestValidate_CleanPackage
```

## Architecture

- **No schema registration**: Table definitions derived lazily from struct `db:"..."` tags at first use, cached per type via `sync.Map` in `table_cache.go`.
- **State in DB**: Schema snapshots stored as JSONB in `schema_migrations` table (column `snapshot`). No `state.json` file.
- **CLI is flag-driven**: No config file (`mirage.yaml` removed). All options via `--flags`.
- **Root package `mirage`**: Exports `DB`, `Open()`, `OpenPool()`, CRUD methods (`Insert`, `Select`, `Update`, `Delete`, `Upsert`, `Duplicate`), raw SQL helpers (`QueryStruct`, `QueryStructs`), transactions, and LISTEN/NOTIFY.
- **Migration pipeline**: `scanner.Scan()` → `validate.Validate()` → `diff.Diff()` → `generator.Generate()` → SQL files on disk.

## Key Files

| File | Purpose |
|------|---------|
| `db.go` | DB struct, Open/Close, transaction management, Exec/Query |
| `table_cache.go` | Lazy per-type table definition cache (`cachedTable()`) |
| `query.go` | `QueryStruct`/`QueryStructs` for raw SQL → struct scanning |
| `db_repository.go` | All CRUD operations using cached table definitions |
| `internal/schema/struct_table.go` | `ConvertStructToTable()`, `ConvertRowsToStruct()` |
| `internal/schema/constraints.go` | PrimaryKey, ForeignKey, UniqueConstraint, Index, CheckConstraint, Partition |
| `internal/schema/enum.go` | Enum, Package types, SQLName() methods |
| `internal/schema/naming.go` | Constraint name generation (PKName, FKName, UQName, IdxName, ChkName, Truncate) |
| `internal/runner/tracker.go` | `EnsureTracker()`, `SaveSnapshot()`, `LoadSnapshot()` |
| `internal/dialect/postgres/postgres.go` | All PostgreSQL SQL generation, `schema_migrations` DDL |

## Conventions

- Struct tag `db:"pk,type:bigserial"` defines schema — no annotations, no registration.
- Tests use table-driven pattern with `tt` subtests.
- Migration files: `V{version}_{description}.sql` with `-- +migrate Up` / `-- +migrate Down` markers.
- No `.golangci.yml` — default lint rules.
