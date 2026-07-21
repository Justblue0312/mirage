package introspect

import (
	"testing"

	"github.com/justblue/mirage/internal/schema"
)

func TestMapCatalogType(t *testing.T) {
	tests := []struct {
		name      string
		udtName   string
		dataType  string
		charMaxLen int
		want      string
	}{
		// Basic types
		{"int4", "int4", "integer", 0, "integer"},
		{"int8", "int8", "bigint", 0, "bigint"},
		{"int2", "int2", "smallint", 0, "smallint"},
		{"bool", "bool", "boolean", 0, "boolean"},
		{"text", "text", "text", 0, "text"},
		{"jsonb", "jsonb", "jsonb", 0, "jsonb"},
		{"uuid", "uuid", "uuid", 0, "uuid"},
		{"bytea", "bytea", "bytea", 0, "bytea"},

		// Parameterized varchar
		{"varchar(255)", "varchar", "character varying", 255, "varchar(255)"},
		{"varchar(100)", "varchar", "character varying", 100, "varchar(100)"},
		{"char(10)", "char", "character", 10, "char(10)"},
		{"bit(8)", "bit", "bit", 8, "bit(8)"},
		{"bit varying(16)", "bit varying", "bit varying", 16, "bit varying(16)"},

		// Canonical names (data_type fallback)
		{"character varying", "xyz_custom", "character varying", 0, "varchar"},
		{"double precision via data_type", "xyz_custom", "double precision", 0, "double precision"},

		// Serial types
		{"serial", "serial", "integer", 0, "serial"},
		{"bigserial", "bigserial", "bigint", 0, "bigserial"},
		{"smallserial", "smallserial", "smallint", 0, "smallserial"},
		{"serial4", "serial4", "integer", 0, "serial"},
		{"serial8", "serial8", "bigint", 0, "bigserial"},

		// Timestamp variants
		{"timestamptz", "timestamptz", "timestamp with time zone", 0, "timestamp with time zone"},
		{"timestamp", "timestamp", "timestamp without time zone", 0, "timestamp without time zone"},

		// Arrays
		{"_int4 (integer[])", "_int4", "ARRAY", 0, "integer[]"},
		{"_text (text[])", "_text", "ARRAY", 0, "text[]"},
		{"_varchar (varchar[])", "_varchar", "ARRAY", 0, "varchar[]"},
		{"_bool (boolean[])", "_bool", "ARRAY", 0, "boolean[]"},

		// Custom enum type (unknown to mapping)
		{"status_type", "status_type", "USER-DEFINED", 0, "status_type"},
		{"color_enum", "color_enum", "USER-DEFINED", 0, "color_enum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCatalogType(tt.udtName, tt.dataType, tt.charMaxLen)
			if got != tt.want {
				t.Errorf("mapCatalogType(%q, %q, %d) = %q, want %q",
					tt.udtName, tt.dataType, tt.charMaxLen, got, tt.want)
			}
		})
	}
}

func TestMapCatalogDataType(t *testing.T) {
	tests := []struct {
		name     string
		udtName  string
		dataType string
		want     schema.DataType
	}{
		{"int4 → Integer", "int4", "integer", schema.Integer},
		{"int8 → BigInt", "int8", "bigint", schema.BigInt},
		{"int2 → SmallInt", "int2", "smallint", schema.SmallInt},
		{"bool → Boolean", "bool", "boolean", schema.Boolean},
		{"text → Text", "text", "text", schema.Text},
		{"varchar → Varchar", "varchar", "character varying", schema.Varchar},
		{"jsonb → JsonB", "jsonb", "jsonb", schema.JsonB},
		{"uuid → UUID", "uuid", "uuid", schema.UUID},
		{"serial → Serial", "serial", "integer", schema.Serial},
		{"bigserial → BigSerial", "bigserial", "bigint", schema.BigSerial},
		{"timestamptz → TimestampWithTimeZone", "timestamptz", "timestamp with time zone", schema.TimestampWithTimeZone},
		{"_int4 → Integer (array base)", "_int4", "ARRAY", schema.Integer},
		{"unknown → Text (fallback)", "status_type", "USER-DEFINED", schema.Text},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCatalogDataType(tt.udtName, tt.dataType)
			if got != tt.want {
				t.Errorf("mapCatalogDataType(%q, %q) = %v, want %v",
					tt.udtName, tt.dataType, got, tt.want)
			}
		})
	}
}

func TestFindTable(t *testing.T) {
	tables := []schema.Table{
		{Name: "users"},
		{Name: "orders"},
		{Name: "products"},
	}

	if idx := findTable(tables, "orders"); idx != 1 {
		t.Errorf("findTable(orders) = %d, want 1", idx)
	}
	if idx := findTable(tables, "nonexistent"); idx != -1 {
		t.Errorf("findTable(nonexistent) = %d, want -1", idx)
	}
}
