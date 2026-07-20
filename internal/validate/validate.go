package validate

import (
	"strings"

	"github.com/justblue/mirage/internal/schema"
)

type ErrorCode string

const (
	ErrDuplicateTableName       ErrorCode = "duplicate_table_name"
	ErrDuplicateColumnName      ErrorCode = "duplicate_column_name"
	ErrDuplicateConstraintName  ErrorCode = "duplicate_constraint_name"
	ErrMissingFKTarget          ErrorCode = "missing_fk_target_table"
	ErrMissingFKColumn          ErrorCode = "missing_fk_target_column"
	ErrUnresolvedEnumRef        ErrorCode = "unresolved_enum_reference"
	ErrMissingPK                ErrorCode = "missing_primary_key"
	ErrInvalidOnDelete          ErrorCode = "invalid_on_delete_action"
	ErrInvalidOnUpdate          ErrorCode = "invalid_on_update_action"
	ErrConflictingPKSource      ErrorCode = "conflicting_pk_source"
	ErrSelfRefFKWithoutDefer    ErrorCode = "self_ref_fk_without_defer"
	ErrInvalidPartitionStrategy ErrorCode = "invalid_partition_strategy"
	ErrEmptyEnum                ErrorCode = "empty_enum_values"
	ErrDuplicateEnumValue       ErrorCode = "duplicate_enum_value"
	ErrIndexOnNonexistentCol    ErrorCode = "index_on_nonexistent_column"
	ErrUniqueOnNonexistentCol   ErrorCode = "unique_on_nonexistent_column"
	ErrNotNullNoDefault         ErrorCode = "not_null_no_default"

	ErrDuplicateFunctionName  ErrorCode = "duplicate_function_name"
	ErrDuplicateTriggerName   ErrorCode = "duplicate_trigger_name"
	ErrDuplicatePolicyName    ErrorCode = "duplicate_policy_name"
	ErrEmptyViewQuery         ErrorCode = "empty_view_query"
	ErrEmptyMatViewQuery      ErrorCode = "empty_materialized_view_query"
	ErrEmptyFunctionBody      ErrorCode = "empty_function_body"
	ErrFunctionMissingLang    ErrorCode = "function_missing_language"
	ErrTriggerMissingFunction ErrorCode = "trigger_missing_function"
	ErrTriggerMissingTable    ErrorCode = "trigger_missing_table"
	ErrTriggerInvalidTiming   ErrorCode = "trigger_invalid_timing"
	ErrTriggerInvalidEvents   ErrorCode = "trigger_invalid_events"
	ErrGrantInvalidObjectType ErrorCode = "grant_invalid_object_type"
	ErrPolicyMissingTable     ErrorCode = "policy_missing_table"
)

var warnOnlyCodes = map[ErrorCode]bool{
	ErrMissingPK:             true,
	ErrSelfRefFKWithoutDefer: true,
	ErrNotNullNoDefault:      true,
}

var validActions = map[string]bool{
	"": true, "CASCADE": true, "SET NULL": true, "SET DEFAULT": true, "RESTRICT": true, "NO ACTION": true,
}

type ValidationError struct {
	Code    ErrorCode
	Message string
	Table   string
	Column  string
}

func (e ValidationError) IsError() bool {
	return !warnOnlyCodes[e.Code]
}

func (e ValidationError) IsWarn() bool {
	return !e.IsError()
}

