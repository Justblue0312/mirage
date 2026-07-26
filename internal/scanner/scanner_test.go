package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/justblue/mirage/internal/schema"
)

func fieldNames(fields []RawField) []string {
	var names []string
	for _, f := range fields {
		names = append(names, f.GoName)
	}
	return names
}

func TestGetStructTag(t *testing.T) {
	tests := []struct {
		name string
		src  string
		tag  string
		want string
	}{
		{
			name: "standard db tag",
			src:  "type Foo struct { _ struct{} `db:\"name=users,comment=Test\"` }",
			tag:  "db",
			want: "name=users,comment=Test",
		},
		{
			name: "db tag with type parens",
			src:  "type Foo struct { _ struct{} `db:\"name=users,type=varchar(100)\"` }",
			tag:  "db",
			want: "name=users,type=varchar(100)",
		},
		{
			name: "json tag",
			src:  "type Foo struct { Name string `json:\"name\"` }",
			tag:  "json",
			want: "name",
		},
		{
			name: "multiple tags",
			src:  "type Foo struct { Name string `db:\"name=col\" json:\"name\"` }",
			tag:  "json",
			want: "name",
		},
		{
			name: "missing tag",
			src:  "type Foo struct { Name string `db:\"name=col\"` }",
			tag:  "json",
			want: "",
		},
		{
			name: "no tag",
			src:  "type Foo struct { Name string }",
			tag:  "db",
			want: "",
		},
		{
			name: "complex FK tag",
			src:  "type Foo struct { UserID int64 `db:\"name=user_id,type=bigint,ref=users.id ON DELETE CASCADE\"` }",
			tag:  "db",
			want: "name=user_id,type=bigint,ref=users.id ON DELETE CASCADE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, "test.go", "package test\n"+tt.src, 0)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			for _, decl := range node.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, f := range st.Fields.List {
						got := getStructTag(f, tt.tag)
						if got != tt.want {
							t.Errorf("getStructTag() = %q, want %q", got, tt.want)
						}
					}
				}
			}
		})
	}
}

func TestParseFile_SingleStruct(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Table{
	StructName: "User",
	Name:       "users",
	Description:    "Users table",
})
type User struct {
	ID    int64  ` + "`" + `db:"pk,identity,type=bigserial"` + "`" + `
	Name  string ` + "`" + `db:"name=name,type=varchar(255),notnull"` + "`" + `
	Email string ` + "`" + `db:"name=email,type=varchar(255),notnull,unique"` + "`" + `
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.GoName != "User" {
		t.Errorf("GoName = %q, want User", d.GoName)
	}

	if len(d.Fields) != 3 {
		t.Fatalf("got %d fields, want 3 (ID, Name, Email); fields: %v", len(d.Fields), fieldNames(d.Fields))
	}

	if d.Fields[0].GoName != "ID" {
		t.Errorf("field 0 GoName = %q, want ID", d.Fields[0].GoName)
	}
	if d.Fields[0].Attrs.String("type", "") != "bigserial" {
		t.Errorf("field 0 type = %q, want bigserial", d.Fields[0].Attrs.String("type", ""))
	}
	if !d.Fields[0].Attrs.Has("pk") {
		t.Error("field 0 should have pk")
	}
	if !d.Fields[0].Attrs.Has("generated") {
		t.Error("field 0 should have generated (identity)")
	}

	if d.Fields[1].GoName != "Name" {
		t.Errorf("field 1 GoName = %q, want Name", d.Fields[1].GoName)
	}
	if d.Fields[1].Attrs.String("type", "") != "varchar(255)" {
		t.Errorf("field 1 type = %q, want varchar(255)", d.Fields[1].Attrs.String("type", ""))
	}
	if !d.Fields[1].Attrs.Has("notnull") {
		t.Error("field 1 should have notnull")
	}

	if d.Fields[2].GoName != "Email" {
		t.Errorf("field 2 GoName = %q, want Email", d.Fields[2].GoName)
	}
	if !d.Fields[2].Attrs.Has("unique") {
		t.Error("field 2 should have unique")
	}
}

