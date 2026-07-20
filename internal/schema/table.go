package schema

import (
	"fmt"
	"reflect"
	"strings"
)

type TableType uint8

const (
	InvalidTableType TableType = iota
	TableTypeBase
	TableTypeView
	TableTypePresenter
)

var DatabaseTableTypes = []TableType{TableTypeBase, TableTypeView, TableTypePresenter}

func (t TableType) String() string {
	switch t {
	case TableTypeBase:
		return "base table"
	case TableTypeView:
		return "view"
	case TableTypePresenter:
		return "presentation"
	default:
		return ""
	}
}

func ParseTableType(s string) TableType {
	switch strings.ToUpper(s) {
	case "BASE TABLE":
		return TableTypeBase
	case "VIEW":
		return TableTypeView
	case "PRESENTATION":
		return TableTypePresenter
	default:
		return InvalidTableType
	}
}

type TableFilter interface {
	FilterTable(*Table) bool
}

type TableFilterFunc = func(*Table) bool

type ColumnFilter interface {
	FilterColumn(*Column) bool
}

type Expressions []Expression

func (e Expressions) FilterTable(t *Table) bool {
	for _, expr := range e {
		colName := expr.ColumnPath
		if idx := strings.IndexByte(colName, '.'); idx != -1 {
			colName = colName[:idx]
		}
		for _, col := range t.Columns {
			if col.Name == colName {
				return true
			}
		}
	}
	return false
}

type Expression struct {
	TablePath  string
	ColumnPath string
	ColumnType reflect.Type
}

func NewExpression(path string, typ reflect.Type) Expression {
	parts := strings.SplitN(path, ".", 2)
	expr := Expression{ColumnType: typ}
	if len(parts) == 2 {
		expr.TablePath = parts[0]
		expr.ColumnPath = parts[1]
	} else {
		expr.ColumnPath = parts[0]
	}
	return expr
}

type Table struct {
	RegisteredPosition int
	StructName         string
	StructType         reflect.Type
	SearchPath         string
	Name               string
	Description        string
	Type               TableType
	Columns            []*Column
	PasswordHandler    *PasswordHandler
	Strict             bool

	Options     string
	Partitioned *Partition
	Ignore      bool
	PrimaryKey  *PrimaryKey
	ForeignKeys []ForeignKey
	Uniques     []UniqueConstraint
	Indexes     []Index
	Checks      []CheckConstraint
}

// SQLName returns the fully-qualified table name (schema.name or just name).
func (td *Table) SQLName() string {
	if td.SearchPath == "" || td.SearchPath == GetDefaultSearchPath() {
		return td.Name
	}
	return td.SearchPath + "." + td.Name
}

func (td *Table) IsReadOnly() bool {
	return td.Type == TableTypeView || td.Type == TableTypePresenter
}

func (td *Table) SetStrict(strict bool) {
	td.Strict = strict
}

func (td *Table) IsType(types ...TableType) bool {
	if len(types) == 0 {
		return true
	}
	for _, t := range types {
		if td.Type == t {
			return true
		}
	}
	return false
}

func (td *Table) FindPrimaryKey() (*Column, bool) {
	for _, col := range td.Columns {
		if col.PrimaryKey {
			return col, true
		}
	}
	return nil, false
}

func (td *Table) GetUsernameColumn() *Column {
	for _, col := range td.Columns {
		if col.Username {
			return col
		}
	}
	return nil
}

func (td *Table) GetPasswordColumn() *Column {
	for _, col := range td.Columns {
		if col.Password {
			return col
		}
	}
	return nil
}

func (td *Table) GetColumnByName(name string) *Column {
	for _, col := range td.Columns {
		if col.Name == name {
			return col
		}
	}
	return nil
}

func (td *Table) AddColumns(columns ...*Column) {
	td.Columns = append(td.Columns, columns...)
}

func (td *Table) ListColumnNamesExcept(except ...string) []string {
	exceptSet := make(map[string]bool, len(except))
	for _, name := range except {
		exceptSet[name] = true
	}
	var names []string
	for _, col := range td.Columns {
		if !exceptSet[col.Name] {
			names = append(names, col.Name)
		}
	}
	return names
}

func (td *Table) GetHumanName() string {
	name := td.Name
	if td.StructName != "" {
		name = td.StructName
	}
	if td.SearchPath != "" && td.SearchPath != "public" {
		name = td.SearchPath + "." + name
	}
	return fmt.Sprintf("%s table", name)
}
