package schema

import (
	"reflect"
	"testing"
)

type EmbedTimestamps struct {
	CreatedAt string `db:"name=created_at,type=timestamptz"`
	UpdatedAt string `db:"name=updated_at,type=timestamptz"`
}

type EmbedUser struct {
	ID int64 `db:"name=id,pk,type=bigserial"`
	EmbedTimestamps
	Name string `db:"name=name,type=text"`
}

func TestLookupStructFields_EmbeddedFlatten(t *testing.T) {
	typ := reflect.TypeOf(EmbedUser{})
	fields := lookupStructFields(typ, nil)

	names := make([]string, len(fields))
	for i, f := range fields {
		tag := f.Tag.Get(GetDefaultTag())
		col := &Column{}
		parseColumnTag(tag, col)
		if col.Name != "" {
			names[i] = col.Name
		} else {
			names[i] = f.Name
		}
	}

	expected := []string{"id", "created_at", "updated_at", "name"}
	if len(names) != len(expected) {
		t.Fatalf("got %d fields %v, want %d fields %v", len(names), names, len(expected), expected)
	}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("field[%d] = %q, want %q", i, names[i], want)
		}
	}
}

func TestConvertStructToTable_AutoInferType(t *testing.T) {
	type AutoUser struct {
		ID     int64   `db:"pk"`
		Name   string  `db:"name=name"`
		Score  float64 `db:"name=score"`
		Active bool    `db:"name=active"`
		Avatar []byte  `db:"name=avatar"`
	}
	td, err := ConvertStructToTable("auto_users", reflect.TypeOf(AutoUser{}))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]string{
		"id":     "bigint",
		"name":   "text",
		"score":  "double precision",
		"active": "boolean",
		"avatar": "bytea",
	}
	for _, col := range td.Columns {
		want, ok := tests[col.Name]
		if !ok {
			continue
		}
		got := col.Type.String()
		if got != want {
			t.Errorf("column %q type = %q, want %q", col.Name, got, want)
		}
	}
}
