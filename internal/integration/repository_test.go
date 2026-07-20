//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	mirage "github.com/justblue/mirage"
)

// testMirageDSN resolves the same connection string setupTestDB uses, and
// piggybacks on setupTestDB to guarantee Postgres is actually running
// (via podman-compose locally, or the CI service container) before we
// open our own *mirage.DB against it.
func testMirageDSN(t *testing.T) string {
	t.Helper()
	setupTestDB(t) // ensures Postgres is up; we open our own connection via mirage.Open below

	dsn := dsnFromEnv()
	return dsn
}

func dsnFromEnv() string {
	if v := os.Getenv("MIRAGE_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://test:test@localhost:15432/mirage_test?sslmode=disable"
}

// widget is a minimal fixture type exercising the exact pattern that was
// broken: a "_" table-name marker field and a bigserial primary key with
// no other tags.
type widget struct {
	_    struct{} `db:"widgets_repo_test"` // not read by internal/schema; see TableName() below
	ID   int64    `db:"pk"`
	Name string   `db:"type:text"`
}

// TableName overrides the auto-derived table name ("widgets", from
// pluralizing the Go type name) so this fixture's table can have a name
// that won't collide with anything else in the test database. This also
// exercises the mirage.TableNamer override path itself, which otherwise
// has no test coverage at all.
func (widget) TableName() string { return "widgets_repo_test" }

func setupWidgetsTable(t *testing.T, db *mirage.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `DROP TABLE IF EXISTS widgets_repo_test;
		CREATE TABLE widgets_repo_test (id bigserial PRIMARY KEY, name text);`)
	if err != nil {
		t.Fatalf("creating widgets_repo_test table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DROP TABLE IF EXISTS widgets_repo_test;`)
	})
}

// TestRepository_TableNameResolution locks in the fix for cachedTable
// always deriving an empty table name (every repository-pattern query
// targeted a table literally named ""). Before the fix this failed with
// "zero-length delimited identifier".
func TestRepository_TableNameResolution(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupWidgetsTable(t, db)

	repo := mirage.NewRepository[widget](db)
	w := &widget{Name: "first"}
	if err := repo.InsertReturning(ctx, w); err != nil {
		t.Fatalf("InsertReturning: %v", err)
	}
	if w.ID == 0 {
		t.Fatalf("expected a non-zero generated id, got 0 (table-name resolution likely still broken)")
	}
}

// TestRepository_AutoIncrementPrimaryKeyNotInserted locks in the fix for
// extractColumns including a zero-valued bigserial primary key in the
// INSERT column list, which wrote id=0 explicitly and made every insert
// after the first fail with a primary-key collision.
func TestRepository_AutoIncrementPrimaryKeyNotInserted(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupWidgetsTable(t, db)

	repo := mirage.NewRepository[widget](db)
	w1 := &widget{Name: "first"}
	if err := repo.InsertReturning(ctx, w1); err != nil {
		t.Fatalf("first InsertReturning: %v", err)
	}
	w2 := &widget{Name: "second"}
	if err := repo.InsertReturning(ctx, w2); err != nil {
		t.Fatalf("second InsertReturning: %v (a second insert failing here means the PK is being "+
			"written explicitly instead of left to the sequence)", err)
	}
	if w1.ID == w2.ID {
		t.Fatalf("expected distinct generated ids, got %d and %d", w1.ID, w2.ID)
	}
}

// TestRepository_UpdateAllColumns locks in the fix for db.Update (and
// UpdateOnlyColumns with a nil column list) always resolving to zero
// columns to update, which made the simplest, most obvious call in the
// repository API -- db.Update(ctx, &value) -- fail on every call with
// "no columns to update".
func TestRepository_UpdateAllColumns(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupWidgetsTable(t, db)

	repo := mirage.NewRepository[widget](db)
	w := &widget{Name: "before"}
	if err := repo.InsertReturning(ctx, w); err != nil {
		t.Fatalf("InsertReturning: %v", err)
	}

	n, err := repo.Update(ctx, &widget{ID: w.ID, Name: "after"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row updated, got %d", n)
	}

	got, err := repo.SelectByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("SelectByID: %v", err)
	}
	if got.Name != "after" {
		t.Fatalf("expected name %q after update, got %q", "after", got.Name)
	}
}

// TestRepository_FullLifecycle exercises Insert -> Exists -> Update ->
// SelectByID -> Delete -> Exists end to end, the same sequence used to
// discover the three bugs above, kept together as an end-to-end smoke test.
func TestRepository_FullLifecycle(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupWidgetsTable(t, db)

	repo := mirage.NewRepository[widget](db)
	w := &widget{Name: "gadget"}
	if err := repo.InsertReturning(ctx, w); err != nil {
		t.Fatalf("InsertReturning: %v", err)
	}

	exists, err := repo.Exists(ctx, &widget{ID: w.ID})
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected row to exist after insert")
	}

	if _, err := repo.Update(ctx, &widget{ID: w.ID, Name: "widget-updated"}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.SelectByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("SelectByID: %v", err)
	}
	if got.Name != "widget-updated" {
		t.Fatalf("expected updated name, got %q", got.Name)
	}

	n, err := repo.Delete(ctx, &widget{ID: w.ID})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row deleted, got %d", n)
	}

	exists, err = repo.Exists(ctx, &widget{ID: w.ID})
	if err != nil {
		t.Fatalf("Exists after delete: %v", err)
	}
	if exists {
		t.Fatalf("expected row to no longer exist after delete")
	}
}

// TestUnlisten locks in the fix for Unlisten sending "SELECT UNLISTEN $1;",
// which is not valid PostgreSQL syntax under any circumstance (UNLISTEN is
// a command, not a callable function, and takes no bound parameter) and
// failed on every call.
func TestUnlisten(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	l, err := db.Listen(ctx, "repo_test_channel")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close(ctx)

	if err := db.Unlisten(ctx, "repo_test_channel"); err != nil {
		t.Fatalf("Unlisten: %v", err)
	}
}
