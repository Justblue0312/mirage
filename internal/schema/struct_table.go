package schema

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type Rows interface {
	FieldDescriptions() []pgconn.FieldDescription
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}

func ConvertStructToTable(tableName string, typ reflect.Type) (*Table, error) {
	typ = IndirectType(typ)
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct type, got %s", typ.Kind())
	}

	td := &Table{
		StructName: ToStructName(tableName),
		StructType: typ,
		Name:       tableName,
		SearchPath: GetDefaultSearchPath(),
		Type:       TableTypeBase,
	}

	fields := lookupStructFields(typ, nil)
	ordinalPosition := 1

	for _, field := range fields {
		tag := field.Tag.Get(GetDefaultTag())
		if tag == "" || tag == "-" {
			continue
		}

		col := &Column{
			Table:           td,
			TableName:       tableName,
			OrdinalPosition: ordinalPosition,
			FieldIndex:      field.Index,
			FieldType:       field.Type,
			FieldName:       field.Name,
			Nullable:        true,
		}

		parseColumnTag(tag, col)

		// Auto-infer SQL type from Go field type when no explicit type= tag
		if col.Type == 0 && col.SQLType == "" {
			col.Type = inferTypeFromGo(field.Type)
		}

		if col.Name == "" {
			col.Name = GetToColumnName()(field)
		}

		td.Columns = append(td.Columns, col)
		ordinalPosition++
	}

	return td, nil
}

func ConvertRowsToStruct(td *Table, rows Rows, destPtr any) error {
	destVal := reflect.ValueOf(destPtr)
	if destVal.Kind() != reflect.Pointer || destVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("destPtr must be a pointer to struct, got %T", destPtr)
	}
	destVal = destVal.Elem()

	colMap := make(map[string]*Column, len(td.Columns))
	for _, col := range td.Columns {
		colMap[col.Name] = col
	}

	fieldDescs := rows.FieldDescriptions()
	dests := make([]any, len(fieldDescs))
	// For JSON columns we scan into a temporary []byte holder, then unmarshal into the struct field.
	jsonHolders := make([]*[]byte, len(fieldDescs))
	jsonCols := make([]*Column, len(fieldDescs))
	jsonFields := make([]reflect.Value, len(fieldDescs))
	for i, fd := range fieldDescs {
		col, ok := colMap[fd.Name]
		if ok && col.FieldIndex != nil {
			field := destVal.FieldByIndex(col.FieldIndex)
			if field.CanAddr() && NeedsJSONHandling(col) {
				holder := new([]byte)
				dests[i] = holder
				jsonHolders[i] = holder
				jsonCols[i] = col
				jsonFields[i] = field
				continue
			}
			if field.CanAddr() {
				dests[i] = field.Addr().Interface()
			} else {
				dests[i] = new(any)
			}
		} else {
			dests[i] = new(any)
		}
	}

	if err := rows.Scan(dests...); err != nil {
		return err
	}
	for i, holder := range jsonHolders {
		if holder == nil {
			continue
		}
		if holder != nil && *holder == nil {
			// NULL → keep zero value / nil pointer
			continue
		}
		if err := UnmarshalJSONValue(*holder, jsonFields[i]); err != nil {
			return err
		}
		_ = jsonCols[i]
	}

	return nil
}

func parseColumnTag(tag string, col *Column) {
	parts := splitTagParts(tag)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if idx := strings.IndexByte(part, '='); idx != -1 {
			key := strings.TrimSpace(part[:idx])
			value := strings.TrimSpace(part[idx+1:])
			handleTagKV(key, value, col)
			continue
		}

		handleTagFlag(part, col)
	}
}

func handleTagKV(key, value string, col *Column) {
	switch key {
	case "name":
		col.Name = value
	case "type":
		lower := strings.ToLower(value)
		switch lower {
		case "json":
			col.Type = Json
			col.SQLType = "json"
		case "jsonb":
			col.Type = JsonB
			col.SQLType = "jsonb"
		default:
			dt, arg := ParseDataType(value)
			col.Type = dt
			col.TypeArgument = arg
			// Preserve original spelling for DDL fallback when SQLType is used.
			if dt == Json || dt == JsonB {
				col.SQLType = lower
			}
		}
	case "default":
		col.Default = value
	case "check":
		col.CheckConstraint = value
	case "unique_index":
		col.UniqueIndex = value
	case "index":
		col.Index = ParseIndexType(value)
	case "collate":
		col.Collate = value
	case "using":
		col.TypeChangeUsing = value
	}
}

func handleTagFlag(flag string, col *Column) {
	switch flag {
	case "pk":
		col.PrimaryKey = true
	case "identity":
		col.Identity = true
	case "unique":
		col.Unique = true
	case "notnull":
		col.Nullable = false
	case "username":
		col.Username = true
	case "password":
		col.Password = true
	case "presenter":
		col.Presenter = true
	case "unscannable":
		col.Unscannable = true
	}
}

func splitTagParts(raw string) []string {
	var parts []string
	var current strings.Builder
	inParen := false

	for _, ch := range raw {
		switch ch {
		case '(':
			inParen = true
			current.WriteRune(ch)
		case ')':
			inParen = false
			current.WriteRune(ch)
		case ',':
			if inParen {
				current.WriteRune(ch)
			} else {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func inferTypeFromGo(ft reflect.Type) DataType {
	// Only dereference pointers, not slices/arrays/maps for the top-level kind check.
	orig := ft
	for ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	switch ft.Kind() {
	case reflect.Int:
		return Integer
	case reflect.Int8:
		return SmallInt
	case reflect.Int16:
		return SmallInt
	case reflect.Int32:
		return Integer
	case reflect.Int64:
		return BigInt
	case reflect.Uint:
		return Integer
	case reflect.Uint8:
		return SmallInt
	case reflect.Uint16:
		return SmallInt
	case reflect.Uint32:
		return Integer
	case reflect.Uint64:
		return BigInt
	case reflect.Float32:
		return Real
	case reflect.Float64:
		return Double
	case reflect.Bool:
		return Boolean
	case reflect.String:
		return Text
	case reflect.Slice, reflect.Array:
		if ft.Elem().Kind() == reflect.Uint8 {
			return Bytea
		}
		// Slice/array of structs, maps, or other composite -> JSONB by default.
		elem := ft.Elem()
		for elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Struct {
			if elem.PkgPath() == "time" && elem.Name() == "Time" {
				// []time.Time is not common; treat as text fallback
				return Text
			}
			return JsonB
		}
		if elem.Kind() == reflect.Map || elem.Kind() == reflect.Slice {
			return JsonB
		}
		return JsonB
	case reflect.Map:
		return JsonB
	case reflect.Struct:
		if ft.PkgPath() == "time" && ft.Name() == "Time" {
			return TimestampWithTimeZone
		}
		// Any other struct (Foo, map-like structs) → JSONB.
		return JsonB
	}
	// Fallback: if original was pointer to composite, check again.
	if orig.Kind() == reflect.Pointer {
		return inferTypeFromGo(ft)
	}
	return Text
}
