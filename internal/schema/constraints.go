package schema

// PrimaryKey represents a table-level primary key constraint.
type PrimaryKey struct {
	Name    string
	Columns []string
}

// ForeignKey represents a table-level foreign key constraint.
type ForeignKey struct {
	Name        string
	FromTable   string
	FromColumns []string
	ToTable     string
	ToColumns   []string
	OnDelete    string
	OnUpdate    string
}

// UniqueConstraint represents a table-level unique constraint.
type UniqueConstraint struct {
	Name    string
	Columns []string
}

// Index represents a table-level index.
type Index struct {
	Name    string
	Table   string
	Columns []string
	Kind    string
	Sort    string
}

// CheckConstraint represents a table-level check constraint.
type CheckConstraint struct {
	Name       string
	Expression string
}

// Partition represents partition configuration for a table.
type Partition struct {
	Strategy string
	Column   string
}
