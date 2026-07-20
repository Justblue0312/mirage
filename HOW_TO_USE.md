# How to Use Mirage

A practical guide to using Mirage as a Go library and CLI tool.

## Installation

```bash
# Go library
go get github.com/justblue/mirage

# CLI tool
go install github.com/justblue/mirage/cmd/mirage@latest
```

## Defining Models

All models use Go struct tags with the `db` key to define schema. Database objects like functions, views, and triggers are defined via `mirage.Register()` — see [Registering Database Objects](#registering-database-objects) below.

### Basic Model

```go
package models

import "time"

type User struct {
    _ struct{} `db:"name=users"`

    ID        int64     `db:"pk,identity,type=bigserial"`
    Email     string    `db:"name=email,type=varchar(255),unique,notnull"`
    Name      string    `db:"name=name,type=varchar(100),notnull"`
    CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
    UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}
```

### Foreign Keys

```go
type Order struct {
    _ struct{} `db:"name=orders"`

    ID     int64   `db:"pk,identity,type=bigserial"`
    UserID int64   `db:"name=user_id,type=bigint,notnull,ref=users.id ON DELETE CASCADE"`
    Total  float64 `db:"name=total,type=numeric(10,2),notnull,check=total >= 0"`
}
```

### Enums

Define enums as `type X string` with `const` values:

```go
type OrderStatus string

const (
    OrderPending   OrderStatus = "pending"
    OrderShipped   OrderStatus = "shipped"
    OrderDelivered OrderStatus = "delivered"
    OrderCancelled OrderStatus = "cancelled"
)

type Order struct {
    _ struct{} `db:"name=orders"`

    ID     int64       `db:"pk,identity,type=bigserial"`
    Status OrderStatus `db:"name=status,type=order_status,notnull,default='pending'"`
}
```

### Indexes

```go
type Product struct {
    _ struct{} `db:"name=products"`

    ID    int64  `db:"pk,identity,type=bigserial"`
    Name  string `db:"name=name,type=varchar(200),notnull"`
    Tags  string `db:"name=tags,type=tsvector,index=gin"`
    SKU   string `db:"name=sku,type=varchar(50),unique_index=idx_product_sku"`
}
```

### Partitioned Tables

```go
type Event struct {
    _ struct{} `db:"name=events,partitioned(RANGE,created_at)"`

    ID        int64     `db:"pk,identity,type=bigserial"`
    CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull"`
    Payload   string    `db:"name=payload,type=jsonb"`
}
```

### Full Tag Reference

| Key | Format | Description |
|-----|--------|-------------|
| `name` | `name=col_name` | Custom column name |
| `type` | `type=varchar(255)` | SQL data type |
| `pk` | `pk` | Primary key |
| `identity` | `identity` | SERIAL / GENERATED ALWAYS AS IDENTITY |
| `unique` | `unique` | Unique constraint |
| `notnull` | `notnull` | NOT NULL |
| `null` | `null` | Explicitly nullable |
| `default` | `default=NOW()` | Default value |
| `check` | `check=price >= 0` | CHECK constraint |
| `ref` | `ref=users.id ON DELETE CASCADE` | Foreign key |
| `index` | `index` or `index=gin` | Create index |
| `unique_index` | `unique_index=idx_name` | Create unique index |
| `generated` | `generated=always` | Generated column |
| `collate` | `collate=C` | COLLATE clause |
| `comment` | `comment=...` | Column comment |
| `ignore` | `ignore` | Skip this field |

## Registering Database Objects

Beyond struct-based tables and enums, mirage supports registering database functions, views, materialized views, triggers, procedures, grants, and policies via `mirage.Register()`.

The scanner detects `var _ = mirage.Register(...)` and `func init() { mirage.Register(...) }` patterns at scan time and includes them in migration generation.

### Functions and Triggers

```go
package models

import "github.com/justblue/mirage"

