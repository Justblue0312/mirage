//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	mirage "github.com/justblue/mirage"
)

type lockWidget struct {
	ID   int64  `db:"pk,type:bigserial"`
	Name string `db:"type:text,notnull"`
}

func init() {
	_ = mirage.Register(mirage.TableConfig{StructName: "lockWidget", Name: "lock_widgets_test"})
}

func setupLockWidgetsTable(t *testing.T, db *mirage.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `DROP TABLE IF EXISTS lock_widgets_test;
		CREATE TABLE lock_widgets_test (id bigserial PRIMARY KEY, name text NOT NULL);`)
	if err != nil {
		t.Fatalf("creating lock_widgets_test table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DROP TABLE IF EXISTS lock_widgets_test;`)
	})
}

// TestLock_BasicAcquireAndUse verifies that SelectByIDForUpdate inside a
// transaction acquires a lock that is usable for a subsequent update.
func TestLock_BasicAcquireAndUse(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupLockWidgetsTable(t, db)
	_, _ = db.Exec(ctx, `INSERT INTO lock_widgets_test (name) VALUES ('original')`)

	err = db.InTransaction(ctx, func(tx *mirage.DB) error {
		txRepo := mirage.NewRepository[lockWidget](tx)
		w, err := txRepo.SelectByIDForUpdate(ctx, int64(1), mirage.ForUpdate())
		if err != nil {
			return err
		}
		w.Name = "updated"
		n, err := txRepo.Update(ctx, w)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("expected 1 row updated, got %d", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}

	// Verify the update persisted.
	var name string
	if err := db.QueryRow(ctx, `SELECT name FROM lock_widgets_test WHERE id = 1`).Scan(&name); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if name != "updated" {
		t.Errorf("expected 'updated', got %q", name)
	}
}

// TestLock_GuardRailOutsideTransaction verifies that SelectByIDForUpdate
// returns an error when called outside a transaction.
func TestLock_GuardRailOutsideTransaction(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupLockWidgetsTable(t, db)
	_, _ = db.Exec(ctx, `INSERT INTO lock_widgets_test (name) VALUES ('test')`)

	repo := mirage.NewRepository[lockWidget](db)
	_, err = repo.SelectByIDForUpdate(ctx, int64(1), mirage.ForUpdate())
	if err == nil {
		t.Fatal("expected error when calling SelectByIDForUpdate outside a transaction")
	}
	t.Logf("got expected error: %v", err)
}

// TestLock_SkipLocked verifies that SKIP LOCKED skips rows locked by
// another transaction instead of blocking.
func TestLock_SkipLocked(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupLockWidgetsTable(t, db)
	_, _ = db.Exec(ctx, `INSERT INTO lock_widgets_test (name) VALUES ('row-1'), ('row-2'), ('row-3')`)

	// Hold a lock on row 1 in a separate connection (simulating another
	// transaction). We use raw SQL for precise control over the lock.
	lockConn, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening lock conn: %v", err)
	}
	defer lockConn.Close()

	// Start a transaction and lock row 1 — hold it open.
	tx, err := lockConn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `SELECT * FROM lock_widgets_test WHERE id = 1 FOR UPDATE`)
	if err != nil {
		t.Fatalf("locking row 1: %v", err)
	}

	// Now use SKIP LOCKED in another transaction — should get rows 2 and 3.
	err = db.InTransaction(ctx, func(dbc *mirage.DB) error {
		txRepo := mirage.NewRepository[lockWidget](dbc)
		rows, err := txRepo.QueryForUpdate(ctx, mirage.ForUpdateSkipLocked(),
			`SELECT * FROM lock_widgets_test ORDER BY id`)
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Errorf("expected 2 rows (skipping locked row 1), got %d", len(rows))
		}
		if len(rows) >= 1 && rows[0].Name != "row-2" {
			t.Errorf("expected first unlocked row to be 'row-2', got %q", rows[0].Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
}

// TestLock_NoWait verifies that NOWAIT fails immediately when the row
// is already locked, instead of blocking.
func TestLock_NoWait(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupLockWidgetsTable(t, db)
	_, _ = db.Exec(ctx, `INSERT INTO lock_widgets_test (name) VALUES ('test')`)

	// Hold a lock on row 1 in a separate connection.
	lockConn, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening lock conn: %v", err)
	}
	defer lockConn.Close()

	tx, err := lockConn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `SELECT * FROM lock_widgets_test WHERE id = 1 FOR UPDATE`)
	if err != nil {
		t.Fatalf("locking row 1: %v", err)
	}

	// Use NOWAIT in another transaction — should fail immediately.
	// Return the error so InTransaction calls rollback (not commit),
	// because the NOWAIT error puts the PG transaction in aborted state.
	nwErr := db.InTransaction(ctx, func(dbc *mirage.DB) error {
		txRepo := mirage.NewRepository[lockWidget](dbc)
		_, err := txRepo.SelectByIDForUpdate(ctx, int64(1), mirage.ForUpdateNoWait())
		if err == nil {
			t.Fatal("expected lock_not_available error with NOWAIT")
		}
		return err
	})
	if nwErr == nil {
		t.Fatal("expected error from NOWAIT transaction")
	}
	t.Logf("got expected NOWAIT error: %v", nwErr)
}

// TestLock_ConcurrentBlocking verifies that a second FOR UPDATE call on
// the same row blocks until the first transaction commits or rolls back.
func TestLock_ConcurrentBlocking(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupLockWidgetsTable(t, db)
	_, _ = db.Exec(ctx, `INSERT INTO lock_widgets_test (name) VALUES ('contested')`)

	lockConn, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening lock conn: %v", err)
	}
	defer lockConn.Close()

	locked := make(chan error, 1)
	ready := make(chan struct{})
	unblock := make(chan struct{})

	// Goroutine A: holds the lock until signaled.
	go func() {
		tx, err := lockConn.Begin(ctx)
		if err != nil {
			locked <- err
			return
		}
		_, err = tx.Exec(ctx, `SELECT * FROM lock_widgets_test WHERE id = 1 FOR UPDATE`)
		if err != nil {
			locked <- err
			return
		}
		locked <- nil // signal: lock acquired
		<-unblock     // wait for signal to release
		_ = tx.Rollback(ctx)
	}()

	// Wait for goroutine A to acquire the lock.
	if err := <-locked; err != nil {
		t.Fatalf("goroutine A failed to lock: %v", err)
	}

	// Goroutine B: tries to lock the same row — should block.
	done := make(chan error, 1)
	go func() {
		conn2, err := mirage.Open(ctx, dsn)
		if err != nil {
			done <- err
			return
		}
		defer conn2.Close()

		err = conn2.InTransaction(ctx, func(dbc *mirage.DB) error {
			repo := mirage.NewRepository[lockWidget](dbc)
			ready <- struct{}{} // signal: about to attempt the lock
			_, err := repo.SelectByIDForUpdate(ctx, int64(1), mirage.ForUpdate())
			return err
		})
		done <- err
	}()

	// Wait for goroutine B to signal it's about to attempt the lock,
	// then release goroutine A's lock. B will block until A releases.
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("goroutine B failed to set up: %v", err)
	}
	close(unblock)

	// Goroutine B should now complete.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("goroutine B: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine B did not unblock within 5 seconds after lock release")
	}
}