func TestParseFile_Enum(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

type Status string
const (
	StatusActive  Status = "active"
	StatusDeleted Status = "deleted"
)
var _ = mirage.Register(mirage.Table{
	StructName: "Post",
	Name:       "posts",
})
type Post struct {
	ID     int64  ` + "`" + `db:"pk,type=bigserial"` + "`" + `
	Status Status ` + "`" + `db:"name=status,type=status,notnull"` + "`" + `
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	if len(decls) != 2 {
		t.Fatalf("got %d decls, want 2 (enum + struct)", len(decls))
	}

	// Find the enum
	var enumDecl *RawDecl
	var structDecl *RawDecl
	for i := range decls {
		if decls[i].Kind == DeclEnum {
			enumDecl = &decls[i]
		} else {
			structDecl = &decls[i]
		}
	}

	if enumDecl == nil {
		t.Fatal("no enum decl found")
		return
	}
	if enumDecl.GoName != "Status" {
		t.Errorf("enum GoName = %q, want Status", enumDecl.GoName)
	}
	if len(enumDecl.EnumValues) != 2 {
		t.Errorf("enum has %d values, want 2", len(enumDecl.EnumValues))
	}
	if enumDecl.EnumValues[0] != "active" {
		t.Errorf("enum value 0 = %q, want active", enumDecl.EnumValues[0])
	}

	if structDecl == nil {
		t.Fatal("no struct decl found")
		return
	}
	if len(structDecl.Fields) != 2 {
		t.Errorf("struct has %d fields, want 2 (ID, Status)", len(structDecl.Fields))
	}
}

func TestParseFile_Embedded(t *testing.T) {
	src := `package test

import (
	"time"
	"github.com/justblue/mirage"
)

type Timestamps struct {
	CreatedAt time.Time ` + "`" + `db:"name=created_at,type=timestamptz,notnull"` + "`" + `
}
var _ = mirage.Register(mirage.Table{
	StructName: "User",
	Name:       "users",
})
type User struct {
	ID int64 ` + "`" + `db:"pk,type=bigserial"` + "`" + `
	Timestamps
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	var userDecl *RawDecl
	for i := range decls {
		if decls[i].GoName == "User" {
			userDecl = &decls[i]
		}
	}
	if userDecl == nil {
		t.Fatal("no User decl found")
	}

	if len(userDecl.EmbeddedTypes) != 1 || userDecl.EmbeddedTypes[0] != "Timestamps" {
		t.Errorf("embedded types = %v, want [Timestamps]", userDecl.EmbeddedTypes)
	}
}

func TestParseFile_PKArgs(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Table{
	StructName: "Foo",
	Name:       "foo",
})
type Foo struct {
	A int64 ` + "`" + `db:"pk(col1,col2)"` + "`" + `
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	f := decls[0].Fields[0]
	if !f.Attrs.Has("pk") {
		t.Error("field should have pk")
	}
}

func TestParseFile_CheckConstraint(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Table{
	StructName: "Foo",
	Name:       "foo",
})
type Foo struct {
	Status string ` + "`" + `db:"name=status,type=varchar(20),check=status IN ('active','deleted')"` + "`" + `
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	f := decls[0].Fields[0]
	check := f.Attrs.String("check", "")
	if !strings.Contains(check, "active") {
		t.Errorf("check = %q, should contain 'active'", check)
	}
}

func TestParseFile_EnumWithIota(t *testing.T) {
	src := `package test
type Status string
const (
	StatusActive  Status = "active"
	StatusPending Status = "pending"
	StatusDeleted Status = "deleted"
)
`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}
	if decls[0].Kind != DeclEnum {
		t.Errorf("kind = %v, want DeclEnum", decls[0].Kind)
	}
	if len(decls[0].EnumValues) != 3 {
		t.Errorf("enum has %d values, want 3", len(decls[0].EnumValues))
	}
}

func TestParseTag(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantFlags []string
		wantKV    map[string]string
	}{
		{
			name:      "flags only",
			raw:       "pk,notnull,unique",
			wantFlags: []string{"pk", "notnull", "unique"},
			wantKV:    map[string]string{},
		},
		{
			name:      "kv only",
			raw:       "name=users,type=varchar(100)",
			wantFlags: []string{},
			wantKV:    map[string]string{"name": "users", "type": "varchar(100)"},
		},
		{
			name:      "mixed",
			raw:       "pk,notnull,type=bigint,default=0",
			wantFlags: []string{"pk", "notnull"},
			wantKV:    map[string]string{"type": "bigint", "default": "0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, kv, _ := ParseTag(tt.raw)
			if len(flags) != len(tt.wantFlags) {
				t.Errorf("flags = %v, want %v", flags, tt.wantFlags)
			}
			for k, want := range tt.wantKV {
				if got := kv[k]; got != want {
					t.Errorf("kv[%s] = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestParseColumnTag(t *testing.T) {
	tag := "name=username,type=varchar(100),notnull,unique_index=idx_username,comment=Login handle"
	col := ParseColumnTag(tag)

	if col.Name != "username" {
		t.Errorf("Name = %q, want username", col.Name)
	}
	if col.SQLType != "varchar(100)" {
		t.Errorf("SQLType = %q, want varchar(100)", col.SQLType)
	}
	if !col.NotNull {
		t.Error("NotNull should be true")
	}
	if col.UniqueIndex != "idx_username" {
		t.Errorf("UniqueIndex = %q, want idx_username", col.UniqueIndex)
	}
	if col.Comment != "Login handle" {
		t.Errorf("Comment = %q, want 'Login handle'", col.Comment)
	}
}

func TestParseFile_RegisterFunction(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Function{
	Name: "update_timestamp",
	Language: "plpgsql",
	Body: "BEGIN NEW.updated_at := now(); END;",
	Arguments: []mirage.FunctionArgument{
		{Name: "p_id", Type: "bigint", Mode: "IN"},
	},
})`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.Kind != DeclFunction {
		t.Errorf("Kind = %v, want DeclFunction", d.Kind)
	}
	if d.Function == nil {
		t.Fatal("Function is nil")
	}
	if d.Function.Name != "update_timestamp" {
		t.Errorf("Function.Name = %q, want update_timestamp", d.Function.Name)
	}
	if d.Function.Language != "plpgsql" {
		t.Errorf("Function.Language = %q, want plpgsql", d.Function.Language)
	}
	if d.Function.Body != "BEGIN NEW.updated_at := now(); END;" {
		t.Errorf("Function.Body = %q", d.Function.Body)
	}
	if len(d.Function.Arguments) != 1 {
		t.Fatalf("Function.Arguments len = %d, want 1", len(d.Function.Arguments))
	}
	if d.Function.Arguments[0].Name != "p_id" {
		t.Errorf("Arguments[0].Name = %q, want p_id", d.Function.Arguments[0].Name)
	}
	if d.Function.Arguments[0].Type != "bigint" {
		t.Errorf("Arguments[0].Type = %q, want bigint", d.Function.Arguments[0].Type)
	}
	if d.Function.Arguments[0].Mode != "IN" {
		t.Errorf("Arguments[0].Mode = %q, want IN", d.Function.Arguments[0].Mode)
	}
}

func TestParseFile_RegisterView(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.View{
	Name:  "active_users",
	Query: "SELECT * FROM users WHERE active = true",
})`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.Kind != DeclView {
		t.Errorf("Kind = %v, want DeclView", d.Kind)
	}
	if d.View == nil {
		t.Fatal("View is nil")
	}
	if d.View.Name != "active_users" {
		t.Errorf("View.Name = %q, want active_users", d.View.Name)
	}
	if d.View.Query != "SELECT * FROM users WHERE active = true" {
		t.Errorf("View.Query = %q", d.View.Query)
	}
}

func TestParseFile_RegisterMatView(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.MaterializedView{
	Name:  "user_stats",
	Query: "SELECT count(*) FROM users",
})`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.Kind != DeclMatView {
		t.Errorf("Kind = %v, want DeclMatView", d.Kind)
	}
	if d.MaterializedView == nil {
		t.Fatal("MaterializedView is nil")
	}
	if d.MaterializedView.Name != "user_stats" {
		t.Errorf("MaterializedView.Name = %q, want user_stats", d.MaterializedView.Name)
	}
	if d.MaterializedView.Query != "SELECT count(*) FROM users" {
		t.Errorf("MaterializedView.Query = %q", d.MaterializedView.Query)
	}
}

