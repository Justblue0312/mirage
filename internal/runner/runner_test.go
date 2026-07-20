//go:build integration

package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/justblue/mirage/internal/dialect/postgres"
)

func testDSN(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("MIRAGE_TEST_DATABASE_URL"); v != "" {
		return v
	}
	t.Skip("MIRAGE_TEST_DATABASE_URL not set; skipping lock integration test")
	return ""
}

// getTestDialect returns the concrete Postgres dialect used across the
// integration tests for the runner package.
func getTestDialect() *postgres.Postgres {
	return postgres.New()
}

// resetTracker truncates the migration tracker table so each test starts from
// a clean history. The runner integration tests share a single database, so
// without this isolation an applied migration recorded by one test would be
// seen as "already applied" (and checksum-checked) by a later test writing a
// different file under the same version number.
func resetTracker(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	// Drop the tracker table (it is recreated lazily by EnsureTracker on the
	// next Migrate/Rollback) and any user tables left behind by a previous
	// test, so each test starts from a fully clean database. The runner
	// integration tests share a single database, so without this isolation an
	// applied migration recorded by one test would be seen as "already applied"
	// (and checksum-checked) by a later test writing a different file under the
	// same version number, and its CREATE TABLE would collide with the prior
	// run's table.
	if _, err := pool.Exec(context.Background(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatalf("resetting tracker: %v", err)
	}
}

// writeTestMigration writes a complete migration file (Up + Down blocks) into
// dir. The body is the raw SQL placed inside the -- +migrate Up block; a
// matching, best-effort Down block is derived so parseDownBlock never fails.
func writeTestMigration(t *testing.T, dir, name, upBody string) {
	t.Helper()
	content := "-- +migrate Up\n" + upBody + "\n-- +migrate Down\nDROP TABLE IF EXISTS tamper_guard;\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writing migration %s: %v", name, err)
	}
}

// TestWithMigrationLock_SerializesConcurrentCallers is the regression test
// for the migration-runner advisory lock: two concurrent callers against
// the same database must never run their critical section at the same
// time. Without the lock, this test fails close to 100% of the time (the
// two goroutines' [start,end] windows overlap); with it, Postgres itself
// enforces mutual exclusion regardless of how the two goroutines are
// scheduled.
func TestWithMigrationLock_SerializesConcurrentCallers(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parsing dsn: %v", err)
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer pool.Close()

	type window struct{ start, end time.Time }
	windows := make([]window, 2)

	var wg sync.WaitGroup
	errs := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = withMigrationLock(ctx, pool, func(ctx context.Context) error {
				windows[i].start = time.Now()
				time.Sleep(150 * time.Millisecond)
				windows[i].end = time.Now()
				return nil
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	a, b := windows[0], windows[1]
	overlap := a.start.Before(b.end) && b.start.Before(a.end)
	if overlap {
		t.Fatalf("critical sections overlapped: goroutine 0 ran [%v, %v], goroutine 1 ran [%v, %v] -- the advisory lock did not serialize them",
			a.start, a.end, b.start, b.end)
	}
}

// TestWithMigrationLock_ReleasesOnError confirms a failing critical
// section still releases the lock (via defer), so a failed migration
// doesn't permanently wedge the database for every future migrate/rollback
// attempt.
func TestWithMigrationLock_ReleasesOnError(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parsing dsn: %v", err)
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer pool.Close()

	sentinel := context.DeadlineExceeded // any non-nil error works here
	err = withMigrationLock(ctx, pool, func(ctx context.Context) error {
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("expected the sentinel error to propagate, got %v", err)
	}

	// If the lock leaked, this second acquisition would hang until the
	// test's own timeout kills it. Bound it explicitly so a regression
	// fails fast with a clear message instead of just timing out.
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- withMigrationLock(lockCtx, pool, func(ctx context.Context) error { return nil })
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("re-acquiring the lock after a failed critical section: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("lock was not released after the critical section returned an error -- it leaked")
	}
}

// TestMigrate_RejectsTamperedAppliedMigration is the regression test for the
// applied-checksum re-verification added in this round. It exercises the full
// sequence against a real database:
//
//  1. apply a migration (its checksum is recorded as 'applied')
//  2. tamper with the on-disk file (change its content, changing its checksum)
//  3. attempt another Migrate -- it must refuse because the already-applied
//     file no longer matches the stored checksum.
//
// This catches post-apply tampering that the pending-only checksum check
// would miss (pending files aren't applied yet, so their mismatch is a
// different, expected path).
func TestMigrate_RejectsTamperedAppliedMigration(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parsing dsn: %v", err)
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer pool.Close()

	migrationsDir := t.TempDir()
	writeTestMigration(t, migrationsDir, "V00001_init.sql", "CREATE TABLE tamper_guard (id bigint);")

	r := New(getTestDialect())

	resetTracker(t, pool)

	// 1. Apply the migration and record its checksum.
	if err := r.Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}

	// 2. Tamper with the already-applied file on disk.
	tampered := "-- +migrate Up\nCREATE TABLE tamper_guard (id bigint, extra text);\n-- +migrate Down\nDROP TABLE tamper_guard;\n"
	if err := os.WriteFile(filepath.Join(migrationsDir, "V00001_init.sql"), []byte(tampered), 0644); err != nil {
		t.Fatalf("writing tampered migration: %v", err)
	}

	// 3. A subsequent migrate must refuse to proceed.
	err = r.Migrate(ctx, pool, migrationsDir)
	if err == nil {
		t.Fatal("expected migrate to refuse after the applied file was tampered, got nil error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected a checksum-mismatch error, got: %v", err)
	}
}

// TestDryMigrate_RejectsTamperedAppliedMigration mirrors the above for the
// dry-run path: even without applying anything, DryMigrate must surface
// tampering of already-applied files before printing a (now untrustworthy)
// plan.
func TestDryMigrate_RejectsTamperedAppliedMigration(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parsing dsn: %v", err)
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer pool.Close()

	migrationsDir := t.TempDir()
	writeTestMigration(t, migrationsDir, "V00001_init.sql", "CREATE TABLE tamper_guard_dry (id bigint);")

	r := New(getTestDialect())

	resetTracker(t, pool)

	if err := r.Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}

	tampered := "-- +migrate Up\nCREATE TABLE tamper_guard_dry (id bigint, extra text);\n-- +migrate Down\nDROP TABLE tamper_guard_dry;\n"
	if err := os.WriteFile(filepath.Join(migrationsDir, "V00001_init.sql"), []byte(tampered), 0644); err != nil {
		t.Fatalf("writing tampered migration: %v", err)
	}

	err = r.DryMigrate(ctx, pool, migrationsDir)
	if err == nil {
		t.Fatal("expected dry-run to refuse after the applied file was tampered, got nil error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected a checksum-mismatch error, got: %v", err)
	}
}
