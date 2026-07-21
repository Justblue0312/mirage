package introspect

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/justblue/mirage/internal/schema"
)

// FromLiveDatabase builds a schema.Package by reading the live database's
// catalog tables (information_schema + pg_catalog), rather than parsing
// Go source. The result is diff-compatible with a scanner-produced
// Package: feed both into diff.Diff to get a drift report using the
// exact same engine used for migration generation.
func FromLiveDatabase(ctx context.Context, pool *pgxpool.Pool, searchPath string) (*schema.Package, error) {
	pkg := &schema.Package{}

	if err := introspectTables(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting tables: %w", err)
	}
	if err := introspectPrimaryKeys(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting primary keys: %w", err)
	}
	if err := introspectForeignKeys(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting foreign keys: %w", err)
	}
	if err := introspectUniqueConstraints(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting unique constraints: %w", err)
	}
	if err := introspectIndexes(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting indexes: %w", err)
	}
	if err := introspectCheckConstraints(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting check constraints: %w", err)
	}
	if err := introspectEnums(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting enums: %w", err)
	}
	if err := introspectExtensions(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting extensions: %w", err)
	}
	if err := introspectViews(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting views: %w", err)
	}
	if err := introspectMaterializedViews(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting materialized views: %w", err)
	}
	if err := introspectFunctions(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting functions: %w", err)
	}
	if err := introspectProcedures(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting procedures: %w", err)
	}
	if err := introspectTriggers(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting triggers: %w", err)
	}
	if err := introspectGrants(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting grants: %w", err)
	}
	if err := introspectPolicies(ctx, pool, searchPath, pkg); err != nil {
		return nil, fmt.Errorf("introspecting policies: %w", err)
	}

	return pkg, nil
}

// introspectTables reads all base tables and their columns from
// information_schema.columns.
func introspectTables(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		c.table_name, c.column_name, c.ordinal_position,
		c.data_type, c.udt_name, c.character_maximum_length,
		c.numeric_precision, c.numeric_scale,
		c.is_nullable, c.column_default,
		CASE WHEN c.is_identity = 'YES' THEN true ELSE false END AS is_identity,
		tobj.table_type
	FROM information_schema.columns c
	JOIN information_schema.tables tobj
		ON c.table_schema = tobj.table_schema AND c.table_name = tobj.table_name
	WHERE c.table_schema = $1
	ORDER BY c.table_name, c.ordinal_position`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	type tableBuilder struct {
		table   schema.Table
		columns []*schema.Column
	}
	tables := make(map[string]*tableBuilder)

	for rows.Next() {
		var (
			tableName, colName     string
			ordinalPos             int
			dataType, udtName      string
			charMaxLen             *int
			numericPrec, numericSc *int
			isNullable             string
			colDefault             *string
			isIdentity             bool
			tableType              string
		)
		if err := rows.Scan(
			&tableName, &colName, &ordinalPos,
			&dataType, &udtName, &charMaxLen,
			&numericPrec, &numericSc,
			&isNullable, &colDefault,
			&isIdentity, &tableType,
		); err != nil {
			return fmt.Errorf("scanning column %s.%s: %w", tableName, colName, err)
		}

		tb, ok := tables[tableName]
		if !ok {
			tt := schema.TableTypeBase
			if tableType == "VIEW" {
				tt = schema.TableTypeView
			}
			tb = &tableBuilder{
				table: schema.Table{
					SearchPath: searchPath,
					Name:       tableName,
					Type:       tt,
				},
			}
			tables[tableName] = tb
		}

		cml := 0
		if charMaxLen != nil {
			cml = *charMaxLen
		}

		col := &schema.Column{
			TableName:       tableName,
			Name:            colName,
			OrdinalPosition: ordinalPos,
			Type:            mapCatalogDataType(udtName, dataType),
			SQLType:         mapCatalogType(udtName, dataType, cml),
			Nullable:        isNullable == "YES",
			Identity:        isIdentity,
		}

		if colDefault != nil {
			col.Default = *colDefault
			// Detect identity columns by their default expression
			if strings.Contains(*colDefault, "nextval(") {
				col.Identity = true
			}
		}

		tb.columns = append(tb.columns, col)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	// Sort columns within each table by ordinal position, then append to pkg.
	for _, tb := range tables {
		sort.Slice(tb.columns, func(i, j int) bool {
			return tb.columns[i].OrdinalPosition < tb.columns[j].OrdinalPosition
		})
		tb.table.Columns = tb.columns
		pkg.Tables = append(pkg.Tables, tb.table)
	}

	// Sort tables alphabetically.
	sort.Slice(pkg.Tables, func(i, j int) bool {
		return pkg.Tables[i].Name < pkg.Tables[j].Name
	})

	return nil
}

// introspectPrimaryKeys reads all primary key constraints from pg_constraint.
func introspectPrimaryKeys(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		tc.constraint_name, tc.table_name,
		ARRAY_AGG(kcu.column_name ORDER BY kcu.ordinal_position) AS columns
	FROM information_schema.table_constraints tc
	JOIN information_schema.key_column_usage kcu
		ON tc.constraint_name = kcu.constraint_name
		AND tc.table_schema = kcu.table_schema
	WHERE tc.constraint_type = 'PRIMARY KEY'
		AND tc.table_schema = $1
	GROUP BY tc.constraint_name, tc.table_name`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var constraintName, tableName string
		var columns []string
		if err := rows.Scan(&constraintName, &tableName, &columns); err != nil {
			return err
		}
		if idx := findTable(pkg.Tables, tableName); idx >= 0 {
			pkg.Tables[idx].PrimaryKey = &schema.PrimaryKey{
				Name:    constraintName,
				Columns: columns,
			}
		}
	}
	return rows.Err()
}

