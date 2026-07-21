package mirage

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"time"

	schemapkg "github.com/justblue/mirage/internal/schema"
)

// Repository provides type-safe CRUD operations for a single table mapped
// to the struct type T. T must be a struct with db:"..." tags.
//
// Example:
//
//	type User struct {
//	    ID   int64  `db:"pk,type:bigserial"`
//	    Name string `db:"type:text"`
//	}
//
//	repo := mirage.NewRepository[User](db)
//	err := repo.Insert(ctx, &User{Name: "Alice"})
type Repository[T any] struct {
	db          *DB
	table       *schemapkg.Table
	cache       Cache
	cacheTTLMin time.Duration
	cacheTTLMax time.Duration
	retry       RetryOptions
	retryEnabled bool
}

// RepositoryOption configures a Repository.
type RepositoryOption func(*repositoryConfig)

type repositoryConfig struct {
	cache       Cache
	cacheTTLMin time.Duration
	cacheTTLMax time.Duration
	retry       RetryOptions
	retryEnabled bool
}

// WithCache enables result caching for the repository with a fixed TTL.
func WithCache(cache Cache, ttl time.Duration) RepositoryOption {
	return func(cfg *repositoryConfig) {
		cfg.cache = cache
		cfg.cacheTTLMin = ttl
		cfg.cacheTTLMax = 0
	}
}

// WithCacheJitter enables result caching with a random TTL range to avoid
// cache stampede. TTL will be random in [ttlMin, ttlMax].
func WithCacheJitter(cache Cache, ttlMin, ttlMax time.Duration) RepositoryOption {
	return func(cfg *repositoryConfig) {
		cfg.cache = cache
		cfg.cacheTTLMin = ttlMin
		cfg.cacheTTLMax = ttlMax
	}
}

// WithRetry enables automatic retry of serialization failures and deadlocks
// for this repository's write operations, using opts. Reads are never retried.
// When the repository is called from inside an existing transaction (e.g. via
// uow.Do), the retry wrapper is skipped and the operation runs directly in the
// caller's transaction — retrying only makes sense when the call is the one
// opening its own transaction.
func WithRetry(opts RetryOptions) RepositoryOption {
	return func(cfg *repositoryConfig) {
		cfg.retry = opts
		cfg.retryEnabled = true
	}
}

// NewRepository creates a new Repository for the given struct type T.
func NewRepository[T any](db *DB, opts ...RepositoryOption) *Repository[T] {
	var zero T
	typ := reflect.TypeOf(zero)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	td, err := cachedTable(typ)
	if err != nil {
		panic(fmt.Sprintf("mirage: cannot create repository for %T: %v", zero, err))
	}

	cfg := repositoryConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Repository[T]{
		db:           db,
		table:        td,
		cache:        cfg.cache,
		cacheTTLMin:  cfg.cacheTTLMin,
		cacheTTLMax:  cfg.cacheTTLMax,
		retry:        cfg.retry,
		retryEnabled: cfg.retryEnabled,
	}
}

// inRetryTransaction returns true when retry is enabled and the call is NOT
// already inside an existing transaction — i.e. the call should open its own
// retriable transaction. Returns false when retry is disabled or when already
// inside a transaction (nested call — skip retry to avoid double-wrapping).
func (r *Repository[T]) inRetryTransaction() bool {
	return r.retryEnabled && !r.db.IsTransaction()
}

// doWithRetry runs fn inside a retriable transaction when retry is enabled and
// the call is not already inside an existing transaction. When inside a
// transaction (e.g. from uow.Do), fn runs directly without retry to avoid
// double-wrapping. The cache is invalidated on success.
func (r *Repository[T]) doWithRetry(ctx context.Context, fn func(tx *DB) error) error {
	if r.inRetryTransaction() {
		err := r.db.InTransactionWithRetry(ctx, r.retry, fn)
		if err == nil {
			r.invalidateCache(ctx)
		}
		return err
	}
	// Already inside a transaction or retry disabled — run directly.
	return fn(r.db)
}

