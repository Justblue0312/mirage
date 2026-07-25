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

	Register(TableConfig{StructName: "testWidget", Name: "custom_widgets"})

	typ := reflect.TypeOf(testWidget{})
	name := resolveTableName(typ)
	if name != "custom_widgets" {
		t.Errorf("expected %q, got %q", "custom_widgets", name)
	}
}

func TestResolveTableName_RegistryOverTableName(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	Register(TableConfig{StructName: "namedWidget", Name: "registry_wins"})

	typ := reflect.TypeOf(namedWidget{})
	name := resolveTableName(typ)
	if name != "registry_wins" {
		t.Errorf("expected %q, got %q", "registry_wins", name)
	}
}

func TestResolveTableName_FallbackToTableName(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	typ := reflect.TypeOf(namedWidget{})
	name := resolveTableName(typ)
	if name != "legacy_widgets" {
		t.Errorf("expected %q, got %q", "legacy_widgets", name)
	}
}

func TestResolveTableName_FallbackToSnakeCase(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	type orderItem struct{}

	typ := reflect.TypeOf(orderItem{})
	name := resolveTableName(typ)
	if name != "order_items" {
		t.Errorf("expected %q, got %q", "order_items", name)
	}
}

func TestResolveTableName_EmptyRegistryName(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	type gizmo struct{}

	Register(TableConfig{StructName: "gizmo", Name: ""})

	typ := reflect.TypeOf(gizmo{})
	name := resolveTableName(typ)
	if name != "gizmos" {
		t.Errorf("expected %q, got %q", "gizmos", name)
	}
}
