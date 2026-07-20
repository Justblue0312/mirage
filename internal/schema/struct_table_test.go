package schema

import (
	"reflect"
	"testing"
)

type TestUser struct {
	ID    int64  `db:"pk,type:bigserial"`
	Name  string `db:"type:varchar(255),notnull"`
	Email string `db:"type:varchar(255),unique"`
}

func TestConvertStructToTable(t *testing.T) {
	table, err := ConvertStructToTable("test_users", reflect.TypeOf(TestUser{}))
	if err != nil {
		t.Fatal(err)
	}
	if table.Name != "test_users" {
		t.Errorf("expected 'test_users', got %q", table.Name)
	}
	if len(table.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(table.Columns))
	}
}

func TestBuildInsertQuery(t *testing.T) {
	table, err := ConvertStructToTable("test_users", reflect.TypeOf(TestUser{}))
	if err != nil {
		t.Fatal(err)
	}
	user := &TestUser{Name: "Alice", Email: "a@b.com"}
	query, args, err := BuildInsertQuery(table, reflect.ValueOf(user).Elem(), nil, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if query == "" {
		t.Error("empty query")
	}
	if len(args) < 2 {
		t.Errorf("expected >=2 args, got %d", len(args))
	}
}

func TestBuildUpdateQuery(t *testing.T) {
	table, err := ConvertStructToTable("test_users", reflect.TypeOf(TestUser{}))
	if err != nil {
		t.Fatal(err)
	}
	pk, ok := table.FindPrimaryKey()
	if !ok {
		t.Fatal("no primary key")
	}
	user := &TestUser{ID: 1, Name: "Bob"}
	query, args, err := BuildUpdateQuery(user, []string{"name"}, false, pk)
	if err != nil {
		t.Fatal(err)
	}
	if query == "" {
		t.Error("empty query")
	}
	if len(args) < 1 {
		t.Errorf("expected >=1 args, got %d", len(args))
	}
}

func TestBuildDeleteQuery(t *testing.T) {
	table, err := ConvertStructToTable("test_users", reflect.TypeOf(TestUser{}))
	if err != nil {
		t.Fatal(err)
	}
	u1 := &TestUser{ID: 1}
	u2 := &TestUser{ID: 2}
	query, ids, err := BuildDeleteQuery(table, []any{u1, u2})
	if err != nil {
		t.Fatal(err)
	}
	if query == "" {
		t.Error("empty query")
	}
	// The query uses = ANY($1), so exactly one bind parameter (the typed
	// slice) must be produced regardless of how many rows are deleted.
	if len(ids) != 1 {
		t.Fatalf("expected 1 bind parameter (the id array), got %d", len(ids))
	}

	// The single parameter must be a concretely-typed slice ([]T), not a
	// []any, so pgx can determine the array element OID for custom PK types.
	rv := reflect.ValueOf(ids[0])
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected the id parameter to be a slice, got %T", ids[0])
	}
	if rv.Type().Elem().Kind() == reflect.Interface {
		t.Errorf("id parameter must be a concretely-typed slice, got []%s", rv.Type().Elem())
	}
	if rv.Len() != 2 {
		t.Errorf("expected 2 ids in the typed slice, got %d", rv.Len())
	}
	pkType := reflect.TypeOf(TestUser{}.ID)
	if rv.Type().Elem() != pkType {
		t.Errorf("typed slice element = %s, want %s (the PK Go type)", rv.Type().Elem(), pkType)
	}
}

func TestBuildExistsQuery(t *testing.T) {
	table, err := ConvertStructToTable("test_users", reflect.TypeOf(TestUser{}))
	if err != nil {
		t.Fatal(err)
	}
	user := &TestUser{ID: 1}
	query, args, err := BuildExistsQuery(table, reflect.ValueOf(user).Elem())
	if err != nil {
		t.Fatal(err)
	}
	if query == "" {
		t.Error("empty query")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestParseColumnTag(t *testing.T) {
	col := &Column{}
	parseColumnTag("pk,type=bigserial", col)
	if !col.PrimaryKey {
		t.Error("expected primary key")
	}
	if col.Type != BigSerial {
		t.Errorf("expected BigSerial, got %v", col.Type)
	}
}

func TestParseColumnTag_NotNull(t *testing.T) {
	col := &Column{Nullable: true}
	parseColumnTag("type=text,notnull", col)
	if col.Nullable {
		t.Error("expected notnull to set Nullable=false")
	}
}

func TestParseColumnTag_Default(t *testing.T) {
	col := &Column{}
	parseColumnTag("type=text,default='hello'", col)
	if col.Default != "'hello'" {
		t.Errorf("expected 'hello', got %q", col.Default)
	}
}
