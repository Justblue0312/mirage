// Package uow implements the Unit of Work pattern for coordinating
// transactions across module boundaries in a modular monolith.
//
// Design goals:
//   - Modules never import pgx directly in their repositories; they depend
//     only on the Querier interface, so the same repo code runs whether or
//     not it's inside a transaction.
//   - The active tx (if any) is threaded through context.Context, not
//     stored as struct state, so it's safe for concurrent request handling
//     and composable across service calls from different modules.
//   - Nested Do() calls use real SAVEPOINTs (via pgx's pseudo-nested tx
//     support) rather than silently flattening into the outer tx. This
//     matters when an inner unit of work needs to roll back independently
//     of the outer one (e.g. "try to reserve inventory, fall back if it
//     fails, but keep the order row").
package mirage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the minimal surface repositories need. *pgxpool.Pool and
// pgx.Tx both satisfy it, so repository code is written once and works in
// both transactional and non-transactional contexts.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ctxKey struct{}

// UnitOfWork is what services depend on. Each module's service layer takes
// a UnitOfWork; repositories stay pool/tx-agnostic via Q().
type UnitOfWork interface {
	// Do runs fn inside a transaction. If ctx already carries a tx (a Do
	// call nested inside another Do call, possibly from a different
	// module's service), it opens a SAVEPOINT instead of a new top-level
	// transaction, so the inner failure can roll back independently.
	Do(ctx context.Context, fn func(ctx context.Context) error) error

	// DoWithOptions is Do with explicit isolation/access mode. Only
	// meaningful at the top level — nested calls (savepoints) inherit the
	// outer transaction's settings, per Postgres semantics.
	DoWithOptions(ctx context.Context, opts pgx.TxOptions, fn func(ctx context.Context) error) error
}

type pgUnitOfWork struct {
	pool *pgxpool.Pool
}

// NewUnitOfWork creates a UnitOfWork backed by the given connection pool.
func NewUnitOfWork(pool *pgxpool.Pool) UnitOfWork {
	return &pgUnitOfWork{pool: pool}
}

func (u *pgUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return u.DoWithOptions(ctx, pgx.TxOptions{}, fn)
}

func (u *pgUnitOfWork) DoWithOptions(ctx context.Context, opts pgx.TxOptions, fn func(ctx context.Context) error) error {
	if outer, ok := ctx.Value(ctxKey{}).(pgx.Tx); ok {
		// Nested call: Begin() on an existing Tx issues a real SAVEPOINT
		// under the hood, and the returned pgx.Tx's Commit/Rollback map to
		// RELEASE SAVEPOINT / ROLLBACK TO SAVEPOINT rather than the
		// top-level COMMIT/ROLLBACK. Using `outer` itself here (instead of
		// the savepoint Begin() returns) would make runTx call Commit/
		// Rollback directly on the outer transaction from inside the
		// nested call -- closing it early and making the eventual outer
		// runTx's own Commit fail with "tx is closed".
		savepoint, err := outer.Begin(ctx)
		if err != nil {
			return fmt.Errorf("uow: begin savepoint: %w", err)
		}
		return u.runTx(ctx, savepoint, fn)
	}

	tx, err := u.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("uow: begin tx: %w", err)
	}
	return u.runTx(ctx, tx, fn)
}

func (u *pgUnitOfWork) runTx(ctx context.Context, tx pgx.Tx, fn func(ctx context.Context) error) (err error) {
	txCtx := context.WithValue(ctx, ctxKey{}, tx)

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx) // best-effort; ctx (not txCtx) — must survive a cancelled parent
			panic(p)
		}
	}()

	if err = fn(txCtx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return fmt.Errorf("uow: %w (rollback failed: %v)", err, rbErr)
		}
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("uow: commit: %w", err)
	}
	return nil
}

// Q returns the Querier a repository should use for this call: the active
// tx/savepoint if ctx carries one, otherwise the pool directly. Every
// repository method calls this first instead of holding a pool/tx field.
func Q(ctx context.Context, pool *pgxpool.Pool) Querier {
	if tx, ok := ctx.Value(ctxKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}
