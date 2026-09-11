package scanner

import "testing"

func TestGoTypeToSQLType(t *testing.T) {
	tests := []struct {
		goType string
		want   string
	}{
		{"int", "int"},
		{"int8", "smallint"},
		{"int16", "smallint"},
		{"int32", "int"},
		{"int64", "bigint"},
		{"uint", "int"},
		{"uint8", "smallint"},
		{"uint16", "smallint"},
		{"uint32", "int"},
		{"uint64", "bigint"},
		{"float32", "real"},
		{"float64", "double precision"},
		{"bool", "boolean"},
		{"string", "text"},
		{"[]byte", "bytea"},
		{"time.Time", "timestamptz"},
		{"net.IP", "inet"},
		{"uuid.UUID", "uuid"},
		{"json.RawMessage", "jsonb"},
		{"unknown_custom", ""},
	}
	for _, tt := range tests {
		t.Run(tt.goType, func(t *testing.T) {
			got := goTypeToSQLType(tt.goType)
			if got != tt.want {
				t.Errorf("goTypeToSQLType(%q) = %q, want %q", tt.goType, got, tt.want)
			}
		})
	}
}
