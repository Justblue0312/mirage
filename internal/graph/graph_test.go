package graph

import (
	"reflect"
	"strings"
	"testing"

	"github.com/justblue/mirage/internal/schema"
)

func TestBuildAndSort_LinearDependency(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A",
			Name:       "a",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_a_b", ToTable: "b"},
			},
		},
		{
			StructName: "B",
			Name:       "b",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_b_c", ToTable: "c"},
			},
		},
		{
			StructName: "C",
			Name:       "c",
		},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"c", "b", "a"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestBuildAndSort_DiamondDependency(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A",
			Name:       "a",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_a_b", ToTable: "b"},
				{Name: "fk_a_c", ToTable: "c"},
			},
		},
		{
			StructName: "B",
			Name:       "b",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_b_d", ToTable: "d"},
			},
		},
		{
			StructName: "C",
			Name:       "c",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_c_d", ToTable: "d"},
			},
		},
		{
			StructName: "D",
			Name:       "d",
		},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pos := make(map[string]int)
	for i, name := range result {
		pos[name] = i
	}

	if pos["d"] >= pos["b"] || pos["d"] >= pos["c"] {
		t.Errorf("d must come before b and c; got %v", result)
	}
	if pos["b"] >= pos["a"] || pos["c"] >= pos["a"] {
		t.Errorf("b and c must come before a; got %v", result)
	}
}

func TestBuildAndSort_IsolatedTables(t *testing.T) {
	tables := []schema.Table{
		{StructName: "A", Name: "a"},
		{StructName: "B", Name: "b"},
		{StructName: "C", Name: "c"},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestBuildAndSort_CycleDetection(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A",
			Name:       "a",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_a_b", ToTable: "b"},
			},
		},
		{
			StructName: "B",
			Name:       "b",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_b_a", ToTable: "a"},
			},
		},
	}

	g := Build(tables)
	_, err := g.Sort()
	if err == nil {
		t.Fatal("expected error for circular dependency, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("error message should contain 'circular dependency', got: %v", err)
	}
}

func TestBuild_SelfReferentialFKSkipped(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A",
			Name:       "a",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_a_a", ToTable: "a"},
			},
		},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestSort_Determinism(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "E",
			Name:       "e",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_e_a", ToTable: "a"},
				{Name: "fk_e_b", ToTable: "b"},
			},
		},
		{
			StructName: "D",
			Name:       "d",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_d_a", ToTable: "a"},
			},
		},
		{
			StructName: "C",
			Name:       "c",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_c_b", ToTable: "b"},
			},
		},
		{
			StructName: "B",
			Name:       "b",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_b_a", ToTable: "a"},
			},
		},
		{
			StructName: "A",
			Name:       "a",
		},
	}

	var firstResult []string
	for i := 0; i < 10; i++ {
		g := Build(tables)
		result, err := g.Sort()
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", i, err)
		}
		if i == 0 {
			firstResult = result
		} else if !reflect.DeepEqual(result, firstResult) {
			t.Errorf("run %d: got %v, want %v", i, result, firstResult)
		}
	}
}

func TestBuildAndSort_ColumnLevelFK(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "Child",
			Name:       "child",
			Columns: []*schema.Column{
				{
					Name:                "parent_id",
					ReferenceTableName:  "parent",
					ReferenceColumnName: "id",
				},
			},
		},
		{
			StructName: "Parent",
			Name:       "parent",
		},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pos := make(map[string]int)
	for i, name := range result {
		pos[name] = i
	}

	if pos["parent"] >= pos["child"] {
		t.Errorf("parent must come before child; got %v", result)
	}
}

func TestBuildAndSort_SelfReferentialColumnFKSkipped(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A",
			Name:       "a",
			Columns: []*schema.Column{
				{
					Name:                "parent_id",
					ReferenceTableName:  "a",
					ReferenceColumnName: "id",
				},
			},
		},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestBuildAndSort_EmptyTables(t *testing.T) {
	g := Build(nil)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %v, want empty slice", result)
	}
}

func TestBuildAndSort_SingleTable(t *testing.T) {
	tables := []schema.Table{
		{StructName: "A", Name: "a"},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestBuildAndSort_FKToUnknownTableIgnored(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A",
			Name:       "a",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_a_unknown", ToTable: "unknown"},
			},
		},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestBuildAndSort_ThreeNodeCycle(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A",
			Name:       "a",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_a_b", ToTable: "b"},
			},
		},
		{
			StructName: "B",
			Name:       "b",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_b_c", ToTable: "c"},
			},
		},
		{
			StructName: "C",
			Name:       "c",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_c_a", ToTable: "a"},
			},
		},
	}

	g := Build(tables)
	_, err := g.Sort()
	if err == nil {
		t.Fatal("expected error for circular dependency, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("error message should contain 'circular dependency', got: %v", err)
	}
}

func TestBuildAndSort_MultipleIndependentChains(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A1",
			Name:       "a1",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_a1_a2", ToTable: "a2"},
			},
		},
		{
			StructName: "A2",
			Name:       "a2",
		},
		{
			StructName: "B1",
			Name:       "b1",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_b1_b2", ToTable: "b2"},
			},
		},
		{
			StructName: "B2",
			Name:       "b2",
		},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pos := make(map[string]int)
	for i, name := range result {
		pos[name] = i
	}

	if pos["a2"] >= pos["a1"] {
		t.Errorf("a2 must come before a1; got %v", result)
	}
	if pos["b2"] >= pos["b1"] {
		t.Errorf("b2 must come before b1; got %v", result)
	}
}

func TestBuild_DuplicateEdgesDeduplicated(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A",
			Name:       "a",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_a_b_1", ToTable: "b"},
				{Name: "fk_a_b_2", ToTable: "b"},
			},
		},
		{
			StructName: "B",
			Name:       "b",
		},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"b", "a"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestBuildAndSort_SchemaQualifiedNames(t *testing.T) {
	tables := []schema.Table{
		{
			StructName: "A",
			SearchPath: "public", Name: "a",
			ForeignKeys: []schema.ForeignKey{
				{Name: "fk_a_b", ToTable: "other.b"},
			},
		},
		{
			StructName: "B",
			SearchPath: "other", Name: "b",
		},
	}

	g := Build(tables)
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pos := make(map[string]int)
	for i, name := range result {
		pos[name] = i
	}

	if pos["other.b"] >= pos["a"] {
		t.Errorf("other.b must come before a; got %v", result)
	}
}