// introspectForeignKeys reads all foreign key constraints from
// information_schema.
func introspectForeignKeys(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		tc.constraint_name, tc.table_name,
		ARRAY_AGG(DISTINCT kcu.column_name ORDER BY kcu.column_name) AS from_columns,
		ccu.table_name AS to_table,
		ARRAY_AGG(DISTINCT ccu.column_name ORDER BY ccu.column_name) AS to_columns,
		rc.update_rule, rc.delete_rule
	FROM information_schema.table_constraints tc
	JOIN information_schema.key_column_usage kcu
		ON tc.constraint_name = kcu.constraint_name
		AND tc.table_schema = kcu.table_schema
	JOIN information_schema.constraint_column_usage ccu
		ON tc.constraint_name = ccu.constraint_name
		AND tc.table_schema = ccu.table_schema
	JOIN information_schema.referential_constraints rc
		ON tc.constraint_name = rc.constraint_name
		AND tc.table_schema = rc.constraint_schema
	WHERE tc.constraint_type = 'FOREIGN KEY'
		AND tc.table_schema = $1
	GROUP BY tc.constraint_name, tc.table_name, ccu.table_name, rc.update_rule, rc.delete_rule`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var constraintName, fromTable, toTable, onUpdate, onDelete string
		var fromColumns, toColumns []string
		if err := rows.Scan(&constraintName, &fromTable, &fromColumns, &toTable, &toColumns, &onUpdate, &onDelete); err != nil {
			return err
		}
		if idx := findTable(pkg.Tables, fromTable); idx >= 0 {
			pkg.Tables[idx].ForeignKeys = append(pkg.Tables[idx].ForeignKeys, schema.ForeignKey{
				Name:        constraintName,
				FromTable:   fromTable,
				FromColumns: fromColumns,
				ToTable:     toTable,
				ToColumns:   toColumns,
				OnUpdate:    onUpdate,
				OnDelete:    onDelete,
			})
		}
	}
	return rows.Err()
}

// introspectUniqueConstraints reads all unique constraints from
// information_schema.
func introspectUniqueConstraints(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		tc.constraint_name, tc.table_name,
		ARRAY_AGG(kcu.column_name ORDER BY kcu.ordinal_position) AS columns
	FROM information_schema.table_constraints tc
	JOIN information_schema.key_column_usage kcu
		ON tc.constraint_name = kcu.constraint_name
		AND tc.table_schema = kcu.table_schema
	WHERE tc.constraint_type = 'UNIQUE'
		AND tc.table_schema = $1
	GROUP BY tc.constraint_name, tc.table_name`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var constraintName, tableName string
		var columns []string
		if err := rows.Scan(&constraintName, &tableName, &columns); err != nil {
			return err
		}
		if idx := findTable(pkg.Tables, tableName); idx >= 0 {
			pkg.Tables[idx].Uniques = append(pkg.Tables[idx].Uniques, schema.UniqueConstraint{
				Name:    constraintName,
				Columns: columns,
			})
		}
	}
	return rows.Err()
}