// A PL/pgSQL function that auto-updates the updated_at column.
var _ = mirage.Register(mirage.Function{
    Name:        "update_timestamps",
    Description: "Sets updated_at to current timestamp",
    Language:    "plpgsql",
    ReturnType:  "trigger",
    Body: `BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;`,
})

// A trigger that fires the function before every UPDATE on users.
var _ = mirage.Register(mirage.Trigger{
    Name:     "trg_users_updated_at",
    Table:    "users",
    Timing:   "BEFORE",
    Events:   []string{"INSERT", "UPDATE"},
    Function: "update_timestamps",
})
```

### Views and Materialized Views

```go
// Regular view — always reflects current data.
var _ = mirage.Register(mirage.View{
    Name:  "active_users",
    Query: "SELECT id, username, email FROM users WHERE status = 'active'",
})

// Materialized view — must be explicitly refreshed.
var _ = mirage.Register(mirage.MaterializedView{
    Name:  "user_stats",
    Query: "SELECT role, count(*) AS total FROM users GROUP BY role",
})
```

### Procedures

```go
var _ = mirage.Register(mirage.Procedure{
    Name:     "refresh_stats",
    Language: "plpgsql",
    Body:     `BEGIN REFRESH MATERIALIZED VIEW user_stats; END;`,
})
```

### Grants

```go
var _ = mirage.Register(mirage.Grant{
    ObjectType: "table",
    ObjectName: "users",
    Privileges: []string{"SELECT", "INSERT", "UPDATE"},
    Roles:      []string{"app_role"},
})
```

### Policies (Row-Level Security)

```go
var _ = mirage.Register(mirage.Policy{
    Name:       "user_isolation",
    Table:      "users",
    Command:    "ALL",
    Roles:      []string{"app_role"},
    Using:      "auth.uid() = id",
    Permissive: "PERMISSIVE",
})
```

### Function Arguments

```go
var _ = mirage.Register(mirage.Function{
    Name:       "find_users",
    Language:   "sql",
    ReturnType: "SETOF users",
    Arguments: []mirage.FunctionArgument{
        {Name: "p_role", Type: "user_role", Mode: "IN"},
        {Name: "p_limit", Type: "int", Mode: "IN"},
    },
    Body: `SELECT * FROM users WHERE role = p_role LIMIT p_limit`,
})
```

### init() Pattern

For more complex registrations, use `func init()`:

```go
func init() {
    mirage.Register(
        mirage.Function{Name: "fn1", Language: "plpgsql", Body: "...", ReturnType: "void"},
        mirage.View{Name: "v1", Query: "SELECT 1"},
    )
}
```

### Migration Ordering

Mirage automatically orders generated SQL so that dependencies are respected:

- **Up:** functions/procedures → tables → views/mat views/triggers → grants/policies
- **Down:** reverse order (policies/grants → triggers → views → tables → functions)

Changed objects are always drop+recreated for consistency.

## Migration Workflow

### 1. Initialize

```bash
mirage init --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
```

Creates a `migrations/` directory and the `schema_migrations` tracking table.

### 2. Generate Migrations

After defining or modifying your models:

```bash
mirage generate \
  --source ./internal/models \
  --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable" \
  -m "create users table"
```

This scans your structs, diffs against saved state, and writes a migration file like `V20260101120000_create_users_table.sql`.

Preview without writing:

```bash
mirage generate --source ./internal/models --dry-run -m "create users table"
```

### 3. Apply Migrations

```bash
mirage migrate --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
```

Preview what would run:

```bash
mirage migrate --db "postgres://..." --dry-run
```

### 4. Check Status

```bash
mirage status --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
```

### 5. Rollback

```bash
# Roll back last 1 migration
mirage rollback --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable"

# Roll back last 3 migrations
mirage rollback 3 --db "postgres://user:pass@localhost:5432/mydb?sslmode=disable"

