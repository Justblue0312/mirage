package mirage

// LockStrength controls the row-level locking behavior for a read.
type LockStrength string

const (
	// LockForUpdate acquires an exclusive row lock. The locked row cannot be
	// modified by other transactions until this transaction commits or rolls back.
	LockForUpdate LockStrength = "FOR UPDATE"

	// LockForNoKeyUpdate acquires a weaker exclusive lock that does not lock
	// the row's primary key. Allows updates to the row's non-key columns by
	// other transactions.
	LockForNoKeyUpdate LockStrength = "FOR NO KEY UPDATE"

	// LockForShare acquires a shared lock. Multiple transactions can hold
	// SHARE locks on the same row, but no transaction can update it.
	LockForShare LockStrength = "FOR SHARE"

	// LockForKeyShare acquires a weaker shared lock that locks only the
	// row's primary key.
	LockForKeyShare LockStrength = "FOR KEY SHARE"
)

// LockWaitMode controls what happens when a row is already locked.
type LockWaitMode string

const (
	// LockWaitBlock is the default: block until the row becomes available.
	LockWaitBlock LockWaitMode = ""

	// LockWaitNoWait fails immediately with a lock_not_available error
	// (SQLSTATE 55P03) instead of blocking.
	LockWaitNoWait LockWaitMode = "NOWAIT"

	// LockWaitSkip skips locked rows entirely, returning only rows that
	// were immediately available. Standard pattern for job-queue / outbox
	// "claim the next available row" reads.
	LockWaitSkip LockWaitMode = "SKIP LOCKED"
)

// LockOption configures row-locking behavior for a read.
type LockOption struct {
	Strength LockStrength
	Wait     LockWaitMode
}

// ForUpdate requests FOR UPDATE row locking on the read.
func ForUpdate() LockOption {
	return LockOption{Strength: LockForUpdate}
}

// ForNoKeyUpdate requests FOR NO KEY UPDATE row locking on the read.
func ForNoKeyUpdate() LockOption {
	return LockOption{Strength: LockForNoKeyUpdate}
}

// ForShare requests FOR SHARE row locking on the read.
func ForShare() LockOption {
	return LockOption{Strength: LockForShare}
}

// ForUpdateSkipLocked requests FOR UPDATE SKIP LOCKED — the standard
// pattern for job-queue / outbox-style "claim the next available row"
// reads, where a locked-and-busy row should simply be skipped rather
// than blocking the caller.
func ForUpdateSkipLocked() LockOption {
	return LockOption{Strength: LockForUpdate, Wait: LockWaitSkip}
}

// ForUpdateNoWait requests FOR UPDATE NOWAIT — fail immediately with a
// lock_not_available error instead of blocking if the row is already locked.
func ForUpdateNoWait() LockOption {
	return LockOption{Strength: LockForUpdate, Wait: LockWaitNoWait}
}

// sql returns the SQL lock clause to append to a SELECT statement, or ""
// for a zero-value (no locking).
func (l LockOption) sql() string {
	if l.Strength == "" {
		return ""
	}
	clause := " " + string(l.Strength)
	if l.Wait != "" {
		clause += " " + string(l.Wait)
	}
	return clause
}
