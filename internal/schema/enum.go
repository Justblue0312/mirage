package schema

// Enum represents a PostgreSQL enum type.
type Enum struct {
	StructName  string
	SearchPath  string
	Name        string
	Values      []string
	Description string
}

// SQLName returns the fully-qualified enum name (schema.name or just name).
func (e *Enum) SQLName() string {
	if e.SearchPath == "" || e.SearchPath == GetDefaultSearchPath() {
		return e.Name
	}
	return e.SearchPath + "." + e.Name
}

// Package represents a scanned Go package containing table and enum definitions.
type Package struct {
	Tables            []Table
	Enums             []Enum
	Extensions        []Extension
	Functions         []Function
	Views             []View
	MaterializedViews []MaterializedView
	Triggers          []Trigger
	Procedures        []Procedure
	Grants            []Grant
	Policies          []Policy
}
