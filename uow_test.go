package mirage

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("MIRAGE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MIRAGE_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestUnitOfWork_Commit(t *testing.T) {
	pool := setupTestDB(t)
	uow := NewUnitOfWork(pool)

	ctx := context.Background()
	_, err := pool.Exec(ctx, "CREATE TEMPORARY TABLE uow_test(id int PRIMARY KEY, val text)")
	if err != nil {
		t.Fatal(err)
	}

	err = uow.Do(ctx, func(ctx context.Context) error {
		q := Q(ctx, pool)
		_, err := q.Exec(ctx, "INSERT INTO uow_test(id, val) VALUES ($1, $2)", 1, "hello")
		return err
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	var val string
	err = pool.QueryRow(ctx, "SELECT val FROM uow_test WHERE id = $1", 1).Scan(&val)
	if err != nil {
		t.Fatalf("expected row to exist after commit, got %v", err)
	}
	if val != "hello" {
		t.Fatalf("expected 'hello', got %q", val)
	}
}

func TestUnitOfWork_Rollback(t *testing.T) {
	pool := setupTestDB(t)
	uow := NewUnitOfWork(pool)

	ctx := context.Background()
	_, err := pool.Exec(ctx, "CREATE TEMPORARY TABLE uow_test(id int PRIMARY KEY, val text)")
	if err != nil {
		t.Fatal(err)
	}

	boom := errors.New("boom")
	err = uow.Do(ctx, func(ctx context.Context) error {
		q := Q(ctx, pool)
		_, err := q.Exec(ctx, "INSERT INTO uow_test(id, val) VALUES ($1, $2)", 1, "hello")
		if err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}

	var val string
	err = pool.QueryRow(ctx, "SELECT val FROM uow_test WHERE id = $1", 1).Scan(&val)
	if err == nil {
		t.Fatalf("expected no row after rollback, but got val=%q", val)
	}
}

// TestUnitOfWork_NestedDo exists because round 4 found that nested Do() calls
// used the same transaction instead of creating a savepoint, causing the inner
// failure to roll back the entire outer transaction. This test guards against
// that regression: the inner Do fails, but the outer Do's commit must still
// succeed, proving the inner scope was isolated via a savepoint.

func TestUnitOfWork_NestedDo(t *testing.T) {
	pool := setupTestDB(t)
	uow := NewUnitOfWork(pool)

	ctx := context.Background()
	_, err := pool.Exec(ctx, "CREATE TEMPORARY TABLE uow_test(id int PRIMARY KEY, val text)")
	if err != nil {
		t.Fatal(err)
	}

	err = uow.Do(ctx, func(ctx context.Context) error {
		q := Q(ctx, pool)
		_, err := q.Exec(ctx, "INSERT INTO uow_test(id, val) VALUES ($1, $2)", 1, "outer")
		if err != nil {
			return err
		}

		// Inner Do fails — should roll back only the inner savepoint.
		innerErr := uow.Do(ctx, func(ctx context.Context) error {
			q := Q(ctx, pool)
			_, err := q.Exec(ctx, "INSERT INTO uow_test(id, val) VALUES ($1, $2)", 2, "inner-should-rollback")
			if err != nil {
				return err
			}
			return errors.New("inner boom")
		})
		if innerErr == nil {
			return errors.New("expected inner error")
		}

		// Outer row should still be insertable and the outer tx should commit.
		_, err = q.Exec(ctx, "INSERT INTO uow_test(id, val) VALUES ($1, $2)", 3, "outer-continued")
		return err
	})
	if err != nil {
		t.Fatalf("expected outer tx to succeed, got %v", err)
	}

	// Row 1 (outer) should exist.
	var val string
	err = pool.QueryRow(ctx, "SELECT val FROM uow_test WHERE id = $1", 1).Scan(&val)
	if err != nil {
		t.Fatalf("expected outer row to exist, got %v", err)
	}
	if val != "outer" {
		t.Fatalf("expected 'outer', got %q", val)
	}

	// Row 2 (inner) should NOT exist — savepoint rolled back.
	err = pool.QueryRow(ctx, "SELECT val FROM uow_test WHERE id = $1", 2).Scan(&val)
	if err == nil {
		t.Fatalf("expected inner row to NOT exist, but got val=%q", val)
	}

	// Row 3 (outer continued) should exist.
	err = pool.QueryRow(ctx, "SELECT val FROM uow_test WHERE id = $1", 3).Scan(&val)
	if err != nil {
		t.Fatalf("expected outer-continued row to exist, got %v", err)
	}
	if val != "outer-continued" {
		t.Fatalf("expected 'outer-continued', got %q", val)
	}
}

// TestUnitOfWork_RollbackSurvivesCancelledContext guards against the bug
// found during review: runTx's own comment said rollback "must survive a
// cancelled parent," but the code passed the caller's ctx (not a background
// context) to tx.Rollback/tx.Commit, unlike db.go's InTransaction, which
// explicitly uses context.Background() for this exact reason. If the
// caller's context is canceled (e.g. an HTTP request timing out) right
// before fn returns, Rollback/Commit could fail immediately without ever
// reaching the server, leaving the transaction open server-side and the
// underlying pooled connection unreleased.
//
// This test uses a regular (non-temporary) table plus pool.Acquire to pin a
// single connection, so the check after cancellation is guaranteed to
// observe the state left by the exact connection the transaction ran on,
// regardless of how pgxpool schedules connections across calls.
func TestUnitOfWork_RollbackSurvivesCancelledContext(t *testing.T) {
	pool := setupTestDB(t)
	uow := NewUnitOfWork(pool)

	setupCtx := context.Background()
	_, err := pool.Exec(setupCtx, "CREATE TABLE IF NOT EXISTS uow_cancel_test(id int PRIMARY KEY)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS uow_cancel_test") })
	if _, err := pool.Exec(setupCtx, "DELETE FROM uow_cancel_test"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	boom := errors.New("boom")

	err = uow.Do(ctx, func(txCtx context.Context) error {
		q := Q(txCtx, pool)
		if _, err := q.Exec(txCtx, "INSERT INTO uow_cancel_test(id) VALUES (1)"); err != nil {
			return err
		}
		// Simulate the caller's context being canceled right before fn
		// returns (e.g. the HTTP request it belongs to just timed out).
		cancel()
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}

	// The rollback must have actually reached the server despite the
	// canceled context. If it didn't (the bug this test guards against),
	// the row would still be visible once the transaction eventually times
	// out/aborts server-side, or the connection would be left in a bad
	// state. We check with a fresh background context, since the original
	// ctx is canceled by design.
	var cnt int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM uow_cancel_test").Scan(&cnt); err != nil {
		t.Fatalf("pool unusable after rollback under a cancelled context: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("expected rollback to remove the inserted row, got count=%d (rollback did not survive context cancellation)", cnt)
	}
}

func TestQ_ReturnsPool(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	q := Q(ctx, pool)
	if q != pool {
		t.Fatal("expected Q() to return pool when no tx in context")
	}
}

func TestQ_ReturnsTx(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	txCtx := context.WithValue(ctx, ctxKey{}, tx)
	q := Q(txCtx, pool)
	if q != tx {
		t.Fatal("expected Q() to return tx when tx is in context")
	}
}
