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
	for i, fd := range fieldDescs {
		col, ok := colMap[fd.Name]
		if ok && col.FieldIndex != nil {
			field := destVal.FieldByIndex(col.FieldIndex)
			if field.CanAddr() {
				dests[i] = field.Addr().Interface()
			} else {
				dests[i] = new(any)
			}
		} else {
			dests[i] = new(any)
		}
	}

	return rows.Scan(dests...)
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
		if value == "json" {
			col.Type = JsonB
		} else {
			dt, arg := ParseDataType(value)
			col.Type = dt
			col.TypeArgument = arg
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

func parseRefTag(value string) (tableName, columnName, onDelete, onUpdate string) {
	value = strings.TrimSuffix(value, ")")
	parenIdx := strings.IndexByte(value, '(')
	if parenIdx == -1 {
		return value, "", "", ""
	}

	tableName = value[:parenIdx]
	rest := strings.TrimSpace(value[parenIdx+1:])

	tokens := strings.Fields(rest)
	if len(tokens) == 0 {
		return
	}

	columnName = tokens[0]

	i := 1
	for i < len(tokens) {
		if strings.ToUpper(tokens[i]) == "ON" && i+2 < len(tokens) {
			direction := strings.ToUpper(tokens[i+1])
			if direction == "DELETE" || direction == "UPDATE" {
				j := i + 2
				action := strings.ToUpper(tokens[j])
				j++
				for j < len(tokens) {
					next := strings.ToUpper(tokens[j])
					if next == "ON" || next == "DEFERRABLE" || next == "NOT" {
						break
					}
					action += " " + next
					j++
				}
				if direction == "DELETE" {
					onDelete = action
				} else {
					onUpdate = action
				}
				i = j
				continue
			}
		}
		i++
	}

	return
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
