package mirage

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsErrSerializationFailure(t *testing.T) {
	serErr := &pgconn.PgError{Code: sqlStateSerializationFailure}
	if !IsErrSerializationFailure(serErr) {
		t.Error("expected serialization failure to be detected")
	}
	if !IsErrSerializationFailure(fmt.Errorf("wrapped: %w", serErr)) {
		t.Error("expected wrapped serialization failure to be detected")
	}
	if IsErrSerializationFailure(&pgconn.PgError{Code: "23505"}) {
		t.Error("unique violation should not be a serialization failure")
	}
	if IsErrSerializationFailure(nil) {
		t.Error("nil should not be a serialization failure")
	}
}

func TestIsErrDeadlock(t *testing.T) {
	dlErr := &pgconn.PgError{Code: sqlStateDeadlockDetected}
	if !IsErrDeadlock(dlErr) {
		t.Error("expected deadlock to be detected")
	}
	if !IsErrDeadlock(fmt.Errorf("wrapped: %w", dlErr)) {
		t.Error("expected wrapped deadlock to be detected")
	}
	if IsErrDeadlock(&pgconn.PgError{Code: sqlStateSerializationFailure}) {
		t.Error("serialization failure should not be a deadlock")
	}
}

func TestIsErrRetryable(t *testing.T) {
	if !IsErrRetryable(&pgconn.PgError{Code: sqlStateSerializationFailure}) {
		t.Error("serialization failure should be retryable")
	}
	if !IsErrRetryable(&pgconn.PgError{Code: sqlStateDeadlockDetected}) {
		t.Error("deadlock should be retryable")
	}
	if IsErrRetryable(errors.New("some other error")) {
		t.Error("generic error should not be retryable")
	}
}

// TestIsErrDuplicate_SurvivesWrapping guards against the class of bug found
// during review: DB.Exec/DB.Query wrap every error as fmt.Errorf("exec: %w",
// err) / fmt.Errorf("query: %w", err) (see db.go), so any classifier that
// parses err.Error() with a fixed prefix or that doesn't unwrap breaks the
// moment a caller uses the normal DB/Repository API instead of pgx directly.
func TestIsErrDuplicate_SurvivesWrapping(t *testing.T) {
	pgErr := &pgconn.PgError{
		Severity:       "ERROR",
		Code:           "23505",
		Message:        `duplicate key value violates unique constraint "users_email_key"`,
		ConstraintName: "users_email_key",
	}

	if name, ok := IsErrDuplicate(pgErr); !ok || name != "users_email_key" {
		t.Fatalf("direct error: got (%q, %v), want (users_email_key, true)", name, ok)
	}

	wrapped := fmt.Errorf("exec: %w", pgErr)
	if name, ok := IsErrDuplicate(wrapped); !ok || name != "users_email_key" {
		t.Fatalf("wrapped error: got (%q, %v), want (users_email_key, true) -- classifier must unwrap, not string-match", name, ok)
	}

	doubleWrapped := fmt.Errorf("mirage: %w", wrapped)
	if _, ok := IsErrDuplicate(doubleWrapped); !ok {
		t.Fatal("double-wrapped error should still be detected as a duplicate")
	}

	fkErr := &pgconn.PgError{Code: "23503", ConstraintName: "fk_food"}
	if _, ok := IsErrDuplicate(fkErr); ok {
		t.Fatal("foreign key violation must not be reported as a duplicate")
	}

	if _, ok := IsErrDuplicate(nil); ok {
		t.Fatal("nil error must not be reported as a duplicate")
	}
}

func TestIsErrForeignKey_SurvivesWrapping(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23503", ConstraintName: "fk_food"}

	wrapped := fmt.Errorf("transaction: exec: %w", pgErr)
	if name, ok := IsErrForeignKey(wrapped); !ok || name != "fk_food" {
		t.Fatalf("wrapped error: got (%q, %v), want (fk_food, true)", name, ok)
	}

	dupErr := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}
	if _, ok := IsErrForeignKey(dupErr); ok {
		t.Fatal("unique violation must not be reported as a foreign key error")
	}
}

