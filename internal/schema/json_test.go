package schema

import (
	"reflect"
	"testing"
)

type FooJSON struct {
	A string `json:"a"`
	B int    `json:"b"`
}

type BarJSON struct {
	ID    int64          `db:"pk,identity,type=bigserial"`
	Foo   FooJSON        `db:"name=foo,type=jsonb,notnull,default='{}'"`
	Ptr   *FooJSON       `db:"name=ptr,type=jsonb,null"`
	Items []FooJSON      `db:"name=items,type=jsonb,null"`
	Extra *[]FooJSON     `db:"name=extra,type=jsonb,null"`
	M     map[string]any `db:"name=m,type=jsonb,null"`
	Raw   string         `db:"name=raw,type=text"`
}

func TestJSONColumns_NotFlattened(t *testing.T) {
	td, err := ConvertStructToTable("bars", reflect.TypeOf(BarJSON{}))
	if err != nil {
		t.Fatalf("ConvertStructToTable: %v", err)
	}
	if len(td.Columns) != 7 {
		t.Fatalf("expected 7 columns, got %d: %v", len(td.Columns), func() []string {
			var n []string
			for _, c := range td.Columns { n = append(n, c.Name) }
			return n
		}())
	}
	// Ensure foo, ptr, items etc present and flagged as json
	for _, name := range []string{"foo", "ptr", "items", "extra", "m"} {
		col := td.GetColumnByName(name)
		if col == nil {
			t.Fatalf("missing column %s", name)
		}
		if !IsJSONColumn(col) {
			t.Errorf("column %s should be JSON", name)
		}
		if !NeedsJSONHandling(col) && name != "m" {
			// m is map, needs handling; others also
			t.Errorf("column %s should need JSON handling", name)
		}
	}
}

func TestJSONMarshalExtract(t *testing.T) {
	td, _ := ConvertStructToTable("bars", reflect.TypeOf(BarJSON{}))
	val := reflect.ValueOf(BarJSON{
		ID: 1, Foo: FooJSON{A: "hello", B: 42},
		Items: []FooJSON{{A: "x", B: 1}},
		M: map[string]any{"k": float64(1)},
	})
	_, args, err := BuildInsertQuery(td, val, nil, "", false)
	if err != nil {
		t.Fatalf("BuildInsertQuery: %v", err)
	}
	// args should contain marshaled foo JSON as []byte; find it by scanning args for that payload
	foundFoo := false
	for _, a := range args {
		if b, ok := a.([]byte); ok && string(b) == `{"a":"hello","b":42}` {
			foundFoo = true
			break
		}
	}
	if !foundFoo {
		t.Errorf("foo json []byte not found in args %v", args)
	}
	// Also check items json
	foundItems := false
	for _, a := range args {
		if b, ok := a.([]byte); ok && string(b) == `[{"a":"x","b":1}]` {
			foundItems = true
			break
		}
	}
	if !foundItems {
		t.Errorf("items json not found in args %v", args)
	}
}

func TestPartitionColumnsList(t *testing.T) {
	p := &Partition{Strategy: "RANGE", Columns: []string{"created_at", "id"}}
	if len(p.ColumnsList()) != 2 || p.ColumnsList()[0] != "created_at" {
		t.Fatalf("ColumnsList = %v", p.ColumnsList())
	}
	p2 := &Partition{Strategy: "RANGE", Column: "created_at"}
	if len(p2.ColumnsList()) != 1 || p2.ColumnsList()[0] != "created_at" {
		t.Fatalf("fallback Column = %v", p2.ColumnsList())
	}
	if (&Partition{Strategy: "RANGE", Column: "a", Columns: []string{"b"}}).ColumnsList()[0] != "b" {
		t.Error("Columns should take precedence over Column")
	}
}
