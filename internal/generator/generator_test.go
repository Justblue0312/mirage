package generator

import (
	"strings"
	"testing"

	"github.com/justblue/mirage/internal/dialect/postgres"
	"github.com/justblue/mirage/internal/diff"
	"github.com/justblue/mirage/internal/schema"
)

func helperNewPkg(tables []schema.Table, enums []schema.Enum) *schema.Package {
	return &schema.Package{Tables: tables, Enums: enums}
}

func TestGenerate_OneColumnAdded(t *testing.T) {
	d := postgres.New()
	oldPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
			},
		},
	}, nil)
	newPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
				{Name: "email", SQLType: "varchar(255)", Nullable: false},
			},
		},
	}, nil)

	events := []diff.DeltaEvent{
		{
			Kind:      diff.ColumnAdded,
			Table:     "users",
			Column:    "email",
			Severity:  diff.Info,
			NewColumn: &schema.Column{Name: "email", SQLType: "varchar(255)", Nullable: false},
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) != 1 {
		t.Fatalf("expected 1 up statement, got %d", len(mf.UpStatements))
	}
	if !strings.Contains(mf.UpStatements[0], "ALTER TABLE") || !strings.Contains(mf.UpStatements[0], "ADD COLUMN") {
		t.Errorf("up statement should contain ADD COLUMN, got: %s", mf.UpStatements[0])
	}

	if len(mf.DownStatements) != 1 {
		t.Fatalf("expected 1 down statement, got %d", len(mf.DownStatements))
	}
	if !strings.Contains(mf.DownStatements[0], "ALTER TABLE") || !strings.Contains(mf.DownStatements[0], "DROP COLUMN") {
		t.Errorf("down statement should contain DROP COLUMN, got: %s", mf.DownStatements[0])
	}
}

func TestGenerate_DestructiveEvents(t *testing.T) {
	tests := []struct {
		name            string
		events          []diff.DeltaEvent
		wantDestructive bool
	}{
		{
			name: "table_dropped is destructive",
			events: []diff.DeltaEvent{
				{
					Kind:     diff.TableDropped,
					Table:    "users",
					Severity: diff.Destructive,
					OldTable: &schema.Table{Name: "users"},
				},
			},
			wantDestructive: true,
		},
		{
			name: "column_dropped is destructive",
			events: []diff.DeltaEvent{
				{
					Kind:      diff.ColumnDropped,
					Table:     "users",
					Column:    "name",
					Severity:  diff.Destructive,
					OldColumn: &schema.Column{Name: "name", SQLType: "varchar(100)"},
				},
			},
			wantDestructive: true,
		},
		{
			name: "column_added is not destructive",
			events: []diff.DeltaEvent{
				{
					Kind:      diff.ColumnAdded,
					Table:     "users",
					Column:    "email",
					Severity:  diff.Info,
					NewColumn: &schema.Column{Name: "email", SQLType: "varchar(255)"},
				},
			},
			wantDestructive: false,
		},
		{
			name: "mixed events with destructive",
			events: []diff.DeltaEvent{
				{
					Kind:      diff.ColumnAdded,
					Table:     "users",
					Column:    "email",
					Severity:  diff.Info,
					NewColumn: &schema.Column{Name: "email", SQLType: "varchar(255)"},
				},
				{
					Kind:     diff.TableDropped,
					Table:    "old_table",
					Severity: diff.Destructive,
					OldTable: &schema.Table{Name: "old_table"},
				},
			},
			wantDestructive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := postgres.New()
			oldPkg := helperNewPkg(nil, nil)
			newPkg := helperNewPkg(nil, nil)

			g := New(d, tt.events, oldPkg, newPkg)
			mf, err := g.Generate()
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}

			if mf.HasDestructive != tt.wantDestructive {
				t.Errorf("HasDestructive = %v, want %v", mf.HasDestructive, tt.wantDestructive)
			}
		})
	}
}

func TestGenerate_ChecksumStable(t *testing.T) {
	d := postgres.New()
	oldPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
			},
		},
	}, nil)
	newPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
				{Name: "email", SQLType: "varchar(255)", Nullable: false},
			},
		},
	}, nil)

	events := []diff.DeltaEvent{
		{
			Kind:      diff.ColumnAdded,
			Table:     "users",
			Column:    "email",
			Severity:  diff.Info,
			NewColumn: &schema.Column{Name: "email", SQLType: "varchar(255)", Nullable: false},
		},
	}

	g1 := New(d, events, oldPkg, newPkg)
	mf1, err := g1.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	g2 := New(d, events, oldPkg, newPkg)
	mf2, err := g2.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if mf1.Checksum != mf2.Checksum {
		t.Errorf("checksums differ: %s != %s", mf1.Checksum, mf2.Checksum)
	}

	if !strings.HasPrefix(mf1.Checksum, "sha256:") {
		t.Errorf("checksum should start with sha256:, got: %s", mf1.Checksum)
	}
}

