# Mirage

A PostgreSQL schema migration tool and Go database toolkit.

Mirage scans Go struct definitions annotated with `db` struct tags, diffs them against a saved state, and generates versioned SQL migration files. It also scans `mirage.Register()` calls to detect database functions, views, triggers, procedures, grants, and policies. It provides a full-featured Go library for PostgreSQL with CRUD operations, transactions, raw SQL queries, and real-time notifications.

## Overview

Mirage has two components:

- **CLI tool** (`cmd/mirage`) - Scans Go source code, detects schema changes, and generates migration SQL files.
- **Go library** (root package `mirage`) - A PostgreSQL toolkit built on [pgx](https://github.com/jackc/pgx) with connection pooling, CRUD helpers, transaction management, and LISTEN/NOTIFY support.

## Quick Start

### As a CLI Tool

```bash
go install github.com/justblue/mirage/cmd/mirage@latest
```

Initialize mirage in your project:

```bash
mirage init --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
```

Define your models using Go structs with `db` struct tags:

```go
package models

type User struct {
    _ struct{} `db:"name=users"`

    ID        int64     `db:"pk,identity,type=bigserial"`
    Email     string    `db:"name=email,type=varchar(255),unique,notnull"`
    Name      string    `db:"name=name,type=varchar(100),notnull"`
    Role      UserRole  `db:"name=role,type=user_role,notnull,default='member'"`
    CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
    UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}
```

Generate and apply migrations:

```bash
mirage generate --source ./internal/models -m "create users table" --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
mirage migrate --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
```

### As a Go Library

```bash
go get github.com/justblue/mirage
```

```go
package main

import (
    "context"
    "fmt"
    "log"

    mirage "github.com/justblue/mirage"
)

type User struct {
    _ struct{} `db:"name=users"`

    ID    int64  `db:"pk,identity,type=bigserial"`
    Name  string `db:"name=name,type=varchar(100),notnull"`
    Email string `db:"name=email,type=varchar(255),unique,notnull"`
}

func main() {
    ctx := context.Background()
    db, err := mirage.Open(ctx,
        "postgres://user:pass@localhost:5432/mydb?sslmode=disable&search_path=public")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    repo := mirage.NewRepository[User](db)

    // Insert a record
    user := &User{Name: "Alice", Email: "alice@example.com"}
    if err := repo.InsertReturning(ctx, user); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created user with ID: %d\n", user.ID)

    // Select by ID (transparent cache when configured)
    found, err := repo.SelectByID(ctx, user.ID)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Found user: %+v\n", found)
}
```

## CLI Reference

### Commands

| Command | Description |
|---------|-------------|
| `mirage init` | Initialize mirage in a project (creates `migrations/` directory) |
| `mirage generate` | Scan Go source, diff against saved state, generate migration SQL files |
| `mirage migrate` | Apply pending migrations to the database |
| `mirage rollback [N]` | Roll back the last N migrations (default: 1) |
| `mirage status` | Show applied and pending migrations |
| `mirage create` | Create an empty migration file |
| `mirage reset` | Clear all migration records from the database |
| `mirage validate` | Validate model struct tags without generating migrations |

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--no-color` | `false` | Disable colorized output |

### Command Flags

#### `init`

| Flag | Default | Description |
|------|---------|-------------|
| `--dir` | `.` | Project root directory |
| `--db` | - | Database connection string (creates `schema_migrations` table) |

#### `generate`

| Flag | Default | Description |
|------|---------|-------------|
| `--source` | - | Source directories to scan (required, repeatable) |
| `--migrations-dir` | `./migrations` | Migrations output directory |
| `--db` | - | Database connection string (loads/saves snapshot state) |
| `--message, -m` | - | Migration description |
| `--recursive, -r` | `false` | Scan subdirectories |
| `--dry-run` | `false` | Print DDL without writing files |
| `--force` | `false` | Skip destructive-change prompt |
| `--reset-state` | `false` | Discard saved state and regenerate |
| `--sequential-naming` | `false` | Use sequential version numbers |
| `--verbose, -v` | `false` | Verbose diagnostic output |

#### `migrate`

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | - | Database connection string (required) |
| `--dir` | `./migrations` | Migrations directory |
| `--dry-run` | `false` | Print pending migrations without applying |

#### `rollback`

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | - | Database connection string (required) |
| `--dir` | `./migrations` | Migrations directory |
| `--force` | `false` | Skip confirmation |
| `--dry-run` | `false` | Print down statements without applying |

**Args:** `[N]` - optional rollback count (default: 1)

#### `status`

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | - | Database connection string (optional) |
| `--dir` | `./migrations` | Migrations directory |
| `--format` | `table` | Output format: `table` or `json` |
| `--check-drift` | `false` | Compare live database schema against the last snapshot and report manual changes (requires `--db`) |

#### Drift Detection

`mirage status --check-drift` connects to a live PostgreSQL database, reads its catalog (tables, columns, constraints, enums, etc.), and compares it against the last applied migration snapshot. Any differences are reported as drift events — schema changes made outside of mirage migrations.

```bash
mirage status --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable" --check-drift
```

Drift is detected for: tables (added/dropped columns, renamed columns), enums (added/removed values), indexes, foreign keys, unique constraints, check constraints, and extensions.

#### `create`

| Flag | Default | Description |
|------|---------|-------------|
| `--dir` | `./migrations` | Migrations directory |
| `--message, -m` | - | Migration description |
| `--sequential-naming` | `false` | Use sequential version numbers |

#### `reset`

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | - | Database connection string (required) |
| `--force` | `false` | Skip confirmation |

#### `validate`

| Flag | Default | Description |
|------|---------|-------------|
| `--source` | - | Source directories to scan (required, repeatable) |
| `--recursive, -r` | `false` | Scan subdirectories |

## Struct Tag Syntax (`db` tag)

### Table Declaration

Every model struct needs a blank `_` field with a `db` tag to declare the table name:

```go
type User struct {
    _ struct{} `db:"name=users,comment=User accounts"`

    ID int64 `db:"pk,identity,type=bigserial"`
    // ...
}
```

### Column Tags

| Key | Format | Description |
|-----|--------|-------------|
| `name` | `name=col_name` | Custom column name (default: snake_case of field name) |
| `type` | `type=varchar(255)` | SQL data type |
| `pk` | `pk` | Primary key (bare flag) |
| `identity` | `identity` | SERIAL / GENERATED ALWAYS AS IDENTITY |
| `unique` | `unique` | Unique constraint |
| `notnull` | `notnull` | NOT NULL |
| `null` | `null` | Explicitly nullable |
| `default` | `default=NOW()` | Default value |
| `check` | `check=price >= 0` | CHECK constraint |
| `ref` | `ref=users.id ON DELETE CASCADE` | Foreign key (space before ON DELETE) |
| `index` | `index` or `index=gin` | Create index |
| `unique_index` | `unique_index=idx_name` | Create unique index |
| `generated` | `generated=always` | Generated column |
| `collate` | `collate=C` | COLLATE clause |
| `comment` | `comment=...` | Column comment |
| `ignore` | `ignore` | Skip this field |

### Args Syntax (parenthesized)

Some tags accept parenthesized arguments for composite or multi-value options:

```go
type Order struct {
    _ struct{} `db:"name=orders,partitioned(RANGE,created_at)"`

    ID       int64 `db:"pk,identity,type=bigserial"`
    UserID   int64 `db:"name=user_id,type=bigint,notnull,ref=users.id ON DELETE RESTRICT"`
    TotalCents int64 `db:"name=total_cents,type=bigint,notnull,check=total_cents >= 0"`
}
```

### Full Example

```go
type User struct {
    _ struct{} `db:"name=users,comment=User accounts"`

    ID        int64     `db:"pk,identity,type=bigserial"`
    Email     string    `db:"name=email,type=varchar(255),unique,notnull"`
    Name      string    `db:"name=name,type=varchar(100),notnull"`
    Age       int       `db:"name=age,type=int,default=0,check=age >= 0"`
    Config    string    `db:"name=config,type=jsonb,default='{}'"`
    Status    string    `db:"name=status,type=varchar(20),notnull,default='active',check=status IN ('active','suspended','deleted')"`
    CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
    UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}
```

### Enum Types

Defined as `type X string` with `const` values:

```go
type UserRole string

const (
    RoleAdmin  UserRole = "admin"
    RoleEditor UserRole = "editor"
    RoleViewer UserRole = "viewer"
)
```

### Registering Database Objects

Functions, views, materialized views, triggers, procedures, grants, and policies are registered via `mirage.Register()`. The scanner detects these calls at scan time and includes them in migration generation.

```go
package models

import "github.com/justblue/mirage"

// Function — CREATE FUNCTION
var _ = mirage.Register(mirage.Function{
    Name:       "update_timestamps",
    Language:   "plpgsql",
    ReturnType: "trigger",
    Body: `BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;`,
})

// Trigger — CREATE TRIGGER
var _ = mirage.Register(mirage.Trigger{
    Name:     "trg_users_updated_at",
    Table:    "users",
    Timing:   "BEFORE",
    Events:   []string{"INSERT", "UPDATE"},
    Function: "update_timestamps",
})

// View — CREATE VIEW
var _ = mirage.Register(mirage.View{
    Name:  "active_users",
    Query: "SELECT * FROM users WHERE status = 'active'",
})

// Materialized View — CREATE MATERIALIZED VIEW
var _ = mirage.Register(mirage.MaterializedView{
    Name:  "user_stats",
    Query: "SELECT role, count(*) AS total FROM users GROUP BY role",
})

// Procedure — CREATE PROCEDURE
var _ = mirage.Register(mirage.Procedure{
    Name:     "refresh_stats",
    Language: "plpgsql",
    Body:     `BEGIN REFRESH MATERIALIZED VIEW user_stats; END;`,
})

// Grant — GRANT / REVOKE
var _ = mirage.Register(mirage.Grant{
    ObjectType: "table",
    ObjectName: "users",
    Privileges: []string{"SELECT", "INSERT"},
    Roles:      []string{"app_role"},
})

// Policy — Row-level security
var _ = mirage.Register(mirage.Policy{
    Name:    "user_isolation",
    Table:   "users",
    Command: "ALL",
    Roles:   []string{"app_role"},
    Using:   "auth.uid() = id",
})
```

Objects can also be registered inside `init()`:

```go
func init() {
    mirage.Register(mirage.View{
        Name:  "v_orders",
        Query: "SELECT * FROM orders WHERE status = 'pending'",
    })
}
```

**Key fields:**

| Type | Required | Optional |
|------|----------|----------|
| `Function` | `Name`, `Language`, `Body`, `ReturnType` | `Arguments`, `Description`, `Volatility`, `Security` |
| `View` | `Name`, `Query` | `Description` |
| `MaterializedView` | `Name`, `Query` | `Description` |
| `Trigger` | `Name`, `Table`, `Timing`, `Events`, `Function` | `Description`, `Constraint` |
| `Procedure` | `Name`, `Language`, `Body` | `Arguments`, `Description` |
| `Grant` | `ObjectType`, `ObjectName`, `Privileges`, `Roles` | `SearchPath` |
| `Policy` | `Name`, `Table`, `Command`, `Roles`, `Using` | `Check`, `Permissive` |

Triggers reference functions by name — the referenced function must also be registered. The scanner validates this and produces errors for missing references.

## Library API

### Connection

```go
// Open from connection string
db, err := mirage.Open(ctx, connString, mirage.WithLogger(logger))

// Open from existing pool
db := mirage.OpenPool(pool)
```

### CRUD Operations

Table definitions are automatically derived from struct tags on first use. No manual registration needed.

```go
// Create a repository
repo := mirage.NewRepository[User](db)

// Insert
user := &User{Name: "Alice", Email: "alice@example.com"}
err := repo.InsertReturning(ctx, user)

// Insert many (batch)
err = repo.InsertMany(ctx, []*User{user1, user2, user3})

// Insert many with RETURNING
err = repo.InsertManyReturning(ctx, []*User{user1, user2})

// Upsert
err = repo.Upsert(ctx, user, "DO UPDATE SET name = EXCLUDED.name")

// Upsert many
err = repo.UpsertMany(ctx, []*User{user1, user2}, "DO UPDATE SET name = EXCLUDED.name")

// Select by ID (transparent cache when configured)
found, err := repo.SelectByID(ctx, 42)

// Exists
exists, err := repo.Exists(ctx, &User{Email: "alice@example.com"})

// Update
user.Name = "Bob"
rowsAffected, err := repo.Update(ctx, user)

// Update specific columns only
rowsAffected, err = repo.UpdateOnlyColumns(ctx, []string{"name"}, user)

// Update all except specific columns
rowsAffected, err = repo.UpdateExceptColumns(ctx, []string{"created_at"}, user)

// Update many
rowsAffected, err = repo.UpdateMany(ctx, []*User{user1, user2})

// Delete
rowsAffected, err = repo.Delete(ctx, user)

// Delete many
rowsAffected, err = repo.DeleteMany(ctx, []*User{user1, user2})

// Duplicate (copy a row with new ID)
copy, err := repo.Duplicate(ctx, user)

// Query with raw SQL
users, err := repo.Query(ctx, "SELECT * FROM users WHERE active = $1", true)

// Single row query
user, err = repo.QuerySingle(ctx, "SELECT * FROM users WHERE email = $1", "alice@example.com")

// Query with cache
users, err = repo.QueryWithCache(ctx, "users:active", 5*time.Minute,
    "SELECT * FROM users WHERE active = $1", true)

// Sanitize SQL identifier
safe := mirage.QuoteIdentifier("users")
```

### Raw SQL

```go
// Query boolean
ok, err := db.QueryBoolean(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", "alice@example.com")

// Raw pgx queries
rows, err := db.Query(ctx, "SELECT * FROM users WHERE id = $1", 42)
defer rows.Close()
row := db.QueryRow(ctx, "SELECT count(*) FROM users")
```

For typed row scanning, use the Repository methods `Query` and `QuerySingle` which scan rows into `[]*T` and `*T` respectively.

### Transactions

```go
// Automatic commit/rollback
err := db.InTransaction(ctx, func(tx *mirage.DB) error {
    _, err := tx.Exec(ctx, "INSERT INTO ...")
    return err // rolls back on error
})

// Return ErrIntentionalRollback to force rollback without error
err := db.InTransaction(ctx, func(tx *mirage.DB) error {
    // ... validation ...
    if invalid {
        return mirage.ErrIntentionalRollback
    }
    return nil // commits
})

// Manual transaction
tx, err := db.Begin(ctx)
defer tx.Rollback(ctx)
// ... queries ...
err = tx.Commit(ctx)

// Concurrent transaction (goroutine-safe)
tx, err := db.BeginConcurrent(ctx)

// Transaction with automatic retry on serialization failures
err = db.InTransactionWithRetry(ctx, mirage.RetryOptions{
    MaxAttempts: 3,
}, func(tx *mirage.DB) error {
    _, err := tx.Exec(ctx, "INSERT INTO ...")
    return err
})
```

### Generic Query Helpers

```go
// Sanitize SQL identifier
safe := mirage.QuoteIdentifier("users")
```

### Notifications

```go
// Listen for notifications
listener, err := db.Listen(ctx, "my_channel")
defer listener.Close(ctx)

for {
    notification, err := listener.Accept(ctx)
    // handle notification
}

// Stop listening
err = db.Unlisten(ctx, "my_channel")

// Send notifications
err = db.Notify(ctx, "my_channel", "hello")
err = db.Notify(ctx, "my_channel", map[string]any{"key": "value"}) // auto-JSON

// Listen for table changes (auto-creates triggers)
closer, err := db.ListenTable(ctx, &mirage.ListenTableOptions{
    Tables:  map[string][]mirage.TableChangeType{"users": {mirage.TableChangeTypeInsert, mirage.TableChangeTypeUpdate}},
    Channel: "user_changes",
}, func(n mirage.TableNotificationJSON, err error) error {
    if err != nil {
        return err
    }
    fmt.Printf("Table: %s, Change: %s\n", n.Table, n.Change)
    return nil
})
defer closer.Close(ctx)
```

### Caching

```go
// Enable transparent caching for SelectByID and Exists
cache := mirage.NewInMemoryCache()
repo := mirage.NewRepository[User](db, mirage.WithCacheJitter(cache, 5*time.Minute, 6*time.Minute))

// SelectByID and Exists auto-use the cache
user, err := repo.SelectByID(ctx, 42)

// Explicit cache for custom queries
users, err := repo.QueryWithCache(ctx, "users:active", 5*time.Minute,
    `SELECT * FROM users WHERE active = $1`, true)

// Invalidate cache after mutations (automatic on Insert/Update/Delete)
// or manually:
err = repo.InvalidateCache(ctx, "users:")
```

### Row-Level Locking

```go
// Select with FOR UPDATE (exclusive lock, blocks until available)
user, err := repo.SelectByIDForUpdate(ctx, 42, mirage.ForUpdate())

// Fail immediately if row is locked (SQLSTATE 55P03)
user, err = repo.SelectByIDForUpdate(ctx, 42, mirage.ForUpdateNoWait())

// Skip locked rows (job-queue / outbox pattern)
user, err = repo.SelectByIDForUpdate(ctx, 42, mirage.ForUpdateSkipLocked())

// Lock options also work for raw SQL queries
users, err := repo.QueryForUpdate(ctx, mirage.ForUpdate(),
    "SELECT * FROM users WHERE status = $1 FOR UPDATE", "active")
```

Locking never reads from cache and bypasses the repository cache entirely. When inside a transaction, the lock is held until the transaction commits or rolls back.

### Retry Helpers

```go
// Retry on serialization failures and deadlocks (exponential backoff)
repo := mirage.NewRepository[User](db, mirage.WithRetry(mirage.RetryOptions{
    MaxAttempts: 3,
    BaseDelay:   10 * time.Millisecond,
}))

// Custom retry predicate
repo = mirage.NewRepository[User](db, mirage.WithRetry(mirage.RetryOptions{
    MaxAttempts: 5,
    ShouldRetry: func(err error) bool {
        return mirage.IsErrRetryable(err) || myCustomCheck(err)
    },
}))

// Direct transaction-level retry (not per-repo)
err := db.InTransactionWithRetry(ctx, mirage.RetryOptions{
    MaxAttempts: 3,
}, func(tx *mirage.DB) error {
    _, err := tx.Exec(ctx, "UPDATE ...")
    return err
})
```

Retry is skipped when the repository is already inside an existing transaction.

### Unit of Work

```go
// Create a Unit of Work for cross-module transactional coordination
uow := mirage.NewUnitOfWork(pool)

// Start a unit of work with context
ctx, err := uow.Begin(ctx)
defer uow.Rollback(ctx)

// Repositories get the active transaction from context
repo := mirage.NewRepository[User](db)
// ...

err = uow.Commit(ctx)
```

### Config File

Mirage supports an optional YAML config file. It searches from the current directory upward for `mirage.yaml` or `.mirage.yaml`. CLI flags always take precedence over config values.

```yaml
# mirage.yaml
source:
  - ./internal/models
  - ./internal/legacy
migrations_dir: ./migrations
db: "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
idempotent: true
verbose: false
```

Environment variables are expanded in the `db` field (`${DATABASE_URL}` or `$DATABASE_URL` syntax). A missing config file is not an error — all values fall back to their CLI defaults.

### Executing Embedded SQL Files

```go
// Execute SQL files from an embed.FS inside a transaction
err := db.ExecFiles(ctx, embedFS, "migrations/001_init.sql", "migrations/002_seed.sql")
```

### Authentication Helper

```go
// Select by username + password (uses PostgreSQL crypt())
repo := mirage.NewRepository[Account](db)
account, err := repo.SelectByUsernameAndPassword(ctx, "alice@example.com", "secret123")
```

### Concurrent Transactions

```go
// Goroutine-safe transaction using mutex-protected ConcurrentTx
tx, err := db.BeginConcurrent(ctx)
defer tx.Rollback(ctx)

var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        tx.Exec(ctx, "UPDATE counters SET val = val + 1 WHERE id = $1", i)
    }()
}
wg.Wait()
err = tx.Commit(ctx)
```

### Deserializing Notifications

```go
type UserEvent struct {
    UserEmail string `json:"user_email"`
    Action    string `json:"action"`
}

