package mirage

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	schemapkg "github.com/justblue/mirage/internal/schema"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
)

func SetDefaultTag(tag string) {
	schemapkg.SetDefaultTag(tag)
}

func SetDefaultSearchPath(searchPath string) {
	schemapkg.SetDefaultSearchPath(searchPath)
}

// QuoteIdentifier sanitizes a string identifier for use in SQL queries.
func QuoteIdentifier(identifier string) string {
	return pgx.Identifier{identifier}.Sanitize()
}

var (
	defaultColumnNameMapper = func(field reflect.StructField) string { return schemapkg.SnakeCase(field.Name) }
	NoColumnNameMapper      = func(field reflect.StructField) string { return field.Name }
	JSONColumnNameMapper    = func(field reflect.StructField) string {
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			return field.Name
		}
		return strings.SplitN(jsonTag, ",", 2)[0]
	}
)

func SetDefaultColumnNameMapper(fn func(field reflect.StructField) string) {
	if fn == nil {
		schemapkg.SetToColumnName(defaultColumnNameMapper)
	} else {
		schemapkg.SetToColumnName(fn)
	}
}

type (
	Row  = pgx.Row
	Rows = pgx.Rows
)

type DB struct {
	Pool              *pgxpool.Pool
	ConnectionOptions *pgx.ConnConfig
	searchPath        string

	tx pgx.Tx

	tableChangeNotifyMu           *sync.Mutex
	tableChangeNotifyFunctionOnce bool
	tableChangeNotifyTriggerOnce  *sync.Map
}

type ConnectionOption func(*pgxpool.Config) error

var WithLogger = func(logger tracelog.Logger) ConnectionOption {
	return func(poolConfig *pgxpool.Config) error {
		tracer := &tracelog.TraceLog{
			Logger:   logger,
			LogLevel: tracelog.LogLevelTrace,
		}
		poolConfig.ConnConfig.Tracer = tracer
		return nil
	}
}

func Open(ctx context.Context, connString string, opts ...ConnectionOption) (*DB, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err = opt(config); err != nil {
			return nil, err
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("open: ping failed: %w", err)
	}

	return OpenPool(pool), nil
}

func OpenPool(pool *pgxpool.Pool) *DB {
	config := pool.Config().ConnConfig.Copy()

	searchPath, ok := config.RuntimeParams["search_path"]
	if !ok || strings.TrimSpace(searchPath) == "" {
		searchPath = schemapkg.DefaultSearchPath
	}

	return &DB{
		Pool:                         pool,
		ConnectionOptions:            config,
		searchPath:                   searchPath,
		tableChangeNotifyMu:          new(sync.Mutex),
		tableChangeNotifyTriggerOnce: &sync.Map{},
	}
}

func (db *DB) Close() {
	db.Pool.Close()
}

func (db *DB) clone(tx pgx.Tx) *DB {
	return &DB{
		Pool:                          db.Pool,
		ConnectionOptions:             db.ConnectionOptions,
		tx:                            tx,
		searchPath:                    db.searchPath,
		tableChangeNotifyMu:           db.tableChangeNotifyMu,
		tableChangeNotifyFunctionOnce: db.tableChangeNotifyFunctionOnce,
		tableChangeNotifyTriggerOnce:  db.tableChangeNotifyTriggerOnce,
	}
}

func (db *DB) SearchPath() string {
	return db.searchPath
}

var ErrIntentionalRollback = errors.New("skip error: intentional rollback")

func (db *DB) InTransaction(ctx context.Context, fn func(*DB) error) (err error) {
	if db.IsTransaction() {
		return fn(db)
	}

	var tx *DB
	tx, err = db.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		// Use a background context for rollback/commit so that a cancelled
		// parent context (e.g. request timeout) does not prevent cleanup.
		bgCtx := context.Background()
		if p := recover(); p != nil {
			_ = tx.Rollback(bgCtx)
			panic(p)
		} else if err != nil {
			if errors.Is(err, ErrIntentionalRollback) {
				err = tx.Rollback(bgCtx)
				return
			}
			rollbackErr := tx.Rollback(bgCtx)
			if rollbackErr != nil {
				err = fmt.Errorf("%w: %s", err, rollbackErr.Error())
			}
		} else {
			err = tx.Commit(bgCtx)
		}
	}()

	err = fn(tx)
	return err
}

// RetryOptions configures the retry behavior of InTransactionWithRetry.
type RetryOptions struct {
	// MaxAttempts is the total number of times the transaction body may run,
	// including the first attempt. Values <= 0 default to 3.
	MaxAttempts int
	// BaseDelay is the delay before the first retry. Each subsequent retry
	// doubles the delay (capped by MaxDelay). Values <= 0 default to 5ms.
	BaseDelay time.Duration
	// MaxDelay caps the exponential backoff delay. Values <= 0 default to 1s.
	MaxDelay time.Duration
	// ShouldRetry reports whether the given error is retryable. When nil,
	// IsErrRetryable is used (serialization failures and deadlocks).
	ShouldRetry func(error) bool
}

func (o RetryOptions) withDefaults() RetryOptions {
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 3
	}
	if o.BaseDelay <= 0 {
		o.BaseDelay = 5 * time.Millisecond
	}
	if o.MaxDelay <= 0 {
		o.MaxDelay = time.Second
	}
	if o.ShouldRetry == nil {
		o.ShouldRetry = IsErrRetryable
	}
	return o
}