func TestGenerate_DownStatementsReverseOrder(t *testing.T) {
	d := postgres.New()
	oldPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
			},
		},
	}, nil)
	newPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
				{Name: "email", SQLType: "varchar(255)"},
				{Name: "name", SQLType: "varchar(100)"},
				{Name: "age", SQLType: "integer"},
			},
		},
	}, nil)

	events := []diff.DeltaEvent{
		{
			Kind:      diff.ColumnAdded,
			Table:     "users",
			Column:    "email",
			Severity:  diff.Info,
			NewColumn: &schema.Column{Name: "email", SQLType: "varchar(255)"},
		},
		{
			Kind:      diff.ColumnAdded,
			Table:     "users",
			Column:    "name",
			Severity:  diff.Info,
			NewColumn: &schema.Column{Name: "name", SQLType: "varchar(100)"},
		},
		{
			Kind:      diff.ColumnAdded,
			Table:     "users",
			Column:    "age",
			Severity:  diff.Info,
			NewColumn: &schema.Column{Name: "age", SQLType: "integer"},
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) != 3 {
		t.Fatalf("expected 3 up statements, got %d", len(mf.UpStatements))
	}
	if len(mf.DownStatements) != 3 {
		t.Fatalf("expected 3 down statements, got %d", len(mf.DownStatements))
	}

	for i := 0; i < len(mf.UpStatements); i++ {
		upIdx := i
		downIdx := len(mf.DownStatements) - 1 - i

		upStmt := mf.UpStatements[upIdx]
		downStmt := mf.DownStatements[downIdx]

		if strings.Contains(upStmt, "email") && !strings.Contains(downStmt, "email") {
			t.Errorf("up[%d] references email but down[%d] does not: up=%s, down=%s", upIdx, downIdx, upStmt, downStmt)
		}
		if strings.Contains(upStmt, "name") && !strings.Contains(downStmt, "name") {
			t.Errorf("up[%d] references name but down[%d] does not: up=%s, down=%s", upIdx, downIdx, upStmt, downStmt)
		}
		if strings.Contains(upStmt, "age") && !strings.Contains(downStmt, "age") {
			t.Errorf("up[%d] references age but down[%d] does not: up=%s, down=%s", upIdx, downIdx, upStmt, downStmt)
		}
	}

	if !strings.Contains(mf.DownStatements[0], "age") {
		t.Errorf("first down statement should drop 'age' (last added), got: %s", mf.DownStatements[0])
	}
	if !strings.Contains(mf.DownStatements[2], "email") {
		t.Errorf("last down statement should drop 'email' (first added), got: %s", mf.DownStatements[2])
	}
}

func TestGenerate_TableAdded(t *testing.T) {
	d := postgres.New()
	newTable := schema.Table{
		StructName: "User",
		Name:       "users",
		Columns: []*schema.Column{
			{Name: "id", SQLType: "integer", Nullable: false},
			{Name: "name", SQLType: "varchar(100)", Nullable: false},
		},
		PrimaryKey: &schema.PrimaryKey{
			Name:    "users_pkey",
			Columns: []string{"id"},
		},
	}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := helperNewPkg([]schema.Table{newTable}, nil)

	events := []diff.DeltaEvent{
		{
			Kind:     diff.TableAdded,
			Table:    "users",
			Severity: diff.Info,
			NewTable: &newTable,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements for CreateTable")
	}

	foundCreate := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "CREATE TABLE") {
			foundCreate = true
			break
		}
	}
	if !foundCreate {
		t.Errorf("up statements should contain CREATE TABLE, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements for DropTable")
	}
	if !strings.Contains(mf.DownStatements[0], "DROP TABLE") {
		t.Errorf("down statement should contain DROP TABLE, got: %s", mf.DownStatements[0])
	}
}