func TestParseFile_RegisterTrigger(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Trigger{
	Name:     "trg_update",
	Table:    "users",
	Timing:   "BEFORE",
	Events:   []string{"UPDATE"},
	Function: "update_timestamp",
})`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.Kind != DeclTrigger {
		t.Errorf("Kind = %v, want DeclTrigger", d.Kind)
	}
	if d.Trigger == nil {
		t.Fatal("Trigger is nil")
	}
	if d.Trigger.Name != "trg_update" {
		t.Errorf("Trigger.Name = %q, want trg_update", d.Trigger.Name)
	}
	if d.Trigger.Table != "users" {
		t.Errorf("Trigger.Table = %q, want users", d.Trigger.Table)
	}
	if d.Trigger.Timing != "BEFORE" {
		t.Errorf("Trigger.Timing = %q, want BEFORE", d.Trigger.Timing)
	}
	if len(d.Trigger.Events) != 1 || d.Trigger.Events[0] != "UPDATE" {
		t.Errorf("Trigger.Events = %v, want [UPDATE]", d.Trigger.Events)
	}
	if d.Trigger.Function != "update_timestamp" {
		t.Errorf("Trigger.Function = %q, want update_timestamp", d.Trigger.Function)
	}
}

func TestParseFile_RegisterProcedure(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Procedure{
	Name:     "refresh_stats",
	Language: "plpgsql",
	Body:     "BEGIN REFRESH MATERIALIZED VIEW user_stats; END;",
})`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.Kind != DeclProcedure {
		t.Errorf("Kind = %v, want DeclProcedure", d.Kind)
	}
	if d.Procedure == nil {
		t.Fatal("Procedure is nil")
	}
	if d.Procedure.Name != "refresh_stats" {
		t.Errorf("Procedure.Name = %q, want refresh_stats", d.Procedure.Name)
	}
	if d.Procedure.Language != "plpgsql" {
		t.Errorf("Procedure.Language = %q, want plpgsql", d.Procedure.Language)
	}
	if d.Procedure.Body != "BEGIN REFRESH MATERIALIZED VIEW user_stats; END;" {
		t.Errorf("Procedure.Body = %q", d.Procedure.Body)
	}
}

