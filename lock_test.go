package mirage

import "testing"

func TestLockOption_SQL(t *testing.T) {
	tests := []struct {
		name string
		opt  LockOption
		want string
	}{
		{"zero value", LockOption{}, ""},
		{"ForUpdate", ForUpdate(), " FOR UPDATE"},
		{"ForNoKeyUpdate", ForNoKeyUpdate(), " FOR NO KEY UPDATE"},
		{"ForShare", ForShare(), " FOR SHARE"},
		{"ForUpdateSkipLocked", ForUpdateSkipLocked(), " FOR UPDATE SKIP LOCKED"},
		{"ForUpdateNoWait", ForUpdateNoWait(), " FOR UPDATE NOWAIT"},
		{"custom", LockOption{Strength: LockForShare, Wait: LockWaitSkip}, " FOR SHARE SKIP LOCKED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opt.sql()
			if got != tt.want {
				t.Errorf("sql() = %q, want %q", got, tt.want)
			}
		})
	}
}
