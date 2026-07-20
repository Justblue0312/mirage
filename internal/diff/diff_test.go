package diff

import (
	"testing"

	"github.com/justblue/mirage/internal/schema"
)

func TestDiff(t *testing.T) {
	t.Run("empty to pkg_with_one_table yields TableAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
					},
				},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != TableAdded {
			t.Errorf("expected Kind=%q, got %q", TableAdded, events[0].Kind)
		}
		if events[0].Table != "users" {
			t.Errorf("expected Table=%q, got %q", "users", events[0].Table)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].NewTable == nil {
			t.Error("expected NewTable to be populated")
		}
	})

	t.Run("identical packages yield no events", func(t *testing.T) {
		pkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "name", SQLType: "varchar(100)"},
					},
				},
			},
		}

		events := Diff(pkg, pkg)

		if len(events) != 0 {
			t.Errorf("expected 0 events for identical packages, got %d", len(events))
			for i, e := range events {
				t.Logf("event[%d]: %s", i, e.Kind)
			}
		}
	})

	t.Run("new column yields ColumnAdded", func(t *testing.T) {
		oldPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
					},
				},
			},
		}
		newPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "email", SQLType: "varchar(255)"},
					},
				},
			},
		}

		events := Diff(oldPkg, newPkg)

		var found bool
		for _, e := range events {
			if e.Kind == ColumnAdded && e.Column == "email" {
				found = true
				if e.NewColumn == nil {
					t.Error("expected NewColumn to be populated")
				}
				if e.Severity != Info {
					t.Errorf("expected Severity=Info, got %d", e.Severity)
				}
				break
			}
		}
		if !found {
			t.Fatal("expected ColumnAdded event for column 'email'")
		}
	})

	t.Run("TableDropped has OldTable populated", func(t *testing.T) {
		oldPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
					},
				},
			},
		}
		newPkg := &schema.Package{}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != TableDropped {
			t.Fatalf("expected Kind=%q, got %q", TableDropped, events[0].Kind)
		}
		if events[0].OldTable == nil {
			t.Fatal("expected OldTable to be populated")
		}
		if events[0].OldTable.Name != "users" {
			t.Errorf("expected OldTable.SQL.Name=%q, got %q", "users", events[0].OldTable.Name)
		}
		if events[0].Severity != Destructive {
			t.Errorf("expected Severity=Destructive, got %d", events[0].Severity)
		}
	})

	t.Run("ColumnTypeChanged with varchar(100) to text", func(t *testing.T) {
		oldPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "desc", SQLType: "varchar(100)"},
					},
				},
			},
		}
		newPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "desc", SQLType: "text"},
					},
				},
			},
		}

		events := Diff(oldPkg, newPkg)

		var found bool
		for _, e := range events {
			if e.Kind == ColumnTypeChanged && e.Column == "desc" {
				found = true
				if e.OldValue != "varchar(100)" {
					t.Errorf("expected OldValue=%q, got %q", "varchar(100)", e.OldValue)
				}
				if e.NewValue != "text" {
					t.Errorf("expected NewValue=%q, got %q", "text", e.NewValue)
				}
				if e.Severity != Warning {
					t.Errorf("expected Severity=Warning, got %d", e.Severity)
				}
				break
			}
		}
		if !found {
			t.Fatal("expected ColumnTypeChanged event for column 'desc'")
		}
	})

	t.Run("ColumnNullChanged false to true", func(t *testing.T) {
		oldPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "email", SQLType: "varchar(255)", Nullable: true},
					},
				},
			},
		}
		newPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "email", SQLType: "varchar(255)", Nullable: false},
					},
				},
			},
		}

		events := Diff(oldPkg, newPkg)

		var found bool
		for _, e := range events {
			if e.Kind == ColumnNullChanged && e.Column == "email" {
				found = true
				if e.OldValue != "true" {
					t.Errorf("expected OldValue=%q, got %q", "true", e.OldValue)
				}
				if e.NewValue != "false" {
					t.Errorf("expected NewValue=%q, got %q", "false", e.NewValue)
				}
				break
			}
		}
		if !found {
			t.Fatal("expected ColumnNullChanged event for column 'email'")
		}
	})

	t.Run("ColumnDefaultChanged empty to now()", func(t *testing.T) {
		oldPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "created_at", SQLType: "timestamp", Nullable: false, Default: ""},
					},
				},
			},
		}
		newPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "created_at", SQLType: "timestamp", Nullable: false, Default: "now()"},
					},
				},
			},
		}

		events := Diff(oldPkg, newPkg)

		var found bool
		for _, e := range events {
			if e.Kind == ColumnDefaultChanged && e.Column == "created_at" {
				found = true
				if e.OldValue != "" {
					t.Errorf("expected OldValue=%q, got %q", "", e.OldValue)
				}
				if e.NewValue != "now()" {
					t.Errorf("expected NewValue=%q, got %q", "now()", e.NewValue)
				}
				break
			}
		}
		if !found {
			t.Fatal("expected ColumnDefaultChanged event for column 'created_at'")
		}
	})

	t.Run("new enum yields EnumAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			Enums: []schema.Enum{
				{
					StructName: "Status",
					Name:       "status",
					Values:     []string{"active", "inactive"},
				},
			},
		}

		events := Diff(oldPkg, newPkg)

		var found bool
		for _, e := range events {
			if e.Kind == EnumAdded && e.Enum == "status" {
				found = true
				if e.Severity != Info {
					t.Errorf("expected Severity=Info, got %d", e.Severity)
				}
				break
			}
		}
		if !found {
			t.Fatal("expected EnumAdded event for enum 'status'")
		}
	})

	t.Run("new enum value yields EnumValueAdded", func(t *testing.T) {
		oldPkg := &schema.Package{
			Enums: []schema.Enum{
				{
					StructName: "Status",
					Name:       "status",
					Values:     []string{"active", "inactive"},
				},
			},
		}
		newPkg := &schema.Package{
			Enums: []schema.Enum{
				{
					StructName: "Status",
					Name:       "status",
					Values:     []string{"active", "inactive", "pending"},
				},
			},
		}

		events := Diff(oldPkg, newPkg)

		var found bool
		for _, e := range events {
			if e.Kind == EnumValueAdded && e.NewValue == "pending" {
				found = true
				if e.Enum != "status" {
					t.Errorf("expected Enum=%q, got %q", "status", e.Enum)
				}
				if e.Severity != Info {
					t.Errorf("expected Severity=Info, got %d", e.Severity)
				}
				break
			}
		}
		if !found {
			t.Fatal("expected EnumValueAdded event for value 'pending'")
		}
	})

	t.Run("removed enum value yields EnumValueDropped with Destructive severity", func(t *testing.T) {
		oldPkg := &schema.Package{
			Enums: []schema.Enum{
				{
					StructName: "Status",
					Name:       "status",
					Values:     []string{"active", "inactive", "pending"},
				},
			},
		}
		newPkg := &schema.Package{
			Enums: []schema.Enum{
				{
					StructName: "Status",
					Name:       "status",
					Values:     []string{"active", "inactive"},
				},
			},
		}

		events := Diff(oldPkg, newPkg)

		var found bool
		for _, e := range events {
			if e.Kind == EnumValueDropped && e.OldValue == "pending" {
				found = true
				if e.Enum != "status" {
					t.Errorf("expected Enum=%q, got %q", "status", e.Enum)
				}
				if e.Severity != Destructive {
					t.Errorf("expected Severity=Destructive, got %d", e.Severity)
				}
				break
			}
		}
		if !found {
			t.Fatal("expected EnumValueDropped event for value 'pending'")
		}
	})

	t.Run("removed column yields ColumnDropped with Destructive severity and OldColumn populated", func(t *testing.T) {
		oldPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "legacy_field", SQLType: "text"},
					},
				},
			},
		}
		newPkg := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User",
					Name:       "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
					},
				},
			},
		}

		events := Diff(oldPkg, newPkg)

		var found bool
		for _, e := range events {
			if e.Kind == ColumnDropped && e.Column == "legacy_field" {
				found = true
				if e.Severity != Destructive {
					t.Errorf("expected Severity=Destructive, got %d", e.Severity)
				}
				if e.OldColumn == nil {
					t.Fatal("expected OldColumn to be populated")
				}
				if e.OldColumn.SQLType != "text" {
					t.Errorf("expected OldColumn.SQLType=%q, got %q", "text", e.OldColumn.SQLType)
				}
				break
			}
		}
		if !found {
			t.Fatal("expected ColumnDropped event for column 'legacy_field'")
		}
	})
}

