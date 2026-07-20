package schema

import (
	"math/big"
	"net"
	"reflect"
)

// Zeroer is the interface that wraps the basic IsZero method.
type Zeroer interface {
	IsZero() bool
}

func isZero(v interface{}) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		return rv.IsNil()
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.String:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.Struct:
		if z, ok := v.(Zeroer); ok {
			return z.IsZero()
		}
		if _, ok := v.(net.IP); ok {
			return len(v.(net.IP)) == 0
		}
		if b, ok := v.(*big.Int); ok {
			return b.Sign() == 0
		}
		if r, ok := v.(*big.Rat); ok {
			return r.Sign() == 0
		}
		if f, ok := v.(*big.Float); ok {
			return f.Sign() == 0
		}
		return rv.IsZero()
	case reflect.Invalid:
		return true
	}
	return false
}
