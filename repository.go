package mirage

import (
	"context"
	"fmt"
	"reflect"
	"strings"

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
	db           *DB
	table        *schemapkg.Table
	retry        RetryOptions
	retryEnabled bool
}

// RepositoryOption configures a Repository.
type RepositoryOption func(*repositoryConfig)

type repositoryConfig struct {
	retry        RetryOptions
	retryEnabled bool
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
		retry:        cfg.retry,
		retryEnabled: cfg.retryEnabled,
	}
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
	return r.insertTableRecord(ctx, structValue, nil, "", false)
}

// InsertReturning inserts a single record and scans all returned columns
// back into value. This uses RETURNING * so all database-generated values
// (defaults, computed columns, etc.) are populated.
// When retry is enabled and not inside an existing transaction, the insert
// runs in a retriable transaction.
func (r *Repository[T]) InsertReturning(ctx context.Context, value *T) error {
	if r.retryEnabled && !r.db.IsTransaction() {
		return r.db.InTransactionWithRetry(ctx, r.retry, func(tx *DB) error {
			txRepo := &Repository[T]{db: tx, table: r.table}
			return txRepo.InsertReturning(ctx, value)
		})
	}
	structValue := schemapkg.IndirectValue(value)
	primaryKey, ok := r.table.FindPrimaryKey()
	if !ok {
		return fmt.Errorf("no primary key found for table %s", r.table.Name)
	}
	idPtr := structValue.FieldByIndex(primaryKey.FieldIndex).Addr().Interface()
	return r.insertTableRecord(ctx, structValue, idPtr, "", false)
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
		return n, err
	}
	columnsToUpdate := r.table.ListColumnNamesExcept()
	return r.updateTableRecords(ctx, columnsToUpdate, false, []any{value})
}

// UpdateReturning updates a record and scans all returned columns back
// into value. Returns the number of rows affected.
// When retry is enabled and not inside an existing transaction, the update
// runs in a retriable transaction.
func (r *Repository[T]) UpdateReturning(ctx context.Context, value *T) (int64, error) {
	if r.retryEnabled && !r.db.IsTransaction() {
		var n int64
		err := r.db.InTransactionWithRetry(ctx, r.retry, func(tx *DB) error {
			txRepo := &Repository[T]{db: tx, table: r.table}
			var err error
			n, err = txRepo.UpdateReturning(ctx, value)
			return err
		})
		return n, err
	}
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
	return tag.RowsAffected(), nil
}

// UpdateOnlyColumns updates only the specified columns. Pass nil to update
// all non-generated columns. Returns the number of rows affected.
// When retry is enabled and not inside an existing transaction, the update
// runs in a retriable transaction.
func (r *Repository[T]) UpdateOnlyColumns(ctx context.Context, columns []string, value *T) (int64, error) {
	if r.retryEnabled && !r.db.IsTransaction() {
		var n int64
		err := r.db.InTransactionWithRetry(ctx, r.retry, func(tx *DB) error {
			txRepo := &Repository[T]{db: tx, table: r.table}
			var err error
			n, err = txRepo.UpdateOnlyColumns(ctx, columns, value)
			return err
		})
		return n, err
	}
	if columns == nil {
		columns = r.table.ListColumnNamesExcept()
	}
	return r.updateTableRecords(ctx, columns, false, []any{value})
}

// UpdateExceptColumns updates all columns except the specified ones.
// Returns the number of rows affected.
// When retry is enabled and not inside an existing transaction, the update
// runs in a retriable transaction.
func (r *Repository[T]) UpdateExceptColumns(ctx context.Context, columns []string, value *T) (int64, error) {
	if r.retryEnabled && !r.db.IsTransaction() {
		var n int64
		err := r.db.InTransactionWithRetry(ctx, r.retry, func(tx *DB) error {
			txRepo := &Repository[T]{db: tx, table: r.table}
			var err error
			n, err = txRepo.UpdateExceptColumns(ctx, columns, value)
			return err
		})
		return n, err
	}
	columnsToUpdate := r.table.ListColumnNamesExcept(columns...)
	return r.updateTableRecords(ctx, columnsToUpdate, false, []any{value})
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
	return r.insertTableRecord(ctx, structValue, nil, forceOnConflictExpr, true)
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
	return r.insertTableRecord(ctx, structValue, idPtr, forceOnConflictExpr, true)
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
	return r.deleteTableRecords(ctx, anyValues)
}

// SelectByID fetches a single record by its primary key. Returns a
// pointer to T or ErrNoRows if not found.
func (r *Repository[T]) SelectByID(ctx context.Context, id any) (*T, error) {
	var result T
	err := r.selectTableRecordByID(ctx, r.table, &result, id, LockOption{})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// SelectByIDForUpdate reads a single row with row-level locking. It must
// be called inside a transaction (via db.InTransaction or uow.Do) — outside
// one, the lock is released the instant the statement completes and the
// call returns an error rather than silently doing nothing.
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
func (r *Repository[T]) QueryForUpdate(ctx context.Context, opt LockOption, sql string, args ...any) ([]*T, error) {
	if !r.db.IsTransaction() {
		return nil, fmt.Errorf("mirage: QueryForUpdate requires an active transaction (call inside db.InTransaction or uow.Do); outside a transaction the lock is released before the caller's next statement runs")
	}

	lockClause := opt.sql()
	if lockClause == "" {
		return r.Query(ctx, sql, args...)
	}

	cleaned := sql
	for len(cleaned) > 0 && (cleaned[len(cleaned)-1] == ';' || cleaned[len(cleaned)-1] == ' ' || cleaned[len(cleaned)-1] == '\n' || cleaned[len(cleaned)-1] == '\t') {
		cleaned = cleaned[:len(cleaned)-1]
	}
	lockSQL := cleaned + "\n" + strings.TrimSpace(lockClause) + ";"

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
// exists in the database.
func (r *Repository[T]) Exists(ctx context.Context, value *T) (bool, error) {
	structValue := schemapkg.IndirectValue(value)
	return r.tableRecordExists(ctx, r.table, structValue)
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
	return r.SelectByID(ctx, newID)
}

// InsertMany inserts multiple records in a single transaction.
func (r *Repository[T]) InsertMany(ctx context.Context, values []*T) error {
	if len(values) == 0 {
		return nil
	}
	txFn := func(db *DB) error {
		txRepo := &Repository[T]{db: db, table: r.table}
		for _, v := range values {
			if err := txRepo.Insert(ctx, v); err != nil {
				return err
			}
		}
		return nil
	}
	if r.retryEnabled && !r.db.IsTransaction() {
		return r.db.InTransactionWithRetry(ctx, r.retry, txFn)
	}
	return r.db.InTransaction(ctx, txFn)
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
	if r.retryEnabled && !r.db.IsTransaction() {
		return r.db.InTransactionWithRetry(ctx, r.retry, txFn)
	}
	return r.db.InTransaction(ctx, txFn)
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
	return r.updateTableRecords(ctx, nil, false, anyValues)
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
	if r.retryEnabled && !r.db.IsTransaction() {
		return r.db.InTransactionWithRetry(ctx, r.retry, txFn)
	}
	return r.db.InTransaction(ctx, txFn)
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
	return r.deleteTableRecords(ctx, anyValues)
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

// --- Internal helpers ---

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
