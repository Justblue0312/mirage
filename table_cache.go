package mirage

import (
	"reflect"
	"sync"

	schemapkg "github.com/justblue/mirage/internal/schema"
)

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

func cachedTable(typ reflect.Type) (*schemapkg.Table, error) {
	typ = schemapkg.IndirectType(typ)
	if cached, ok := tableCache.Load(typ); ok {
		return cached.(*schemapkg.Table), nil
	}

	tableName := resolveTableName(typ)

	td, err := schemapkg.ConvertStructToTable(tableName, typ)
	if err != nil {
		return nil, err
	}
	actual, _ := tableCache.LoadOrStore(typ, td)
	return actual.(*schemapkg.Table), nil
}

func resolveTableName(typ reflect.Type) string {
	if named, ok := reflect.New(typ).Interface().(TableNamer); ok {
		if name := named.TableName(); name != "" {
			return name
		}
	}
	return schemapkg.ToTableName(typ.Name())
}
