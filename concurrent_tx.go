package mirage

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConcurrentTx struct {
	pgx.Tx
	mu sync.Mutex
}

func NewConcurrentTx(ctx context.Context, p *pgxpool.Pool) (*ConcurrentTx, error) {
	tx, err := p.Begin(ctx)
	if err != nil {
		return nil, err
	}

	return &ConcurrentTx{Tx: tx}, nil
}

func (ct *ConcurrentTx) Rollback(ctx context.Context) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	return ct.Tx.Rollback(ctx)
}

func (ct *ConcurrentTx) Commit(ctx context.Context) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	return ct.Tx.Commit(ctx)
}

// QueryRow eagerly executes the query and buffers the single result row
// under the lock, releasing the lock before returning. This avoids holding
// the connection mutex across the API boundary: if the caller never scanned
// the row, the lock would previously deadlock every subsequent operation on
// the transaction. The buffered values are decoded lazily on Scan.
func (ct *ConcurrentTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	rows, err := ct.Tx.Query(ctx, sql, args...)
	if err != nil {
		return &ConcurrentRow{err: err}
	}
	defer rows.Close()

	// Eagerly check if a row exists and buffer its raw bytes under the lock
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return &ConcurrentRow{err: err}
		}
		return &ConcurrentRow{err: pgx.ErrNoRows}
	}

	// Buffer the raw bytes and field descriptions before releasing the lock.
	// Both RawValues() and FieldDescriptions() return memory owned by the
	// underlying connection: the inner byte slices alias the connection's read
	// buffer and the field-description slice is reused across queries. Once the
	// lock is released the connection may be reused by another goroutine, which
	// overwrites that memory while this row is still being decoded in Scan. A
	// shallow copy of the [][]byte header is not enough -- each inner slice and
	// the field descriptions must be deep-copied so Scan is safe after unlock.
	rawVals := rows.RawValues()
	fieldDescs := rows.FieldDescriptions()

	bufferedVals := make([][]byte, len(rawVals))
	for i, v := range rawVals {
		if v == nil {
			continue
		}
		vc := make([]byte, len(v))
		copy(vc, v)
		bufferedVals[i] = vc
	}

	bufferedDescs := make([]pgconn.FieldDescription, len(fieldDescs))
	copy(bufferedDescs, fieldDescs)

	return &ConcurrentRow{bufferedValues: bufferedVals, fieldDescriptions: bufferedDescs}
}

func (ct *ConcurrentTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	ct.mu.Lock()

	rows, err := ct.Tx.Query(ctx, sql, args...)
	if err != nil {
		ct.mu.Unlock()
		return nil, err
	}

	return &ConcurrentRows{rows: rows, mu: &ct.mu}, nil
}

func (ct *ConcurrentTx) Exec(ctx context.Context, sql string, args ...any) (commandTag pgconn.CommandTag, err error) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	return ct.Tx.Exec(ctx, sql, args...)
}

func (ct *ConcurrentTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	return ct.Tx.Prepare(ctx, name, sql)
}

func (ct *ConcurrentTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	ct.mu.Lock()

	// The lock is held until the returned ConcurrentBatchResults.Close is
	// called. If SendBatch panics before returning, release the lock so the
	// mutex is not leaked, then re-panic to preserve the original failure.
	defer func() {
		if r := recover(); r != nil {
			ct.mu.Unlock()
			panic(r)
		}
	}()

	br := ct.Tx.SendBatch(ctx, b)
	return &ConcurrentBatchResults{br: br, mu: &ct.mu}
}

func (ct *ConcurrentTx) Begin(ctx context.Context) (pgx.Tx, error) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	tx, err := ct.Tx.Begin(ctx)
	if err != nil {
		return nil, err
	}

	return &ConcurrentTx{Tx: tx}, nil
}

// ConcurrentRow buffers the result of a QueryRow that was executed and read
// eagerly under the transaction lock. The connection lock has already been
// released by the time this value is returned, so Scan is safe to call at any
// point (or never) without risking a deadlock.
type ConcurrentRow struct {
	rows              pgx.Rows
	err               error
	bufferedValues    [][]byte
	fieldDescriptions []pgconn.FieldDescription
}

// Scan decodes the buffered row into dest. It mirrors pgx.Row.Scan semantics:
// it returns pgx.ErrNoRows when the query produced no rows and always ensures
// the underlying rows are closed.
func (cr *ConcurrentRow) Scan(dest ...any) error {
	if cr.err != nil {
		return cr.err
	}

	if cr.bufferedValues != nil {
		// Use pgx's codec to decode the buffered values
		return cr.decodeBufferedValues(dest...)
	}

	// Fallback for backward compatibility
	defer cr.rows.Close()
	if !cr.rows.Next() {
		if err := cr.rows.Err(); err != nil {
			return err
		}
		return pgx.ErrNoRows
	}
	return cr.rows.Scan(dest...)
}

func (cr *ConcurrentRow) decodeBufferedValues(dest ...any) error {
	if len(dest) != len(cr.fieldDescriptions) {
		return fmt.Errorf("number of destination variables must equal number of columns (%d != %d)",
			len(dest), len(cr.fieldDescriptions))
	}

	for i, d := range dest {
		fieldDesc := cr.fieldDescriptions[i]
		rawVal := cr.bufferedValues[i]

		err := pgtype.NewMap().Scan(fieldDesc.DataTypeOID, pgx.BinaryFormatCode, rawVal, d)
		if err != nil {
			return err
		}
	}

	return nil
}

// Discard closes the buffered rows without decoding them. It is retained for
// backward compatibility; because the connection lock is no longer held past
// QueryRow, calling it is optional.
func (cr *ConcurrentRow) Discard() {
	if cr.rows != nil {
		cr.rows.Close()
	}
}

type ConcurrentBatchResults struct {
	br pgx.BatchResults
	mu *sync.Mutex
}

func (cbr *ConcurrentBatchResults) Exec() (pgconn.CommandTag, error) { return cbr.br.Exec() }
func (cbr *ConcurrentBatchResults) Query() (pgx.Rows, error)         { return cbr.br.Query() }
func (cbr *ConcurrentBatchResults) QueryRow() pgx.Row                { return cbr.br.QueryRow() }

// Close releases the mutex. pgx's own BatchResults contract requires Close
// to be called before the underlying connection can be reused for
// anything else, which makes it the correct point to release the lock --
// exactly mirroring ConcurrentRows.Close() for Query.
func (cbr *ConcurrentBatchResults) Close() error {
	defer cbr.mu.Unlock()
	return cbr.br.Close()
}

type ConcurrentRows struct {
	rows pgx.Rows
	mu   *sync.Mutex
}

func (cr *ConcurrentRows) Next() bool             { return cr.rows.Next() }
func (cr *ConcurrentRows) Scan(dest ...any) error { return cr.rows.Scan(dest...) }
func (cr *ConcurrentRows) Close()                 { cr.rows.Close(); cr.mu.Unlock() }
func (cr *ConcurrentRows) Err() error             { return cr.rows.Err() }
func (cr *ConcurrentRows) FieldDescriptions() []pgconn.FieldDescription {
	return cr.rows.FieldDescriptions()
}
func (cr *ConcurrentRows) Values() ([]any, error)        { return cr.rows.Values() }
func (cr *ConcurrentRows) RawValues() [][]byte           { return cr.rows.RawValues() }
func (cr *ConcurrentRows) CommandTag() pgconn.CommandTag { return cr.rows.CommandTag() }
func (cr *ConcurrentRows) Conn() *pgx.Conn               { return cr.rows.Conn() }
