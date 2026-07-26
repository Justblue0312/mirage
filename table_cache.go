package mirage

import (
	"reflect"
	"sync"

	schemapkg "github.com/justblue/mirage/internal/schema"
)

// cacheKey pairs a type with its registry so different registries get
// independent cache entries.
type cacheKey struct {
	typ      reflect.Type
	registry *Registry
}

var tableCache sync.Map

// TableNamer lets a struct override its auto-derived table name, e.g.:
//
//	func (Person) TableName() string { return "people" }
//
// Without this, the table name is derived from the Go type name via
// pluralized snake_case (User -> "users", OrderItem -> "order_items").
type TableNamer interface {
	TableName() string
}

func cachedTable(typ reflect.Type, reg *Registry) (*schemapkg.Table, error) {
	typ = schemapkg.IndirectType(typ)
	key := cacheKey{typ: typ, registry: reg}
	if cached, ok := tableCache.Load(key); ok {
		return cached.(*schemapkg.Table), nil
	}

	tableName := resolveTableName(typ, reg)

	td, err := schemapkg.ConvertStructToTable(tableName, typ)
	if err != nil {
		return nil, err
	}
	actual, _ := tableCache.LoadOrStore(key, td)
	return actual.(*schemapkg.Table), nil
}

func resolveTableName(typ reflect.Type, reg *Registry) string {
	if reg == nil {
		reg = registry // fall back to global singleton
	}
	// 1. Check registry for a matching Table
	if tc := reg.TableByGoName(typ.Name()); tc != nil && tc.Name != "" {
		return tc.Name
	}
	// 2. Legacy fallback: TableName() interface
	if named, ok := reflect.New(typ).Interface().(TableNamer); ok {
		if name := named.TableName(); name != "" {
			return name
		}
	}
	// 3. Default: snake_case plural of Go type name
	return schemapkg.ToTableName(typ.Name())
}