// introspectIndexes reads all non-predicate indexes from pg_indexes.
// Indexes that back constraints (PK, UNIQUE) are excluded — they are
// already represented as constraints and including them would produce
// false-positive diffs.
func introspectIndexes(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		i.relname AS index_name,
		t.relname AS table_name,
		am.amname AS method,
		ARRAY_AGG(a.attname ORDER BY x.ordinal_position) AS columns
	FROM pg_index ix
	JOIN pg_class i ON ix.indexrelid = i.oid
	JOIN pg_class t ON ix.indrelid = t.oid
	JOIN pg_am am ON i.relam = am.oid
	JOIN pg_namespace n ON t.relnamespace = n.oid
	JOIN unnest(ix.indkey) WITH ORDINALITY AS x(attnum, ordinal_position) ON true
	JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = x.attnum
	WHERE n.nspname = $1
		AND NOT ix.indisprimary
		AND NOT ix.indisunique
	GROUP BY i.relname, t.relname, am.amname`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var indexName, tableName, method string
		var columns []string
		if err := rows.Scan(&indexName, &tableName, &method, &columns); err != nil {
			return err
		}
		if idx := findTable(pkg.Tables, tableName); idx >= 0 {
			pkg.Tables[idx].Indexes = append(pkg.Tables[idx].Indexes, schema.Index{
				Name:    indexName,
				Table:   tableName,
				Columns: columns,
				Kind:    method,
			})
		}
	}
	return rows.Err()
}

// introspectCheckConstraints reads all check constraints from pg_constraint.
func introspectCheckConstraints(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		c.conname, t.relname,
		pg_get_constraintdef(c.oid, true) AS expression
	FROM pg_constraint c
	JOIN pg_class t ON c.conrelid = t.oid
	JOIN pg_namespace n ON t.relnamespace = n.oid
	WHERE c.contype = 'c'
		AND n.nspname = $1`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var constraintName, tableName, expression string
		if err := rows.Scan(&constraintName, &tableName, &expression); err != nil {
			return err
		}
		if idx := findTable(pkg.Tables, tableName); idx >= 0 {
			pkg.Tables[idx].Checks = append(pkg.Tables[idx].Checks, schema.CheckConstraint{
				Name:       constraintName,
				Expression: expression,
			})
		}
	}
	return rows.Err()
}

// introspectEnums reads all enum types from pg_type + pg_enum.
func introspectEnums(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		t.typname AS enum_name,
		ARRAY_AGG(e.enumlabel ORDER BY e.enumsortorder) AS values
	FROM pg_type t
	JOIN pg_enum e ON t.oid = e.enumtypid
	JOIN pg_namespace n ON t.typnamespace = n.oid
	WHERE t.typtype = 'e'
		AND n.nspname = $1
	GROUP BY t.typname`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var enumName string
		var values []string
		if err := rows.Scan(&enumName, &values); err != nil {
			return err
		}
		pkg.Enums = append(pkg.Enums, schema.Enum{
			SearchPath: searchPath,
			Name:       enumName,
			Values:     values,
		})
	}
	return rows.Err()
}

// introspectExtensions reads installed extensions from pg_extension.
func introspectExtensions(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT e.extname, e.extversion, n.nspname AS schema_name
	FROM pg_extension e
	JOIN pg_namespace n ON e.extnamespace = n.oid
	WHERE n.nspname = $1`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var extName, extVersion, schemaName string
		if err := rows.Scan(&extName, &extVersion, &schemaName); err != nil {
			return err
		}
		pkg.Extensions = append(pkg.Extensions, schema.Extension{
			SearchPath: searchPath,
			Name:       extName,
			Version:    extVersion,
		})
	}
	return rows.Err()
}

// introspectViews reads all views from pg_views.
func introspectViews(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT viewname, definition
	FROM pg_views
	WHERE schemaname = $1`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var viewName, definition string
		if err := rows.Scan(&viewName, &definition); err != nil {
			return err
		}
		pkg.Views = append(pkg.Views, schema.View{
			SearchPath: searchPath,
			Name:       viewName,
			Query:      definition,
		})
	}
	return rows.Err()
}

// introspectMaterializedViews reads all materialized views.
func introspectMaterializedViews(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT matviewname, definition
	FROM pg_matviews
	WHERE schemaname = $1`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var mvName, definition string
		if err := rows.Scan(&mvName, &definition); err != nil {
			return err
		}
		pkg.MaterializedViews = append(pkg.MaterializedViews, schema.MaterializedView{
			SearchPath: searchPath,
			Name:       mvName,
			Query:      definition,
		})
	}
	return rows.Err()
}