// Insert inserts a single record. The primary key is scanned into value's
// primary key field if it is auto-generated (bigserial, identity, etc.).
// When retry is enabled and not inside an existing transaction, the insert
// runs in a retriable transaction.
func (r *Repository[T]) Insert(ctx context.Context, value *T) error {
	if r.retryEnabled && !r.db.IsTransaction() {
		return r.db.InTransactionWithRetry(ctx, r.retry, func(tx *DB) error {
			txRepo := &Repository[T]{db: tx, table: r.table}
			return txRepo.Insert(ctx, value)
		})
	}
	structValue := schemapkg.IndirectValue(value)
	err := r.insertTableRecord(ctx, structValue, nil, "", false)
	if err == nil {
		r.invalidateCache(ctx)
	}
	return err
}

// InsertReturning inserts a single record and scans all returned columns
// back into value. This uses RETURNING * so all database-generated values
// (defaults, computed columns, etc.) are populated.
func (r *Repository[T]) InsertReturning(ctx context.Context, value *T) error {
	structValue := schemapkg.IndirectValue(value)
	primaryKey, ok := r.table.FindPrimaryKey()
	if !ok {
		return fmt.Errorf("no primary key found for table %s", r.table.Name)
	}
	idPtr := structValue.FieldByIndex(primaryKey.FieldIndex).Addr().Interface()
	err := r.insertTableRecord(ctx, structValue, idPtr, "", false)
	if err == nil {
		r.invalidateCache(ctx)
	}
	return err
}

// Update persists all columns of a record, using the primary key to
// identify the row. Returns the number of rows affected.
func (r *Repository[T]) Update(ctx context.Context, value *T) (int64, error) {
	if r.retryEnabled && !r.db.IsTransaction() {
		var n int64
		err := r.db.InTransactionWithRetry(ctx, r.retry, func(tx *DB) error {
			txRepo := &Repository[T]{db: tx, table: r.table}
			var err error
			n, err = txRepo.Update(ctx, value)
			return err
		})
		if err == nil {
			r.invalidateCache(ctx)
		}
		return n, err
	}
	columnsToUpdate := r.table.ListColumnNamesExcept()
	n, err := r.updateTableRecords(ctx, columnsToUpdate, false, []any{value})
	if err == nil {
		r.invalidateCache(ctx)
	}
	return n, err
}

// UpdateReturning updates a record and scans all returned columns back
// into value. Returns the number of rows affected.
func (r *Repository[T]) UpdateReturning(ctx context.Context, value *T) (int64, error) {
	primaryKey, ok := r.table.FindPrimaryKey()
	if !ok {
		return 0, fmt.Errorf("no primary key found in table definition: %s", r.table.Name)
	}
	columnsToUpdate := r.table.ListColumnNamesExcept()
	query, args, err := schemapkg.BuildUpdateQuery(value, columnsToUpdate, false, primaryKey, "*")
	if err != nil {
		return 0, err
	}
	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	r.invalidateCache(ctx)
	return tag.RowsAffected(), nil
}

// UpdateOnlyColumns updates only the specified columns. Pass nil to update
// all non-generated columns. Returns the number of rows affected.
func (r *Repository[T]) UpdateOnlyColumns(ctx context.Context, columns []string, value *T) (int64, error) {
	if columns == nil {
		columns = r.table.ListColumnNamesExcept()
	}
	n, err := r.updateTableRecords(ctx, columns, false, []any{value})
	if err == nil {
		r.invalidateCache(ctx)
	}
	return n, err
}

// UpdateExceptColumns updates all columns except the specified ones.
// Returns the number of rows affected.
func (r *Repository[T]) UpdateExceptColumns(ctx context.Context, columns []string, value *T) (int64, error) {
	columnsToUpdate := r.table.ListColumnNamesExcept(columns...)
	n, err := r.updateTableRecords(ctx, columnsToUpdate, false, []any{value})
	if err == nil {
		r.invalidateCache(ctx)
	}
	return n, err
}

