package validate

import (
	"testing"

	"github.com/justblue/mirage/internal/schema"
)

func TestValidate_CleanPackage(t *testing.T) {
	pkg := &schema.Package{
		Tables: []schema.Table{
			{
				StructName: "User",
				SearchPath: "public", Name: "users",
				Columns: []*schema.Column{
					{Name: "id", SQLType: "int", PrimaryKey: true},
					{Name: "name", SQLType: "text"},
				},
				PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
			},
			{
				StructName: "Order",
				SearchPath: "public", Name: "orders",
				Columns: []*schema.Column{
					{Name: "id", SQLType: "int", PrimaryKey: true},
					{Name: "user_id", SQLType: "int"},
				},
				PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
				ForeignKeys: []schema.ForeignKey{
					{
						Name:        "fk_orders_user_id",
						FromTable:   "orders",
						FromColumns: []string{"user_id"},
						ToTable:     "users",
						ToColumns:   []string{"id"},
						OnDelete:    "CASCADE",
					},
				},
			},
		},
		Enums: []schema.Enum{
			{
				StructName: "OrderStatus",
				SearchPath: "public", Name: "order_status",
				Values: []string{"pending", "completed", "cancelled"},
			},
		},
	}

	errs := Validate(pkg)
	if len(errs) != 0 {
		t.Errorf("expected zero errors for clean package, got %d: %v", len(errs), errs)
	}
}

