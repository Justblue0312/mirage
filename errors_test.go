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