func Validate(pkg *schema.Package) []ValidationError {
	var errs []ValidationError

	tableSet := make(map[string]*schema.Table)
	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		key := t.SQLName()

		if existing, ok := tableSet[key]; ok {
			errs = append(errs, ValidationError{
				Code:    ErrDuplicateTableName,
				Message: "table " + key + " defined multiple times (also at " + existing.StructName + ")",
				Table:   key,
			})
		}
		tableSet[key] = t
	}

	colNameSet := make(map[string]bool)
	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		clear(colNameSet)
		for _, c := range t.Columns {
			if colNameSet[c.Name] {
				errs = append(errs, ValidationError{
					Code:    ErrDuplicateColumnName,
					Message: "column " + c.Name + " defined multiple times in table " + t.SQLName(),
					Table:   t.SQLName(),
					Column:  c.Name,
				})
			}
			colNameSet[c.Name] = true
		}
	}

	constraintNames := make(map[string]string)
	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		if t.PrimaryKey != nil && t.PrimaryKey.Name != "" {
			if prev, ok := constraintNames[t.PrimaryKey.Name]; ok {
				errs = append(errs, ValidationError{
					Code:    ErrDuplicateConstraintName,
					Message: "constraint " + t.PrimaryKey.Name + " defined on both " + prev + " and " + t.SQLName(),
					Table:   t.SQLName(),
				})
			}
			constraintNames[t.PrimaryKey.Name] = t.SQLName()
		}
		for _, fk := range t.ForeignKeys {
			if fk.Name != "" {
				if prev, ok := constraintNames[fk.Name]; ok {
					errs = append(errs, ValidationError{
						Code:    ErrDuplicateConstraintName,
						Message: "constraint " + fk.Name + " defined on both " + prev + " and " + t.SQLName(),
						Table:   t.SQLName(),
					})
				}
				constraintNames[fk.Name] = t.SQLName()
			}
		}
		for _, uq := range t.Uniques {
			if uq.Name != "" {
				if prev, ok := constraintNames[uq.Name]; ok {
					errs = append(errs, ValidationError{
						Code:    ErrDuplicateConstraintName,
						Message: "constraint " + uq.Name + " defined on both " + prev + " and " + t.SQLName(),
						Table:   t.SQLName(),
					})
				}
				constraintNames[uq.Name] = t.SQLName()
			}
		}
		for _, idx := range t.Indexes {
			if idx.Name != "" {
				if prev, ok := constraintNames[idx.Name]; ok {
					errs = append(errs, ValidationError{
						Code:    ErrDuplicateConstraintName,
						Message: "constraint " + idx.Name + " defined on both " + prev + " and " + t.SQLName(),
						Table:   t.SQLName(),
					})
				}
				constraintNames[idx.Name] = t.SQLName()
			}
		}
		for _, chk := range t.Checks {
			if chk.Name != "" {
				if prev, ok := constraintNames[chk.Name]; ok {
					errs = append(errs, ValidationError{
						Code:    ErrDuplicateConstraintName,
						Message: "constraint " + chk.Name + " defined on both " + prev + " and " + t.SQLName(),
						Table:   t.SQLName(),
					})
				}
				constraintNames[chk.Name] = t.SQLName()
			}
		}
	}

	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		for _, fk := range t.ForeignKeys {
			if _, ok := tableSet[fk.ToTable]; !ok {
				errs = append(errs, ValidationError{
					Code:    ErrMissingFKTarget,
					Message: "references table " + fk.ToTable + " which does not exist",
					Table:   t.SQLName(),
				})
			}
		}
		for _, c := range t.Columns {
			if c.ReferenceTableName != "" {
				if _, ok := tableSet[c.ReferenceTableName]; !ok {
					errs = append(errs, ValidationError{
						Code:    ErrMissingFKTarget,
						Message: "references table " + c.ReferenceTableName + " which does not exist",
						Table:   t.SQLName(),
						Column:  c.Name,
					})
				} else {
					targetTable := tableSet[c.ReferenceTableName]
					toCol := c.ReferenceColumnName
					found := false
					for _, tc := range targetTable.Columns {
						if tc.Name == toCol {
							found = true
							break
						}
					}
					if !found {
						errs = append(errs, ValidationError{
							Code:    ErrMissingFKColumn,
							Message: "references column " + toCol + " which does not exist in table " + c.ReferenceTableName,
							Table:   t.SQLName(),
							Column:  c.Name,
						})
					}
				}
			}
		}
	}

	enumMap := make(map[string]*schema.Enum)
	for i := range pkg.Enums {
		e := &pkg.Enums[i]
		enumMap[e.Name] = e
		enumMap[e.SQLName()] = e
	}

	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		for _, c := range t.Columns {
			if _, ok := enumMap[c.SQLType]; ok {
				continue
			}
			if looksLikeEnumRef(c.SQLType) {
				errs = append(errs, ValidationError{
					Code:    ErrUnresolvedEnumRef,
					Message: "column " + c.Name + " references enum type " + c.SQLType + " which does not exist",
					Table:   t.SQLName(),
					Column:  c.Name,
				})
			}
		}
	}

	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		for _, fk := range t.ForeignKeys {
			if !validActions[fk.OnDelete] {
				errs = append(errs, ValidationError{
					Code:    ErrInvalidOnDelete,
					Message: "invalid ON DELETE action: " + fk.OnDelete,
					Table:   t.SQLName(),
				})
			}
			if !validActions[fk.OnUpdate] {
				errs = append(errs, ValidationError{
					Code:    ErrInvalidOnUpdate,
					Message: "invalid ON UPDATE action: " + fk.OnUpdate,
					Table:   t.SQLName(),
				})
			}
		}
		for _, c := range t.Columns {
			if c.ReferenceTableName != "" {
				if !validActions[c.ReferenceOnDelete] {
					errs = append(errs, ValidationError{
						Code:    ErrInvalidOnDelete,
						Message: "invalid ON DELETE action: " + c.ReferenceOnDelete,
						Table:   t.SQLName(),
						Column:  c.Name,
					})
				}
				if !validActions[c.ReferenceOnUpdate] {
					errs = append(errs, ValidationError{
						Code:    ErrInvalidOnUpdate,
						Message: "invalid ON UPDATE action: " + c.ReferenceOnUpdate,
						Table:   t.SQLName(),
						Column:  c.Name,
					})
				}
			}
		}
	}

	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		hasFieldPK := false
		for _, c := range t.Columns {
			if c.PrimaryKey {
				hasFieldPK = true
				break
			}
		}
		if hasFieldPK && t.PrimaryKey != nil && len(t.PrimaryKey.Columns) > 0 {
			tableLevelPK := false
			for _, c := range t.Columns {
				if c.PrimaryKey {
					for _, pkCol := range t.PrimaryKey.Columns {
						if c.Name == pkCol {
							tableLevelPK = true
							break
						}
					}
				}
			}
			if !tableLevelPK {
				errs = append(errs, ValidationError{
					Code:    ErrConflictingPKSource,
					Message: "table has both field-level pk and table-level pk",
					Table:   t.SQLName(),
				})
			}
		}
	}

	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		colSet := make(map[string]bool)
		for _, c := range t.Columns {
			colSet[c.Name] = true
		}
		for _, uq := range t.Uniques {
			for _, col := range uq.Columns {
				if !colSet[col] {
					errs = append(errs, ValidationError{
						Code:    ErrUniqueOnNonexistentCol,
						Message: "unique constraint references non-existent column " + col,
						Table:   t.SQLName(),
					})
				}
			}
		}
		for _, idx := range t.Indexes {
			for _, col := range idx.Columns {
				if !colSet[col] {
					errs = append(errs, ValidationError{
						Code:    ErrIndexOnNonexistentCol,
						Message: "index references non-existent column " + col,
						Table:   t.SQLName(),
					})
				}
			}
		}
	}

	for i := range pkg.Enums {
		e := &pkg.Enums[i]
		if len(e.Values) == 0 {
			errs = append(errs, ValidationError{
				Code:    ErrEmptyEnum,
				Message: "enum " + e.SQLName() + " has no values",
			})
		}
		seen := make(map[string]bool)
		for _, v := range e.Values {
			if seen[v] {
				errs = append(errs, ValidationError{
					Code:    ErrDuplicateEnumValue,
					Message: "duplicate enum value " + v + " in " + e.SQLName(),
				})
			}
			seen[v] = true
		}
	}

	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		if t.Partitioned != nil {
			s := strings.ToUpper(t.Partitioned.Strategy)
			if s != "RANGE" && s != "LIST" && s != "HASH" {
				errs = append(errs, ValidationError{
					Code:    ErrInvalidPartitionStrategy,
					Message: "invalid partition strategy: " + t.Partitioned.Strategy,
					Table:   t.SQLName(),
				})
			}
		}
	}

	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		if t.PrimaryKey == nil {
			errs = append(errs, ValidationError{
				Code:    ErrMissingPK,
				Message: "no primary key defined",
				Table:   t.SQLName(),
			})
		}
	}

	for i := range pkg.Tables {
		t := &pkg.Tables[i]
		for _, fk := range t.ForeignKeys {
			if fk.FromTable == fk.ToTable {
				if fk.OnDelete == "" && fk.OnUpdate == "" {
					errs = append(errs, ValidationError{
						Code:    ErrSelfRefFKWithoutDefer,
						Message: "self-referencing FK without DEFERRABLE",
						Table:   t.SQLName(),
					})
				}
			}
		}
		for _, c := range t.Columns {
			if c.ReferenceTableName != "" && c.TableName == c.ReferenceTableName {
				if c.ReferenceOnDelete == "" && c.ReferenceOnUpdate == "" {
					errs = append(errs, ValidationError{
						Code:    ErrSelfRefFKWithoutDefer,
						Message: "self-referencing FK without DEFERRABLE",
						Table:   t.SQLName(),
						Column:  c.Name,
					})
				}
			}
		}
	}

	// --- Registered Objects ---

	// Functions
	funcNames := make(map[string]bool)
	for _, fn := range pkg.Functions {
		name := fn.SQLName()
		if funcNames[name] {
			errs = append(errs, ValidationError{
				Code:    ErrDuplicateFunctionName,
				Message: "function " + name + " defined multiple times",
			})
		}
		funcNames[name] = true
		if fn.Body == "" {
			errs = append(errs, ValidationError{
				Code:    ErrEmptyFunctionBody,
				Message: "function " + name + " has empty body",
			})
		}
		if fn.Language == "" {
			errs = append(errs, ValidationError{
				Code:    ErrFunctionMissingLang,
				Message: "function " + name + " has no language specified",
			})
		}
	}

	// Views
	for _, v := range pkg.Views {
		if v.Query == "" {
			errs = append(errs, ValidationError{
				Code:    ErrEmptyViewQuery,
				Message: "view " + v.SQLName() + " has empty query",
			})
		}
	}

	// Materialized Views
	for _, mv := range pkg.MaterializedViews {
		if mv.Query == "" {
			errs = append(errs, ValidationError{
				Code:    ErrEmptyMatViewQuery,
				Message: "materialized view " + mv.SQLName() + " has empty query",
			})
		}
	}

	// Triggers
	validTriggerTiming := map[string]bool{"BEFORE": true, "AFTER": true, "INSTEAD OF": true}
	validTriggerEvents := map[string]bool{"INSERT": true, "UPDATE": true, "DELETE": true, "TRUNCATE": true}
	for _, t := range pkg.Triggers {
		// Check function exists
		if !funcNames[t.Function] {
			found := false
			for fname := range funcNames {
				if strings.HasSuffix(fname, "."+t.Function) || fname == t.Function {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, ValidationError{
					Code:    ErrTriggerMissingFunction,
					Message: "trigger " + t.Name + " references function " + t.Function + " which does not exist",
					Table:   t.Table,
				})
			}
		}
		// Check table exists
		if _, ok := tableSet[t.Table]; !ok {
			errs = append(errs, ValidationError{
				Code:    ErrTriggerMissingTable,
				Message: "trigger " + t.Name + " references table " + t.Table + " which does not exist",
				Table:   t.Table,
			})
		}
		// Validate timing
		if !validTriggerTiming[strings.ToUpper(t.Timing)] {
			errs = append(errs, ValidationError{
				Code:    ErrTriggerInvalidTiming,
				Message: "trigger " + t.Name + " has invalid timing: " + t.Timing,
				Table:   t.Table,
			})
		}
		// Validate events
		for _, event := range t.Events {
			if !validTriggerEvents[strings.ToUpper(event)] {
				errs = append(errs, ValidationError{
					Code:    ErrTriggerInvalidEvents,
					Message: "trigger " + t.Name + " has invalid event: " + event,
					Table:   t.Table,
				})
			}
		}
	}

	// Grants
	validGrantObjectTypes := map[string]bool{
		"table": true, "view": true, "sequence": true,
		"function": true, "procedure": true, "schema": true,
	}
	for _, g := range pkg.Grants {
		if !validGrantObjectTypes[strings.ToLower(g.ObjectType)] {
			errs = append(errs, ValidationError{
				Code:    ErrGrantInvalidObjectType,
				Message: "grant on " + g.ObjectName + " has invalid object type: " + g.ObjectType,
			})
		}
	}

	// Policies
	for _, pol := range pkg.Policies {
		if _, ok := tableSet[pol.Table]; !ok {
			errs = append(errs, ValidationError{
				Code:    ErrPolicyMissingTable,
				Message: "policy " + pol.Name + " references table " + pol.Table + " which does not exist",
				Table:   pol.Table,
			})
		}
	}

	return errs
}