// introspectFunctions reads all functions from pg_proc.
func introspectFunctions(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		p.proname AS name,
		p.prosrc AS body,
		l.lanname AS language,
		CASE p.prokind
			WHEN 'f' THEN 'function'
			WHEN 'p' THEN 'procedure'
		END AS kind
	FROM pg_proc p
	JOIN pg_namespace n ON p.pronamespace = n.oid
	JOIN pg_language l ON p.prolang = l.oid
	WHERE n.nspname = $1
		AND p.prokind IN ('f', 'p')`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, body, language, kind string
		if err := rows.Scan(&name, &body, &language, &kind); err != nil {
			return err
		}
		switch kind {
		case "function":
			pkg.Functions = append(pkg.Functions, schema.Function{
				SearchPath: searchPath,
				Name:       name,
				Language:   language,
				Body:       body,
			})
		case "procedure":
			pkg.Procedures = append(pkg.Procedures, schema.Procedure{
				SearchPath: searchPath,
				Name:       name,
				Language:   language,
				Body:       body,
			})
		}
	}
	return rows.Err()
}

// introspectProcedures reads all procedures from pg_proc (prokind = 'p').
func introspectProcedures(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	// Already handled by introspectFunctions — procedures are prokind = 'p'.
	return nil
}

// introspectTriggers reads all triggers from pg_trigger.
func introspectTriggers(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		t.tgname AS name,
		c.relname AS table_name,
		CASE
			WHEN t.tgtype & 2 = 2 THEN 'BEFORE'
			WHEN t.tgtype & 4 = 4 THEN 'AFTER'
			WHEN t.tgtype & 8 = 8 THEN 'INSTEAD OF'
		END AS timing,
		ARRAY_REMOVE(ARRAY_REMOVE(ARRAY[
			CASE WHEN t.tgtype & 4 = 4 THEN 'INSERT' END,
			CASE WHEN t.tgtype & 8 = 8 THEN 'DELETE' END,
			CASE WHEN t.tgtype & 16 = 16 THEN 'UPDATE' END,
			CASE WHEN t.tgtype & 20 = 20 THEN 'TRUNCATE' END
		], NULL) AS events,
		p.proname AS function_name,
		CASE WHEN t.tgconstraint != 0 THEN con.conname END AS constraint_name
	FROM pg_trigger t
	JOIN pg_class c ON t.tgrelid = c.oid
	JOIN pg_proc p ON t.tgfoid = p.oid
	JOIN pg_namespace n ON c.relnamespace = n.oid
	LEFT JOIN pg_constraint con ON t.tgconstraint = con.oid
	WHERE NOT t.tgisinternal
		AND n.nspname = $1`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, tableName, timing, functionName string
		var events []string
		var constraintName *string
		if err := rows.Scan(&name, &tableName, &timing, &events, &functionName, &constraintName); err != nil {
			return err
		}
		t := schema.Trigger{
			SearchPath: searchPath,
			Name:       name,
			Table:      tableName,
			Timing:     timing,
			Events:     events,
			Function:   functionName,
		}
		if constraintName != nil {
			t.Constraint = *constraintName
		}
		pkg.Triggers = append(pkg.Triggers, t)
	}
	return rows.Err()
}

// introspectGrants reads all grants from information_schema.role_table_grants.
func introspectGrants(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT DISTINCT
		privilege_type, table_name, grantee
	FROM information_schema.role_table_grants
	WHERE table_schema = $1`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Aggregate grants by (table_name, grantee) to match schema.Grant structure.
	type grantKey struct {
		table, grantee string
	}
	grantMap := make(map[grantKey]*schema.Grant)

	for rows.Next() {
		var privilege, tableName, grantee string
		if err := rows.Scan(&privilege, &tableName, &grantee); err != nil {
			return err
		}
		key := grantKey{tableName, grantee}
		if g, ok := grantMap[key]; ok {
			g.Privileges = append(g.Privileges, privilege)
		} else {
			grantMap[key] = &schema.Grant{
				SearchPath: searchPath,
				ObjectType: "TABLE",
				ObjectName: tableName,
				Privileges: []string{privilege},
				Roles:      []string{grantee},
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, g := range grantMap {
		pkg.Grants = append(pkg.Grants, *g)
	}
	return nil
}

// introspectPolicies reads all policies from pg_policies.
func introspectPolicies(ctx context.Context, pool *pgxpool.Pool, searchPath string, pkg *schema.Package) error {
	query := `SELECT
		policyname, tablename, roles, cmd, qual, with_check
	FROM pg_policies
	WHERE schemaname = $1`

	rows, err := pool.Query(ctx, query, searchPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var policyName, tableName, cmd string
		var roles []string
		var qual, withCheck *string
		if err := rows.Scan(&policyName, &tableName, &roles, &cmd, &qual, &withCheck); err != nil {
			return err
		}
		p := schema.Policy{
			SearchPath: searchPath,
			Name:       policyName,
			Table:      tableName,
			Command:    cmd,
			Roles:      roles,
		}
		if qual != nil {
			p.Using = *qual
		}
		if withCheck != nil {
			p.Check = *withCheck
		}
		pkg.Policies = append(pkg.Policies, p)
	}
	return rows.Err()
}

// findTable returns the index of the table with the given name in the
// slice, or -1 if not found.
func findTable(tables []schema.Table, name string) int {
	for i := range tables {
		if tables[i].Name == name {
			return i
		}
	}
	return -1
}