// Upsert inserts a record with ON CONFLICT semantics. forceOnConflictExpr
// is appended to the ON CONFLICT clause and may be empty.
// When retry is enabled and not inside an existing transaction, the upsert
// runs in a retriable transaction.
func (r *Repository[T]) Upsert(ctx context.Context, value *T, forceOnConflictExpr string) error {
	if r.retryEnabled && !r.db.IsTransaction() {
		return r.db.InTransactionWithRetry(ctx, r.retry, func(tx *DB) error {
			txRepo := &Repository[T]{db: tx, table: r.table}
			return txRepo.Upsert(ctx, value, forceOnConflictExpr)
		})
	}
	structValue := schemapkg.IndirectValue(value)
	err := r.insertTableRecord(ctx, structValue, nil, forceOnConflictExpr, true)
	if err == nil {
		r.invalidateCache(ctx)
	}
	return err
}

// UpsertReturning inserts a record with ON CONFLICT semantics and scans
// all returned columns back into value.
func (r *Repository[T]) UpsertReturning(ctx context.Context, value *T, forceOnConflictExpr string) error {
	structValue := schemapkg.IndirectValue(value)
	primaryKey, ok := r.table.FindPrimaryKey()
	if !ok {
		return fmt.Errorf("no primary key found for table %s", r.table.Name)
	}
	idPtr := structValue.FieldByIndex(primaryKey.FieldIndex).Addr().Interface()
	err := r.insertTableRecord(ctx, structValue, idPtr, forceOnConflictExpr, true)
	if err == nil {
		r.invalidateCache(ctx)
	}
	return err
}

// Delete removes one or more records by primary key. Returns the number
// of rows deleted.
func (r *Repository[T]) Delete(ctx context.Context, values ...*T) (int64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	anyValues := make([]any, len(values))
	for i, v := range values {
		anyValues[i] = v
	}
	n, err := r.deleteTableRecords(ctx, anyValues)
	if err == nil {
		r.invalidateCache(ctx)
	}
	return n, err
}

// SelectByID fetches a single record by its primary key. Returns a
// pointer to T or ErrNoRows if not found. When a cache is configured,
// results are cached transparently.
func (r *Repository[T]) SelectByID(ctx context.Context, id any) (*T, error) {
	if r.cache != nil {
		key := fmt.Sprintf("%s:pk:%v", r.table.Name, id)
		var cached T
		found, err := r.cache.Get(ctx, key, &cached)
		if err != nil {
			return nil, err
		}
		if found {
			return &cached, nil
		}
	}

	var result T
	err := r.selectTableRecordByID(ctx, r.table, &result, id, LockOption{})
	if err != nil {
		return nil, err
	}

	if r.cache != nil {
		key := fmt.Sprintf("%s:pk:%v", r.table.Name, id)
		_ = r.cache.Set(ctx, key, result, r.jitteredTTL())
	}

	return &result, nil
}

