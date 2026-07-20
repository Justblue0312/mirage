//go:build integration

package integration

import (
	"context"
	"testing"

	mirage "github.com/justblue/mirage"
)

// snowflakeID is a custom defined type over int64. This is the exact case that
// a []any-based array parameter mishandles: pgx v5 cannot determine a single
// element OID for []any, but a concretely-typed []snowflakeID encodes fine.
type snowflakeID int64

type gadget struct {
	_    struct{}    `db:"gadgets_batch_delete_test"`
	ID   snowflakeID `db:"pk"`
	Name string      `db:"type:text"`
}

func (gadget) TableName() string { return "gadgets_batch_delete_test" }

// TestRepository_BatchDeleteCustomPKType is the regression/verification test
// for batch-delete array encoding with a non-trivial PK type. It inserts
// several rows with explicit custom-typed IDs and deletes them in a single
// repo.Delete call, which routes through BuildDeleteQuery's `= ANY($1)` clause.
func TestRepository_BatchDeleteCustomPKType(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(ctx, `DROP TABLE IF EXISTS gadgets_batch_delete_test;
		CREATE TABLE gadgets_batch_delete_test (id bigint PRIMARY KEY, name text);`)
	if err != nil {
		t.Fatalf("creating table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DROP TABLE IF EXISTS gadgets_batch_delete_test;`)
	})

	rows := []*gadget{
		{ID: 101, Name: "a"},
		{ID: 102, Name: "b"},
		{ID: 103, Name: "c"},
	}
	for _, r := range rows {
		if _, err := db.Exec(ctx,
			`INSERT INTO gadgets_batch_delete_test (id, name) VALUES ($1, $2)`,
			int64(r.ID), r.Name); err != nil {
			t.Fatalf("insert %d: %v", r.ID, err)
		}
	}

	// Delete two of the three in one batch call. Before the fix this would
	// fail (or silently match nothing) because []any could not be encoded as a
	// bigint[] for the ANY($1) parameter.
	repo := mirage.NewRepository[gadget](db)
	n, err := repo.Delete(ctx, rows[0], rows[2])
	if err != nil {
		t.Fatalf("batch Delete with custom PK type: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows deleted, got %d", n)
	}

	var remaining int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM gadgets_batch_delete_test`).Scan(&remaining); err != nil {
		t.Fatalf("counting remaining rows: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected 1 remaining row, got %d", remaining)
	}
}