func TestParseFile_RegisterGrant(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Grant{
	ObjectType: "table",
	ObjectName: "users",
	Privileges: []string{"SELECT", "INSERT"},
	Roles:      []string{"app_user"},
})`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.Kind != DeclGrant {
		t.Errorf("Kind = %v, want DeclGrant", d.Kind)
	}
	if d.Grant == nil {
		t.Fatal("Grant is nil")
	}
	if d.Grant.ObjectType != "table" {
		t.Errorf("Grant.ObjectType = %q, want table", d.Grant.ObjectType)
	}
	if d.Grant.ObjectName != "users" {
		t.Errorf("Grant.ObjectName = %q, want users", d.Grant.ObjectName)
	}
	if len(d.Grant.Privileges) != 2 {
		t.Errorf("Grant.Privileges len = %d, want 2", len(d.Grant.Privileges))
	}
	if len(d.Grant.Roles) != 1 || d.Grant.Roles[0] != "app_user" {
		t.Errorf("Grant.Roles = %v, want [app_user]", d.Grant.Roles)
	}
}

func TestParseFile_RegisterPolicy(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Policy{
	Name:    "user_isolation",
	Table:   "users",
	Command: "ALL",
	Using:   "auth.uid() = id",
})`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.Kind != DeclPolicy {
		t.Errorf("Kind = %v, want DeclPolicy", d.Kind)
	}
	if d.Policy == nil {
		t.Fatal("Policy is nil")
	}
	if d.Policy.Name != "user_isolation" {
		t.Errorf("Policy.Name = %q, want user_isolation", d.Policy.Name)
	}
	if d.Policy.Table != "users" {
		t.Errorf("Policy.Table = %q, want users", d.Policy.Table)
	}
	if d.Policy.Command != "ALL" {
		t.Errorf("Policy.Command = %q, want ALL", d.Policy.Command)
	}
	if d.Policy.Using != "auth.uid() = id" {
		t.Errorf("Policy.Using = %q, want auth.uid() = id", d.Policy.Using)
	}
}