// SelectByIDForUpdate reads a single row with row-level locking. It must
// be called inside a transaction (via db.InTransaction or uow.Do) — outside
// one, the lock is released the instant the statement completes and the
// call returns an error rather than silently doing nothing.
//
// This method never reads from or writes to the cache: a locked read is
// inherently about seeing the current, authoritative state inside a
// transaction for the purpose of modifying it.
func (r *Repository[T]) SelectByIDForUpdate(ctx context.Context, id any, opt LockOption) (*T, error) {
	if !r.db.IsTransaction() {
		return nil, fmt.Errorf("mirage: SelectByIDForUpdate requires an active transaction (call inside db.InTransaction or uow.Do); outside a transaction the lock is released before the caller's next statement runs")
	}

	var result T
	err := r.selectTableRecordByID(ctx, r.table, &result, id, opt)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// QueryForUpdate executes a SQL query with row-level locking and scans all
// resulting rows into a slice of *T. The lock option is appended to the
// query. Must be called inside a transaction.
//
// This method never reads from or writes to the cache.
func (r *Repository[T]) QueryForUpdate(ctx context.Context, opt LockOption, sql string, args ...any) ([]*T, error) {
	if !r.db.IsTransaction() {
		return nil, fmt.Errorf("mirage: QueryForUpdate requires an active transaction (call inside db.InTransaction or uow.Do); outside a transaction the lock is released before the caller's next statement runs")
	}

	// Append the lock clause to the user's SQL. The lock clause goes
	// after WHERE ... ORDER BY ... LIMIT ... but before the final semicolon.
	lockClause := opt.sql()
	if lockClause == "" {
		return r.Query(ctx, sql, args...)
	}

	// Strip trailing semicolon/whitespace, append lock clause, re-add semicolon.
	cleaned := sql
	for len(cleaned) > 0 && (cleaned[len(cleaned)-1] == ';' || cleaned[len(cleaned)-1] == ' ' || cleaned[len(cleaned)-1] == '\n' || cleaned[len(cleaned)-1] == '\t') {
		cleaned = cleaned[:len(cleaned)-1]
	}
	lockSQL := cleaned + lockClause + ";"

	var results []*T
	rows, err := r.db.Query(ctx, lockSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		elem := new(T)
		if err := scanRow(r.table, rows, elem); err != nil {
			return nil, err
		}
		results = append(results, elem)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// Exists reports whether a record matching the non-zero fields of value
// exists in the database. When a cache is configured, results are cached
// transparently.
func (r *Repository[T]) Exists(ctx context.Context, value *T) (bool, error) {
	if r.cache != nil {
		structValue := schemapkg.IndirectValue(value)
		key, keyErr := r.existsCacheKey(structValue)
		if keyErr != nil {
			// Can't build a safe cache key for this value (e.g. a field
			// isn't JSON-encodable) -- skip the cache rather than fall
			// back to fmt's %v, which prints raw pointer addresses for
			// pointer-typed fields (the standard idiom here for nullable
			// columns). Two calls with equal *values* behind different
			// pointers would then get different cache keys every time,
			// so the cache would silently never hit for any table with a
			// nullable column -- worse than not caching at all, because
			// it looks like caching is working.
			exists, err := r.tableRecordExists(ctx, r.table, structValue)
			return exists, err
		}

		var cached bool
		found, err := r.cache.Get(ctx, key, &cached)
		if err != nil {
			return false, err
		}
		if found {
			return cached, nil
		}

		exists, err := r.tableRecordExists(ctx, r.table, structValue)
		if err != nil {
			return false, err
		}
		_ = r.cache.Set(ctx, key, exists, r.jitteredTTL())
		return exists, nil
	}

	structValue := schemapkg.IndirectValue(value)
	return r.tableRecordExists(ctx, r.table, structValue)
}

// existsCacheKey builds a cache key from the value's content, not its
// memory layout. json.Marshal dereferences pointer fields (the common
// idiom for nullable columns) into their actual values, so two structs
// that are equal in content always get the same key -- unlike fmt's %v,
// which prints a pointer field's address and produces a different key on
// every call even when the pointed-to value is identical.
func (r *Repository[T]) existsCacheKey(structValue reflect.Value) (string, error) {
	b, err := json.Marshal(structValue.Interface())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:exists:%s", r.table.Name, b), nil
}

// Duplicate copies an existing record, inserting a new row with all the
// same column values except the primary key. Returns the newly created
// record or an error.
func (r *Repository[T]) Duplicate(ctx context.Context, value *T) (*T, error) {
	primaryKey, ok := r.table.FindPrimaryKey()
	if !ok {
		return nil, fmt.Errorf("duplicate: primary key is required")
	}
	val := schemapkg.IndirectValue(value)
	idValue, err := schemapkg.ExtractPrimaryKeyValue(primaryKey, val)
	if err != nil {
		return nil, err
	}
	newID := reflect.New(primaryKey.FieldType).Interface()
	err = r.duplicateTableRecord(ctx, idValue, newID)
	if err != nil {
		return nil, err
	}
	r.invalidateCache(ctx)
	return r.SelectByID(ctx, newID)
}

// InsertMany inserts multiple records in a single transaction.
func (r *Repository[T]) InsertMany(ctx context.Context, values []*T) error {
	if len(values) == 0 {
		return nil
	}
	txFn := func(db *DB) error {
		// Bind to the transactional db handle, not r (which is bound to
		// r.db, the connection this Repository was constructed with). Using
		// r.Insert here would silently run each row as its own autocommit
		// statement on the pool instead of inside this transaction: every
		// row before a failing one would stay committed, and this method's
		// "single transaction" guarantee would be a no-op. No cache on
		// txRepo -- invalidation happens once, after commit, below.
		txRepo := &Repository[T]{db: db, table: r.table}
		for _, v := range values {
			if err := txRepo.Insert(ctx, v); err != nil {
				return err
			}
		}
		return nil
	}
	var err error
	if r.retryEnabled && !r.db.IsTransaction() {
		err = r.db.InTransactionWithRetry(ctx, r.retry, txFn)
	} else {
		err = r.db.InTransaction(ctx, txFn)
	}
	if err == nil {
		r.invalidateCache(ctx)
	}
	return err
}

// InsertManyReturning inserts multiple records with RETURNING * in a
// single transaction. Each value's generated columns are populated.
func (r *Repository[T]) InsertManyReturning(ctx context.Context, values []*T) error {
	if len(values) == 0 {
		return nil
	}
	txFn := func(db *DB) error {
		txRepo := &Repository[T]{db: db, table: r.table}
		for _, v := range values {
			if err := txRepo.InsertReturning(ctx, v); err != nil {
				return err
			}
		}
		return nil
	}
	var err error
	if r.retryEnabled && !r.db.IsTransaction() {
		err = r.db.InTransactionWithRetry(ctx, r.retry, txFn)
	} else {
		err = r.db.InTransaction(ctx, txFn)
	}
	if err == nil {
		r.invalidateCache(ctx)
	}
	return err
}

// UpdateMany persists all columns of multiple records in a single
// transaction. Returns the total number of rows affected.
func (r *Repository[T]) UpdateMany(ctx context.Context, values []*T) (int64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	anyValues := make([]any, len(values))
	for i, v := range values {
		anyValues[i] = v
	}
	n, err := r.updateTableRecords(ctx, nil, false, anyValues)
	if err == nil {
		r.invalidateCache(ctx)
	}
	return n, err
}

// UpsertMany inserts or updates multiple records in a single transaction.
func (r *Repository[T]) UpsertMany(ctx context.Context, values []*T, forceOnConflictExpr string) error {
	if len(values) == 0 {
		return nil
	}
	txFn := func(db *DB) error {
		txRepo := &Repository[T]{db: db, table: r.table}
		for _, v := range values {
			if err := txRepo.Upsert(ctx, v, forceOnConflictExpr); err != nil {
				return err
			}
		}
		return nil
	}
	var err error
	if r.retryEnabled && !r.db.IsTransaction() {
		err = r.db.InTransactionWithRetry(ctx, r.retry, txFn)
	} else {
		err = r.db.InTransaction(ctx, txFn)
	}
	if err == nil {
		r.invalidateCache(ctx)
	}
	return err
}

// DeleteMany removes multiple records by primary key in a single
// transaction. Returns the total number of rows deleted.
func (r *Repository[T]) DeleteMany(ctx context.Context, values []*T) (int64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	anyValues := make([]any, len(values))
	for i, v := range values {
		anyValues[i] = v
	}
	n, err := r.deleteTableRecords(ctx, anyValues)
	if err == nil {
		r.invalidateCache(ctx)
	}
	return n, err
}

// Query executes a SQL query and scans all resulting rows into a slice
// of *T. Returns nil (not an error) if no rows are returned.
func (r *Repository[T]) Query(ctx context.Context, sql string, args ...any) ([]*T, error) {
	var results []*T
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		elem := new(T)
		if err := scanRow(r.table, rows, elem); err != nil {
			return nil, err
		}
		results = append(results, elem)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// QuerySingle executes a SQL query and scans the single resulting row
// into a *T. Returns ErrNoRows if no row is returned.
func (r *Repository[T]) QuerySingle(ctx context.Context, sql string, args ...any) (*T, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", r.table.GetHumanName(), err)
		}
		return nil, fmt.Errorf("%s: %w", r.table.GetHumanName(), ErrNoRows)
	}

	result := new(T)
	if err := scanRow(r.table, rows, result); err != nil {
		return nil, err
	}

	return result, nil
}

// QueryWithCache is like Query but caches the result using the repository's
// configured cache. key is the cache key, ttl overrides the default TTL.
// If no cache is configured, this behaves identically to Query.
func (r *Repository[T]) QueryWithCache(ctx context.Context, key string, ttl time.Duration, sql string, args ...any) ([]*T, error) {
	if r.cache != nil {
		var cached []*T
		found, err := r.cache.Get(ctx, key, &cached)
		if err != nil {
			return nil, err
		}
		if found {
			return cached, nil
		}
	}

	results, err := r.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	if r.cache != nil && results != nil {
		_ = r.cache.Set(ctx, key, results, ttl)
	}

	return results, nil
}

// QuerySingleWithCache is like QuerySingle but caches the result.
// If no cache is configured, this behaves identically to QuerySingle.
func (r *Repository[T]) QuerySingleWithCache(ctx context.Context, key string, ttl time.Duration, sql string, args ...any) (*T, error) {
	if r.cache != nil {
		var cached T
		found, err := r.cache.Get(ctx, key, &cached)
		if err != nil {
			return nil, err
		}
		if found {
			return &cached, nil
		}
	}

	result, err := r.QuerySingle(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	if r.cache != nil {
		_ = r.cache.Set(ctx, key, result, ttl)
	}

	return result, nil
}

// InvalidateCache removes all cached values whose keys start with prefix.
// No-op if no cache is configured.
func (r *Repository[T]) InvalidateCache(ctx context.Context, prefix string) error {
	if r.cache == nil {
		return nil
	}
	return r.cache.Invalidate(ctx, prefix)
}

// scanRow maps row columns to struct fields using the table definition.
func scanRow(td *schemapkg.Table, rows Rows, dest any) error {
	destVal := reflect.ValueOf(dest).Elem()

	colMap := make(map[string]*schemapkg.Column, len(td.Columns))
	for _, col := range td.Columns {
		colMap[col.Name] = col
	}

	fieldDescs := rows.FieldDescriptions()
	dests := make([]any, len(fieldDescs))
	for i, fd := range fieldDescs {
		col, ok := colMap[fd.Name]
		if ok && col.FieldIndex != nil {
			field := destVal.FieldByIndex(col.FieldIndex)
			if field.CanAddr() {
				dests[i] = field.Addr().Interface()
			} else {
				dests[i] = new(any)
			}
		} else {
			dests[i] = new(any)
		}
	}

	return rows.Scan(dests...)
}

// --- Internal helpers (moved from DB methods) ---

func (r *Repository[T]) jitteredTTL() time.Duration {
	if r.cacheTTLMax <= r.cacheTTLMin {
		return r.cacheTTLMin
	}
	delta := r.cacheTTLMax - r.cacheTTLMin
	return r.cacheTTLMin + time.Duration(rand.Int63n(int64(delta)))
}

func (r *Repository[T]) invalidateCache(ctx context.Context) {
	if r.cache != nil {
		_ = r.cache.Invalidate(ctx, r.table.Name+":")
	}
}

func (r *Repository[T]) insertTableRecord(ctx context.Context, structValue reflect.Value, idPtr any, forceOnConflictExpr string, upsert bool, returningColumns ...string) error {
	query, args, err := schemapkg.BuildInsertQuery(r.table, structValue, idPtr, forceOnConflictExpr, upsert, returningColumns...)
	if err != nil {
		return err
	}
	if idPtr != nil || len(returningColumns) > 0 {
		return r.db.QueryRow(ctx, query, args...).Scan(idPtr)
	}
	_, err = r.db.Exec(ctx, query, args...)
	return err
}

func (r *Repository[T]) updateTableRecord(ctx context.Context, value any, columnsToUpdate []string, reportNotFound bool, primaryKey *schemapkg.Column, returningColumns ...string) (int64, error) {
	query, args, err := schemapkg.BuildUpdateQuery(value, columnsToUpdate, reportNotFound, primaryKey, returningColumns...)
	if err != nil {
		return 0, err
	}
	if reportNotFound {
		scanErr := r.db.QueryRow(ctx, query, args...).Scan(nil)
		if scanErr != nil {
			return 0, scanErr
		}
		return 1, nil
	}
	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *Repository[T]) updateTableRecords(ctx context.Context, columnsToUpdate []string, reportNotFound bool, values []any) (int64, error) {
	primaryKey, ok := r.table.FindPrimaryKey()
	if !ok {
		return 0, fmt.Errorf("no primary key found in table definition: %s", r.table.Name)
	}
	if len(values) == 1 {
		return r.updateTableRecord(ctx, values[0], columnsToUpdate, reportNotFound, primaryKey)
	}
	var totalRowsAffected int64
	txFn := func(db *DB) error {
		// txRepo, not r: r.updateTableRecord always executes against r.db,
		// which would bypass this transaction entirely (see InsertMany).
		txRepo := &Repository[T]{db: db, table: r.table}
		for _, value := range values {
			rowsAffected, err := txRepo.updateTableRecord(ctx, value, columnsToUpdate, reportNotFound, primaryKey)
			if err != nil {
				return err
			}
			totalRowsAffected += rowsAffected
		}
		return nil
	}
	var err error
	if r.retryEnabled && !r.db.IsTransaction() {
		err = r.db.InTransactionWithRetry(ctx, r.retry, txFn)
	} else {
		err = r.db.InTransaction(ctx, txFn)
	}
	if err != nil {
		return 0, err
	}
	return totalRowsAffected, nil
}

func (r *Repository[T]) deleteTableRecords(ctx context.Context, values []any) (int64, error) {
	query, ids, err := schemapkg.BuildDeleteQuery(r.table, values)
	if err != nil {
		return 0, err
	}
	tag, err := r.db.Exec(ctx, query, ids...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *Repository[T]) duplicateTableRecord(ctx context.Context, id any, newIDPtr any, returningColumns ...string) error {
	if id == nil {
		return fmt.Errorf("duplicate: id is required")
	}
	query, err := schemapkg.BuildDuplicateQuery(r.table, newIDPtr, returningColumns...)
	if err != nil {
		return err
	}
	if newIDPtr != nil {
		err = r.db.QueryRow(ctx, query, id).Scan(newIDPtr)
	} else {
		_, err = r.db.Exec(ctx, query, id)
	}
	return err
}

func (r *Repository[T]) selectTableRecordByID(ctx context.Context, td *schemapkg.Table, destPtr any, id any, lock LockOption) error {
	primaryCol, ok := td.FindPrimaryKey()
	if !ok {
		return fmt.Errorf("no primary key found in table definition: %s", td.Name)
	}
	query := fmt.Sprintf(`SELECT * FROM %s.%s WHERE %s = $1 LIMIT 1%s;`,
		QuoteIdentifier(r.db.searchPath), QuoteIdentifier(td.Name), QuoteIdentifier(primaryCol.Name), lock.sql())
	return r.selectSingleTable(ctx, td, destPtr, query, id)
}

func (r *Repository[T]) selectSingleTable(ctx context.Context, td *schemapkg.Table, destPtr any, query string, args ...any) error {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return fmt.Errorf("%s: %w", td.GetHumanName(), err)
		}
		return fmt.Errorf("%s: %w", td.GetHumanName(), ErrNoRows)
	}
	return schemapkg.ConvertRowsToStruct(td, rows, destPtr)
}

func (r *Repository[T]) tableRecordExists(ctx context.Context, td *schemapkg.Table, structValue reflect.Value) (bool, error) {
	query, args, err := schemapkg.BuildExistsQuery(td, structValue)
	if err != nil {
		return false, err
	}
	var exists bool
	err = r.db.QueryRow(ctx, query, args...).Scan(&exists)
	return exists, err
}
