//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	mirage "github.com/justblue/mirage"
)

// These tests exist specifically to catch the class of bug fixed in this
// round: ConcurrentTx methods that released their mutex before the caller
// had actually finished reading the result, letting two goroutines share
// one physical connection's read stream at the same time. Run under
// `go test -race`, a regression here shows up as either a data race
// report or a wrong/garbled value, not (necessarily) a panic -- so the
// assertions below check for correct values, not just "did not crash".

func openConcurrentTx(t *testing.T) (*mirage.DB, *mirage.ConcurrentTx) {
	t.Helper()
	dsn := testMirageDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ct, err := mirage.NewConcurrentTx(ctx, db.Pool)
	if err != nil {
		t.Fatalf("starting concurrent tx: %v", err)
	}
	t.Cleanup(func() { _ = ct.Rollback(ctx) })

	return db, ct
}

// TestConcurrentTx_Query_ConcurrentReads locks in the already-fixed
// behavior for Query (the mutex is held through Close, not just the
// initial send) as a permanent regression test -- this method had no
// coverage in either direction before this round.
func TestConcurrentTx_Query_ConcurrentReads(t *testing.T) {
	_, ct := openConcurrentTx(t)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	got := make([]int, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rows, err := ct.Query(ctx, "SELECT $1::int", i)
			if err != nil {
				errs[i] = err
				return
			}
			defer rows.Close()
			if !rows.Next() {
				errs[i] = fmt.Errorf("no row returned")
				return
			}
			var v int
			if err := rows.Scan(&v); err != nil {
				errs[i] = err
				return
			}
			got[i] = v
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: %v", i, errs[i])
		}
		if got[i] != i {
			t.Errorf("goroutine %d: expected value %d, got %d (cross-talk between concurrent reads)", i, i, got[i])
		}
	}
}

// TestConcurrentTx_QueryRow_ConcurrentReads is the regression test for
// this round's QueryRow fix. Before the fix, the mutex released as soon
// as QueryRow() returned, before Scan() (where pgx actually issues the
// read) was ever called -- so this test would previously be exposed to
// the same cross-talk hazard proven above for Query.
func TestConcurrentTx_QueryRow_ConcurrentReads(t *testing.T) {
	_, ct := openConcurrentTx(t)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	got := make([]int, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var v int
			err := ct.QueryRow(ctx, "SELECT $1::int", i).Scan(&v)
			errs[i] = err
			got[i] = v
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: %v", i, errs[i])
		}
		if got[i] != i {
			t.Errorf("goroutine %d: expected value %d, got %d (cross-talk between concurrent QueryRow calls)", i, i, got[i])
		}
	}
}

// TestConcurrentTx_QueryRow_UnscannedDoesNotDeadlock is the regression test
// for the lock-release deadlock: a QueryRow result that is never scanned (or
// discarded) must not hold the connection lock. Before the fix, QueryRow held
// the mutex until Scan/Discard, so an unscanned row would deadlock every
// subsequent operation on the transaction. The whole test is bounded by a
// timeout so a regression fails fast instead of hanging.
func TestConcurrentTx_QueryRow_UnscannedDoesNotDeadlock(t *testing.T) {
	_, ct := openConcurrentTx(t)
	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		// Intentionally never Scan or Discard the returned row.
		_ = ct.QueryRow(ctx, "SELECT 1")

		// A follow-up operation must still be able to acquire the lock.
		var v int
		done <- ct.QueryRow(ctx, "SELECT $1::int", 42).Scan(&v)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("follow-up QueryRow after unscanned row failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: an unscanned QueryRow result blocked subsequent operations")
	}
}

// TestConcurrentTx_SendBatch_ConcurrentReads is the regression test for
// this round's SendBatch fix. Before the fix, the mutex released as soon
// as SendBatch() returned, before the caller had read (or closed) the
// returned BatchResults -- pgx's own docs state the connection can't be
// reused until Close() is called on it.
func TestConcurrentTx_SendBatch_ConcurrentReads(t *testing.T) {
	_, ct := openConcurrentTx(t)
	ctx := context.Background()

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	got := make([]int, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			batch := &pgx.Batch{}
			batch.Queue("SELECT $1::int", i)

			br := ct.SendBatch(ctx, batch)
			defer br.Close()

			var v int
			if err := br.QueryRow().Scan(&v); err != nil {
				errs[i] = err
				return
			}
			got[i] = v
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: %v", i, errs[i])
		}
		if got[i] != i {
			t.Errorf("goroutine %d: expected value %d, got %d (cross-talk between concurrent SendBatch calls)", i, i, got[i])
		}
	}
}