notification, err := listener.Accept(ctx)
event, err := mirage.UnmarshalNotification[UserEvent](notification)
fmt.Printf("Event: %s by %s\n", event.Action, event.UserEmail)
```

### Manual Trigger Setup

```go
// Prepare table change triggers without starting a listener
err := db.PrepareListenTable(ctx, &mirage.ListenTableOptions{
    Tables:  map[string][]mirage.TableChangeType{"users": {mirage.TableChangeTypeInsert}},
    Channel: "user_events",
})
```

### Pool Statistics

```go
stat := db.PoolStat()
fmt.Printf("Acquired: %d, Idle: %d, Max: %d\n", stat.AcquiredConns, stat.IdleConns, stat.MaxConns)
```

### Global Configuration

```go
// Override struct tag key (default: "db")
mirage.SetDefaultTag("gorm")

// Override default search path (default: "public")
mirage.SetDefaultSearchPath("myschema")
```

### Custom Table Names

```go
// Implement TableNamer to override auto-derived table name
type SpecialItem struct {
    _ struct{} `db:"special_items"`
    ID int64   `db:"pk,identity,type=bigserial"`
}

func (SpecialItem) TableName() string {
    return "special_items" // overrides pluralized snake_case
}
```

### Error Helpers

```go
if mirage.IsErrNoRows(err) {
    // no rows found
}