func TestGenerate_EnumAdded(t *testing.T) {
	d := postgres.New()
	enum := schema.Enum{
		StructName: "UserRole",
		Name:       "user_role",
		Values:     []string{"admin", "user", "guest"},
	}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := helperNewPkg(nil, []schema.Enum{enum})

	events := []diff.DeltaEvent{
		{
			Kind:     diff.EnumAdded,
			Enum:     "user_role",
			Severity: diff.Info,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements for CreateEnum")
	}

	foundCreateEnum := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "CREATE TYPE") && strings.Contains(stmt, "AS ENUM") {
			foundCreateEnum = true
			break
		}
	}
	if !foundCreateEnum {
		t.Errorf("up statements should contain CREATE TYPE ... AS ENUM, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements for DropEnum")
	}
	if !strings.Contains(mf.DownStatements[0], "DROP TYPE") {
		t.Errorf("down statement should contain DROP TYPE, got: %s", mf.DownStatements[0])
	}
}

func TestGenerate_AutocommitSegregation(t *testing.T) {
	d := postgres.New()
	enum := schema.Enum{
		StructName: "Status",
		Name:       "status",
		Values:     []string{"active", "inactive", "pending"},
	}
	oldPkg := helperNewPkg(nil, []schema.Enum{
		{
			StructName: "Status",
			Name:       "status",
			Values:     []string{"active", "inactive"},
		},
	})
	newPkg := helperNewPkg(nil, []schema.Enum{enum})

	events := []diff.DeltaEvent{
		{
			Kind:     diff.EnumValueAdded,
			Enum:     "status",
			NewValue: "pending",
			Severity: diff.Info,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.AutocommitStatements) > 0 {
		t.Errorf("expected no autocommit statements, got %v", mf.AutocommitStatements)
	}
	if len(mf.UpStatements) == 0 {
		t.Error("expected UpStatements for enum value addition")
	}
}

func TestGenerate_EmptyEvents(t *testing.T) {
	d := postgres.New()
	oldPkg := helperNewPkg(nil, nil)
	newPkg := helperNewPkg(nil, nil)

	g := New(d, []diff.DeltaEvent{}, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) != 0 {
		t.Errorf("expected empty UpStatements, got %d statements", len(mf.UpStatements))
	}
	if len(mf.DownStatements) != 0 {
		t.Errorf("expected empty DownStatements, got %d statements", len(mf.DownStatements))
	}
	if mf.HasDestructive {
		t.Error("expected HasDestructive = false for empty events")
	}
	if mf.Checksum == "" {
		t.Error("expected non-empty checksum even for empty events")
	}
}

func TestDescribe(t *testing.T) {
	tests := []struct {
		name     string
		events   []diff.DeltaEvent
		wantDesc string
	}{
		{
			name: "single ColumnAdded",
			events: []diff.DeltaEvent{
				{
					Kind:   diff.ColumnAdded,
					Table:  "users",
					Column: "email",
				},
			},
			wantDesc: "add_email_to_users",
		},
		{
			name: "single TableAdded",
			events: []diff.DeltaEvent{
				{
					Kind:  diff.TableAdded,
					Table: "users",
				},
			},
			wantDesc: "create_users",
		},
		{
			name: "single TableDropped",
			events: []diff.DeltaEvent{
				{
					Kind:  diff.TableDropped,
					Table: "users",
				},
			},
			wantDesc: "drop_table_users",
		},
		{
			name: "single ColumnDropped",
			events: []diff.DeltaEvent{
				{
					Kind:   diff.ColumnDropped,
					Table:  "users",
					Column: "name",
				},
			},
			wantDesc: "drop_name_from_users",
		},
		{
			name: "two column adds",
			events: []diff.DeltaEvent{
				{
					Kind:   diff.ColumnAdded,
					Table:  "users",
					Column: "email",
				},
				{
					Kind:   diff.ColumnAdded,
					Table:  "users",
					Column: "name",
				},
			},
			wantDesc: "add_email_add_name_users",
		},
		{
			name: "three events batch",
			events: []diff.DeltaEvent{
				{
					Kind:   diff.ColumnAdded,
					Table:  "users",
					Column: "email",
				},
				{
					Kind:   diff.ColumnAdded,
					Table:  "users",
					Column: "name",
				},
				{
					Kind:   diff.ColumnAdded,
					Table:  "users",
					Column: "age",
				},
			},
			wantDesc: "batch_3_changes",
		},
		{
			name: "mixed events batch",
			events: []diff.DeltaEvent{
				{
					Kind:   diff.ColumnAdded,
					Table:  "users",
					Column: "email",
				},
				{
					Kind:  diff.TableAdded,
					Table: "orders",
				},
			},
			wantDesc: "batch_2_changes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := postgres.New()
			oldPkg := helperNewPkg(nil, nil)
			newPkg := helperNewPkg(nil, nil)

			g := New(d, tt.events, oldPkg, newPkg)
			desc := g.describe()

			if desc != tt.wantDesc {
				t.Errorf("describe() = %q, want %q", desc, tt.wantDesc)
			}
		})
	}
}

