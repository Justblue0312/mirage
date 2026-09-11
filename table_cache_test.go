package mirage

import (
	"reflect"
	"testing"
)

type namedWidget struct{}

func (namedWidget) TableName() string { return "legacy_widgets" }

func TestResolveTableName_Registry(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	type testWidget struct{}

	Register(Table{StructName: "testWidget", Name: "custom_widgets"})

	typ := reflect.TypeOf(testWidget{})
	name := resolveTableName(typ, nil)
	if name != "custom_widgets" {
		t.Errorf("expected %q, got %q", "custom_widgets", name)
	}
}

func TestResolveTableName_RegistryOverTableName(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	Register(Table{StructName: "namedWidget", Name: "registry_wins"})

	typ := reflect.TypeOf(namedWidget{})
	name := resolveTableName(typ, nil)
	if name != "registry_wins" {
		t.Errorf("expected %q, got %q", "registry_wins", name)
	}
}

func TestResolveTableName_FallbackToTableName(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	typ := reflect.TypeOf(namedWidget{})
	name := resolveTableName(typ, nil)
	if name != "legacy_widgets" {
		t.Errorf("expected %q, got %q", "legacy_widgets", name)
	}
}

func TestResolveTableName_FallbackToSnakeCase(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	type orderItem struct{}

	typ := reflect.TypeOf(orderItem{})
	name := resolveTableName(typ, nil)
	if name != "order_items" {
		t.Errorf("expected %q, got %q", "order_items", name)
	}
}

func TestResolveTableName_EmptyRegistryName(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	type gizmo struct{}

	Register(Table{StructName: "gizmo", Name: ""})

	typ := reflect.TypeOf(gizmo{})
	name := resolveTableName(typ, nil)
	if name != "gizmos" {
		t.Errorf("expected %q, got %q", "gizmos", name)
	}
}

func TestResolveTableName_CustomRegistry(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	type demoItem struct{}

	customReg := NewRegistry()
	customReg.Register(Table{StructName: "demoItem", Name: "custom_items"})

	typ := reflect.TypeOf(demoItem{})
	if name := resolveTableName(typ, nil); name != "demo_items" {
		t.Errorf("global registry: expected %q, got %q", "demo_items", name)
	}
	if name := resolveTableName(typ, customReg); name != "custom_items" {
		t.Errorf("custom registry: expected %q, got %q", "custom_items", name)
	}
}

func TestCachedTable_IsolatesBetweenRegistries(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	type demoCacheItem struct{}

	customReg := NewRegistry()
	customReg.Register(Table{StructName: "demoCacheItem", Name: "isolated_items"})

	typ := reflect.TypeOf(demoCacheItem{})
	tbl1, err := cachedTable(typ, nil)
	if err != nil {
		t.Fatalf("cachedTable (global): %v", err)
	}
	tbl2, err := cachedTable(typ, customReg)
	if err != nil {
		t.Fatalf("cachedTable (custom): %v", err)
	}
	if tbl1.SQLName() == tbl2.SQLName() {
		t.Errorf("expected different table names for global vs custom registry, both got %q", tbl1.SQLName())
	}
}

func TestNewRepository_WithRegistry(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	type demoRepoItem struct {
		ID int64 `db:"pk,type:bigserial"`
	}

	customReg := NewRegistry()
	customReg.Register(Table{StructName: "demoRepoItem", Name: "repo_custom_items"})

	db := &DB{}
	repo := NewRepository[demoRepoItem](db, WithRegistry(customReg))

	if repo.table.SQLName() != "repo_custom_items" {
		t.Errorf("expected table name %q, got %q", "repo_custom_items", repo.table.SQLName())
	}
}