func TestDiffTableDriven(t *testing.T) {
	tests := []struct {
		name        string
		oldPkg      *schema.Package
		newPkg      *schema.Package
		expectKinds []EventKind
		expectCount int
		checkEvents func(t *testing.T, events []DeltaEvent)
	}{
		{
			name:        "empty_to_empty",
			oldPkg:      &schema.Package{},
			newPkg:      &schema.Package{},
			expectCount: 0,
		},
		{
			name:   "multiple_tables_added",
			oldPkg: &schema.Package{},
			newPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
					{StructName: "Posts", Name: "posts", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
				},
			},
			expectCount: 2,
			expectKinds: []EventKind{TableAdded, TableAdded},
		},
		{
			name: "multiple_columns_added",
			oldPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
				},
			},
			newPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{
						{Name: "id", SQLType: "integer"},
						{Name: "name", SQLType: "varchar(100)"},
						{Name: "email", SQLType: "varchar(255)"},
					}},
				},
			},
			expectCount: 2,
			expectKinds: []EventKind{ColumnAdded, ColumnAdded},
		},
		{
			name: "column_type_and_null_change_together",
			oldPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "bio", SQLType: "varchar(50)", Nullable: true},
					}},
				},
			},
			newPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "bio", SQLType: "text", Nullable: false},
					}},
				},
			},
			expectCount: 2,
			checkEvents: func(t *testing.T, events []DeltaEvent) {
				var typeChanged, nullChanged bool
				for _, e := range events {
					if e.Kind == ColumnTypeChanged && e.Column == "bio" {
						typeChanged = true
						if e.OldValue != "varchar(50)" || e.NewValue != "text" {
							t.Errorf("ColumnTypeChanged: expected varchar(50)->text, got %s->%s", e.OldValue, e.NewValue)
						}
						if e.Severity != Warning {
							t.Errorf("ColumnTypeChanged severity: expected Warning, got %d", e.Severity)
						}
					}
					if e.Kind == ColumnNullChanged && e.Column == "bio" {
						nullChanged = true
						if e.OldValue != "true" || e.NewValue != "false" {
							t.Errorf("ColumnNullChanged: expected true->false, got %s->%s", e.OldValue, e.NewValue)
						}
					}
				}
				if !typeChanged {
					t.Error("expected ColumnTypeChanged event")
				}
				if !nullChanged {
					t.Error("expected ColumnNullChanged event")
				}
			},
		},
		{
			name: "column_default_changed_from_value_to_empty",
			oldPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "role", SQLType: "varchar(50)", Nullable: false, Default: "'admin'"},
					}},
				},
			},
			newPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "role", SQLType: "varchar(50)", Nullable: false, Default: ""},
					}},
				},
			},
			expectCount: 1,
			checkEvents: func(t *testing.T, events []DeltaEvent) {
				e := events[0]
				if e.Kind != ColumnDefaultChanged {
					t.Fatalf("expected ColumnDefaultChanged, got %s", e.Kind)
				}
				if e.OldValue != "'admin'" {
					t.Errorf("expected OldValue=%q, got %q", "'admin'", e.OldValue)
				}
				if e.NewValue != "" {
					t.Errorf("expected NewValue=%q, got %q", "", e.NewValue)
				}
			},
		},
		{
			name:   "multiple_enums_added",
			oldPkg: &schema.Package{},
			newPkg: &schema.Package{
				Enums: []schema.Enum{
					{StructName: "Status", Name: "status", Values: []string{"a", "b"}},
					{StructName: "Role", Name: "role", Values: []string{"admin", "user"}},
				},
			},
			expectCount: 2,
			expectKinds: []EventKind{EnumAdded, EnumAdded},
		},
		{
			name: "enum_multiple_values_added_and_dropped",
			oldPkg: &schema.Package{
				Enums: []schema.Enum{
					{StructName: "Status", Name: "status", Values: []string{"old1", "old2", "kept"}},
				},
			},
			newPkg: &schema.Package{
				Enums: []schema.Enum{
					{StructName: "Status", Name: "status", Values: []string{"new1", "new2", "kept"}},
				},
			},
			expectCount: 4,
			checkEvents: func(t *testing.T, events []DeltaEvent) {
				var added, dropped int
				for _, e := range events {
					if e.Kind == EnumValueAdded {
						added++
						if e.Severity != Info {
							t.Errorf("EnumValueAdded severity: expected Info, got %d", e.Severity)
						}
					}
					if e.Kind == EnumValueDropped {
						dropped++
						if e.Severity != Destructive {
							t.Errorf("EnumValueDropped severity: expected Destructive, got %d", e.Severity)
						}
					}
				}
				if added != 2 {
					t.Errorf("expected 2 EnumValueAdded, got %d", added)
				}
				if dropped != 2 {
					t.Errorf("expected 2 EnumValueDropped, got %d", dropped)
				}
			},
		},
		{
			name:   "table_and_enum_added_together",
			oldPkg: &schema.Package{},
			newPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
				},
				Enums: []schema.Enum{
					{StructName: "Status", Name: "status", Values: []string{"active"}},
				},
			},
			expectCount: 2,
			checkEvents: func(t *testing.T, events []DeltaEvent) {
				var tableAdded, enumAdded bool
				for _, e := range events {
					if e.Kind == TableAdded {
						tableAdded = true
					}
					if e.Kind == EnumAdded {
						enumAdded = true
					}
				}
				if !tableAdded {
					t.Error("expected TableAdded event")
				}
				if !enumAdded {
					t.Error("expected EnumAdded event")
				}
			},
		},
		{
			name: "table_dropped_and_enum_dropped_together",
			oldPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
				},
				Enums: []schema.Enum{
					{StructName: "Status", Name: "status", Values: []string{"active"}},
				},
			},
			newPkg:      &schema.Package{},
			expectCount: 2,
			checkEvents: func(t *testing.T, events []DeltaEvent) {
				var tableDropped, enumDropped bool
				for _, e := range events {
					if e.Kind == TableDropped {
						tableDropped = true
						if e.Severity != Destructive {
							t.Errorf("TableDropped severity: expected Destructive, got %d", e.Severity)
						}
						if e.OldTable == nil {
							t.Error("TableDropped: OldTable not populated")
						}
					}
					if e.Kind == EnumDropped {
						enumDropped = true
						if e.Severity != Destructive {
							t.Errorf("EnumDropped severity: expected Destructive, got %d", e.Severity)
						}
					}
				}
				if !tableDropped {
					t.Error("expected TableDropped event")
				}
				if !enumDropped {
					t.Error("expected EnumDropped event")
				}
			},
		},
		{
			name: "column_null_changed_true_to_false",
			oldPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "name", SQLType: "varchar(100)", Nullable: false},
					}},
				},
			},
			newPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
						{Name: "name", SQLType: "varchar(100)", Nullable: true},
					}},
				},
			},
			expectCount: 1,
			checkEvents: func(t *testing.T, events []DeltaEvent) {
				e := events[0]
				if e.Kind != ColumnNullChanged {
					t.Fatalf("expected ColumnNullChanged, got %s", e.Kind)
				}
				if e.OldValue != "false" || e.NewValue != "true" {
					t.Errorf("expected false->true, got %s->%s", e.OldValue, e.NewValue)
				}
			},
		},
		{
			name: "column_dropped_multiple_columns",
			oldPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{
						{Name: "id", SQLType: "integer"},
						{Name: "col_a", SQLType: "text"},
						{Name: "col_b", SQLType: "integer"},
						{Name: "col_c", SQLType: "boolean"},
					}},
				},
			},
			newPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", Name: "users", Columns: []*schema.Column{
						{Name: "id", SQLType: "integer"},
					}},
				},
			},
			expectCount: 3,
			checkEvents: func(t *testing.T, events []DeltaEvent) {
				dropped := make(map[string]bool)
				for _, e := range events {
					if e.Kind == ColumnDropped {
						dropped[e.Column] = true
						if e.Severity != Destructive {
							t.Errorf("ColumnDropped(%s) severity: expected Destructive, got %d", e.Column, e.Severity)
						}
						if e.OldColumn == nil {
							t.Errorf("ColumnDropped(%s): OldColumn not populated", e.Column)
						}
					}
				}
				for _, col := range []string{"col_a", "col_b", "col_c"} {
					if !dropped[col] {
						t.Errorf("expected ColumnDropped for %s", col)
					}
				}
			},
		},
		{
			name:   "qualified_name_with_schema",
			oldPkg: &schema.Package{},
			newPkg: &schema.Package{
				Tables: []schema.Table{
					{StructName: "Users", SearchPath: "app", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
				},
				Enums: []schema.Enum{
					{StructName: "Status", SearchPath: "app", Name: "status", Values: []string{"active"}},
				},
			},
			expectCount: 2,
			checkEvents: func(t *testing.T, events []DeltaEvent) {
				var tableFound, enumFound bool
				for _, e := range events {
					if e.Kind == TableAdded && e.Table == "app.users" {
						tableFound = true
					}
					if e.Kind == EnumAdded && e.Enum == "app.status" {
						enumFound = true
					}
				}
				if !tableFound {
					t.Error("expected TableAdded with Table='app.users'")
				}
				if !enumFound {
					t.Error("expected EnumAdded with Enum='app.status'")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := Diff(tt.oldPkg, tt.newPkg)

			if len(events) != tt.expectCount {
				t.Fatalf("expected %d events, got %d", tt.expectCount, len(events))
			}

			if tt.expectKinds != nil {
				for i, kind := range tt.expectKinds {
					if i >= len(events) {
						t.Fatalf("expected event[%d] with Kind=%s, but no event exists", i, kind)
					}
					if events[i].Kind != kind {
						t.Errorf("expected events[%d].Kind=%s, got %s", i, kind, events[i].Kind)
					}
				}
			}

			if tt.checkEvents != nil {
				tt.checkEvents(t, events)
			}
		})
	}
}