func TestGenerate_ColumnTypeChanged_DownUsesOldColumn(t *testing.T) {
	d := postgres.New()
	oldCol := &schema.Column{Name: "status", SQLType: "varchar(50)", Nullable: false}
	newCol := &schema.Column{Name: "status", SQLType: "integer", Nullable: false}

	oldPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns:    []*schema.Column{oldCol},
		},
	}, nil)
	newPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns:    []*schema.Column{newCol},
		},
	}, nil)

	events := []diff.DeltaEvent{
		{
			Kind:      diff.ColumnTypeChanged,
			Table:     "users",
			Column:    "status",
			OldValue:  "varchar(50)",
			NewValue:  "integer",
			Severity:  diff.Warning,
			OldColumn: oldCol,
			NewColumn: newCol,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements for AlterColumnType")
	}
	if !strings.Contains(mf.UpStatements[0], "integer") {
		t.Errorf("up statement should use new type 'integer', got: %s", mf.UpStatements[0])
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements for reversing type change")
	}
	if !strings.Contains(mf.DownStatements[0], "varchar(50)") {
		t.Errorf("down statement should use old type 'varchar(50)', got: %s", mf.DownStatements[0])
	}
}

func TestGenerate_ColumnTypeChanged_UsesCustomUsingClause(t *testing.T) {
	d := postgres.New()
	oldCol := &schema.Column{Name: "count", SQLType: "varchar(50)", Nullable: true}
	newCol := &schema.Column{Name: "count", SQLType: "integer", Nullable: true, TypeChangeUsing: "CAST(count AS integer)"}

	oldPkg := helperNewPkg([]schema.Table{
		{StructName: "User", Name: "users", Columns: []*schema.Column{oldCol}},
	}, nil)
	newPkg := helperNewPkg([]schema.Table{
		{StructName: "User", Name: "users", Columns: []*schema.Column{newCol}},
	}, nil)

	events := []diff.DeltaEvent{
		{
			Kind:      diff.ColumnTypeChanged,
			Table:     "users",
			Column:    "count",
			Severity:  diff.Warning,
			OldColumn: oldCol,
			NewColumn: newCol,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements for AlterColumnType")
	}
	got := mf.UpStatements[0]
	if !strings.Contains(got, "TYPE integer USING CAST(count AS integer)") {
		t.Errorf("expected custom USING clause, got: %s", got)
	}
	if strings.Contains(got, "integer::integer") {
		t.Errorf("default USING fallback should not be applied when TypeChangeUsing is set, got: %s", got)
	}
}

func TestGenerate_ColumnRenamed(t *testing.T) {
	d := postgres.New()
	col := &schema.Column{Name: "email_address", SQLType: "text"}

	oldPkg := helperNewPkg([]schema.Table{
		{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "email", SQLType: "text"}}},
	}, nil)
	newPkg := helperNewPkg([]schema.Table{
		{StructName: "User", Name: "users", Columns: []*schema.Column{col}},
	}, nil)

	events := []diff.DeltaEvent{
		{
			Kind:     diff.ColumnRenamed,
			Table:    "users",
			Column:   "email_address",
			OldValue: "email",
			NewValue: "email_address",
			Severity: diff.Info,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) != 1 {
		t.Fatalf("expected 1 up statement, got %d: %v", len(mf.UpStatements), mf.UpStatements)
	}
	wantUp := `ALTER TABLE users RENAME COLUMN "email" TO "email_address";`
	if mf.UpStatements[0] != wantUp {
		t.Errorf("up statement = %q, want %q", mf.UpStatements[0], wantUp)
	}

	if len(mf.DownStatements) != 1 {
		t.Fatalf("expected 1 down statement, got %d: %v", len(mf.DownStatements), mf.DownStatements)
	}
	wantDown := `ALTER TABLE users RENAME COLUMN "email_address" TO "email";`
	if mf.DownStatements[0] != wantDown {
		t.Errorf("down statement = %q, want %q", mf.DownStatements[0], wantDown)
	}
}