func looksLikeEnumRef(sqlType string) bool {
	if sqlType == "" {
		return false
	}
	if strings.Contains(sqlType, "(") || strings.Contains(sqlType, " ") {
		return false
	}
	// Strip PostgreSQL array suffix (e.g. text[] → text) before checking known schema.
	base := strings.ToLower(strings.TrimSuffix(sqlType, "[]"))
	knownTypes := map[string]bool{
		"int": true, "integer": true, "bigint": true, "smallint": true,
		"serial": true, "bigserial": true, "smallserial": true,
		"varchar": true, "char": true, "text": true, "uuid": true,
		"bool": true, "boolean": true,
		"date": true, "time": true, "timestamp": true, "timestamptz": true, "datetime": true,
		"interval": true,
		"real":     true, "double": true, "numeric": true, "decimal": true,
		"json": true, "jsonb": true, "xml": true,
		"bytea": true, "inet": true, "cidr": true, "macaddr": true,
		"money": true, "bit": true, "varbit": true,
		"tsvector": true, "tsquery": true,
	}
	return !knownTypes[base]
}

type ColumnAddition struct {
	Table     string
	Column    string
	NewColumn *schema.Column
}

func ValidateNotNullNoDefault(adds []ColumnAddition, existingTables map[string]*schema.Table) []ValidationError {
	var warns []ValidationError
	for _, a := range adds {
		if _, exists := existingTables[a.Table]; !exists {
			continue
		}
		if a.NewColumn != nil && !a.NewColumn.Nullable && a.NewColumn.Default == "" && a.NewColumn.GeneratedExpression == "" {
			warns = append(warns, ValidationError{
				Code:    ErrNotNullNoDefault,
				Message: "column \"" + a.Column + "\" on table \"" + a.Table + "\" is NOT NULL with no default; this will fail on populated tables",
				Table:   a.Table,
				Column:  a.Column,
			})
		}
	}
	return warns
}