// InTransactionWithRetry runs fn inside a transaction, retrying the entire
// transaction when it fails with a retryable error (by default, PostgreSQL
// serialization failures and deadlocks). Between attempts it waits with
// exponential backoff, honoring ctx cancellation. When already inside a
// transaction, it runs fn once without retrying, since a nested savepoint
// retry could not restore the outer transaction's state.
func (db *DB) InTransactionWithRetry(ctx context.Context, opts RetryOptions, fn func(*DB) error) error {
	if db.IsTransaction() {
		return fn(db)
	}

	return retryLoop(ctx, opts, func() error {
		return db.InTransaction(ctx, fn)
	})
}

// retryLoop runs attempt with retries governed by opts. It is separated from
// InTransactionWithRetry so the backoff/attempt-count logic can be unit-tested
// without a live database.
func retryLoop(ctx context.Context, opts RetryOptions, attempt func() error) error {
	opts = opts.withDefaults()

	delay := opts.BaseDelay
	var lastErr error
	for n := 1; n <= opts.MaxAttempts; n++ {
		lastErr = attempt()
		if lastErr == nil {
			return nil
		}
		if !opts.ShouldRetry(lastErr) || n == opts.MaxAttempts {
			return lastErr
		}

		select {
		case <-ctx.Done():
			return errors.Join(lastErr, ctx.Err())
		case <-time.After(delay):
		}

		delay *= 2
		if delay > opts.MaxDelay {
			delay = opts.MaxDelay
		}
	}
	return lastErr
}

func (db *DB) IsTransaction() bool {
	return db.tx != nil
}

func (db *DB) Begin(ctx context.Context) (*DB, error) {
	var (
		tx  pgx.Tx
		err error
	)
	if db.tx != nil {
		tx, err = db.tx.Begin(ctx)
	} else {
		tx, err = db.Pool.BeginTx(ctx, pgx.TxOptions{})
	}
	if err != nil {
		return nil, err
	}
	return db.clone(tx), nil
}

func (db *DB) BeginConcurrent(ctx context.Context) (*DB, error) {
	var (
		tx  pgx.Tx
		err error
	)
	if db.tx != nil {
		tx, err = db.tx.Begin(ctx)
	} else {
		tx, err = NewConcurrentTx(ctx, db.Pool)
	}
	if err != nil {
		return nil, err
	}
	return db.clone(tx), nil
}

func (db *DB) Rollback(ctx context.Context) error {
	if db.tx == nil {
		return nil
	}
	err := db.tx.Rollback(ctx)
	if err == nil {
		db.tx = nil
	}
	return err
}

func (db *DB) Commit(ctx context.Context) error {
	if db.tx == nil {
		return nil
	}
	err := db.tx.Commit(ctx)
	if err == nil {
		db.tx = nil
	}
	return err
}

func (db *DB) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	if db.tx != nil {
		rows, err := db.tx.Query(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("transaction: query: %w", err)
		}
		return rows, nil
	}
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	return rows, nil
}

func (db *DB) QueryRow(ctx context.Context, query string, args ...any) Row {
	if db.tx != nil {
		return db.tx.QueryRow(ctx, query, args...)
	}
	return db.Pool.QueryRow(ctx, query, args...)
}

func (db *DB) QueryBoolean(ctx context.Context, query string, args ...any) (ok bool, err error) {
	err = db.QueryRow(ctx, query, args...).Scan(&ok)
	return
}

func (db *DB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if db.tx != nil {
		tag, err := db.tx.Exec(ctx, query, args...)
		if err != nil {
			return tag, fmt.Errorf("transaction: exec: %w", err)
		}
		return tag, nil
	}
	tag, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return tag, fmt.Errorf("exec: %w", err)
	}
	return tag, nil
}

func (db *DB) ExecFiles(ctx context.Context, fileReader interface {
	ReadFile(name string) ([]byte, error)
}, filenames ...string) error {
	if fileReader == nil || len(filenames) == 0 {
		return nil
	}

	type file struct {
		name     string
		contents string
	}

	files := make([]file, 0, len(filenames))
	for _, filename := range filenames {
		b, err := fileReader.ReadFile(filename)
		if err != nil {
			return err
		}
		if len(b) == 0 {
			continue
		}
		files = append(files, file{name: filename, contents: string(b)})
	}

	return db.InTransaction(ctx, func(db *DB) error {
		for _, f := range files {
			_, err := db.Exec(ctx, f.contents)
			if err != nil {
				return fmt.Errorf("exec file %s: %w", f.name, err)
			}
		}
		return nil
	})
}

func (db *DB) Listen(ctx context.Context, channel string) (*Listener, error) {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	query := `LISTEN ` + QuoteIdentifier(channel)
	_, err = conn.Exec(ctx, query)
	if err != nil {
		conn.Release()
		return nil, err
	}
	return &Listener{conn: conn, channel: channel}, nil
}

func (db *DB) Notify(ctx context.Context, channel string, payload any) error {
	switch v := payload.(type) {
	case string:
		return notifyNative(ctx, db, channel, v)
	case []byte:
		return notifyNative(ctx, db, channel, v)
	default:
		return notifyJSON(ctx, db, channel, v)
	}
}

func (db *DB) Unlisten(ctx context.Context, channel string) error {
	query := `UNLISTEN ` + QuoteIdentifier(channel)
	_, err := db.Exec(ctx, query)
	return err
}