if constraintName, ok := mirage.IsErrDuplicate(err); ok {
    // unique violation on constraint: constraintName
}

if constraintName, ok := mirage.IsErrForeignKey(err); ok {
    // foreign key violation on constraint: constraintName
}

if detail, ok := mirage.IsErrInputSyntax(err); ok {
    // invalid input syntax: detail
}

if col, ok := mirage.IsErrColumnNotExists(err); ok {
    // column does not exist: col
}

if mirage.IsErrSerializationFailure(err) {
    // PostgreSQL serialization failure (SQLSTATE 40001)
}

if mirage.IsErrDeadlock(err) {
    // PostgreSQL deadlock detected (SQLSTATE 40P01)
}

if mirage.IsErrRetryable(err) {
    // serialization failure or deadlock — transient, safe to retry
}
```

### Column Name Mapping

```go
// Default: snake_case
mirage.SetDefaultColumnNameMapper(nil)

// Identity: field name as-is
mirage.SetDefaultColumnNameMapper(mirage.NoColumnNameMapper)

// From json tag
mirage.SetDefaultColumnNameMapper(mirage.JSONColumnNameMapper)

// Custom
mirage.SetDefaultColumnNameMapper(func(field reflect.StructField) string {
    return strings.ToLower(field.Name)
})
```

## License

[MIT](LICENSE.md)
