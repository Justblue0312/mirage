package runner

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (r *Runner) EnsureTracker(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, r.dialect.CreateTrackerSQL())
	if err != nil {
		return err
	}
	for _, stmt := range r.dialect.UpgradeTrackerSQL() {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) getApplied(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, r.dialect.GetAppliedSQL())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[fmt.Sprintf("%014d", version)] = true
	}
	return applied, rows.Err()
}

func (r *Runner) getAppliedChecksums(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, r.dialect.GetAppliedChecksumsSQL())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	checksums := make(map[string]string)
	for rows.Next() {
		var version int64
		var cs string
		if err := rows.Scan(&version, &cs); err != nil {
			return nil, err
		}
		checksums[fmt.Sprintf("%014d", version)] = cs
	}
	return checksums, rows.Err()
}

type appliedRow struct {
	version string
	name    string
}

func (r *Runner) getRecentApplied(ctx context.Context, pool *pgxpool.Pool, n int) ([]appliedRow, error) {
	rows, err := pool.Query(ctx, r.dialect.GetRecentAppliedSQL(n))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []appliedRow
	for rows.Next() {
		var row appliedRow
		if err := rows.Scan(&row.version, &row.name); err != nil {
			return nil, err
		}
		row.version = canonVersion(row.version)
		result = append(result, row)
	}
	return result, rows.Err()
}

type txOrPool interface {
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
}

func (r *Runner) recordApplied(ctx context.Context, e txOrPool, version, name, checksum string) error {
	_, err := e.Exec(ctx,
		r.dialect.RecordAppliedSQL(),
		version, name, checksum,
	)
	return err
}

func (r *Runner) recordFailed(ctx context.Context, e txOrPool, version, name string) {
	if _, err := e.Exec(ctx,
		r.dialect.RecordFailedSQL(),
		version, name,
	); err != nil {
		fmt.Printf("  warning: failed to record migration failure for V%s_%s: %v\n", version, name, err)
	}
}

func (r *Runner) SaveSnapshot(ctx context.Context, pool *pgxpool.Pool, version, name string, snapshot any) error {
	var v int64
	if _, err := fmt.Sscanf(version, "%d", &v); err != nil {
		return fmt.Errorf("parsing version: %w", err)
	}
	_, err := pool.Exec(ctx, r.dialect.SaveSnapshotSQL(), v, snapshot, name)
	return err
}

func (r *Runner) LoadSnapshot(ctx context.Context, pool *pgxpool.Pool) ([]byte, error) {
	var raw []byte
	err := pool.QueryRow(ctx, r.dialect.LoadSnapshotSQL()).Scan(&raw)
	if err != nil {
		return nil, err
	}
	return raw, nil
}