func TestDiffSeverityConstants(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		expected int
	}{
		{"Info is zero", Info, 0},
		{"Warning is one", Warning, 1},
		{"Destructive is two", Destructive, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.severity) != tt.expected {
				t.Errorf("expected severity value %d, got %d", tt.expected, tt.severity)
			}
		})
	}
}

func TestDiffEventKindConstants(t *testing.T) {
	expectedKinds := map[string]EventKind{
		"table_added":            TableAdded,
		"table_dropped":          TableDropped,
		"column_added":           ColumnAdded,
		"column_dropped":         ColumnDropped,
		"column_type_changed":    ColumnTypeChanged,
		"column_null_changed":    ColumnNullChanged,
		"column_default_changed": ColumnDefaultChanged,
		"index_added":            IndexAdded,
		"index_dropped":          IndexDropped,
		"unique_added":           UniqueAdded,
		"unique_dropped":         UniqueDropped,
		"fk_added":               FKAdded,
		"fk_dropped":             FKDropped,
		"check_added":            CheckAdded,
		"check_dropped":          CheckDropped,
		"pk_changed":             PKChanged,
		"enum_added":             EnumAdded,
		"enum_dropped":           EnumDropped,
		"enum_value_added":       EnumValueAdded,
		"enum_value_dropped":     EnumValueDropped,
		"comment_changed":        CommentChanged,
	}

	for expected, constant := range expectedKinds {
		t.Run(expected, func(t *testing.T) {
			if string(constant) != expected {
				t.Errorf("expected %q, got %q", expected, constant)
			}
		})
	}
}

