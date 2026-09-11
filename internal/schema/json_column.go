package schema

import (
	"encoding/json"
	"reflect"
	"strings"
)

// IsJSONColumn reports whether the column stores JSON data.
func IsJSONColumn(c *Column) bool {
	if c == nil {
		return false
	}
	if c.SQLType != "" {
		lower := strings.ToLower(c.SQLType)
		return lower == "json" || lower == "jsonb" || strings.HasPrefix(lower, "json")
	}
	return c.Type == Json || c.Type == JsonB
}

// MarshalJSONValue marshals a Go value for a JSON column.
// For nil pointers/slices/maps it returns nil so the driver sends NULL.
func MarshalJSONValue(v reflect.Value) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map:
		if v.IsNil() {
			return nil, nil
		}
	}
	// json.RawMessage is already bytes; pass through.
	if v.Type().PkgPath() == "encoding/json" && v.Type().Name() == "RawMessage" {
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil, nil
		}
		return []byte(v.Bytes()), nil
	}
	b, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	return b, nil
}

// UnmarshalJSONValue unmarshals JSON data into the destination reflect.Value.
// dest must be settable and addressable. data may be []byte, string, or json.RawMessage bytes.
func UnmarshalJSONValue(data any, dest reflect.Value) error {
	if !dest.CanAddr() {
		return nil
	}
	var raw []byte
	switch v := data.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	case nil:
		// NULL → keep zero value (or nil pointer)
		return nil
	default:
		// Unknown type, fall back to marshal then unmarshal
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		raw = b
	}
	if len(raw) == 0 {
		return nil
	}
	// Handle pointer destination: allocate if nil.
	if dest.Kind() == reflect.Pointer {
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		dest = dest.Elem()
	}
	// For slices/maps, json.Unmarshal handles nil vs empty.
	target := dest.Addr().Interface()
	return json.Unmarshal(raw, target)
}

// NeedsJSONHandling reports whether the field type requires JSON marshal/unmarshal.
// Primitive string/[]byte that already maps to json/jsonb via type tag but Go type is string should be passed through.
func NeedsJSONHandling(c *Column) bool {
	if !IsJSONColumn(c) {
		return false
	}
	if c.FieldType == nil {
		return false
	}
	ft := c.FieldType
	for ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	switch ft.Kind() {
	case reflect.Struct:
		// time.Time is not JSON, but timestamp; already handled via IsTime.
		if ft.PkgPath() == "time" && ft.Name() == "Time" {
			return false
		}
		return true
	case reflect.Slice:
		if ft.Elem().Kind() == reflect.Uint8 {
			return false // bytea
		}
		return true
	case reflect.Map, reflect.Array:
		return true
	default:
		// string with jsonb type is stored as text JSON string; driver can handle string directly.
		// But if Go type is string and SQLType is jsonb, we still want to pass string through without marshal?
		// For consistency, only marshal non-string composites.
		return false
	}
}