func TestGenerate_ColumnDefaultChanged_DownUsesOldColumn(t *testing.T) {
	d := postgres.New()
	oldCol := &schema.Column{Name: "status", SQLType: "varchar(50)", Default: "'active'"}
	newCol := &schema.Column{Name: "status", SQLType: "varchar(50)", Default: "'pending'"}

	oldPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns:    []*schema.Column{oldCol},
		},
	}, nil)
	newPkg := helperNewPkg([]schema.Table{
		{
			StructName: "User",
			Name:       "users",
			Columns:    []*schema.Column{newCol},
		},
	}, nil)

	events := []diff.DeltaEvent{
		{
			Kind:      diff.ColumnDefaultChanged,
			Table:     "users",
			Column:    "status",
			OldValue:  "'active'",
			NewValue:  "'pending'",
			Severity:  diff.Info,
			OldColumn: oldCol,
			NewColumn: newCol,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements for AlterColumnDefault")
	}
	if !strings.Contains(mf.UpStatements[0], "pending") {
		t.Errorf("up statement should use new default 'pending', got: %s", mf.UpStatements[0])
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements for reversing default change")
	}
	if !strings.Contains(mf.DownStatements[0], "active") {
		t.Errorf("down statement should use old default 'active', got: %s", mf.DownStatements[0])
	}
}

