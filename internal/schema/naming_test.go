package schema

import (
	"strings"
	"testing"
)

func TestPKName(t *testing.T) {
	tests := []struct {
		name  string
		table string
		want  string
	}{
		{
			name:  "simple table",
			table: "users",
			want:  "pk_users",
		},
		{
			name:  "long table name",
			table: strings.Repeat("a", 70),
			want:  Truncate("pk_" + strings.Repeat("a", 70)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PKName(tt.table)
			if got != tt.want {
				t.Errorf("PKName(%q) = %q, want %q", tt.table, got, tt.want)
			}
			if len(got) > 63 {
				t.Errorf("PKName(%q) length = %d, must be <= 63", tt.table, len(got))
			}
		})
	}
}

func TestFKName(t *testing.T) {
	tests := []struct {
		name      string
		fromTable string
		fromCols  []string
		want      string
	}{
		{
			name:      "single column",
			fromTable: "orders",
			fromCols:  []string{"user_id"},
			want:      "fk_orders_user_id",
		},
		{
			name:      "multiple columns",
			fromTable: "orders",
			fromCols:  []string{"user_id", "tenant_id"},
			want:      "fk_orders_user_id_tenant_id",
		},
		{
			name:      "long table and columns",
			fromTable: strings.Repeat("a", 70),
			fromCols:  []string{strings.Repeat("b", 70)},
			want:      Truncate("fk_" + strings.Repeat("a", 70) + "_" + strings.Repeat("b", 70)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FKName(tt.fromTable, tt.fromCols)
			if got != tt.want {
				t.Errorf("FKName(%q, %v) = %q, want %q", tt.fromTable, tt.fromCols, got, tt.want)
			}
			if len(got) > 63 {
				t.Errorf("FKName(%q, %v) length = %d, must be <= 63", tt.fromTable, tt.fromCols, len(got))
			}
		})
	}
}

func TestUQName(t *testing.T) {
	tests := []struct {
		name  string
		table string
		cols  []string
		want  string
	}{
		{
			name:  "single column",
			table: "users",
			cols:  []string{"email"},
			want:  "uq_users_email",
		},
		{
			name:  "multiple columns",
			table: "orders",
			cols:  []string{"email", "tenant_id"},
			want:  "uq_orders_email_tenant_id",
		},
		{
			name:  "long table and columns",
			table: strings.Repeat("a", 70),
			cols:  []string{strings.Repeat("b", 70), strings.Repeat("c", 70)},
			want:  Truncate("uq_" + strings.Repeat("a", 70) + "_" + strings.Repeat("b", 70) + "_" + strings.Repeat("c", 70)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UQName(tt.table, tt.cols)
			if got != tt.want {
				t.Errorf("UQName(%q, %v) = %q, want %q", tt.table, tt.cols, got, tt.want)
			}
			if len(got) > 63 {
				t.Errorf("UQName(%q, %v) length = %d, must be <= 63", tt.table, tt.cols, len(got))
			}
		})
	}
}

func TestIdxName(t *testing.T) {
	tests := []struct {
		name  string
		table string
		cols  []string
		want  string
	}{
		{
			name:  "single column",
			table: "users",
			cols:  []string{"email"},
			want:  "idx_users_email",
		},
		{
			name:  "multiple columns",
			table: "users",
			cols:  []string{"email", "status"},
			want:  "idx_users_email_status",
		},
		{
			name:  "long table and columns",
			table: strings.Repeat("a", 70),
			cols:  []string{strings.Repeat("b", 70)},
			want:  Truncate("idx_" + strings.Repeat("a", 70) + "_" + strings.Repeat("b", 70)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IdxName(tt.table, tt.cols)
			if got != tt.want {
				t.Errorf("IdxName(%q, %v) = %q, want %q", tt.table, tt.cols, got, tt.want)
			}
			if len(got) > 63 {
				t.Errorf("IdxName(%q, %v) length = %d, must be <= 63", tt.table, tt.cols, len(got))
			}
		})
	}
}

