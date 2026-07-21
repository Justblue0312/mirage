//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	mirage "github.com/justblue/mirage"
)

type lockWidget struct {
	_    struct{} `db:"lock_widgets_test"`
	ID   int64    `db:"pk,type:bigserial"`
	Name string   `db:"type:text,notnull"`
}

func (lockWidget) TableName() string { return "lock_widgets_test" }

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
	err = db.InTransaction(ctx, func(dbc *mirage.DB) error {
		txRepo := mirage.NewRepository[lockWidget](dbc)
		_, err := txRepo.SelectByIDForUpdate(ctx, int64(1), mirage.ForUpdateNoWait())
		if err == nil {
			t.Fatal("expected lock_not_available error with NOWAIT")
		}
		t.Logf("got expected NOWAIT error: %v", err)
		return nil // don't propagate — we expect this error
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
}

// TestLock_CacheBypass verifies that SelectByIDForUpdate never reads from
// the cache, even if one is configured.
func TestLock_CacheBypass(t *testing.T) {
	dsn := testMirageDSN(t)
	ctx := context.Background()

	db, err := mirage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	setupLockWidgetsTable(t, db)
	_, _ = db.Exec(ctx, `INSERT INTO lock_widgets_test (name) VALUES ('original')`)

	cache := mirage.NewInMemoryCache()
	repo := mirage.NewRepository[lockWidget](db, mirage.WithCache(cache, time.Hour))

	// Popululate the cache via a normal SelectByID.
	w, err := repo.SelectByID(ctx, int64(1))
	if err != nil {
		t.Fatalf("SelectByID: %v", err)
	}
	if w.Name != "original" {
		t.Fatalf("expected 'original', got %q", w.Name)
	}

	// Update the row out-of-band (bypassing the repository).
	_, _ = db.Exec(ctx, `UPDATE lock_widgets_test SET name = 'out-of-band-update' WHERE id = 1`)

	// SelectByIDForUpdate should see the current DB value, not the stale cache.
	err = db.InTransaction(ctx, func(dbc *mirage.DB) error {
		txRepo := mirage.NewRepository[lockWidget](dbc, mirage.WithCache(cache, time.Hour))
		locked, err := txRepo.SelectByIDForUpdate(ctx, int64(1), mirage.ForUpdate())
		if err != nil {
			return err
		}
		if locked.Name != "out-of-band-update" {
			t.Errorf("expected 'out-of-band-update' (bypass cache), got %q", locked.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
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

	blocked := make(chan error, 1)
	unblock := make(chan struct{})

	// Goroutine A: holds the lock until signaled.
	go func() {
		tx, err := lockConn.Begin(ctx)
		if err != nil {
			blocked <- err
			return
		}
		_, err = tx.Exec(ctx, `SELECT * FROM lock_widgets_test WHERE id = 1 FOR UPDATE`)
		if err != nil {
			blocked <- err
			return
		}
		blocked <- nil // signal: lock acquired
		<-unblock      // wait for signal to release
		_ = tx.Rollback(ctx)
	}()

	// Wait for goroutine A to acquire the lock.
	if err := <-blocked; err != nil {
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
			start := time.Now()
			_, err := repo.SelectByIDForUpdate(ctx, int64(1), mirage.ForUpdate())
			elapsed := time.Since(start)
			if err != nil {
				return err
			}
			// If we got here, the lock was blocked for some time.
			if elapsed < 50*time.Millisecond {
				t.Errorf("SELECT FOR UPDATE returned too quickly (%v) — blocking may not be working", elapsed)
			}
			return nil
		})
		done <- err
	}()

	// Give goroutine B time to start blocking.
	time.Sleep(100 * time.Millisecond)

	// Release goroutine A's lock.
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
