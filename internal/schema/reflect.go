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

		if ft.Kind() == reflect.Struct && !implementsScanner(ft) {
			if tag == "" || tag == "-" {
				subFields := lookupStructFields(ft, field.Index)
				result = append(result, subFields...)
				continue
			}
			if isSpecialJSONStructure(field) {
				result = append(result, field)
				continue
			}
			subFields := lookupStructFields(ft, field.Index)
			result = append(result, subFields...)
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
	return strings.Contains(tag, "type=json")
}
