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
