package schema

import (
	"reflect"
	"strings"
)

var scannerInterface = reflect.TypeOf((*interface{ Scan(interface{}) error })(nil)).Elem()

func implementsScanner(typ reflect.Type) bool {
	return typ.Implements(scannerInterface) || reflect.PointerTo(typ).Implements(scannerInterface)
}

// IndirectType dereferences pointer, array, chan, map, and slice types.
func IndirectType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Array ||
		typ.Kind() == reflect.Chan || typ.Kind() == reflect.Map ||
		typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	return typ
}

// IndirectValue dereferences a pointer or interface value.
func IndirectValue(v interface{}) reflect.Value {
	rv, ok := v.(reflect.Value)
	if !ok {
		rv = reflect.ValueOf(v)
	}
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		rv = rv.Elem()
	}
	return rv
}

func lookupStructFields(typ reflect.Type, parentIndex []int) []reflect.StructField {
	typ = IndirectType(typ)
	if typ.Kind() != reflect.Struct {
		return nil
	}

	var result []reflect.StructField
	num := typ.NumField()
	for i := 0; i < num; i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldIndex := make([]int, len(parentIndex)+1)
		copy(fieldIndex, parentIndex)
		fieldIndex[len(parentIndex)] = i
		field.Index = fieldIndex

		tag := field.Tag.Get(GetDefaultTag())
		ft := IndirectType(field.Type)
		origKind := field.Type.Kind()
		isSliceOrMap := origKind == reflect.Slice || origKind == reflect.Array || origKind == reflect.Map

		// Struct, slice-of-struct, map-of-struct that are explicitly tagged
		// as json/jsonb must stay as a single column. Otherwise embedded
		// structs without a db tag are flattened.
		if ft.Kind() == reflect.Struct && !implementsScanner(ft) && ft.PkgPath() != "time" {
			if tag == "" || tag == "-" {
				// Embedded struct without own tag → flatten its fields.
				subFields := lookupStructFields(ft, field.Index)
				result = append(result, subFields...)
				continue
			}
			if isSpecialJSONStructure(field) {
				// Explicit json/jsonb → keep as column (Foo, *Foo, []Foo, map, etc).
				result = append(result, field)
				continue
			}
			// Struct with db tag but not json → still flatten (legacy embedded)
			// if it is used as an embedded helper (e.g., Timestamps). However
			// if it has its own column tags, flatten.
			subFields := lookupStructFields(ft, field.Index)
			// If the struct contributed no sub-fields (all fields lacked db tags)
			// fall back to keeping it as a column to avoid silent loss.
			if len(subFields) == 0 {
				result = append(result, field)
				continue
			}
			result = append(result, subFields...)
			continue
		}

		if isSliceOrMap && isJSONTag(tag) {
			result = append(result, field)
			continue
		}

		if tag == "" || tag == "-" {
			continue
		}
		result = append(result, field)
	}

	// De-duplicate by Go field name: later fields (outer struct) win over
	// earlier fields (embedded struct). Keep only the last occurrence of each name.
	lastIdx := make(map[string]int) // field name → last index in result
	for i, f := range result {
		lastIdx[f.Name] = i
	}
	deduped := make([]reflect.StructField, 0, len(lastIdx))
	for i, f := range result {
		if lastIdx[f.Name] == i {
			deduped = append(deduped, f)
		}
	}

	return deduped
}

func isSpecialJSONStructure(field reflect.StructField) bool {
	tag := field.Tag.Get(GetDefaultTag())
	// Explicit json/jsonb columns are stored as a single column, not flattened.
	// We check for both spellings; jsonb contains "json" so the first check
	// covers both, but be explicit for readability.
	if strings.Contains(tag, "type=jsonb") || strings.Contains(tag, "type=json") {
		return true
	}
	return false
}

func isJSONTag(tag string) bool {
	return strings.Contains(tag, "type=jsonb") || strings.Contains(tag, "type=json")
}
