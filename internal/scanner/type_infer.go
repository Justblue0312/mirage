package scanner

import "strings"

// goTypeToSQLType maps a Go type name to its inferred PostgreSQL SQL type.
// Returns empty string for unknown types (caller should error or skip).
func goTypeToSQLType(goType string) string {
	orig := goType
	// Strip leading pointers.
	for strings.HasPrefix(goType, "*") {
		goType = strings.TrimPrefix(goType, "*")
	}
	if strings.HasSuffix(goType, "[]") {
		base := strings.TrimSuffix(goType, "[]")
		if base == "byte" || base == "uint8" {
			return "bytea"
		}
		// Slice/array of structs/maps → jsonb (e.g. []Foo, []*Foo)
		// Primitive slices (e.g. []string, []int) also map to jsonb as flexible storage.
		// Callers that need text[] should declare type explicitly.
		return "jsonb"
	}
	if strings.HasPrefix(goType, "[]") {
		if goType == "[]byte" || goType == "[]uint8" {
			return "bytea"
		}
		return "jsonb"
	}
	if strings.HasPrefix(goType, "map[") {
		return "jsonb"
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
	// Fallback: custom struct / type alias (e.g. Foo, MyStruct, auth.Foo) → jsonb
	// This allows `Foo Foo `db:"name=foo,type=jsonb"` and also implicit
	// `Payload Foo `db:"name=payload"`` to be stored as jsonb.
	// Enums are resolved before this call (via enumByGoName), so they never reach here.
	// We detect custom types by leading uppercase or qualified name.
	if goType != "" {
		// Qualified name like "mypkg.Foo"
		if strings.Contains(goType, ".") {
			base := goType
			if idx := strings.LastIndex(base, "."); idx != -1 {
				base = base[idx+1:]
			}
			if base != "" && base[0] >= 'A' && base[0] <= 'Z' {
				return "jsonb"
			}
		} else if goType[0] >= 'A' && goType[0] <= 'Z' {
			// Preserve original had slice/map prefix handled above; this is a bare struct.
			_ = orig
			return "jsonb"
		}
	}
	return ""
}