func TestParseFile_RegisterInInit(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

func init() {
	mirage.Register(mirage.View{
		Name:  "v_users",
		Query: "SELECT 1",
	})
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.Kind != DeclView {
		t.Errorf("Kind = %v, want DeclView", d.Kind)
	}
	if d.View == nil {
		t.Fatal("View is nil")
	}
	if d.View.Name != "v_users" {
		t.Errorf("View.Name = %q, want v_users", d.View.Name)
	}
	if d.View.Query != "SELECT 1" {
		t.Errorf("View.Query = %q, want SELECT 1", d.View.Query)
	}
}

func TestParseFile_RegisterExtension(t *testing.T) {
	src := `package test

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Extension{
	Name:        "uuid-ossp",
	Schema:      "public",
	IfNotExists: true,
})`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	d := decls[0]
	if d.Kind != DeclExtension {
		t.Errorf("Kind = %v, want DeclExtension", d.Kind)
	}
	if d.Extension == nil {
		t.Fatal("Extension is nil")
	}
	if d.Extension.Name != "uuid-ossp" {
		t.Errorf("Extension.Name = %q, want uuid-ossp", d.Extension.Name)
	}
	if d.Extension.Schema != "public" {
		t.Errorf("Extension.Schema = %q, want public", d.Extension.Schema)
	}
	if !d.Extension.IfNotExists {
		t.Error("Extension.IfNotExists = false, want true")
	}
}

// TestScan_Deterministic asserts that repeated Scan() runs over the same
// source directory produce byte-for-byte identical *schema.Package values.
// The scanner fans out over goroutines and previously appended Tables/Grants/
// Policies in goroutine-completion order, making output nondeterministic and
// breaking the reflect.DeepEqual / JSON fast-path in the diff engine.
func TestParseFile_Table_InInit(t *testing.T) {
	src := `package test

import mirage "github.com/justblue/mirage"

type User struct {
	ID   int64  ` + "`" + `db:"pk,type:bigserial"` + "`" + `
	Name string ` + "`" + `db:"name=name,type:text"` + "`" + `
}

func init() {
	mirage.Register(mirage.Table{
		StructName: "User",
		Name:       "app_users",
		Description:    "Application users",
	})
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatal(err)
	}

	var tableDecl *RawDecl
	for i := range decls {
		if decls[i].Kind == DeclTable && decls[i].GoName == "User" {
			tableDecl = &decls[i]
		}
	}
	if tableDecl == nil {
		t.Fatal("expected DeclTable for User")
	}
	if tableDecl.TableAttrs.String("name", "") != "app_users" {
		t.Errorf("expected table name %q, got %q", "app_users", tableDecl.TableAttrs.String("name", ""))
	}
	if tableDecl.TableAttrs.String("comment", "") != "Application users" {
		t.Errorf("expected comment %q, got %q", "Application users", tableDecl.TableAttrs.String("comment", ""))
	}
	if len(tableDecl.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(tableDecl.Fields))
	}
}

func TestParseFile_Table_AllAttributes(t *testing.T) {
	src := `package test

import mirage "github.com/justblue/mirage"

type Widget struct {
	ID int64 ` + "`" + `db:"pk,type:bigserial"` + "`" + `
}

func init() {
	mirage.Register(mirage.Table{
		StructName: "Widget",
		Name:       "widgets",
		SearchPath: "inventory",
		Description:    "Product widgets",
		Options:    "UNLOGGED",
		Type:       "base table",
		Partitioned: &mirage.Partition{Strategy: "RANGE", Column: "id"},
		Ignore:     false,
	})
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatal(err)
	}

	var tableDecl *RawDecl
	for i := range decls {
		if decls[i].Kind == DeclTable && decls[i].GoName == "Widget" {
			tableDecl = &decls[i]
		}
	}
	if tableDecl == nil {
		t.Fatal("expected DeclTable for Widget")
	}

	tests := []struct {
		key, want string
	}{
		{"name", "widgets"},
		{"schema", "inventory"},
		{"comment", "Product widgets"},
		{"options", "UNLOGGED"},
		{"type", "base table"},
	}
	for _, tt := range tests {
		if got := tableDecl.TableAttrs.String(tt.key, ""); got != tt.want {
			t.Errorf("attr %q = %q, want %q", tt.key, got, tt.want)
		}
	}

	part := tableDecl.TableAttrs.Args("partitioned")
	if len(part) != 2 || part[0] != "RANGE" || part[1] != "id" {
		t.Errorf("partition = %v, want [RANGE id]", part)
	}

	if tableDecl.TableAttrs.Has("ignore") {
		t.Error("ignore should not be set")
	}
}

func TestParseFile_StructWithoutTable(t *testing.T) {
	src := `package test

type Orphan struct {
	ID int64 ` + "`" + `db:"pk,type:bigserial"` + "`" + `
	Name string ` + "`" + `db:"name=name,type:text"` + "`" + `
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(decls))
	}
	if decls[0].Kind != DeclEmbeddable {
		t.Errorf("expected DeclEmbeddable, got %v", decls[0].Kind)
	}
	if decls[0].GoName != "Orphan" {
		t.Errorf("GoName = %q, want Orphan", decls[0].GoName)
	}
}

func TestParseFile_Table_Ignore(t *testing.T) {
	src := `package test

import mirage "github.com/justblue/mirage"

type Secret struct {
	ID int64 ` + "`" + `db:"pk,type:bigserial"` + "`" + `
}

func init() {
	mirage.Register(mirage.Table{
		StructName: "Secret",
		Name:       "secrets",
		Ignore:     true,
	})
}`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatal(err)
	}

	var tableDecl *RawDecl
	for i := range decls {
		if decls[i].Kind == DeclTable && decls[i].GoName == "Secret" {
			tableDecl = &decls[i]
		}
	}
	if tableDecl == nil {
		t.Fatal("expected DeclTable for Secret")
	}
	if !tableDecl.TableAttrs.Has("ignore") {
		t.Error("expected ignore flag to be set")
	}
	if tableDecl.TableAttrs.String("name", "") != "secrets" {
		t.Errorf("name = %q, want secrets", tableDecl.TableAttrs.String("name", ""))
	}
}

func TestScan_Table_Pipeline(t *testing.T) {
	dir := t.TempDir()

	src := `package models

import mirage "github.com/justblue/mirage"

type User struct {
	ID    int64  ` + "`" + `db:"pk,type=bigserial"` + "`" + `
	Name  string ` + "`" + `db:"name=name,type=varchar(100),notnull"` + "`" + `
	Email string ` + "`" + `db:"name=email,type=varchar(255),notnull,unique"` + "`" + `
}

type InternalConfig struct {
	Key string ` + "`" + `db:"pk,type=text"` + "`" + `
}

func init() {
	mirage.Register(mirage.Table{
		StructName: "User",
		Name:       "app_users",
		Description:    "Application users table",
	})
	mirage.Register(mirage.Table{
		StructName: "InternalConfig",
		Name:       "internal_config",
		Ignore:     true,
	})
}`
	if err := os.WriteFile(filepath.Join(dir, "models.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Scanner{SourceDirs: []string{dir}}
	pkg, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(pkg.Tables) != 1 {
		t.Fatalf("expected 1 non-ignored table, got %d", len(pkg.Tables))
	}

	tbl := pkg.Tables[0]
	if tbl.Name != "app_users" {
		t.Errorf("table name = %q, want app_users", tbl.Name)
	}
	if tbl.StructName != "User" {
		t.Errorf("StructName = %q, want User", tbl.StructName)
	}
	if tbl.Description != "Application users table" {
		t.Errorf("Description = %q, want 'Application users table'", tbl.Description)
	}
	if len(tbl.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(tbl.Columns))
	}

	colNames := make(map[string]bool)
	for _, c := range tbl.Columns {
		colNames[c.Name] = true
	}
	for _, want := range []string{"id", "name", "email"} {
		if !colNames[want] {
			t.Errorf("missing column %q", want)
		}
	}
}

func TestScan_Deterministic(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	dir := t.TempDir()

	src := `package fixture

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Table{StructName: "Alpha", Name: "alpha"})
var _ = mirage.Register(mirage.Table{StructName: "Beta", Name: "beta"})
var _ = mirage.Register(mirage.Table{StructName: "Gamma", Name: "gamma"})
var _ = mirage.Register(mirage.Table{StructName: "Delta", Name: "delta"})

type Alpha struct {
	ID int64 ` + "`" + `db:"pk,type=bigserial"` + "`" + `
}

type Beta struct {
	ID int64 ` + "`" + `db:"pk,type=bigserial"` + "`" + `
}

type Gamma struct {
	ID int64 ` + "`" + `db:"pk,type=bigserial"` + "`" + `
}

type Delta struct {
	ID int64 ` + "`" + `db:"pk,type=bigserial"` + "`" + `
}
`

	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(src), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	var prev string
	for i := 0; i < 25; i++ {
		s := &Scanner{SourceDirs: []string{dir}}
		pkg, err := s.Scan()
		if err != nil {
			t.Fatalf("run %d: Scan: %v", i, err)
		}
		if len(pkg.Tables) != 4 {
			t.Fatalf("run %d: expected 4 tables, got %d", i, len(pkg.Tables))
		}
		names := make([]string, len(pkg.Tables))
		for j, tbl := range pkg.Tables {
			names[j] = tbl.SQLName()
		}
		cur := strings.Join(names, ",")
		want := "alpha,beta,delta,gamma"
		if cur != want {
			t.Fatalf("run %d: table order = %q, want %q", i, cur, want)
		}
		if i > 0 && !reflect.DeepEqual(prev, cur) {
			t.Fatalf("run %d: nondeterministic order %q vs %q", i, cur, prev)
		}
		prev = cur
	}
}

func TestScan_ColumnTypeChangeUsing(t *testing.T) {
	dir := t.TempDir()

	src := `package fixture

import "github.com/justblue/mirage"

var _ = mirage.Register(mirage.Table{StructName: "Widget", Name: "widgets"})

type Widget struct {
	ID    int64  ` + "`" + `db:"pk,type=bigserial"` + "`" + `
	Count string ` + "`" + `db:"name=count,type=integer,using=CAST(count AS integer)"` + "`" + `
}
`
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(src), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	s := &Scanner{SourceDirs: []string{dir}}
	pkg, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(pkg.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(pkg.Tables))
	}

	var count *schema.Column
	for _, c := range pkg.Tables[0].Columns {
		if c.Name == "count" {
			count = c
			break
		}
	}
	if count == nil {
		t.Fatal("count column not found")
	}
	if count.TypeChangeUsing != "CAST(count AS integer)" {
		t.Errorf("TypeChangeUsing = %q, want %q", count.TypeChangeUsing, "CAST(count AS integer)")
	}
}

func TestResolveColumn_AutoInferType(t *testing.T) {
	src := `package test
import mirage "github.com/justblue/mirage"
type User struct {
	ID    int64   ` + "`" + `db:"pk"` + "`" + `
	Name  string  ` + "`" + `db:"name=name"` + "`" + `
	Email string  ` + "`" + `db:"name=email,notnull"` + "`" + `
	Age   int32   ` + "`" + `db:"name=age"` + "`" + `
	Score float64 ` + "`" + `db:"name=score"` + "`" + `
	Active bool   ` + "`" + `db:"name=active"` + "`" + `
	Avatar []byte ` + "`" + `db:"name=avatar"` + "`" + `
}
var _ = mirage.Register(mirage.Table{StructName: "User", Name: "users"})
`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}
	for _, f := range decls[0].Fields {
		if f.GoName != "" && f.Attrs.String("type", "") == "" {
			inferred := goTypeToSQLType(f.GoType)
			if inferred == "" {
				t.Errorf("field %q: GoType %q not inferred and no type= tag", f.GoName, f.GoType)
			}
		}
	}
}

func TestScan_CompositeConstraintsViaRegister(t *testing.T) {
	src := `package test
import mirage "github.com/justblue/mirage"
type Order struct {
	ID        int64  ` + "`" + `db:"pk,type=bigserial"` + "`" + `
	UserID    int64  ` + "`" + `db:"name=user_id,type=bigint"` + "`" + `
	ProductID int64  ` + "`" + `db:"name=product_id,type=bigint"` + "`" + `
	Status    string ` + "`" + `db:"name=status,type=varchar(50)"` + "`" + `
}
var _ = mirage.Register(mirage.Table{
	StructName: "Order",
	Name:       "orders",
	Uniques:    []mirage.UniqueConstraint{{Name: "uq_user_product", Columns: []string{"user_id", "product_id"}}},
	Indexes:    []mirage.Index{{Name: "idx_status", Columns: []string{"status"}}},
})
`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}
	uqArgs := decls[0].TableAttrs.AllArgs("uq")
	if len(uqArgs) != 1 || len(uqArgs[0]) != 2 || uqArgs[0][0] != "user_id" {
		t.Errorf("uq args = %v, want [[user_id product_id]]", uqArgs)
	}
	idxArgs := decls[0].TableAttrs.AllArgs("index")
	if len(idxArgs) != 1 || len(idxArgs[0]) != 1 || idxArgs[0][0] != "status" {
		t.Errorf("index args = %v, want [[status]]", idxArgs)
	}
}

func TestExtractRegisteredObject_Enum(t *testing.T) {
	src := `package test
import mirage "github.com/justblue/mirage"
var _ = mirage.Register(mirage.Enum{
	StructName: "UserRole",
	Name:       "user_role",
	Values:     []string{"admin", "member", "viewer"},
	Description: "User role enumeration",
})
`
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	decls, err := ParseFile(fset, node, "test.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}
	if decls[0].Kind != DeclEnum {
		t.Errorf("kind = %v, want DeclEnum", decls[0].Kind)
	}
	if len(decls[0].EnumValues) != 3 {
		t.Errorf("values = %v, want 3 values", decls[0].EnumValues)
	}
	if decls[0].Enum == nil {
		t.Fatal("Enum field is nil")
	}
	if decls[0].Enum.Name != "user_role" {
		t.Errorf("name = %q, want user_role", decls[0].Enum.Name)
	}
	if decls[0].Enum.StructName != "UserRole" {
		t.Errorf("struct name = %q, want UserRole", decls[0].Enum.StructName)
	}
}