# Dry-run rollback
mirage rollback --db "postgres://..." --dry-run
```

### 6. Create Manual Migration

```bash
mirage create -m "add index on users email"
```

Creates an empty migration file with `-- +migrate Up` / `-- +migrate Down` markers for you to fill in.

## Library Usage

### Connecting

```go
import mirage "github.com/justblue/mirage"

ctx := context.Background()

// From connection string
db, err := mirage.Open(ctx,
    "postgres://user:pass@localhost:5432/mydb?sslmode=disable&search_path=public",
    mirage.WithLogger(logger),
)
if err != nil {
    log.Fatal(err)
}
defer db.Close()

// From existing pgxpool
pool, _ := pgxpool.New(ctx, connStr)
db = mirage.OpenPool(pool)
```

### Insert

```go
repo := mirage.NewRepository[User](db)

// Insert single (scans generated ID back)
user := &User{Name: "Alice", Email: "alice@example.com"}
err := repo.InsertReturning(ctx, user)

// Insert multiple (runs in a transaction)
err = repo.InsertMany(ctx, []*User{user1, user2, user3})
```

### Select

```go
// By primary key (transparent cache when configured)
found, err := repo.SelectByID(ctx, 42)

// With raw SQL
var users []*User
users, err = repo.Query(ctx,
    "SELECT * FROM users WHERE created_at > $1 ORDER BY name", cutoff)

// Single row
user, err = repo.QuerySingle(ctx,
    "SELECT * FROM users WHERE email = $1", "alice@example.com")

// Exists check
exists, err := repo.Exists(ctx, &User{Email: "alice@example.com"})
```

### Update

```go
// Update all non-zero fields
user.Name = "Bob"
rowsAffected, err := repo.Update(ctx, user)

// Update only specific columns
rowsAffected, err = repo.UpdateOnlyColumns(ctx, []string{"name", "updated_at"}, user)

// Update all except specific columns
rowsAffected, err = repo.UpdateExceptColumns(ctx, []string{"created_at"}, user)

// Update many
rowsAffected, err = repo.UpdateMany(ctx, []*User{user1, user2})
```

### Delete

```go
rowsAffected, err := repo.Delete(ctx, user)

// Delete many
rowsAffected, err = repo.DeleteMany(ctx, []*User{user1, user2})
```

### Upsert

```go
// Single upsert (ON CONFLICT on "email" column)
err := repo.Upsert(ctx, user, "DO UPDATE SET name = EXCLUDED.name")

// Batch upsert
err = repo.UpsertMany(ctx, []*User{user1, user2, user3}, "DO UPDATE SET name = EXCLUDED.name")
```

### Duplicate

```go
// Copy a row with a new auto-generated ID
copy, err := repo.Duplicate(ctx, user)
```

### Generic Query Helpers

```go
// Safe identifier quoting
table := mirage.QuoteIdentifier("users")
```

### Transactions

```go
// Recommended: automatic commit/rollback
err := db.InTransaction(ctx, func(tx *mirage.DB) error {
    _, err := tx.Exec(ctx, "INSERT INTO orders (...) VALUES (...)")
    if err != nil {
        return err // rolls back
    }
    _, err = tx.Exec(ctx, "UPDATE inventory SET ...")
    return err // commits if nil
})

// Force rollback without error
err = db.InTransaction(ctx, func(tx *mirage.DB) error {
    // validation failed
    return mirage.ErrIntentionalRollback
})

// Manual control
tx, err := db.Begin(ctx)
defer tx.Rollback(ctx)
_, err = tx.Exec(ctx, "...")
err = tx.Commit(ctx)
```

### Concurrent Transactions

For safe multi-goroutine use within a single transaction:

```go
tx, err := db.BeginConcurrent(ctx)
defer tx.Rollback(ctx)

var wg sync.WaitGroup
for _, item := range items {
    wg.Add(1)
    go func(it Item) {
        defer wg.Done()
        tx.Exec(ctx, "UPDATE inventory SET qty = qty - $1 WHERE id = $2", it.Qty, it.ID)
    }(item)
}
wg.Wait()
err = tx.Commit(ctx)
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
    "SELECT * FROM users WHERE active = $1", true)

