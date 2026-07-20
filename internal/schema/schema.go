package schema

import (
	"reflect"
	"sync"
)

// DefaultTag is the default struct tag name used for database field mapping.
var DefaultTag = "db"

// DefaultSearchPath is the default PostgreSQL search path.
var DefaultSearchPath = "public"

// ToColumnName converts a struct field name to a column name (snake_case).
var ToColumnName = func(field reflect.StructField) string { return SnakeCase(field.Name) }

var schemaConfigMu sync.RWMutex

// SetDefaultTag updates the default struct tag under a lock so concurrent
// readers (e.g. table_cache) cannot race with the write.
func SetDefaultTag(tag string) {
	schemaConfigMu.Lock()
	defer schemaConfigMu.Unlock()
	DefaultTag = tag
}

// SetDefaultSearchPath updates the default search path under a lock.
func SetDefaultSearchPath(searchPath string) {
	schemaConfigMu.Lock()
	defer schemaConfigMu.Unlock()
	DefaultSearchPath = searchPath
}

// SetToColumnName updates the column name mapper under a lock.
func SetToColumnName(fn func(field reflect.StructField) string) {
	schemaConfigMu.Lock()
	defer schemaConfigMu.Unlock()
	ToColumnName = fn
}

// GetDefaultTag reads the default struct tag under a lock.
func GetDefaultTag() string {
	schemaConfigMu.RLock()
	defer schemaConfigMu.RUnlock()
	return DefaultTag
}

// GetDefaultSearchPath reads the default search path under a lock.
func GetDefaultSearchPath() string {
	schemaConfigMu.RLock()
	defer schemaConfigMu.RUnlock()
	return DefaultSearchPath
}

// GetToColumnName reads the column name mapper under a lock.
func GetToColumnName() func(field reflect.StructField) string {
	schemaConfigMu.RLock()
	defer schemaConfigMu.RUnlock()
	return ToColumnName
}
