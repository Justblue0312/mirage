package scanner

import "strings"

// goTypeToSQLType maps a Go type name to its inferred PostgreSQL SQL type.
// Returns empty string for unknown types (caller should error or skip).
func goTypeToSQLType(goType string) string {
	goType = strings.TrimPrefix(goType, "*")
	if strings.HasSuffix(goType, "[]") {
		base := strings.TrimSuffix(goType, "[]")
		if base == "byte" || base == "uint8" {
			return "bytea"
		}
		return ""
	}
	if strings.HasPrefix(goType, "[]") {
		if goType == "[]byte" || goType == "[]uint8" {
			return "bytea"
		}
		return ""
	}
	switch goType {
	case "int":
		return "int"
	case "int8":
		return "smallint"
	case "int16":
		return "smallint"
	case "int32":
		return "int"
	case "int64":
		return "bigint"
	case "uint":
		return "int"
	case "uint8":
		return "smallint"
	case "uint16":
		return "smallint"
	case "uint32":
		return "int"
	case "uint64":
		return "bigint"
	case "float32":
		return "real"
	case "float64":
		return "double precision"
	case "bool", "boolean":
		return "boolean"
	case "string":
		return "text"
	case "time.Time":
		return "timestamptz"
	case "net.IP":
		return "inet"
	case "uuid.UUID":
		return "uuid"
	case "json.RawMessage", "json.Marshaler", "json.Unmarshaler":
		return "jsonb"
	}
	return ""
}