func TestChkName(t *testing.T) {
	tests := []struct {
		name  string
		table string
		seq   int
		want  string
	}{
		{
			name:  "sequence 1",
			table: "users",
			seq:   1,
			want:  "chk_users_1",
		},
		{
			name:  "sequence 0",
			table: "users",
			seq:   0,
			want:  "chk_users_0",
		},
		{
			name:  "large sequence",
			table: "users",
			seq:   999999,
			want:  "chk_users_999999",
		},
		{
			name:  "long table name",
			table: strings.Repeat("a", 70),
			seq:   1,
			want:  Truncate("chk_" + strings.Repeat("a", 70) + "_1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChkName(tt.table, tt.seq)
			if got != tt.want {
				t.Errorf("ChkName(%q, %d) = %q, want %q", tt.table, tt.seq, got, tt.want)
			}
			if len(got) > 63 {
				t.Errorf("ChkName(%q, %d) length = %d, must be <= 63", tt.table, tt.seq, len(got))
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Run("no truncation needed", func(t *testing.T) {
		input := "pk_users"
		got := Truncate(input)
		if got != input {
			t.Errorf("Truncate(%q) = %q, want %q", input, got, input)
		}
	})

	t.Run("exactly 63 chars", func(t *testing.T) {
		input := strings.Repeat("a", 63)
		got := Truncate(input)
		if got != input {
			t.Errorf("Truncate(63 chars) = %q (len=%d), want unchanged (len=63)", got, len(got))
		}
	})

	t.Run("over 63 chars truncates", func(t *testing.T) {
		input := "idx_" + strings.Repeat("a", 70)
		got := Truncate(input)
		if len(got) > 63 {
			t.Errorf("Truncate(long name) length = %d, must be <= 63", len(got))
		}
	})

	t.Run("stability", func(t *testing.T) {
		input := "idx_" + strings.Repeat("a", 70)
		got1 := Truncate(input)
		got2 := Truncate(input)
		if got1 != got2 {
			t.Errorf("Truncate is not stable: first=%q, second=%q", got1, got2)
		}
	})

	t.Run("preserves prefix underscore", func(t *testing.T) {
		input := "idx_" + strings.Repeat("a", 70)
		got := Truncate(input)
		if !strings.HasPrefix(got, "idx_") {
			t.Errorf("Truncate(%q) = %q, want prefix 'idx_'", input, got)
		}
	})

	t.Run("preserves table name when possible", func(t *testing.T) {
		input := "idx_users_" + strings.Repeat("a", 70)
		got := Truncate(input)
		if !strings.HasPrefix(got, "idx_users_") {
			t.Errorf("Truncate(%q) = %q, want prefix 'idx_users_'", input, got)
		}
	})

	t.Run("no underscore fallback", func(t *testing.T) {
		input := strings.Repeat("a", 70)
		got := Truncate(input)
		if len(got) > 63 {
			t.Errorf("Truncate(no underscore) length = %d, must be <= 63", len(got))
		}
	})

	t.Run("all name functions produce names <= 63 chars", func(t *testing.T) {
		longTable := strings.Repeat("a", 100)
		longCols := []string{strings.Repeat("b", 100), strings.Repeat("c", 100)}

		names := map[string]string{
			"PKName":  PKName(longTable),
			"FKName":  FKName(longTable, longCols),
			"UQName":  UQName(longTable, longCols),
			"IdxName": IdxName(longTable, longCols),
			"ChkName": ChkName(longTable, 1),
		}

		for fn, name := range names {
			if len(name) > 63 {
				t.Errorf("%s(long inputs) length = %d, must be <= 63, got %q", fn, len(name), name)
			}
		}
	})

	t.Run("hash suffix is 8 hex chars", func(t *testing.T) {
		input := "idx_" + strings.Repeat("a", 70)
		got := Truncate(input)
		suffix := got[len(got)-8:]
		for _, c := range suffix {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				t.Errorf("Truncate suffix %q contains non-hex char %q", suffix, string(c))
				break
			}
		}
	})
}
