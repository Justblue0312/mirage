package benchmark

import (
	"testing"

	"github.com/justblue/mirage/internal/diff"
	"github.com/justblue/mirage/internal/generator"
	"github.com/justblue/mirage/internal/scanner"
	"github.com/justblue/mirage/internal/schema"
	"github.com/justblue/mirage/internal/validate"
)

func BenchmarkDiff_Small(b *testing.B) {
	old := &schema.Package{
		Tables: []schema.Table{
			{
				StructName: "User",
				SearchPath: "public", Name: "users",
				Columns: []*schema.Column{
					{Name: "id", SQLType: "bigserial"},
					{Name: "name", SQLType: "text"},
				},
			},
		},
	}
	new := &schema.Package{
		Tables: []schema.Table{
			{
				StructName: "User",
				SearchPath: "public", Name: "users",
				Columns: []*schema.Column{
					{Name: "id", SQLType: "bigserial"},
					{Name: "name", SQLType: "text"},
					{Name: "email", SQLType: "text"},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diff.Diff(old, new)
	}
}

func BenchmarkDiff_Large(b *testing.B) {
	oldTables := make([]schema.Table, 100)
	newTables := make([]schema.Table, 100)
	for i := 0; i < 100; i++ {
		name := "table_" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		cols := make([]*schema.Column, 20)
		for j := 0; j < 20; j++ {
			cols[j] = &schema.Column{Name: "col_" + string(rune('a'+j)), SQLType: "text"}
		}
		oldTables[i] = schema.Table{
			StructName: "T" + name,
			SearchPath: "public", Name: name,
			Columns: cols,
		}
		newTables[i] = oldTables[i]
	}
	c50 := *newTables[50].Columns[len(newTables[50].Columns)-1]
	newTables[50].Columns = append(newTables[50].Columns, &c50)
	newTables[50].Columns[len(newTables[50].Columns)-1] = &schema.Column{Name: "new_col", SQLType: "int"}

	old := &schema.Package{Tables: oldTables}
	new := &schema.Package{Tables: newTables}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diff.Diff(old, new)
	}
}

func BenchmarkValidate(b *testing.B) {
	pkg := &schema.Package{
		Tables: []schema.Table{
			{
				StructName: "User",
				SearchPath: "public", Name: "users",
				Columns: []*schema.Column{
					{Name: "id", SQLType: "bigserial", PrimaryKey: true},
					{Name: "name", SQLType: "text"},
				},
				PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validate.Validate(pkg)
	}
}

func BenchmarkGenerator_Small(b *testing.B) {
	events := []diff.DeltaEvent{
		{Kind: diff.ColumnAdded, Table: "users", Column: "email", Severity: diff.Info,
			NewColumn: &schema.Column{Name: "email", SQLType: "text"}},
	}
	old := &schema.Package{}
	new := &schema.Package{
		Tables: []schema.Table{
			{
				StructName: "User",
				SearchPath: "public", Name: "users",
				Columns: []*schema.Column{
					{Name: "id", SQLType: "bigserial"},
					{Name: "name", SQLType: "text"},
					{Name: "email", SQLType: "text"},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen := generator.New(nil, events, old, new)
		gen.Generate()
	}
}

func BenchmarkScanner_Small(b *testing.B) {
	s := &scanner.Scanner{
		SourceDirs: []string{"./testdata"},
		Recursive:  false,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Scan()
	}
}

func BenchmarkPackagesEqual(b *testing.B) {
	pkg := &schema.Package{
		Tables: []schema.Table{
			{
				StructName: "User",
				SearchPath: "public", Name: "users",
				Columns: []*schema.Column{
					{Name: "id", SQLType: "bigserial"},
					{Name: "name", SQLType: "text"},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diff.PackagesEqual(pkg, pkg)
	}
}
