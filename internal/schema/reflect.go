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

func lookupFields(typ reflect.Type, parentIndex []int) []reflect.StructField {
	var fields []reflect.StructField
	num := typ.NumField()
	for i := 0; i < num; i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get(GetDefaultTag())
		if tag == "" || tag == "-" {
			continue
		}
		fieldIndex := make([]int, len(parentIndex)+1)
		copy(fieldIndex, parentIndex)
		fieldIndex[len(parentIndex)] = i
		field.Index = fieldIndex
		fields = append(fields, field)
	}
	return fields
}

func lookupStructFields(typ reflect.Type, parentIndex []int) []reflect.StructField {
	typ = IndirectType(typ)
	if typ.Kind() != reflect.Struct {
		return nil
	}
	fields := lookupFields(typ, parentIndex)
	var result []reflect.StructField
	for _, f := range fields {
		ft := IndirectType(f.Type)
		if ft.Kind() == reflect.Struct && !implementsScanner(ft) && !isSpecialJSONStructure(f) {
			subFields := lookupStructFields(ft, f.Index)
			result = append(result, subFields...)
		} else {
			result = append(result, f)
		}
	}
	return result
}

func isSpecialJSONStructure(field reflect.StructField) bool {
	tag := field.Tag.Get(GetDefaultTag())
	return strings.Contains(tag, "type=json")
}