func TestGenerate_FunctionAdded(t *testing.T) {
	d := postgres.New()
	fn := schema.Function{Name: "update_timestamp", Language: "plpgsql", Body: "BEGIN END;", ReturnType: "void"}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{Functions: []schema.Function{fn}}

	events := []diff.DeltaEvent{
		{
			Kind:        diff.FunctionAdded,
			Severity:    diff.Info,
			NewFunction: &fn,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}
	foundUp := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "CREATE OR REPLACE FUNCTION") {
			foundUp = true
			break
		}
	}
	if !foundUp {
		t.Errorf("up should contain CREATE OR REPLACE FUNCTION, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	foundDown := false
	for _, stmt := range mf.DownStatements {
		if strings.Contains(stmt, "DROP FUNCTION IF EXISTS") {
			foundDown = true
			break
		}
	}
	if !foundDown {
		t.Errorf("down should contain DROP FUNCTION IF EXISTS, got: %v", mf.DownStatements)
	}
}

func TestGenerate_FunctionBodyChanged(t *testing.T) {
	d := postgres.New()
	oldFn := schema.Function{Name: "update_timestamp", Language: "plpgsql", Body: "BEGIN NEW.updated_at := now(); END;", ReturnType: "void"}
	newFn := schema.Function{Name: "update_timestamp", Language: "plpgsql", Body: "BEGIN NEW.updated_at := current_timestamp; END;", ReturnType: "void"}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{Functions: []schema.Function{newFn}}

	events := []diff.DeltaEvent{
		{
			Kind:        diff.FunctionBodyChanged,
			Severity:    diff.Info,
			OldFunction: &oldFn,
			NewFunction: &newFn,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) < 2 {
		t.Fatalf("expected at least 2 up statements (drop + create), got %d", len(mf.UpStatements))
	}
	foundDrop := false
	foundCreate := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "DROP FUNCTION IF EXISTS") {
			foundDrop = true
		}
		if strings.Contains(stmt, "CREATE OR REPLACE FUNCTION") {
			foundCreate = true
		}
	}
	if !foundDrop {
		t.Errorf("up should contain DROP FUNCTION IF EXISTS, got: %v", mf.UpStatements)
	}
	if !foundCreate {
		t.Errorf("up should contain CREATE OR REPLACE FUNCTION, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	foundDown := false
	for _, stmt := range mf.DownStatements {
		if strings.Contains(stmt, "CREATE OR REPLACE FUNCTION") {
			foundDown = true
			break
		}
	}
	if !foundDown {
		t.Errorf("down should contain CREATE OR REPLACE FUNCTION to restore old body, got: %v", mf.DownStatements)
	}
}

func TestGenerate_ViewAdded(t *testing.T) {
	d := postgres.New()
	v := schema.View{Name: "active_users", Query: "SELECT * FROM users WHERE active = true"}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{Views: []schema.View{v}}

	events := []diff.DeltaEvent{
		{
			Kind:     diff.ViewAdded,
			Severity: diff.Info,
			NewView:  &v,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}
	foundUp := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "CREATE OR REPLACE VIEW") {
			foundUp = true
			break
		}
	}
	if !foundUp {
		t.Errorf("up should contain CREATE OR REPLACE VIEW, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	foundDown := false
	for _, stmt := range mf.DownStatements {
		if strings.Contains(stmt, "DROP VIEW IF EXISTS") {
			foundDown = true
			break
		}
	}
	if !foundDown {
		t.Errorf("down should contain DROP VIEW IF EXISTS, got: %v", mf.DownStatements)
	}
}

func TestGenerate_ViewQueryChanged(t *testing.T) {
	d := postgres.New()
	oldView := schema.View{Name: "active_users", Query: "SELECT * FROM users WHERE active = true"}
	newView := schema.View{Name: "active_users", Query: "SELECT * FROM users WHERE active = true AND deleted_at IS NULL"}
	oldPkg := &schema.Package{Views: []schema.View{oldView}}
	newPkg := &schema.Package{Views: []schema.View{newView}}

	events := []diff.DeltaEvent{
		{
			Kind:     diff.ViewQueryChanged,
			Severity: diff.Info,
			OldView:  &oldView,
			NewView:  &newView,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}
	foundUp := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "CREATE OR REPLACE VIEW") && strings.Contains(stmt, "deleted_at IS NULL") {
			foundUp = true
			break
		}
	}
	if !foundUp {
		t.Errorf("up should contain CREATE OR REPLACE VIEW with new query, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	foundDown := false
	for _, stmt := range mf.DownStatements {
		if strings.Contains(stmt, "CREATE OR REPLACE VIEW") && strings.Contains(stmt, "active = true") {
			foundDown = true
			break
		}
	}
	if !foundDown {
		t.Errorf("down should contain CREATE OR REPLACE VIEW with old query to restore, got: %v", mf.DownStatements)
	}
}

func TestGenerate_MatViewAdded(t *testing.T) {
	d := postgres.New()
	mv := schema.MaterializedView{Name: "user_stats", Query: "SELECT user_id, COUNT(*) FROM orders GROUP BY user_id"}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{MaterializedViews: []schema.MaterializedView{mv}}

	events := []diff.DeltaEvent{
		{
			Kind:                diff.MaterializedViewAdded,
			Severity:            diff.Info,
			NewMaterializedView: &mv,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}
	foundUp := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "CREATE MATERIALIZED VIEW") {
			foundUp = true
			break
		}
	}
	if !foundUp {
		t.Errorf("up should contain CREATE MATERIALIZED VIEW, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	foundDown := false
	for _, stmt := range mf.DownStatements {
		if strings.Contains(stmt, "DROP MATERIALIZED VIEW IF EXISTS") {
			foundDown = true
			break
		}
	}
	if !foundDown {
		t.Errorf("down should contain DROP MATERIALIZED VIEW IF EXISTS, got: %v", mf.DownStatements)
	}
}

func TestGenerate_TriggerAdded(t *testing.T) {
	d := postgres.New()
	tr := schema.Trigger{
		Name:     "update_timestamp",
		Table:    "users",
		Timing:   "BEFORE",
		Events:   []string{"UPDATE"},
		Function: "update_timestamp",
	}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{Triggers: []schema.Trigger{tr}}

	events := []diff.DeltaEvent{
		{
			Kind:       diff.TriggerAdded,
			Severity:   diff.Info,
			NewTrigger: &tr,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}
	foundUp := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "CREATE TRIGGER") {
			foundUp = true
			break
		}
	}
	if !foundUp {
		t.Errorf("up should contain CREATE TRIGGER, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	foundDown := false
	for _, stmt := range mf.DownStatements {
		if strings.Contains(stmt, "DROP TRIGGER IF EXISTS") {
			foundDown = true
			break
		}
	}
	if !foundDown {
		t.Errorf("down should contain DROP TRIGGER IF EXISTS, got: %v", mf.DownStatements)
	}
}

func TestGenerate_ProcedureAdded(t *testing.T) {
	d := postgres.New()
	proc := schema.Procedure{Name: "refresh_views", Language: "plpgsql", Body: "BEGIN REFRESH MATERIALIZED VIEW user_stats; END;"}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{Procedures: []schema.Procedure{proc}}

	events := []diff.DeltaEvent{
		{
			Kind:         diff.ProcedureAdded,
			Severity:     diff.Info,
			NewProcedure: &proc,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}
	foundUp := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "CREATE OR REPLACE PROCEDURE") {
			foundUp = true
			break
		}
	}
	if !foundUp {
		t.Errorf("up should contain CREATE OR REPLACE PROCEDURE, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	foundDown := false
	for _, stmt := range mf.DownStatements {
		if strings.Contains(stmt, "DROP PROCEDURE IF EXISTS") {
			foundDown = true
			break
		}
	}
	if !foundDown {
		t.Errorf("down should contain DROP PROCEDURE IF EXISTS, got: %v", mf.DownStatements)
	}
}

func TestGenerate_GrantAdded(t *testing.T) {
	d := postgres.New()
	g := schema.Grant{
		ObjectType: "table",
		ObjectName: "users",
		Privileges: []string{"SELECT", "INSERT"},
		Roles:      []string{"app_user"},
	}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{Grants: []schema.Grant{g}}

	events := []diff.DeltaEvent{
		{
			Kind:     diff.GrantAdded,
			Severity: diff.Info,
			NewGrant: &g,
		},
	}

	gen := New(d, events, oldPkg, newPkg)
	mf, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}
	foundUp := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "GRANT") && strings.Contains(stmt, "SELECT") {
			foundUp = true
			break
		}
	}
	if !foundUp {
		t.Errorf("up should contain GRANT, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	foundDown := false
	for _, stmt := range mf.DownStatements {
		if strings.Contains(stmt, "REVOKE") {
			foundDown = true
			break
		}
	}
	if !foundDown {
		t.Errorf("down should contain REVOKE, got: %v", mf.DownStatements)
	}
}

func TestGenerate_PolicyAdded(t *testing.T) {
	d := postgres.New()
	pol := schema.Policy{
		Name:    "user_isolation",
		Table:   "users",
		Command: "ALL",
		Roles:   []string{"app_user"},
		Using:   "user_id = current_setting('app.user_id')::int",
	}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{Policies: []schema.Policy{pol}}

	events := []diff.DeltaEvent{
		{
			Kind:      diff.PolicyAdded,
			Severity:  diff.Info,
			NewPolicy: &pol,
		},
	}

	gen := New(d, events, oldPkg, newPkg)
	mf, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}
	foundUp := false
	for _, stmt := range mf.UpStatements {
		if strings.Contains(stmt, "CREATE POLICY") {
			foundUp = true
			break
		}
	}
	if !foundUp {
		t.Errorf("up should contain CREATE POLICY, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	foundDown := false
	for _, stmt := range mf.DownStatements {
		if strings.Contains(stmt, "DROP POLICY IF EXISTS") {
			foundDown = true
			break
		}
	}
	if !foundDown {
		t.Errorf("down should contain DROP POLICY IF EXISTS, got: %v", mf.DownStatements)
	}
}

func TestGenerate_ExtensionAdded(t *testing.T) {
	d := postgres.New()
	ext := schema.Extension{Name: "uuid-ossp", IfNotExists: true}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{Extensions: []schema.Extension{ext}}

	events := []diff.DeltaEvent{
		{
			Kind:         diff.ExtensionAdded,
			Severity:     diff.Info,
			NewExtension: &ext,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}
	if !strings.Contains(mf.UpStatements[0], "CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"") {
		t.Errorf("up[0] should be CREATE EXTENSION, got: %v", mf.UpStatements)
	}

	if len(mf.DownStatements) == 0 {
		t.Fatal("expected down statements")
	}
	if !strings.Contains(mf.DownStatements[len(mf.DownStatements)-1], "DROP EXTENSION IF EXISTS \"uuid-ossp\"") {
		t.Errorf("last down should be DROP EXTENSION, got: %v", mf.DownStatements)
	}
}

func TestGenerate_ExtensionVersionChanged(t *testing.T) {
	d := postgres.New()
	oldExt := schema.Extension{Name: "postgis", Version: "3.0"}
	newExt := schema.Extension{Name: "postgis", Version: "3.1"}
	oldPkg := helperNewPkg(nil, nil)
	newPkg := &schema.Package{Extensions: []schema.Extension{newExt}}

	events := []diff.DeltaEvent{
		{
			Kind:         diff.ExtensionVersionChanged,
			Severity:     diff.Info,
			OldExtension: &oldExt,
			NewExtension: &newExt,
		},
	}

	g := New(d, events, oldPkg, newPkg)
	mf, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(mf.UpStatements) != 1 {
		t.Fatalf("expected 1 up statement, got %d: %v", len(mf.UpStatements), mf.UpStatements)
	}
	wantUp := `ALTER EXTENSION "postgis" UPDATE TO '3.1';`
	if mf.UpStatements[0] != wantUp {
		t.Errorf("up statement = %q, want %q", mf.UpStatements[0], wantUp)
	}

	if len(mf.DownStatements) != 1 {
		t.Fatalf("expected 1 down statement, got %d: %v", len(mf.DownStatements), mf.DownStatements)
	}
	wantDown := `ALTER EXTENSION "postgis" UPDATE TO '3.0';`
	if mf.DownStatements[0] != wantDown {
		t.Errorf("down statement = %q, want %q", mf.DownStatements[0], wantDown)
	}
}

func TestGenerate_ConstraintDropsEmitted(t *testing.T) {
	d := postgres.New()

	oldTable := schema.Table{
		StructName: "User", Name: "users",
		Columns: []*schema.Column{
			{Name: "id", SQLType: "integer", Nullable: false},
			{Name: "org_id", SQLType: "integer", Nullable: false},
		},
		ForeignKeys: []schema.ForeignKey{
			{Name: "users_org_id_fkey", FromColumns: []string{"org_id"}, ToTable: "orgs", ToColumns: []string{"id"}},
		},
		Uniques: []schema.UniqueConstraint{
			{Name: "users_email_key", Columns: []string{"email"}},
		},
		Checks: []schema.CheckConstraint{
			{Name: "users_age_positive", Expression: "age > 0"},
		},
		Indexes: []schema.Index{
			{Name: "idx_users_name", Table: "users", Columns: []string{"name"}},
		},
	}
	newTable := schema.Table{
		StructName: "User", Name: "users",
		Columns: []*schema.Column{
			{Name: "id", SQLType: "integer", Nullable: false},
			{Name: "org_id", SQLType: "integer", Nullable: false},
		},
	}

	g := New(d, []diff.DeltaEvent{
		{Kind: diff.FKDropped, Table: "users", ConstraintName: "users_org_id_fkey", Severity: diff.Warning},
		{Kind: diff.UniqueDropped, Table: "users", ConstraintName: "users_email_key", Severity: diff.Warning},
		{Kind: diff.CheckDropped, Table: "users", ConstraintName: "users_age_positive", Severity: diff.Warning},
		{Kind: diff.IndexDropped, Table: "users", ConstraintName: "idx_users_name", Severity: diff.Info},
	}, &schema.Package{Tables: []schema.Table{oldTable}}, &schema.Package{Tables: []schema.Table{newTable}})

	mf, err := g.Generate()
	if err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(mf.UpStatements, "\n")
	for _, want := range []string{
		`ALTER TABLE users DROP CONSTRAINT "users_org_id_fkey";`,
		`ALTER TABLE users DROP CONSTRAINT "users_email_key";`,
		`ALTER TABLE users DROP CONSTRAINT "users_age_positive";`,
		`DROP INDEX IF EXISTS "idx_users_name";`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("up statements missing %q\nfull:\n%v", want, mf.UpStatements)
		}
	}
}

func TestGenerate_PKChangedDropsThenAdds(t *testing.T) {
	d := postgres.New()

	oldTable := schema.Table{
		StructName: "User", Name: "users",
		Columns:    []*schema.Column{{Name: "id", SQLType: "integer", Nullable: false}},
		PrimaryKey: &schema.PrimaryKey{Name: "users_pkey", Columns: []string{"id"}},
	}
	newTable := schema.Table{
		StructName: "User", Name: "users",
		Columns:    []*schema.Column{{Name: "id", SQLType: "integer", Nullable: false}},
		PrimaryKey: &schema.PrimaryKey{Name: "users_pkey", Columns: []string{"tenant_id", "id"}},
	}

	g := New(d, []diff.DeltaEvent{
		{Kind: diff.PKChanged, Table: "users", Severity: diff.Warning},
	}, &schema.Package{Tables: []schema.Table{oldTable}}, &schema.Package{Tables: []schema.Table{newTable}})

	mf, err := g.Generate()
	if err != nil {
		t.Fatal(err)
	}

	// The up migration must DROP the old PK before ADDING the new one,
	// otherwise PostgreSQL errors with "multiple primary keys for table".
	dropIdx := -1
	addIdx := -1
	for i, stmt := range mf.UpStatements {
		if strings.Contains(stmt, `DROP CONSTRAINT "users_pkey"`) {
			dropIdx = i
		}
		if strings.Contains(stmt, `ADD CONSTRAINT "users_pkey" PRIMARY KEY`) {
			addIdx = i
		}
	}
	if dropIdx == -1 || addIdx == -1 {
		t.Fatalf("expected both DROP and ADD of PK; got:\n%v", mf.UpStatements)
	}
	if dropIdx > addIdx {
		t.Fatalf("DROP CONSTRAINT must come before ADD CONSTRAINT:\n%v", mf.UpStatements)
	}
}