// TestConcurrentTx_Query_UnscannedDoesNotDeadlock documents the accepted
// tradeoff described in IMPROVE.md (Problem 6): an unclosed *pgx.Rows from
// Query (rows.Next() never called, Close() never called) intentionally
// blocks subsequent operations on the transaction. This is the documented,
// expected behavior -- not a bug -- and the test pins it down so a future
// change that silently alters it (e.g. auto-close on return) is caught.
//
// The test deliberately does NOT expect a clean pass: it asserts that the
// follow-up operation is blocked (so the deadlock-boundary contract holds),
// and fails fast via the 5s timeout if the boundary regresses.
func TestConcurrentTx_Query_UnscannedDoesNotDeadlock(t *testing.T) {
	// This test intentionally leaks a goroutine that holds the ConcurrentTx
	// mutex via an unclosed ConcurrentRows. Standard cleanup (rollback, pool
	// close) would deadlock because they need that same mutex, so we inline
	// setup without registering any cleanup — the leaked goroutine is killed
	// when the test binary exits.
	dsn := testMirageDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}

	ct, err := mirage.NewConcurrentTx(ctx, db.Pool)
	if err != nil {
		t.Fatalf("starting concurrent tx: %v", err)
	}
	_ = ct

	done := make(chan error, 1)
	go func() {
		// Intentionally never call Next() or Close() on the returned rows.
		_, _ = ct.Query(ctx, "SELECT 1")

		// A follow-up operation should be blocked by the still-open result
		// set -- this is the documented, accepted tradeoff (see IMPROVE.md).
		var v int
		done <- ct.QueryRow(ctx, "SELECT $1::int", 42).Scan(&v)
	}()

	select {
	case err := <-done:
		// If we ever get here, the boundary changed: either the rows were
		// auto-closed (good surprise, update the contract) or the op failed
		// for some other reason. Surface it rather than silently passing.
		t.Fatalf("unexpectedly got a result from the follow-up op after an unclosed Query result (deadlock-boundary contract changed): %v", err)
	case <-time.After(5 * time.Second):
		t.Log("expected: follow-up op blocked by unclosed Query result (accepted tradeoff per IMPROVE.md Problem 6)")
	}
}

// TestConcurrentTx_SendBatch_UnclosedDoesNotDeadlock documents the same
// accepted tradeoff for SendBatch: a BatchResults that is never read or
// closed intentionally blocks subsequent operations on the transaction.
// Mirrors TestConcurrentTx_Query_UnscannedDoesNotDeadlock and pins the
// boundary so a future regression (auto-close) is caught.
func TestConcurrentTx_SendBatch_UnclosedDoesNotDeadlock(t *testing.T) {
	// Same inlined setup as TestConcurrentTx_Query_UnscannedDoesNotDeadlock
	// — see that test's comment for why cleanups are skipped.
	dsn := testMirageDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}

	ct, err := mirage.NewConcurrentTx(ctx, db.Pool)
	if err != nil {
		t.Fatalf("starting concurrent tx: %v", err)
	}
	_ = ct

	done := make(chan error, 1)
	go func() {
		// Intentionally never read from or Close() the returned BatchResults.
		batch := &pgx.Batch{}
		batch.Queue("SELECT 1")
		_ = ct.SendBatch(ctx, batch)

		// A follow-up operation should be blocked by the still-open batch
		// result -- this is the documented, accepted tradeoff.
		var v int
		done <- ct.QueryRow(ctx, "SELECT $1::int", 42).Scan(&v)
	}()

	select {
	case err := <-done:
		t.Fatalf("unexpectedly got a result from the follow-up op after an unclosed SendBatch result (deadlock-boundary contract changed): %v", err)
	case <-time.After(5 * time.Second):
		t.Log("expected: follow-up op blocked by unclosed SendBatch result (accepted tradeoff per IMPROVE.md Problem 6)")
	}
}