// Invalidate cache after mutations (automatic on Insert/Update/Delete)
// or manually:
err = repo.InvalidateCache(ctx, "users:")
```

### Executing Embedded SQL Files

```go
//go:embed migrations/*.sql
var migrations embed.FS

err := db.ExecFiles(ctx, migrations, "migrations/001_init.sql")
```

### Notifications

```go
// Listen for custom notifications
listener, err := db.Listen(ctx, "my_channel")
defer listener.Close(ctx)

go func() {
    for {
        n, err := listener.Accept(ctx)
        if err != nil {
            break
        }
        fmt.Printf("Received: %s\n", n.Payload)
    }
}()

// Send notifications (string, []byte, or auto-JSON)
err = db.Notify(ctx, "my_channel", "hello")
err = db.Notify(ctx, "my_channel", map[string]any{"event": "signup"})

// Deserialize generic payloads
type SignupEvent struct {
    Email string `json:"email"`
}
event, err := mirage.UnmarshalNotification[SignupEvent](n)

// Stop listening
err = db.Unlisten(ctx, "my_channel")
```

### Table Change Triggers

Automatically create PostgreSQL triggers that notify on INSERT, UPDATE, DELETE:

```go
closer, err := db.ListenTable(ctx, &mirage.ListenTableOptions{
    Tables: map[string][]mirage.TableChangeType{
        "users":  {mirage.TableChangeTypeInsert, mirage.TableChangeTypeUpdate},
        "orders": {mirage.TableChangeTypeInsert},
    },
    Channel: "data_changes",
}, func(n mirage.TableNotificationJSON, err error) error {
    if err != nil {
        return err
    }
    fmt.Printf("Table: %s, Change: %s\n", n.Table, n.Change)
    return nil
})
defer closer.Close(ctx)
```

To set up triggers without starting a listener:

```go
err := db.PrepareListenTable(ctx, &mirage.ListenTableOptions{
    Tables: map[string][]mirage.TableChangeType{
        "users": {mirage.TableChangeTypeInsert, mirage.TableChangeTypeUpdate, mirage.TableChangeTypeDelete},
    },
})
```

### Error Handling

```go
var user User
err := db.SelectByID(ctx, &user, 999)

if mirage.IsErrNoRows(err) {
    // User not found
}

if name, ok := mirage.IsErrDuplicate(err); ok {
    // Unique violation on constraint: name
}

if name, ok := mirage.IsErrForeignKey(err); ok {
    // FK violation on constraint: name
}

if typeName, ok := mirage.IsErrInputSyntax(err); ok {
    // Invalid input syntax for type: typeName
}

if col, ok := mirage.IsErrColumnNotExists(err); ok {
    // Column does not exist: col
}
```

### Column Name Mapping

```go
// Default: field name -> snake_case (Name -> name, CreatedAt -> created_at)
mirage.SetDefaultColumnNameMapper(nil)

// Identity: field name as-is (Name -> Name)
mirage.SetDefaultColumnNameMapper(mirage.NoColumnNameMapper)

// From json struct tag (falls back to field name)
mirage.SetDefaultColumnNameMapper(mirage.JSONColumnNameMapper)

// Custom mapper
mirage.SetDefaultColumnNameMapper(func(field reflect.StructField) string {
    return strings.ToLower(field.Name)
})
```

### Pool Monitoring

```go
stat := db.PoolStat()
fmt.Printf("Acquired: %d / %d, Idle: %d\n",
    stat.AcquiredConns, stat.MaxConns, stat.IdleConns)
```

### Custom Table Names

Implement the `TableNamer` interface to override the auto-derived table name:

```go
type SpecialItem struct {
    ID int64 `db:"pk,identity,type=bigserial"`
}

func (SpecialItem) TableName() string {
    return "special_items"
}
```