func TestPackagesEqual(t *testing.T) {
	t.Run("identical packages", func(t *testing.T) {
		a := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
			},
			Enums: []schema.Enum{
				{StructName: "Role", Name: "role", Values: []string{"admin", "user"}},
			},
		}
		b := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
			},
			Enums: []schema.Enum{
				{StructName: "Role", Name: "role", Values: []string{"admin", "user"}},
			},
		}
		if !PackagesEqual(a, b) {
			t.Error("PackagesEqual = false, want true")
		}
	})

	t.Run("different tables", func(t *testing.T) {
		a := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
			},
		}
		b := &schema.Package{
			Tables: []schema.Table{
				{StructName: "Post", Name: "posts", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
			},
		}
		if PackagesEqual(a, b) {
			t.Error("PackagesEqual = true, want false")
		}
	})

	t.Run("different columns", func(t *testing.T) {
		a := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{
					{Name: "id", SQLType: "integer"},
					{Name: "name", SQLType: "text"},
				}},
			},
		}
		b := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{
					{Name: "id", SQLType: "integer"},
					{Name: "email", SQLType: "varchar(255)"},
				}},
			},
		}
		if PackagesEqual(a, b) {
			t.Error("PackagesEqual = true, want false")
		}
	})

	t.Run("different enums", func(t *testing.T) {
		a := &schema.Package{
			Enums: []schema.Enum{
				{StructName: "Role", Name: "role", Values: []string{"admin"}},
			},
		}
		b := &schema.Package{
			Enums: []schema.Enum{
				{StructName: "Role", Name: "role", Values: []string{"admin", "user"}},
			},
		}
		if PackagesEqual(a, b) {
			t.Error("PackagesEqual = true, want false")
		}
	})

	t.Run("nil vs empty slices", func(t *testing.T) {
		a := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
			},
		}
		b := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}, Indexes: []schema.Index{}},
			},
		}
		if PackagesEqual(a, b) {
			t.Error("PackagesEqual = true for nil vs empty Indexes, want false")
		}
	})

	t.Run("both empty packages", func(t *testing.T) {
		a := &schema.Package{}
		b := &schema.Package{}
		if !PackagesEqual(a, b) {
			t.Error("PackagesEqual = false, want true")
		}
	})

	t.Run("different Index pointer", func(t *testing.T) {
		a := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User", Name: "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false},
					},
				},
			},
		}
		b := &schema.Package{
			Tables: []schema.Table{
				{
					StructName: "User", Name: "users",
					Columns: []*schema.Column{
						{Name: "id", SQLType: "integer", Nullable: false, Index: schema.Btree, IndexSort: "ASC"},
					},
				},
			},
		}
		if PackagesEqual(a, b) {
			t.Error("PackagesEqual = true for nil vs non-nil Index, want false")
		}
	})
}

func TestPackagesEqualJSON(t *testing.T) {
	t.Run("identical packages", func(t *testing.T) {
		a := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
			},
		}
		b := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
			},
		}
		eq, ja, jb := PackagesEqualJSON(a, b)
		if !eq {
			t.Errorf("PackagesEqualJSON = false, want true\na=%s\nb=%s", ja, jb)
		}
	})

	t.Run("different packages", func(t *testing.T) {
		a := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "integer"}}},
			},
		}
		b := &schema.Package{
			Tables: []schema.Table{
				{StructName: "User", Name: "users", Columns: []*schema.Column{{Name: "id", SQLType: "text"}}},
			},
		}
		eq, _, _ := PackagesEqualJSON(a, b)
		if eq {
			t.Error("PackagesEqualJSON = true, want false")
		}
	})
}

