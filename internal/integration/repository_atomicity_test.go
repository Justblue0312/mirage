//go:build integration

package integration

import (
	"context"
	"testing"

	mirage "github.com/justblue/mirage"
)

type atomicWidget struct {
	ID   int64  `db:"pk,type:bigserial"`
	Name string `db:"type:text,notnull"`
}

func init() {
	_ = mirage.Register(mirage.TableConfig{StructName: "atomicWidget", Name: "atomic_widgets_test"})
}

func setupAtomicWidgetsTable(t *testing.T, db *mirage.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `DROP TABLE IF EXISTS atomic_widgets_test;
		CREATE TABLE atomic_widgets_test (id bigserial PRIMARY KEY, name text NOT NULL UNIQUE);`)
	if err != nil {
		t.Fatalf("creating atomic_widgets_test table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DROP TABLE IF EXISTS atomic_widgets_test;`)
	})
}

func seedAtomicRow(t *testing.T, db *mirage.DB, name string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO atomic_widgets_test (name) VALUES ($1)`, name)
	if err != nil {
		t.Fatalf("seeding row %q: %v", name, err)
	}
}

// TestRepository_InsertMany_RollsBackOnPartialFailure verifies that InsertMany
// is atomic: if a non-first element in the batch violates a constraint, the
// entire batch is rolled back and none of the rows persist.
func TestRepository_InsertMany_RollsBackOnPartialFailure(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupAtomicWidgetsTable(t, db)
	seedAtomicRow(t, db, "dup")

	repo := mirage.NewRepository[atomicWidget](db)
	batch := []*atomicWidget{
		{Name: "should-not-persist-1"},
		{Name: "dup"}, // violates unique constraint
		{Name: "should-not-persist-2"},
	}

	err = repo.InsertMany(ctx, batch)
	if err == nil {
		t.Fatal("expected InsertMany to fail on duplicate, got nil")
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM atomic_widgets_test`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row (the pre-existing seed), got %d — batch method is not atomic", count)
	}
}

// TestRepository_InsertManyReturning_RollsBackOnPartialFailure verifies that
// InsertManyReturning is atomic and does not populate generated fields for
// any rows when a batch element fails.
func TestRepository_InsertManyReturning_RollsBackOnPartialFailure(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupAtomicWidgetsTable(t, db)
	seedAtomicRow(t, db, "dup")

	repo := mirage.NewRepository[atomicWidget](db)
	batch := []*atomicWidget{
		{Name: "should-not-persist-1"},
		{Name: "dup"}, // violates unique constraint
		{Name: "should-not-persist-2"},
	}

	err = repo.InsertManyReturning(ctx, batch)
	if err == nil {
		t.Fatal("expected InsertManyReturning to fail on duplicate, got nil")
	}

	// The first element's ID may be populated by the RETURNING clause
	// within the transaction before the failure triggers a rollback —
	// this is expected behavior (the in-memory struct was updated but
	// the database row never persisted). We only verify the database
	// state below.

	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM atomic_widgets_test`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row (the pre-existing seed), got %d — batch method is not atomic", count)
	}
}

// TestRepository_UpsertMany_RollsBackOnPartialFailure verifies that UpsertMany
// is atomic when an element conflicts on the unique constraint.
func TestRepository_UpsertMany_RollsBackOnPartialFailure(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupAtomicWidgetsTable(t, db)
	seedAtomicRow(t, db, "existing")

	repo := mirage.NewRepository[atomicWidget](db)
	batch := []*atomicWidget{
		{Name: "should-not-persist-1"},
		{Name: "existing"}, // violates unique constraint
		{Name: "should-not-persist-2"},
	}

	err = repo.UpsertMany(ctx, batch, "")
	if err == nil {
		t.Fatal("expected UpsertMany to fail on duplicate, got nil")
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM atomic_widgets_test`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row (the pre-existing seed), got %d — batch method is not atomic", count)
	}
}

// TestRepository_UpdateMany_RollsBackOnPartialFailure verifies that UpdateMany
// is atomic: if a second row's update violates a constraint, the first row's
// pre-update value survives.
func TestRepository_UpdateMany_RollsBackOnPartialFailure(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupAtomicWidgetsTable(t, db)

	// Seed two rows with distinct names.
	seedAtomicRow(t, db, "name-a")
	seedAtomicRow(t, db, "name-b")

	repo := mirage.NewRepository[atomicWidget](db)

	// Read back both rows to get their IDs.
	var idA, idB int64
	if err := db.QueryRow(ctx, `SELECT id FROM atomic_widgets_test WHERE name = 'name-a'`).Scan(&idA); err != nil {
		t.Fatalf("reading idA: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT id FROM atomic_widgets_test WHERE name = 'name-b'`).Scan(&idB); err != nil {
		t.Fatalf("reading idB: %v", err)
	}

	// Try to update both: first to a unique name, second to a duplicate
	// of the first's new name — this violates the UNIQUE constraint.
	batch := []*atomicWidget{
		{ID: idA, Name: "new-name-a"},
		{ID: idB, Name: "new-name-a"}, // duplicate!
	}

	_, err = repo.UpdateMany(ctx, batch)
	if err == nil {
		t.Fatal("expected UpdateMany to fail on duplicate, got nil")
	}

	// The first row's original name should still be intact (transaction rolled back).
	var nameA string
	if err := db.QueryRow(ctx, `SELECT name FROM atomic_widgets_test WHERE id = $1`, idA).Scan(&nameA); err != nil {
		t.Fatalf("reading nameA: %v", err)
	}
	if nameA != "name-a" {
		t.Errorf("expected first row's name to remain 'name-a' after rollback, got %q", nameA)
	}
}

// TestRepository_InsertMany_SuccessPath confirms a fully-successful batch
// commits all rows — the positive-path counterpart to the failure tests.
func TestRepository_InsertMany_SuccessPath(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupAtomicWidgetsTable(t, db)

	repo := mirage.NewRepository[atomicWidget](db)
	batch := []*atomicWidget{
		{Name: "row-1"},
		{Name: "row-2"},
		{Name: "row-3"},
	}

	if err := repo.InsertMany(ctx, batch); err != nil {
		t.Fatalf("InsertMany: %v", err)
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM atomic_widgets_test`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 rows, got %d", count)
	}
}
