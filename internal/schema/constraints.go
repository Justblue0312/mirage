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

// Partition represents partition configuration for a parent table.
type Partition struct {
	Strategy string
	Column   string   // Deprecated: single column, kept for backward compat. Use Columns.
	Columns  []string // Multi-column partition keys. If set, Column is ignored.
}

// ColumnsList returns the effective partition columns (Columns if set, else Column).
func (p *Partition) ColumnsList() []string {
	if p == nil {
		return nil
	}
	if len(p.Columns) > 0 {
		return p.Columns
	}
	if p.Column != "" {
		return []string{p.Column}
	}
	return nil
}

// Equal reports whether two partitions are equivalent.
func (p *Partition) Equal(other *Partition) bool {
	if p == nil && other == nil {
		return true
	}
	if p == nil || other == nil {
		return false
	}
	if p.Strategy != other.Strategy {
		return false
	}
	a := p.ColumnsList()
	b := other.ColumnsList()
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// PartitionBound represents a child partition (PARTITION OF parent).
type PartitionBound struct {
	ParentTable string // parent table name (may be schema-qualified)
	Bounds      string // e.g. "FOR VALUES FROM ('2024-01-01') TO ('2025-01-01')"
}

// Equal reports whether two PartitionBounds are equivalent.
func (pb *PartitionBound) Equal(other *PartitionBound) bool {
	if pb == nil && other == nil {
		return true
	}
	if pb == nil || other == nil {
		return false
	}
	return pb.ParentTable == other.ParentTable && pb.Bounds == other.Bounds
}