// TestIsErrInputSyntax_SurvivesWrapping reproduces the exact bug found
// during review: the old implementation checked strings.HasPrefix(err.Error(),
// "ERROR: "), which can never match once db.go has prefixed the error with
// "exec: " or "query: ". That made the exported helper silently dead code
// for every caller going through the normal DB/Repository API.
func TestIsErrInputSyntax_SurvivesWrapping(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    "22P02",
		Message: `invalid input syntax for type integer: "abc"`,
	}

	if _, ok := IsErrInputSyntax(pgErr); !ok {
		t.Fatal("direct pgconn.PgError should be detected")
	}

	// This is exactly what DB.Exec produces (see db.go: fmt.Errorf("exec: %w", err)).
	wrapped := fmt.Errorf("exec: %w", pgErr)
	if _, ok := IsErrInputSyntax(wrapped); !ok {
		t.Fatal("BUG: error wrapped by DB.Exec is no longer detected as an input-syntax error")
	}

	// And this is what it looks like inside a transaction (db.go:
	// fmt.Errorf("transaction: exec: %w", err)).
	txWrapped := fmt.Errorf("transaction: exec: %w", pgErr)
	if _, ok := IsErrInputSyntax(txWrapped); !ok {
		t.Fatal("BUG: error wrapped by a transactional DB.Exec is no longer detected")
	}

	tsErr := &pgconn.PgError{Code: "42601", Message: "syntax error in tsquery: \"a &\""}
	if _, ok := IsErrInputSyntax(fmt.Errorf("query: %w", tsErr)); !ok {
		t.Fatal("tsquery syntax error should be detected even when wrapped")
	}

	other := &pgconn.PgError{Code: "23505"}
	if _, ok := IsErrInputSyntax(other); ok {
		t.Fatal("unrelated error code must not be reported as input-syntax error")
	}
}

func TestIsErrColumnNotExists_SurvivesWrapping(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:       "42703",
		Message:    `column "deleted_at" does not exist`,
		ColumnName: "deleted_at",
	}

	wrapped := fmt.Errorf("query: %w", pgErr)
	if !IsErrColumnNotExists(wrapped, "deleted_at") {
		t.Fatal("BUG: wrapped column-not-exists error not detected")
	}
	if IsErrColumnNotExists(wrapped, "other_col") {
		t.Fatal("must not match a different column name")
	}
	if IsErrColumnNotExists(nil, "deleted_at") {
		t.Fatal("nil error must not match")
	}
}

func TestRetryLoop_SucceedsAfterRetries(t *testing.T) {
	retryable := errors.New("retry me")
	calls := 0
	opts := RetryOptions{
		MaxAttempts: 5,
		BaseDelay:   time.Millisecond,
		ShouldRetry: func(err error) bool { return errors.Is(err, retryable) },
	}

	err := retryLoop(context.Background(), opts, func() error {
		calls++
		if calls < 3 {
			return retryable
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 attempts, got %d", calls)
	}
}

func TestRetryLoop_ExhaustsAttempts(t *testing.T) {
	retryable := errors.New("always retry")
	calls := 0
	opts := RetryOptions{
		MaxAttempts: 4,
		BaseDelay:   time.Millisecond,
		ShouldRetry: func(err error) bool { return true },
	}

	err := retryLoop(context.Background(), opts, func() error {
		calls++
		return retryable
	})
	if !errors.Is(err, retryable) {
		t.Fatalf("expected final error to be returned, got %v", err)
	}
	if calls != 4 {
		t.Errorf("expected 4 attempts (MaxAttempts), got %d", calls)
	}
}

func TestRetryLoop_NonRetryableStopsImmediately(t *testing.T) {
	fatal := errors.New("fatal")
	calls := 0
	opts := RetryOptions{
		MaxAttempts: 5,
		BaseDelay:   time.Millisecond,
		ShouldRetry: func(err error) bool { return false },
	}

	err := retryLoop(context.Background(), opts, func() error {
		calls++
		return fatal
	})
	if !errors.Is(err, fatal) {
		t.Fatalf("expected fatal error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("non-retryable error should stop after 1 attempt, got %d", calls)
	}
}

func TestRetryLoop_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := RetryOptions{
		MaxAttempts: 5,
		BaseDelay:   time.Hour, // long delay so cancellation wins
		ShouldRetry: func(err error) bool { return true },
	}

	err := retryLoop(ctx, opts, func() error {
		return errors.New("retryable")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled to be joined into error, got %v", err)
	}
}