func TestValidate_ErrDuplicateTableName(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "duplicate table names",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "orders",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "Order",
						SearchPath: "public", Name: "orders",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "unique table names",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "Order",
						SearchPath: "public", Name: "orders",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrDuplicateTableName {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrDuplicateTableName count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrDuplicateColumnName(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "duplicate column names in one table",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "name", SQLType: "text"}, {Name: "name", SQLType: "varchar"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "unique column names",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "name", SQLType: "text"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrDuplicateColumnName {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrDuplicateColumnName count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrDuplicateConstraintName(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "duplicate constraint name across tables",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Name: "pk_shared", Columns: []string{"id"}},
					},
					{
						StructName: "Order",
						SearchPath: "public", Name: "orders",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Name: "pk_shared", Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "unique constraint names",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Name: "pk_users", Columns: []string{"id"}},
					},
					{
						StructName: "Order",
						SearchPath: "public", Name: "orders",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Name: "pk_orders", Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrDuplicateConstraintName {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrDuplicateConstraintName count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrMissingFKTarget(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "fk references non-existent table",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "organization_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"organization_id"},
								ToTable:     "orgs",
								ToColumns:   []string{"id"},
							},
						},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "fk references existing table",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Organization",
						SearchPath: "public", Name: "orgs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "organization_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"organization_id"},
								ToTable:     "orgs",
								ToColumns:   []string{"id"},
							},
						},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrMissingFKTarget {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrMissingFKTarget count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrMissingFKColumn(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "fk references non-existent column",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Organization",
						SearchPath: "public", Name: "orgs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns: []*schema.Column{
							{Name: "id", SQLType: "int", PrimaryKey: true},
							{
								Name:                "org_id",
								SQLType:             "int",
								ReferenceTableName:  "orgs",
								ReferenceColumnName: "uuid",
							},
						},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "fk references existing column",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Organization",
						SearchPath: "public", Name: "orgs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "org_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"org_id"},
								ToTable:     "orgs",
								ToColumns:   []string{"id"},
							},
						},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrMissingFKColumn {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrMissingFKColumn count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrUnresolvedEnumRef(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "enum reference not found",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "status", SQLType: "user_status"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Enums: []schema.Enum{},
			},
			wantErrors: 1,
		},
		{
			name: "enum reference found",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "status", SQLType: "user_status"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Enums: []schema.Enum{
					{
						StructName: "UserStatus",
						SearchPath: "public", Name: "user_status",
						Values: []string{"active", "inactive"},
					},
				},
			},
			wantErrors: 0,
		},
		{
			name: "builtin sql type not treated as enum ref",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "name", SQLType: "text"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrUnresolvedEnumRef {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrUnresolvedEnumRef count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrMissingPK(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
		wantWarn   bool
	}{
		{
			name: "table without primary key",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "AuditLog",
						SearchPath: "public", Name: "audit_log",
						Columns: []*schema.Column{{Name: "message", SQLType: "text"}},
					},
				},
			},
			wantErrors: 1,
			wantWarn:   true,
		},
		{
			name: "table with primary key",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 0,
			wantWarn:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrMissingPK {
					count++
					if tt.wantWarn && !e.IsWarn() {
						t.Errorf("ErrMissingPK should be a warning, IsWarn() = false")
					}
					if tt.wantWarn && e.IsError() {
						t.Errorf("ErrMissingPK should be a warning, IsError() = true")
					}
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrMissingPK count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrInvalidOnDelete(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "invalid on delete action",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Organization",
						SearchPath: "public", Name: "orgs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "org_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"org_id"},
								ToTable:     "orgs",
								ToColumns:   []string{"id"},
								OnDelete:    "INVALID",
							},
						},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "valid on delete action",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Organization",
						SearchPath: "public", Name: "orgs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "org_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"org_id"},
								ToTable:     "orgs",
								ToColumns:   []string{"id"},
								OnDelete:    "CASCADE",
							},
						},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrInvalidOnDelete {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrInvalidOnDelete count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrInvalidOnUpdate(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "invalid on update action",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Organization",
						SearchPath: "public", Name: "orgs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "org_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"org_id"},
								ToTable:     "orgs",
								ToColumns:   []string{"id"},
								OnUpdate:    "INVALID",
							},
						},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "valid on update action",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Organization",
						SearchPath: "public", Name: "orgs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "org_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"org_id"},
								ToTable:     "orgs",
								ToColumns:   []string{"id"},
								OnUpdate:    "SET NULL",
							},
						},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrInvalidOnUpdate {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrInvalidOnUpdate count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrConflictingPKSource(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "both field-level and table-level pk on different columns",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "slug", SQLType: "text"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"slug"}},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "field-level pk matches table-level pk",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantErrors: 0,
		},
		{
			name: "only field-level pk",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns: []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrConflictingPKSource {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrConflictingPKSource count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrSelfRefFKWithoutDefer(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
		wantWarn   bool
	}{
		{
			name: "self-referencing fk without on delete or on update",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "manager_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"manager_id"},
								ToTable:     "users",
								ToColumns:   []string{"id"},
							},
						},
					},
				},
			},
			wantErrors: 1,
			wantWarn:   true,
		},
		{
			name: "self-referencing fk with on delete",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "manager_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"manager_id"},
								ToTable:     "users",
								ToColumns:   []string{"id"},
								OnDelete:    "SET NULL",
							},
						},
					},
				},
			},
			wantErrors: 0,
			wantWarn:   false,
		},
		{
			name: "non-self-referencing fk without defer",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Organization",
						SearchPath: "public", Name: "orgs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "org_id", SQLType: "int"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						ForeignKeys: []schema.ForeignKey{
							{
								FromTable:   "users",
								FromColumns: []string{"org_id"},
								ToTable:     "orgs",
								ToColumns:   []string{"id"},
							},
						},
					},
				},
			},
			wantErrors: 0,
			wantWarn:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrSelfRefFKWithoutDefer {
					count++
					if tt.wantWarn && !e.IsWarn() {
						t.Errorf("ErrSelfRefFKWithoutDefer should be a warning, IsWarn() = false")
					}
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrSelfRefFKWithoutDefer count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrInvalidPartitionStrategy(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "invalid partition strategy",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Log",
						SearchPath: "public", Name: "logs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						Partitioned: &schema.Partition{
							Strategy: "INVALID",
							Column:   "created_at",
						},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "valid partition strategy range",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Log",
						SearchPath: "public", Name: "logs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						Partitioned: &schema.Partition{
							Strategy: "RANGE",
							Column:   "created_at",
						},
					},
				},
			},
			wantErrors: 0,
		},
		{
			name: "valid partition strategy list",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Log",
						SearchPath: "public", Name: "logs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						Partitioned: &schema.Partition{
							Strategy: "LIST",
							Column:   "region",
						},
					},
				},
			},
			wantErrors: 0,
		},
		{
			name: "valid partition strategy hash",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Log",
						SearchPath: "public", Name: "logs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						Partitioned: &schema.Partition{
							Strategy: "HASH",
							Column:   "id",
						},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrInvalidPartitionStrategy {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrInvalidPartitionStrategy count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrEmptyEnum(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "enum with no values",
			pkg: &schema.Package{
				Enums: []schema.Enum{
					{
						StructName: "EmptyStatus",
						SearchPath: "public", Name: "empty_status",
						Values: []string{},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "enum with values",
			pkg: &schema.Package{
				Enums: []schema.Enum{
					{
						StructName: "Status",
						SearchPath: "public", Name: "status",
						Values: []string{"active", "inactive"},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrEmptyEnum {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrEmptyEnum count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrDuplicateEnumValue(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "enum with duplicate values",
			pkg: &schema.Package{
				Enums: []schema.Enum{
					{
						StructName: "Status",
						SearchPath: "public", Name: "status",
						Values: []string{"active", "inactive", "active"},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "enum with unique values",
			pkg: &schema.Package{
				Enums: []schema.Enum{
					{
						StructName: "Status",
						SearchPath: "public", Name: "status",
						Values: []string{"active", "inactive", "pending"},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrDuplicateEnumValue {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrDuplicateEnumValue count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrIndexOnNonexistentCol(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "index references non-existent column",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "name", SQLType: "text"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						Indexes: []schema.Index{
							{Name: "idx_users_email", Columns: []string{"email"}},
						},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "index references existing column",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "email", SQLType: "text"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						Indexes: []schema.Index{
							{Name: "idx_users_email", Columns: []string{"email"}},
						},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrIndexOnNonexistentCol {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrIndexOnNonexistentCol count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_ErrUniqueOnNonexistentCol(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "unique constraint references non-existent column",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "name", SQLType: "text"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						Uniques: []schema.UniqueConstraint{
							{Name: "uq_users_email", Columns: []string{"email"}},
						},
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "unique constraint references existing column",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "email", SQLType: "text"}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						Uniques: []schema.UniqueConstraint{
							{Name: "uq_users_email", Columns: []string{"email"}},
						},
					},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrUniqueOnNonexistentCol {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrUniqueOnNonexistentCol count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidationError_IsError_IsWarn(t *testing.T) {
	tests := []struct {
		name      string
		code      ErrorCode
		wantError bool
		wantWarn  bool
	}{
		{
			name:      "ErrMissingPK is warn only",
			code:      ErrMissingPK,
			wantError: false,
			wantWarn:  true,
		},
		{
			name:      "ErrSelfRefFKWithoutDefer is warn only",
			code:      ErrSelfRefFKWithoutDefer,
			wantError: false,
			wantWarn:  true,
		},
		{
			name:      "ErrDuplicateTableName is error",
			code:      ErrDuplicateTableName,
			wantError: true,
			wantWarn:  false,
		},
		{
			name:      "ErrMissingFKTarget is error",
			code:      ErrMissingFKTarget,
			wantError: true,
			wantWarn:  false,
		},
		{
			name:      "ErrInvalidOnDelete is error",
			code:      ErrInvalidOnDelete,
			wantError: true,
			wantWarn:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := ValidationError{Code: tt.code}
			if ve.IsError() != tt.wantError {
				t.Errorf("IsError() = %v, want %v", ve.IsError(), tt.wantError)
			}
			if ve.IsWarn() != tt.wantWarn {
				t.Errorf("IsWarn() = %v, want %v", ve.IsWarn(), tt.wantWarn)
			}
		})
	}
}

func TestValidActions(t *testing.T) {
	valid := []string{"CASCADE", "SET NULL", "SET DEFAULT", "RESTRICT", "NO ACTION", ""}
	invalid := []string{"INVALID", "SET_ZERO", "DROP", "cascade", "Cascade"}

	for _, action := range valid {
		t.Run("valid_"+action, func(t *testing.T) {
			if !validActions[action] {
				t.Errorf("action %q should be valid", action)
			}
		})
	}

	for _, action := range invalid {
		t.Run("invalid_"+action, func(t *testing.T) {
			if validActions[action] {
				t.Errorf("action %q should be invalid", action)
			}
		})
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	pkg := &schema.Package{
		Tables: []schema.Table{
			{
				StructName: "User",
				SearchPath: "public", Name: "users",
				Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}, {Name: "status", SQLType: "unknown_enum"}},
				PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
				Indexes: []schema.Index{
					{Name: "idx_users_fake", Columns: []string{"fake_col"}},
				},
				Uniques: []schema.UniqueConstraint{
					{Name: "uq_users_fake", Columns: []string{"fake_col"}},
				},
			},
		},
		Enums: []schema.Enum{
			{
				StructName: "EmptyEnum",
				SearchPath: "public", Name: "empty_enum",
				Values: []string{},
			},
		},
	}

	errs := Validate(pkg)

	codeSet := make(map[ErrorCode]bool)
	for _, e := range errs {
		codeSet[e.Code] = true
	}

	expectedCodes := []ErrorCode{
		ErrUnresolvedEnumRef,
		ErrIndexOnNonexistentCol,
		ErrUniqueOnNonexistentCol,
		ErrEmptyEnum,
	}

	for _, code := range expectedCodes {
		if !codeSet[code] {
			t.Errorf("expected error code %q not found", code)
		}
	}
}

func TestValidate_ColumnLevelFK(t *testing.T) {
	tests := []struct {
		name      string
		pkg       *schema.Package
		wantCodes []ErrorCode
		wantCount int
	}{
		{
			name: "column-level fk with invalid on delete",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Organization",
						SearchPath: "public", Name: "orgs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns: []*schema.Column{
							{Name: "id", SQLType: "int", PrimaryKey: true},
							{
								Name:                "org_id",
								SQLType:             "int",
								ReferenceTableName:  "orgs",
								ReferenceColumnName: "id",
								ReferenceOnDelete:   "INVALID",
							},
						},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantCodes: []ErrorCode{ErrInvalidOnDelete},
			wantCount: 1,
		},
		{
			name: "column-level self-ref fk without defer",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User",
						SearchPath: "public", Name: "users",
						Columns: []*schema.Column{
							{Name: "id", SQLType: "int", PrimaryKey: true},
							{
								Name:                "manager_id",
								TableName:           "users",
								SQLType:             "int",
								ReferenceTableName:  "users",
								ReferenceColumnName: "id",
							},
						},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
			},
			wantCodes: []ErrorCode{ErrSelfRefFKWithoutDefer},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			codeSet := make(map[ErrorCode]int)
			for _, e := range errs {
				codeSet[e.Code]++
			}
			for _, code := range tt.wantCodes {
				if codeSet[code] != tt.wantCount {
					t.Errorf("expected %d occurrences of %q, got %d", tt.wantCount, code, codeSet[code])
				}
			}
		})
	}
}

func TestValidate_CaseInsensitivePartitionStrategy(t *testing.T) {
	tests := []struct {
		name       string
		strategy   string
		wantErrors int
	}{
		{"lowercase range", "range", 0},
		{"lowercase list", "list", 0},
		{"lowercase hash", "hash", 0},
		{"mixed case Range", "Range", 0},
		{"invalid strategy", "BY_RANGE", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "Log",
						SearchPath: "public", Name: "logs",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
						Partitioned: &schema.Partition{
							Strategy: tt.strategy,
							Column:   "created_at",
						},
					},
				},
			}
			errs := Validate(pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrInvalidPartitionStrategy {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("strategy %q: ErrInvalidPartitionStrategy count = %d, want %d", tt.strategy, count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_EmptyPackage(t *testing.T) {
	pkg := &schema.Package{}
	errs := Validate(pkg)
	if len(errs) != 0 {
		t.Errorf("expected zero errors for empty package, got %d: %v", len(errs), errs)
	}
}

func TestValidateNotNullNoDefault(t *testing.T) {
	oldTables := map[string]*schema.Table{
		"users": {StructName: "User", SearchPath: "public", Name: "users"},
	}
	adds := []ColumnAddition{
		{Table: "users", Column: "tenant_id", NewColumn: &schema.Column{Name: "tenant_id", SQLType: "bigint", Nullable: false}},
	}
	warns := ValidateNotNullNoDefault(adds, oldTables)
	if len(warns) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warns))
	}
	if warns[0].Code != ErrNotNullNoDefault {
		t.Errorf("wrong code: %s", warns[0].Code)
	}
	if !warns[0].IsWarn() {
		t.Error("expected warning severity")
	}
}

func TestValidateNotNullNoDefault_NoWarnForNewTable(t *testing.T) {
	adds := []ColumnAddition{
		{Table: "users", Column: "tenant_id", NewColumn: &schema.Column{Name: "tenant_id", SQLType: "bigint", Nullable: false}},
	}
	warns := ValidateNotNullNoDefault(adds, map[string]*schema.Table{})
	if len(warns) != 0 {
		t.Fatalf("expected 0 warnings for new table, got %d", len(warns))
	}
}

func TestValidateNotNullNoDefault_NoWarnWithDefault(t *testing.T) {
	oldTables := map[string]*schema.Table{
		"users": {StructName: "User", SearchPath: "public", Name: "users"},
	}
	adds := []ColumnAddition{
		{Table: "users", Column: "tenant_id", NewColumn: &schema.Column{Name: "tenant_id", SQLType: "bigint", Nullable: false, Default: "0"}},
	}
	warns := ValidateNotNullNoDefault(adds, oldTables)
	if len(warns) != 0 {
		t.Fatalf("expected 0 warnings with default, got %d", len(warns))
	}
}

func TestValidateNotNullNoDefault_NoWarnWithGenerated(t *testing.T) {
	oldTables := map[string]*schema.Table{
		"users": {StructName: "User", SearchPath: "public", Name: "users"},
	}
	adds := []ColumnAddition{
		{Table: "users", Column: "full_name", NewColumn: &schema.Column{Name: "full_name", SQLType: "text", Nullable: false, AutoGenerated: true, GeneratedExpression: "always"}},
	}
	warns := ValidateNotNullNoDefault(adds, oldTables)
	if len(warns) != 0 {
		t.Fatalf("expected 0 warnings with generated column, got %d", len(warns))
	}
}

func TestValidate_EmptyFunctionBody(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "function with empty body",
			pkg: &schema.Package{
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_empty", Language: "plpgsql", Body: ""},
				},
			},
			wantErrors: 1,
		},
		{
			name: "function with non-empty body",
			pkg: &schema.Package{
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_ok", Language: "plpgsql", Body: "BEGIN RETURN 1; END;"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrEmptyFunctionBody {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrEmptyFunctionBody count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_FunctionMissingLanguage(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "function with empty language",
			pkg: &schema.Package{
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_nolang", Body: "BEGIN RETURN 1; END;", Language: ""},
				},
			},
			wantErrors: 1,
		},
		{
			name: "function with language set",
			pkg: &schema.Package{
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_ok", Body: "BEGIN RETURN 1; END;", Language: "plpgsql"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrFunctionMissingLang {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrFunctionMissingLang count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_DuplicateFunctionName(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "two functions with same name",
			pkg: &schema.Package{
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_dup", Language: "plpgsql", Body: "BEGIN RETURN 1; END;"},
					{SearchPath: "public", Name: "fn_dup", Language: "plpgsql", Body: "BEGIN RETURN 2; END;"},
				},
			},
			wantErrors: 1,
		},
		{
			name: "two functions with different names",
			pkg: &schema.Package{
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_a", Language: "plpgsql", Body: "BEGIN RETURN 1; END;"},
					{SearchPath: "public", Name: "fn_b", Language: "plpgsql", Body: "BEGIN RETURN 2; END;"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrDuplicateFunctionName {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrDuplicateFunctionName count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_EmptyViewQuery(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "view with empty query",
			pkg: &schema.Package{
				Views: []schema.View{
					{SearchPath: "public", Name: "v_empty", Query: ""},
				},
			},
			wantErrors: 1,
		},
		{
			name: "view with non-empty query",
			pkg: &schema.Package{
				Views: []schema.View{
					{SearchPath: "public", Name: "v_ok", Query: "SELECT id FROM users"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrEmptyViewQuery {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrEmptyViewQuery count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_EmptyMatViewQuery(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "materialized view with empty query",
			pkg: &schema.Package{
				MaterializedViews: []schema.MaterializedView{
					{SearchPath: "public", Name: "mv_empty", Query: ""},
				},
			},
			wantErrors: 1,
		},
		{
			name: "materialized view with non-empty query",
			pkg: &schema.Package{
				MaterializedViews: []schema.MaterializedView{
					{SearchPath: "public", Name: "mv_ok", Query: "SELECT id FROM users"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrEmptyMatViewQuery {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrEmptyMatViewQuery count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_TriggerMissingFunction(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "trigger references non-existent function",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User", SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Triggers: []schema.Trigger{
					{SearchPath: "public", Name: "trg_users", Table: "users", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "fn_x"},
				},
			},
			wantErrors: 1,
		},
		{
			name: "trigger references existing function",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User", SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_x", Language: "plpgsql", Body: "BEGIN RETURN NEW; END;"},
				},
				Triggers: []schema.Trigger{
					{SearchPath: "public", Name: "trg_users", Table: "users", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "fn_x"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrTriggerMissingFunction {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrTriggerMissingFunction count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_TriggerMissingTable(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "trigger references non-existent table",
			pkg: &schema.Package{
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_audit", Language: "plpgsql", Body: "BEGIN RETURN NEW; END;"},
				},
				Triggers: []schema.Trigger{
					{SearchPath: "public", Name: "trg_audit", Table: "nonexistent", Timing: "AFTER", Events: []string{"INSERT"}, Function: "fn_audit"},
				},
			},
			wantErrors: 1,
		},
		{
			name: "trigger references existing table",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User", SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_audit", Language: "plpgsql", Body: "BEGIN RETURN NEW; END;"},
				},
				Triggers: []schema.Trigger{
					{SearchPath: "public", Name: "trg_audit", Table: "users", Timing: "AFTER", Events: []string{"INSERT"}, Function: "fn_audit"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrTriggerMissingTable {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrTriggerMissingTable count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_TriggerInvalidTiming(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "trigger with invalid timing",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User", SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_trg", Language: "plpgsql", Body: "BEGIN RETURN NEW; END;"},
				},
				Triggers: []schema.Trigger{
					{SearchPath: "public", Name: "trg_users", Table: "users", Timing: "INVALID", Events: []string{"INSERT"}, Function: "fn_trg"},
				},
			},
			wantErrors: 1,
		},
		{
			name: "trigger with valid timing",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User", SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_trg", Language: "plpgsql", Body: "BEGIN RETURN NEW; END;"},
				},
				Triggers: []schema.Trigger{
					{SearchPath: "public", Name: "trg_users", Table: "users", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "fn_trg"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrTriggerInvalidTiming {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrTriggerInvalidTiming count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_TriggerInvalidEvents(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "trigger with invalid event",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User", SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_trg", Language: "plpgsql", Body: "BEGIN RETURN NEW; END;"},
				},
				Triggers: []schema.Trigger{
					{SearchPath: "public", Name: "trg_users", Table: "users", Timing: "BEFORE", Events: []string{"INVALID"}, Function: "fn_trg"},
				},
			},
			wantErrors: 1,
		},
		{
			name: "trigger with valid events",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User", SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Functions: []schema.Function{
					{SearchPath: "public", Name: "fn_trg", Language: "plpgsql", Body: "BEGIN RETURN NEW; END;"},
				},
				Triggers: []schema.Trigger{
					{SearchPath: "public", Name: "trg_users", Table: "users", Timing: "BEFORE", Events: []string{"INSERT", "UPDATE"}, Function: "fn_trg"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrTriggerInvalidEvents {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrTriggerInvalidEvents count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_GrantInvalidObjectType(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "grant with invalid object type",
			pkg: &schema.Package{
				Grants: []schema.Grant{
					{SearchPath: "public", ObjectType: "invalid", ObjectName: "users", Privileges: []string{"SELECT"}, Roles: []string{"app_user"}},
				},
			},
			wantErrors: 1,
		},
		{
			name: "grant with valid object type",
			pkg: &schema.Package{
				Grants: []schema.Grant{
					{SearchPath: "public", ObjectType: "table", ObjectName: "users", Privileges: []string{"SELECT"}, Roles: []string{"app_user"}},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrGrantInvalidObjectType {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrGrantInvalidObjectType count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}

func TestValidate_PolicyMissingTable(t *testing.T) {
	tests := []struct {
		name       string
		pkg        *schema.Package
		wantErrors int
	}{
		{
			name: "policy references non-existent table",
			pkg: &schema.Package{
				Policies: []schema.Policy{
					{SearchPath: "public", Name: "pol_users", Table: "nonexistent", Command: "ALL"},
				},
			},
			wantErrors: 1,
		},
		{
			name: "policy references existing table",
			pkg: &schema.Package{
				Tables: []schema.Table{
					{
						StructName: "User", SearchPath: "public", Name: "users",
						Columns:    []*schema.Column{{Name: "id", SQLType: "int", PrimaryKey: true}},
						PrimaryKey: &schema.PrimaryKey{Columns: []string{"id"}},
					},
				},
				Policies: []schema.Policy{
					{SearchPath: "public", Name: "pol_users", Table: "users", Command: "ALL"},
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.pkg)
			count := 0
			for _, e := range errs {
				if e.Code == ErrPolicyMissingTable {
					count++
				}
			}
			if count != tt.wantErrors {
				t.Errorf("ErrPolicyMissingTable count = %d, want %d", count, tt.wantErrors)
			}
		})
	}
}
