package mirage

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SQLSTATE codes for the retryable transaction-isolation failures raised by
// PostgreSQL under REPEATABLE READ / SERIALIZABLE isolation.
const (
	sqlStateSerializationFailure = "40001"
	sqlStateDeadlockDetected     = "40P01"
)

// IsErrSerializationFailure reports whether err is a PostgreSQL serialization
// failure (SQLSTATE 40001). These are raised under SERIALIZABLE (and sometimes
// REPEATABLE READ) isolation and are safe to retry from the beginning of the
// transaction.
func IsErrSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == sqlStateSerializationFailure
}

// IsErrDeadlock reports whether err is a PostgreSQL deadlock (SQLSTATE 40P01).
// The transaction that lost the deadlock is aborted and is safe to retry.
func IsErrDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == sqlStateDeadlockDetected
}

// IsErrRetryable reports whether err is a transient transaction-isolation
// failure that is safe to retry (serialization failure or deadlock).
func IsErrRetryable(err error) bool {
	return IsErrSerializationFailure(err) || IsErrDeadlock(err)
}

var (
	// ErrNoRows is fired from a query when no results are came back.
	// Usually it's ignored and an empty json array is sent to the client instead.
	//
	// This error should be compared using errors.Is() or IsErrNoRows package-level function.
	ErrNoRows = pgx.ErrNoRows
)

// IsErrNoRows reports whether the error is ErrNoRows.
func IsErrNoRows(err error) bool {
	return errors.Is(err, ErrNoRows)
}

// PostgreSQL SQLSTATE codes used for structured error classification below.
// Using codes (rather than parsing the human-readable Message) is required
// because Message is subject to server-side localization (lc_messages) and
// wording changes across PostgreSQL versions, and because db.go wraps every
// error with a "query: "/"exec: "/"transaction: ..." prefix -- any check
// that relied on err.Error() having a specific prefix (e.g. "ERROR: ") broke
// the instant the error passed through DB.Exec/DB.Query. errors.As unwraps
// through any number of %w layers, so these helpers work regardless of how
// many times the error has been wrapped.
const (
	sqlStateUniqueViolation           = "23505"
	sqlStateForeignKeyViolation       = "23503"
	sqlStateInvalidTextRepresentation = "22P02" // invalid input syntax for type X
	sqlStateSyntaxErrorOrAccessRule   = "42601" // covers tsquery syntax errors, etc.
	sqlStateUndefinedColumn           = "42703"
)

// IsErrDuplicate reports whether err was caused by a violation of a unique
// constraint (SQLSTATE 23505). It returns the offending constraint name if
// so. Unlike a substring match on the error message, this survives error
// wrapping (fmt.Errorf("...: %w", err)) and is unaffected by server locale.
func IsErrDuplicate(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlStateUniqueViolation {
		return pgErr.ConstraintName, true
	}

	return "", false
}

// IsErrForeignKey reports whether an insert or update command failed due
// to an invalid foreign key (SQLSTATE 23503): a foreign key is missing or
// its referenced row was not found. It returns the offending constraint
// name if so.
func IsErrForeignKey(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlStateForeignKeyViolation {
		return pgErr.ConstraintName, true
	}
	return "", false
}

// IsErrInputSyntax reports whether err was caused by invalid input syntax
// for a PostgreSQL column type (SQLSTATE 22P02), including tsquery syntax
// errors. It returns a short description of the failure if so.
func IsErrInputSyntax(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return "", false
	}
	switch {
	case pgErr.Code == sqlStateInvalidTextRepresentation:
		return pgErr.Message, true
	case pgErr.Code == sqlStateSyntaxErrorOrAccessRule &&
		(strings.Contains(pgErr.Message, "tsquery") || strings.Contains(pgErr.Routine, "tsquery")):
		return pgErr.Message, true
	}

	return "", false
}

// IsErrColumnNotExists reports whether the error is caused because the
// "col" referenced in a query does not exist (SQLSTATE 42703). It cross
// checks pgErr.ColumnName / Message against col so the caller can still
// tell which specific column was missing when several are referenced.
func IsErrColumnNotExists(err error, col string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != sqlStateUndefinedColumn {
		return false
	}

	if pgErr.ColumnName == col {
		return true
	}
	return strings.Contains(pgErr.Message, fmt.Sprintf(`column "%s" does not exist`, col))
}
