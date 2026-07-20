//go:build integration

package integration

import (
	"context"
	"testing"

	mirage "github.com/justblue/mirage"
)

// TestInTransaction_PropagatesCommitError is the regression test for the
// swallowed-commit-error bug. Before the named-return fix, the deferred
// closure's reassignment to err was discarded, so a COMMIT that failed was
// silently reported as success. Using a DEFERRABLE INITIALLY DEFERRED unique
// constraint, the constraint violation is raised only at COMMIT time, not at
// INSERT time -- so the only way the caller can learn about it is if
// InTransaction correctly propagates the commit error.
func TestInTransaction_PropagatesCommitError(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(ctx, `DROP TABLE IF EXISTS deferred_commit_test`); err != nil {
		t.Fatalf("dropping table: %v", err)
	}
	if _, err := db.Exec(ctx, `
		CREATE TABLE deferred_commit_test (
			id int PRIMARY KEY,
			val int,
			CONSTRAINT deferred_commit_test_val_key UNIQUE (val) DEFERRABLE INITIALLY DEFERRED
		)`); err != nil {
		t.Fatalf("creating table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DROP TABLE IF EXISTS deferred_commit_test`)
	})

	txErr := db.InTransaction(ctx, func(tx *mirage.DB) error {
		if _, err := tx.Exec(ctx, `INSERT INTO deferred_commit_test (id, val) VALUES (1, 100)`); err != nil {
			return err
		}
		// Duplicate val -- the deferred unique constraint only fails at COMMIT.
		if _, err := tx.Exec(ctx, `INSERT INTO deferred_commit_test (id, val) VALUES (2, 100)`); err != nil {
			return err
		}
		return nil
	})

	if txErr == nil {
		t.Fatal("expected InTransaction to propagate the deferred commit error, got nil (commit error swallowed)")
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM deferred_commit_test`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after failed commit, got %d", count)
	}
}