func TestBoolStr(t *testing.T) {
	tests := []struct {
		input    bool
		expected string
	}{
		{true, "true"},
		{false, "false"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := boolStr(tt.input); got != tt.expected {
				t.Errorf("boolStr(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDiff_Determinism(t *testing.T) {
	oldPkg := &schema.Package{
		Tables: []schema.Table{
			{
				StructName: "User",
				SearchPath: "public", Name: "users",
				Columns: []*schema.Column{
					{Name: "id", SQLType: "bigserial", PrimaryKey: true, OrdinalPosition: 0},
					{Name: "name", SQLType: "text", OrdinalPosition: 1},
				},
			},
		},
	}

	newPkg := &schema.Package{
		Tables: []schema.Table{
			{
				StructName: "User",
				SearchPath: "public", Name: "users",
				Columns: []*schema.Column{
					{Name: "id", SQLType: "bigserial", PrimaryKey: true, OrdinalPosition: 0},
					{Name: "name", SQLType: "text", OrdinalPosition: 1},
					{Name: "user_id", SQLType: "bigint", OrdinalPosition: 2},
					{Name: "total", SQLType: "numeric", OrdinalPosition: 3},
					{Name: "status", SQLType: "text", OrdinalPosition: 4},
					{Name: "created_at", SQLType: "timestamptz", OrdinalPosition: 5},
					{Name: "shipped_at", SQLType: "timestamptz", OrdinalPosition: 6},
					{Name: "notes", SQLType: "text", OrdinalPosition: 7},
				},
			},
		},
	}

	const runs = 20
	var firstEvents []DeltaEvent
	for i := 0; i < runs; i++ {
		events := Diff(oldPkg, newPkg)
		if i == 0 {
			firstEvents = events
			continue
		}
		if len(events) != len(firstEvents) {
			t.Fatalf("run %d: got %d events, want %d", i, len(events), len(firstEvents))
		}
		for j, e := range events {
			if e != firstEvents[j] {
				t.Fatalf("run %d, event %d: got %+v, want %+v", i, j, e, firstEvents[j])
			}
		}
	}
}

func TestPkEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *schema.PrimaryKey
		b        *schema.PrimaryKey
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "a nil b not nil",
			a:        nil,
			b:        &schema.PrimaryKey{Columns: []string{"id"}},
			expected: false,
		},
		{
			name:     "a not nil b nil",
			a:        &schema.PrimaryKey{Columns: []string{"id"}},
			b:        nil,
			expected: false,
		},
		{
			name:     "same columns same order",
			a:        &schema.PrimaryKey{Columns: []string{"id"}},
			b:        &schema.PrimaryKey{Columns: []string{"id"}},
			expected: true,
		},
		{
			name:     "same columns different order",
			a:        &schema.PrimaryKey{Columns: []string{"a", "b"}},
			b:        &schema.PrimaryKey{Columns: []string{"b", "a"}},
			expected: false,
		},
		{
			name:     "different columns",
			a:        &schema.PrimaryKey{Columns: []string{"id"}},
			b:        &schema.PrimaryKey{Columns: []string{"uuid"}},
			expected: false,
		},
		{
			name:     "different lengths",
			a:        &schema.PrimaryKey{Columns: []string{"id"}},
			b:        &schema.PrimaryKey{Columns: []string{"id", "name"}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pkEqual(tt.a, tt.b); got != tt.expected {
				t.Errorf("pkEqual() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDetectColumnRenames(t *testing.T) {
	old := &schema.Package{
		Tables: []schema.Table{{
			StructName: "User",
			SearchPath: "public", Name: "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "bigserial"},
				{Name: "name", SQLType: "text"},
				{Name: "user_name", SQLType: "text"},
			},
		}},
	}
	new := &schema.Package{
		Tables: []schema.Table{{
			StructName: "User",
			SearchPath: "public", Name: "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "bigserial"},
				{Name: "name", SQLType: "text"},
				{Name: "username", SQLType: "text"},
			},
		}},
	}

	hints := DetectColumnRenames(old, new)
	if len(hints) != 1 {
		t.Fatalf("expected 1 rename hint, got %d", len(hints))
	}
	if hints[0].OldColumn != "user_name" || hints[0].NewColumn != "username" {
		t.Errorf("wrong hint: %v", hints[0])
	}
}

func TestDetectColumnRenames_NoRenameWhenTooDifferent(t *testing.T) {
	old := &schema.Package{
		Tables: []schema.Table{{
			StructName: "User",
			SearchPath: "public", Name: "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "bigserial"},
				{Name: "email", SQLType: "text"},
			},
		}},
	}
	new := &schema.Package{
		Tables: []schema.Table{{
			StructName: "User",
			SearchPath: "public", Name: "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "bigserial"},
				{Name: "description", SQLType: "text"},
			},
		}},
	}

	hints := DetectColumnRenames(old, new)
	if len(hints) != 0 {
		t.Fatalf("expected 0 rename hints, got %d", len(hints))
	}
}

func TestDetectColumnRenames_NoRenameForNewTable(t *testing.T) {
	new := &schema.Package{
		Tables: []schema.Table{{
			StructName: "User",
			SearchPath: "public", Name: "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "bigserial"},
				{Name: "name", SQLType: "text"},
			},
		}},
	}

	hints := DetectColumnRenames(&schema.Package{}, new)
	if len(hints) != 0 {
		t.Fatalf("expected 0 rename hints for new table, got %d", len(hints))
	}
}

func TestDiff_RegisteredFunctions(t *testing.T) {
	t.Run("empty old with new function yields FunctionAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			Functions: []schema.Function{
				{Name: "fn1", Language: "plpgsql", Body: "begin end;", ReturnType: "void"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != FunctionAdded {
			t.Errorf("expected Kind=%q, got %q", FunctionAdded, events[0].Kind)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].NewFunction == nil {
			t.Fatal("expected NewFunction to be populated")
		}
	})

	t.Run("same function yields no events", func(t *testing.T) {
		pkg := &schema.Package{
			Functions: []schema.Function{
				{Name: "fn1", Language: "plpgsql", Body: "begin end;", ReturnType: "void"},
			},
		}

		events := Diff(pkg, pkg)
		if len(events) != 0 {
			t.Errorf("expected 0 events, got %d", len(events))
		}
	})

	t.Run("function body changed yields FunctionBodyChanged", func(t *testing.T) {
		oldPkg := &schema.Package{
			Functions: []schema.Function{
				{Name: "fn1", Language: "plpgsql", Body: "begin end;", ReturnType: "void"},
			},
		}
		newPkg := &schema.Package{
			Functions: []schema.Function{
				{Name: "fn1", Language: "plpgsql", Body: "begin return 1; end;", ReturnType: "integer"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != FunctionBodyChanged {
			t.Errorf("expected Kind=%q, got %q", FunctionBodyChanged, events[0].Kind)
		}
		if events[0].OldFunction == nil || events[0].NewFunction == nil {
			t.Fatal("expected OldFunction and NewFunction to be populated")
		}
	})

	t.Run("function dropped yields FunctionDropped", func(t *testing.T) {
		oldPkg := &schema.Package{
			Functions: []schema.Function{
				{Name: "fn1", Language: "plpgsql", Body: "begin end;", ReturnType: "void"},
			},
		}
		newPkg := &schema.Package{}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != FunctionDropped {
			t.Errorf("expected Kind=%q, got %q", FunctionDropped, events[0].Kind)
		}
		if events[0].Severity != Warning {
			t.Errorf("expected Severity=Warning, got %d", events[0].Severity)
		}
		if events[0].OldFunction == nil {
			t.Fatal("expected OldFunction to be populated")
		}
	})
}

func TestDiff_RegisteredViews(t *testing.T) {
	t.Run("empty old with new view yields ViewAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			Views: []schema.View{
				{Name: "v1", Query: "SELECT id FROM users"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != ViewAdded {
			t.Errorf("expected Kind=%q, got %q", ViewAdded, events[0].Kind)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].NewView == nil {
			t.Fatal("expected NewView to be populated")
		}
	})

	t.Run("view query changed yields ViewQueryChanged", func(t *testing.T) {
		oldPkg := &schema.Package{
			Views: []schema.View{
				{Name: "v1", Query: "SELECT id FROM users"},
			},
		}
		newPkg := &schema.Package{
			Views: []schema.View{
				{Name: "v1", Query: "SELECT id, name FROM users"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != ViewQueryChanged {
			t.Errorf("expected Kind=%q, got %q", ViewQueryChanged, events[0].Kind)
		}
		if events[0].OldView == nil || events[0].NewView == nil {
			t.Fatal("expected OldView and NewView to be populated")
		}
	})

	t.Run("view dropped yields ViewDropped", func(t *testing.T) {
		oldPkg := &schema.Package{
			Views: []schema.View{
				{Name: "v1", Query: "SELECT id FROM users"},
			},
		}
		newPkg := &schema.Package{}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != ViewDropped {
			t.Errorf("expected Kind=%q, got %q", ViewDropped, events[0].Kind)
		}
		if events[0].Severity != Warning {
			t.Errorf("expected Severity=Warning, got %d", events[0].Severity)
		}
		if events[0].OldView == nil {
			t.Fatal("expected OldView to be populated")
		}
	})
}

func TestDiff_RegisteredMatViews(t *testing.T) {
	t.Run("empty old with new matview yields MaterializedViewAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			MaterializedViews: []schema.MaterializedView{
				{Name: "mv1", Query: "SELECT id FROM users"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != MaterializedViewAdded {
			t.Errorf("expected Kind=%q, got %q", MaterializedViewAdded, events[0].Kind)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].NewMaterializedView == nil {
			t.Fatal("expected NewMaterializedView to be populated")
		}
	})

	t.Run("matview query changed yields MaterializedViewQueryChanged", func(t *testing.T) {
		oldPkg := &schema.Package{
			MaterializedViews: []schema.MaterializedView{
				{Name: "mv1", Query: "SELECT id FROM users"},
			},
		}
		newPkg := &schema.Package{
			MaterializedViews: []schema.MaterializedView{
				{Name: "mv1", Query: "SELECT id, name FROM users"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != MaterializedViewQueryChanged {
			t.Errorf("expected Kind=%q, got %q", MaterializedViewQueryChanged, events[0].Kind)
		}
		if events[0].OldMaterializedView == nil || events[0].NewMaterializedView == nil {
			t.Fatal("expected OldMaterializedView and NewMaterializedView to be populated")
		}
	})

	t.Run("matview dropped yields MaterializedViewDropped", func(t *testing.T) {
		oldPkg := &schema.Package{
			MaterializedViews: []schema.MaterializedView{
				{Name: "mv1", Query: "SELECT id FROM users"},
			},
		}
		newPkg := &schema.Package{}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != MaterializedViewDropped {
			t.Errorf("expected Kind=%q, got %q", MaterializedViewDropped, events[0].Kind)
		}
		if events[0].Severity != Warning {
			t.Errorf("expected Severity=Warning, got %d", events[0].Severity)
		}
		if events[0].OldMaterializedView == nil {
			t.Fatal("expected OldMaterializedView to be populated")
		}
	})
}

func TestDiff_RegisteredTriggers(t *testing.T) {
	t.Run("empty old with new trigger yields TriggerAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			Triggers: []schema.Trigger{
				{Name: "tr1", Table: "users", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "fn1"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != TriggerAdded {
			t.Errorf("expected Kind=%q, got %q", TriggerAdded, events[0].Kind)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].NewTrigger == nil {
			t.Fatal("expected NewTrigger to be populated")
		}
	})

	t.Run("trigger changed yields TriggerChanged", func(t *testing.T) {
		oldPkg := &schema.Package{
			Triggers: []schema.Trigger{
				{Name: "tr1", Table: "users", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "fn1"},
			},
		}
		newPkg := &schema.Package{
			Triggers: []schema.Trigger{
				{Name: "tr1", Table: "users", Timing: "AFTER", Events: []string{"UPDATE"}, Function: "fn2"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != TriggerChanged {
			t.Errorf("expected Kind=%q, got %q", TriggerChanged, events[0].Kind)
		}
		if events[0].OldTrigger == nil || events[0].NewTrigger == nil {
			t.Fatal("expected OldTrigger and NewTrigger to be populated")
		}
	})

	t.Run("trigger dropped yields TriggerDropped", func(t *testing.T) {
		oldPkg := &schema.Package{
			Triggers: []schema.Trigger{
				{Name: "tr1", Table: "users", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "fn1"},
			},
		}
		newPkg := &schema.Package{}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != TriggerDropped {
			t.Errorf("expected Kind=%q, got %q", TriggerDropped, events[0].Kind)
		}
		if events[0].Severity != Warning {
			t.Errorf("expected Severity=Warning, got %d", events[0].Severity)
		}
		if events[0].OldTrigger == nil {
			t.Fatal("expected OldTrigger to be populated")
		}
	})
}

func TestDiff_RegisteredProcedures(t *testing.T) {
	t.Run("empty old with new procedure yields ProcedureAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			Procedures: []schema.Procedure{
				{Name: "proc1", Language: "plpgsql", Body: "begin end;"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != ProcedureAdded {
			t.Errorf("expected Kind=%q, got %q", ProcedureAdded, events[0].Kind)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].NewProcedure == nil {
			t.Fatal("expected NewProcedure to be populated")
		}
	})

	t.Run("procedure body changed yields ProcedureBodyChanged", func(t *testing.T) {
		oldPkg := &schema.Package{
			Procedures: []schema.Procedure{
				{Name: "proc1", Language: "plpgsql", Body: "begin end;"},
			},
		}
		newPkg := &schema.Package{
			Procedures: []schema.Procedure{
				{Name: "proc1", Language: "plpgsql", Body: "begin perform fn1(); end;"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != ProcedureBodyChanged {
			t.Errorf("expected Kind=%q, got %q", ProcedureBodyChanged, events[0].Kind)
		}
		if events[0].OldProcedure == nil || events[0].NewProcedure == nil {
			t.Fatal("expected OldProcedure and NewProcedure to be populated")
		}
	})

	t.Run("procedure dropped yields ProcedureDropped", func(t *testing.T) {
		oldPkg := &schema.Package{
			Procedures: []schema.Procedure{
				{Name: "proc1", Language: "plpgsql", Body: "begin end;"},
			},
		}
		newPkg := &schema.Package{}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != ProcedureDropped {
			t.Errorf("expected Kind=%q, got %q", ProcedureDropped, events[0].Kind)
		}
		if events[0].Severity != Warning {
			t.Errorf("expected Severity=Warning, got %d", events[0].Severity)
		}
		if events[0].OldProcedure == nil {
			t.Fatal("expected OldProcedure to be populated")
		}
	})
}

func TestDiff_RegisteredGrants(t *testing.T) {
	t.Run("empty old with new grant yields GrantAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			Grants: []schema.Grant{
				{ObjectType: "table", ObjectName: "users", Privileges: []string{"SELECT"}, Roles: []string{"app"}},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != GrantAdded {
			t.Errorf("expected Kind=%q, got %q", GrantAdded, events[0].Kind)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].NewGrant == nil {
			t.Fatal("expected NewGrant to be populated")
		}
	})

	t.Run("same grant yields no events", func(t *testing.T) {
		pkg := &schema.Package{
			Grants: []schema.Grant{
				{ObjectType: "table", ObjectName: "users", Privileges: []string{"SELECT"}, Roles: []string{"app"}},
			},
		}

		events := Diff(pkg, pkg)
		if len(events) != 0 {
			t.Errorf("expected 0 events, got %d", len(events))
		}
	})

	t.Run("grant dropped yields GrantDropped", func(t *testing.T) {
		oldPkg := &schema.Package{
			Grants: []schema.Grant{
				{ObjectType: "table", ObjectName: "users", Privileges: []string{"SELECT"}, Roles: []string{"app"}},
			},
		}
		newPkg := &schema.Package{}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != GrantDropped {
			t.Errorf("expected Kind=%q, got %q", GrantDropped, events[0].Kind)
		}
		if events[0].Severity != Warning {
			t.Errorf("expected Severity=Warning, got %d", events[0].Severity)
		}
		if events[0].OldGrant == nil {
			t.Fatal("expected OldGrant to be populated")
		}
	})
}

func TestDiff_RegisteredPolicies(t *testing.T) {
	t.Run("empty old with new policy yields PolicyAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			Policies: []schema.Policy{
				{Name: "p1", Table: "users", Command: "SELECT", Using: "id = current_setting('app.user_id')::bigint"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != PolicyAdded {
			t.Errorf("expected Kind=%q, got %q", PolicyAdded, events[0].Kind)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].NewPolicy == nil {
			t.Fatal("expected NewPolicy to be populated")
		}
	})

	t.Run("policy Using changed yields PolicyChanged", func(t *testing.T) {
		oldPkg := &schema.Package{
			Policies: []schema.Policy{
				{Name: "p1", Table: "users", Command: "SELECT", Using: "true"},
			},
		}
		newPkg := &schema.Package{
			Policies: []schema.Policy{
				{Name: "p1", Table: "users", Command: "SELECT", Using: "active = true"},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != PolicyChanged {
			t.Errorf("expected Kind=%q, got %q", PolicyChanged, events[0].Kind)
		}
		if events[0].OldPolicy == nil || events[0].NewPolicy == nil {
			t.Fatal("expected OldPolicy and NewPolicy to be populated")
		}
	})

	t.Run("policy dropped yields PolicyDropped", func(t *testing.T) {
		oldPkg := &schema.Package{
			Policies: []schema.Policy{
				{Name: "p1", Table: "users", Command: "SELECT", Using: "true"},
			},
		}
		newPkg := &schema.Package{}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != PolicyDropped {
			t.Errorf("expected Kind=%q, got %q", PolicyDropped, events[0].Kind)
		}
		if events[0].Severity != Warning {
			t.Errorf("expected Severity=Warning, got %d", events[0].Severity)
		}
		if events[0].OldPolicy == nil {
			t.Fatal("expected OldPolicy to be populated")
		}
	})
}

func TestDiff_NilPackagesDoNotPanic(t *testing.T) {
	// Diff must not panic when one package is nil and the other is not.
	t.Run("nil old, non-nil new", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Diff panicked on nil old package: %v", r)
			}
		}()
		newPkg := &schema.Package{Tables: []schema.Table{{Name: "t"}}}
		events := Diff(nil, newPkg)
		if events == nil {
			t.Fatal("expected non-nil events slice")
		}
	})

	t.Run("non-nil old, nil new", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Diff panicked on nil new package: %v", r)
			}
		}()
		oldPkg := &schema.Package{Tables: []schema.Table{{Name: "t"}}}
		events := Diff(oldPkg, nil)
		if events == nil {
			t.Fatal("expected non-nil events slice")
		}
	})

	t.Run("both nil", func(t *testing.T) {
		events := Diff(nil, nil)
		if events != nil {
			t.Fatalf("expected nil events for two equal empty packages, got %d", len(events))
		}
	})
}

func TestDiff_PolicyRoleOrderingIsNotSpuriousChange(t *testing.T) {
	// ROLES (a, b) is equivalent to ROLES (b, a); reordering must not emit a
	// PolicyChanged event.
	oldPkg := &schema.Package{
		Policies: []schema.Policy{
			{Name: "p1", Table: "users", Command: "SELECT", Roles: []string{"b", "a"}},
		},
	}
	newPkg := &schema.Package{
		Policies: []schema.Policy{
			{Name: "p1", Table: "users", Command: "SELECT", Roles: []string{"a", "b"}},
		},
	}
	events := Diff(oldPkg, newPkg)
	for _, e := range events {
		if e.Kind == PolicyChanged {
			t.Fatalf("role reordering should not produce PolicyChanged")
		}
	}
}

func TestDiff_GrantKeyNoColonCollision(t *testing.T) {
	// grantKey must use a separator that cannot appear in names, so that a
	// schema-qualified object name cannot collide with a differently-split
	// key.
	a := &schema.Grant{ObjectType: "table", ObjectName: "public:users", Roles: []string{"r"}}
	b := &schema.Grant{ObjectType: "table:public", ObjectName: "users", Roles: []string{"r"}}
	if grantKey(a) == grantKey(b) {
		t.Fatalf("grantKey collided for %+v vs %+v", a, b)
	}
}

func TestApplyRenameHints(t *testing.T) {
	events := []DeltaEvent{
		{Kind: ColumnDropped, Table: "users", Column: "email", Severity: Destructive},
		{Kind: ColumnAdded, Table: "users", Column: "email_address", Severity: Info},
		{Kind: ColumnAdded, Table: "users", Column: "created_at", Severity: Info},
	}
	hints := []RenameHint{
		{Table: "users", OldColumn: "email", NewColumn: "email_address"},
	}

	out := ApplyRenameHints(events, hints)

	if len(out) != 2 {
		t.Fatalf("expected 2 events (rename + unrelated add), got %d: %+v", len(out), out)
	}

	var renames, adds, drops int
	for _, e := range out {
		switch e.Kind {
		case ColumnRenamed:
			renames++
			if e.OldValue != "email" || e.NewValue != "email_address" {
				t.Errorf("rename event has wrong names: OldValue=%q NewValue=%q", e.OldValue, e.NewValue)
			}
			if e.Table != "users" {
				t.Errorf("rename event table = %q, want users", e.Table)
			}
		case ColumnAdded:
			adds++
			if e.Column != "created_at" {
				t.Errorf("remaining add should be created_at, got %q", e.Column)
			}
		case ColumnDropped:
			drops++
		}
	}
	if renames != 1 || adds != 1 || drops != 0 {
		t.Errorf("expected renames=1 adds=1 drops=0, got renames=%d adds=%d drops=%d", renames, adds, drops)
	}
}

func TestApplyRenameHints_NoHintsReturnsInput(t *testing.T) {
	events := []DeltaEvent{
		{Kind: ColumnDropped, Table: "users", Column: "email"},
		{Kind: ColumnAdded, Table: "users", Column: "email_address"},
	}
	out := ApplyRenameHints(events, nil)
	if len(out) != 2 {
		t.Fatalf("expected input returned unchanged, got %d events", len(out))
	}
}

func TestDiff_RegisteredExtensions(t *testing.T) {
	t.Run("empty old with new extension yields ExtensionAdded", func(t *testing.T) {
		oldPkg := &schema.Package{}
		newPkg := &schema.Package{
			Extensions: []schema.Extension{
				{Name: "uuid-ossp", IfNotExists: true},
			},
		}

		events := Diff(oldPkg, newPkg)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != ExtensionAdded {
			t.Errorf("expected Kind=%q, got %q", ExtensionAdded, events[0].Kind)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].NewExtension == nil {
			t.Fatal("expected NewExtension to be populated")
		}
	})

	t.Run("same extension yields no events", func(t *testing.T) {
		pkg := &schema.Package{
			Extensions: []schema.Extension{
				{Name: "uuid-ossp"},
			},
		}

		events := Diff(pkg, pkg)
		if len(events) != 0 {
			t.Errorf("expected 0 events, got %d", len(events))
		}
	})

	t.Run("extension removed yields ExtensionDropped", func(t *testing.T) {
		oldPkg := &schema.Package{
			Extensions: []schema.Extension{
				{Name: "uuid-ossp"},
			},
		}
		newPkg := &schema.Package{}

		events := Diff(oldPkg, newPkg)
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != ExtensionDropped {
			t.Errorf("expected Kind=%q, got %q", ExtensionDropped, events[0].Kind)
		}
		if events[0].Severity != Destructive {
			t.Errorf("expected Severity=Destructive, got %d", events[0].Severity)
		}
	})

	t.Run("extension schema changed yields drop and add", func(t *testing.T) {
		oldPkg := &schema.Package{
			Extensions: []schema.Extension{
				{Name: "hstore", Schema: "public"},
			},
		}
		newPkg := &schema.Package{
			Extensions: []schema.Extension{
				{Name: "hstore", Schema: "extensions"},
			},
		}

		events := Diff(oldPkg, newPkg)
		var kinds []EventKind
		for _, e := range events {
			kinds = append(kinds, e.Kind)
		}
		if len(events) != 2 {
			t.Fatalf("expected 2 events (drop + add), got %d: %v", len(events), kinds)
		}
		if events[0].Kind != ExtensionDropped || events[1].Kind != ExtensionAdded {
			t.Errorf("expected drop then add, got %v", kinds)
		}
	})

	t.Run("extension version changed yields ExtensionVersionChanged", func(t *testing.T) {
		oldPkg := &schema.Package{
			Extensions: []schema.Extension{
				{Name: "postgis", Version: "3.0"},
			},
		}
		newPkg := &schema.Package{
			Extensions: []schema.Extension{
				{Name: "postgis", Version: "3.1"},
			},
		}

		events := Diff(oldPkg, newPkg)
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d: %v", len(events), events)
		}
		if events[0].Kind != ExtensionVersionChanged {
			t.Errorf("expected Kind=%q, got %q", ExtensionVersionChanged, events[0].Kind)
		}
		if events[0].Severity != Info {
			t.Errorf("expected Severity=Info, got %d", events[0].Severity)
		}
		if events[0].OldExtension == nil || events[0].NewExtension == nil {
			t.Fatal("expected OldExtension and NewExtension to be populated")
		}
	})

	t.Run("extension name changed yields drop and add", func(t *testing.T) {
		oldPkg := &schema.Package{
			Extensions: []schema.Extension{
				{Name: "postgis", Version: "3.0"},
			},
		}
		newPkg := &schema.Package{
			Extensions: []schema.Extension{
				{Name: "postgis_topology", Version: "3.0"},
			},
		}

		events := Diff(oldPkg, newPkg)
		var kinds []EventKind
		for _, e := range events {
			kinds = append(kinds, e.Kind)
		}
		if len(events) != 2 {
			t.Fatalf("expected 2 events (drop + add), got %d: %v", len(events), kinds)
		}
		// Iteration over new extensions (add) precedes old extensions (drop),
		// so the add event is emitted first, then the drop.
		if events[0].Kind != ExtensionAdded || events[1].Kind != ExtensionDropped {
			t.Errorf("expected add then drop, got %v", kinds)
		}
	})
}
