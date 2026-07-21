package introspect

import (
	"fmt"
	"strings"

	"github.com/justblue/mirage/internal/schema"
)

// pgCatalogToSQLType maps PostgreSQL catalog names (from udt_name /
// data_type in information_schema) to the SQLType string the scanner
// would produce for the same logical type. This is the reverse mapping of
// schema.dataTypeText. The first entry in each slice is the canonical
// short form that DataType.String() returns — this is what the scanner
// stores as SQLType.
var pgCatalogToSQLType = map[string]string{
	"int8":                  "bigint",
	"bigint":                "bigint",
	"bool":                  "boolean",
	"boolean":               "boolean",
	"bytea":                 "bytea",
	"char":                  "char",
	"character":             "char",
	"cid":                   "cidr",
	"cidr":                  "cidr",
	"date":                  "date",
	"decimal":               "decimal",
	"numeric":               "numeric",
	"float8":                "double precision",
	"double precision":      "double precision",
	"float4":                "real",
	"real":                  "real",
	"float":                 "float",
	"inet":                  "inet",
	"int4":                  "integer",
	"int":                   "integer",
	"integer":               "integer",
	"interval":              "interval",
	"json":                  "json",
	"jsonb":                 "jsonb",
	"money":                 "money",
	"oid":                   "oid",
	"path":                  "path",
	"point":                 "point",
	"polygon":               "polygon",
	"int2":                  "smallint",
	"smallint":              "smallint",
	"text":                  "text",
	"time":                  "time",
	"timetz":                "time with time zone",
	"time with time zone":   "time with time zone",
	"timestamp":             "timestamp without time zone",
	"timestamptz":           "timestamp with time zone",
	"timestamp without time zone":    "timestamp without time zone",
	"timestamp with time zone":       "timestamp with time zone",
	"varchar":               "varchar",
	"character varying":     "varchar",
	"nvarchar":              "varchar",
	"xml":                   "xml",
	"uuid":                  "uuid",
	"tsvector":              "tsvector",
	"tsquery":               "tsquery",
	"bit":                   "bit",
	"bit varying":           "bit varying",
	"varbit":                "bit varying",
	"macaddr":               "macaddr",
	"macaddr8":              "macaddr8",
	"serial":                "serial",
	"serial4":               "serial",
	"bigserial":             "bigserial",
	"serial8":               "bigserial",
	"smallserial":           "smallserial",
	"serial2":               "smallserial",
	"hstore":                "hstore",
	"citext":                "citext",
	"int4range":             "int4range",
	"int4multirange":        "int4multirange",
	"int8range":             "int8range",
	"int8multirange":        "int8multirange",
	"numrange":              "numrange",
	"nummultirange":         "nummultirange",
	"tsrange":               "tsrange",
	"tsmultirange":          "tsmultirange",
	"tstzrange":             "tstzrange",
	"tstzmultirange":        "tstzmultirange",
	"daterange":             "daterange",
	"datemultirange":        "datemultirange",
	"xmlpath":               "xmlpath",
	"line":                  "line",
	"box":                   "box",
	"circle":                "circle",
	"lseg":                  "lseg",
}

// mapCatalogType converts a PostgreSQL catalog type name to the SQLType
// string the scanner would produce for the same logical type.
//
// Parameters:
//   - udtName: the udt_name from information_schema.columns (internal name,
//     e.g. "int4", "varchar", "_int4" for arrays, or a custom enum name)
//   - dataType: the data_type from information_schema.columns (canonical name,
//     e.g. "integer", "character varying", "ARRAY")
//   - charMaxLen: the character_maximum_length from information_schema.columns
//     (non-zero for parameterized types like varchar(255))
//
// Returns the SQLType string. For custom/unknown types, returns the raw udtName.
func mapCatalogType(udtName, dataType string, charMaxLen int) string {
	udtLower := strings.ToLower(udtName)

	// Handle arrays: udt_name starts with '_' (e.g. "_int4" for integer[])
	if strings.HasPrefix(udtLower, "_") {
		baseUDT := udtLower[1:]
		baseType := mapCatalogType(baseUDT, dataType, 0)
		return baseType + "[]"
	}

	// Handle parameterized types: append length to the canonical short form.
	if charMaxLen > 0 {
		if mapped, ok := pgCatalogToSQLType[udtLower]; ok {
			return fmt.Sprintf("%s(%d)", mapped, charMaxLen)
		}
	}

	// Direct lookup on udt_name (short internal form).
	if mapped, ok := pgCatalogToSQLType[udtLower]; ok {
		return mapped
	}

	// Fallback: try the canonical data_type name (e.g. "character varying").
	dataLower := strings.ToLower(dataType)
	if mapped, ok := pgCatalogToSQLType[dataLower]; ok {
		return mapped
	}

	// Unknown type — return the raw udt_name (custom enum, domain, etc.).
	return udtName
}

// mapCatalogDataType converts a PostgreSQL catalog type name to the
// schema.DataType enum value. This is used alongside mapCatalogType to
// also set the Column.Type field.
func mapCatalogDataType(udtName, dataType string) schema.DataType {
	udtLower := strings.ToLower(udtName)

	// Handle arrays — DataType is the same as the base element type
	if strings.HasPrefix(udtLower, "_") {
		return mapCatalogDataType(udtLower[1:], dataType)
	}

	// Handle serial pseudo-types
	switch udtLower {
	case "serial", "serial4":
		return schema.Serial
	case "bigserial", "serial8":
		return schema.BigSerial
	case "smallserial", "serial2":
		return schema.SmallSerial
	}

	// Use ParseDataType for the standard mapping
	sqlType := mapCatalogType(udtName, dataType, 0)
	dt, _ := schema.ParseDataType(sqlType)
	return dt
}
