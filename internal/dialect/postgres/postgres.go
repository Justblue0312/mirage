package postgres

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/justblue/mirage/internal/dialect"
	"github.com/justblue/mirage/internal/schema"
)

// Compile-time assertions that Postgres satisfies every dialect interface
// slice, including the full composite. If a method is dropped or its signature
// drifts, these fail at build time rather than at a distant call site.
var (
	_ dialect.Capabilities   = (*Postgres)(nil)
	_ dialect.QuoteDialect   = (*Postgres)(nil)
	_ dialect.DDLDialect     = (*Postgres)(nil)
	_ dialect.TrackerDialect = (*Postgres)(nil)
	_ dialect.Dialect        = (*Postgres)(nil)
)

type Postgres struct{}

func New() *Postgres {
	return &Postgres{}
}

func (p *Postgres) Name() string { return "postgres" }

func (p *Postgres) SupportsEnum() bool                      { return true }
func (p *Postgres) SupportsReturning() bool                 { return true }
func (p *Postgres) SupportsILike() bool                     { return true }
func (p *Postgres) SupportsTransactionalDDL() bool          { return true }
func (p *Postgres) SupportsAddEnumValueInTransaction() bool { return true }
func (p *Postgres) SupportsIfNotExists() bool               { return true }

func (p *Postgres) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (p *Postgres) quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// dollarTag returns a short, identifier-safe suffix for a dollar-quoting tag,
// derived from the body content so that the tag never collides with a literal
// $function$ (or similar) inside the body itself.
func dollarTag(body string) string {
	sum := sha256.Sum256([]byte(body))
	return fmt.Sprintf("%x", sum[:4])
}

func (p *Postgres) DataType(col schema.Column) string {
	return col.SQLType
}

func (p *Postgres) ConvertDefault(val string) string {
	return val
}

func (p *Postgres) ConvertDefaultForType(val, sqlType string) string {
	if val == "" {
		return val
	}
	if strings.ToUpper(val) == "NULL" {
		return val
	}
	if strings.HasPrefix(val, "'") || strings.HasPrefix(val, "\"") {
		return val
	}
	if strings.Contains(val, "(") {
		return val
	}

	lowerType := strings.ToLower(sqlType)
	isStringType := strings.HasPrefix(lowerType, "varchar") ||
		strings.HasPrefix(lowerType, "char") ||
		lowerType == "text" ||
		lowerType == "uuid" ||
		lowerType == "json" ||
		lowerType == "jsonb" ||
		lowerType == "xml" ||
		strings.HasPrefix(lowerType, "date") ||
		strings.HasPrefix(lowerType, "time") ||
		strings.HasPrefix(lowerType, "timestamp") ||
		lowerType == "inet" ||
		lowerType == "cidr" ||
		lowerType == "macaddr"

	if isStringType || isEnumType(sqlType) {
		return p.quoteLiteral(val)
	}

	if strings.ToLower(val) == "true" || strings.ToLower(val) == "false" {
		return val
	}
	if _, err := fmt.Sscanf(val, "%d", new(int)); err == nil {
		return val
	}
	if _, err := fmt.Sscanf(val, "%f", new(float64)); err == nil {
		return val
	}
	return val
}

func isEnumType(sqlType string) bool {
	if sqlType == "" {
		return false
	}
	if strings.Contains(sqlType, "(") || strings.Contains(sqlType, " ") {
		return false
	}
	knownTypes := map[string]bool{
		"int": true, "integer": true, "bigint": true, "smallint": true, "serial": true, "bigserial": true,
		"varchar": true, "char": true, "text": true, "uuid": true,
		"bool": true, "boolean": true,
		"date": true, "time": true, "timestamp": true, "timestamptz": true, "interval": true,
		"real": true, "double": true, "numeric": true, "decimal": true,
		"json": true, "jsonb": true, "xml": true,
		"bytea": true, "inet": true, "cidr": true, "macaddr": true,
		"money": true, "bit": true, "varbit": true,
		"tsvector": true, "tsquery": true,
	}
	return !knownTypes[strings.ToLower(sqlType)]
}

func (p *Postgres) AutoIncrement() string {
	return ""
}

func (p *Postgres) EnumSQL(e *schema.Enum, col schema.Column) string {
	return e.SQLName()
}

func (p *Postgres) CreateEnumSQL(e *schema.Enum) string {
	vals := make([]string, len(e.Values))
	for i, v := range e.Values {
		vals[i] = p.quoteLiteral(v)
	}
	return fmt.Sprintf("CREATE TYPE %s AS ENUM (%s);", e.SQLName(), strings.Join(vals, ", "))
}

func (p *Postgres) DropEnumSQL(e *schema.Enum) string {
	return fmt.Sprintf("DROP TYPE IF EXISTS %s;", e.SQLName())
}

func (p *Postgres) CreateExtension(e schema.Extension) string {
	var b strings.Builder
	b.WriteString("CREATE EXTENSION ")
	if e.IfNotExists {
		b.WriteString("IF NOT EXISTS ")
	}
	b.WriteString(p.QuoteIdent(e.Name))
	if e.Schema != "" {
		b.WriteString(" SCHEMA " + p.QuoteIdent(e.Schema))
	}
	if e.Version != "" {
		b.WriteString(" VERSION " + p.quoteLiteral(e.Version))
	}
	b.WriteString(";")
	return b.String()
}

func (p *Postgres) DropExtension(e schema.Extension) string {
	var b strings.Builder
	b.WriteString("DROP EXTENSION ")
	if e.IfNotExists {
		b.WriteString("IF EXISTS ")
	}
	b.WriteString(p.QuoteIdent(e.Name))
	if e.Cascade {
		b.WriteString(" CASCADE")
	}
	b.WriteString(";")
	return b.String()
}

func (p *Postgres) AlterEnumAddValue(e *schema.Enum, newValue string) string {
	var lines []string
	if !p.SupportsAddEnumValueInTransaction() {
		lines = append(lines, "-- requires autocommit (cannot run inside BEGIN/COMMIT)")
	}
	lines = append(lines, fmt.Sprintf("ALTER TYPE %s ADD VALUE IF NOT EXISTS %s;", e.SQLName(), p.quoteLiteral(newValue)))
	return strings.Join(lines, "\n")
}

func (p *Postgres) CreateTable(t schema.Table) []string {
	ifNotExists := ""
	if p.SupportsIfNotExists() {
		ifNotExists = "IF NOT EXISTS "
	}

	var colDefs []string
	var constraints []string

	cols := make([]*schema.Column, len(t.Columns))
	copy(cols, t.Columns)
	sort.SliceStable(cols, func(i, j int) bool {
		return cols[i].OrdinalPosition < cols[j].OrdinalPosition
	})

	for _, c := range cols {
		def := p.columnDef(c)
		colDefs = append(colDefs, "    "+def)
	}

	if t.PrimaryKey != nil && len(t.PrimaryKey.Columns) > 0 {
		cols := p.quoteIdents(t.PrimaryKey.Columns)
		constraints = append(constraints, fmt.Sprintf("    CONSTRAINT %s PRIMARY KEY (%s)", p.QuoteIdent(t.PrimaryKey.Name), cols))
	}

	for _, uq := range t.Uniques {
		cols := p.quoteIdents(uq.Columns)
		constraints = append(constraints, fmt.Sprintf("    CONSTRAINT %s UNIQUE (%s)", p.QuoteIdent(uq.Name), cols))
	}

	for _, fk := range t.ForeignKeys {
		fromCols := p.quoteIdents(fk.FromColumns)
		toCols := p.quoteIdents(fk.ToColumns)
		ref := fmt.Sprintf("    CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			p.QuoteIdent(fk.Name), fromCols, p.QuoteIdent(fk.ToTable), toCols)
		if fk.OnDelete != "" {
			ref += " ON DELETE " + fk.OnDelete
		}
		if fk.OnUpdate != "" {
			ref += " ON UPDATE " + fk.OnUpdate
		}
		constraints = append(constraints, ref)
	}

	for _, c := range t.Columns {
		if c.ReferenceTableName != "" {
			fkName := schema.FKName(t.Name, []string{c.Name})
			fromCols := p.QuoteIdent(c.Name)
			toCols := p.QuoteIdent(c.ReferenceColumnName)
			ref := fmt.Sprintf("    CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
				p.QuoteIdent(fkName), fromCols, p.QuoteIdent(c.ReferenceTableName), toCols)
			if c.ReferenceOnDelete != "" {
				ref += " ON DELETE " + c.ReferenceOnDelete
			}
			if c.ReferenceOnUpdate != "" {
				ref += " ON UPDATE " + c.ReferenceOnUpdate
			}
			constraints = append(constraints, ref)
		}
	}

	for _, chk := range t.Checks {
		constraints = append(constraints, fmt.Sprintf("    CONSTRAINT %s CHECK (%s)", p.QuoteIdent(chk.Name), chk.Expression))
	}

	colDefs = append(colDefs, constraints...)
	body := strings.Join(colDefs, ",\n")

	tableType := ""
	if t.Type != schema.InvalidTableType && t.Type != schema.TableTypeBase {
		tableType = " " + strings.ToUpper(t.Type.String())
	}

	partition := ""
	if t.Partitioned != nil {
		partition = fmt.Sprintf(" PARTITION BY %s (%s)", t.Partitioned.Strategy, p.QuoteIdent(t.Partitioned.Column))
	}

	options := ""
	if t.Options != "" {
		options = " " + t.Options
	}

	stmt := fmt.Sprintf("CREATE TABLE %s%s (\n%s\n)%s%s%s;",
		ifNotExists, t.SQLName(), body, tableType, options, partition)

	var stmts []string
	stmts = append(stmts, stmt)

	if t.Description != "" {
		stmts = append(stmts, p.SetTableComment(t.SQLName(), t.Description))
	}

	return stmts
}

func (p *Postgres) DropTable(t schema.Table) []string {
	return []string{fmt.Sprintf("DROP TABLE IF EXISTS %s;", t.SQLName())}
}

func (p *Postgres) AddColumn(table string, col schema.Column) []string {
	def := p.columnDef(&col)
	return []string{fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", table, def)}
}

func (p *Postgres) DropColumn(table, col string) []string {
	return []string{fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", table, p.QuoteIdent(col))}
}

func (p *Postgres) AlterColumnType(table string, col schema.Column) []string {
	using := col.TypeChangeUsing
	if using == "" {
		using = fmt.Sprintf("%s::%s", p.QuoteIdent(col.Name), col.SQLType)
	}
	return []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s;",
		table, p.QuoteIdent(col.Name), col.SQLType, using)}
}

func (p *Postgres) AlterColumnNullability(table string, col schema.Column) []string {
	if !col.Nullable {
		return []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", table, p.QuoteIdent(col.Name))}
	}
	return []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", table, p.QuoteIdent(col.Name))}
}

func (p *Postgres) AlterColumnDefault(table string, col schema.Column) []string {
	if col.Default == "" {
		return []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", table, p.QuoteIdent(col.Name))}
	}
	return []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", table, p.QuoteIdent(col.Name), p.ConvertDefault(col.Default))}
}

func (p *Postgres) RenameColumn(table, oldName, newName string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", table, p.QuoteIdent(oldName), p.QuoteIdent(newName))
}

func (p *Postgres) AddPrimaryKey(table string, pk schema.PrimaryKey) []string {
	cols := p.quoteIdents(pk.Columns)
	return []string{fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s PRIMARY KEY (%s);", table, p.QuoteIdent(pk.Name), cols)}
}

func (p *Postgres) DropPrimaryKey(table string, pk schema.PrimaryKey) []string {
	return []string{fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, p.QuoteIdent(pk.Name))}
}

func (p *Postgres) AddForeignKey(table string, fk schema.ForeignKey) []string {
	fromCols := p.quoteIdents(fk.FromColumns)
	toCols := p.quoteIdents(fk.ToColumns)
	stmt := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
		table, p.QuoteIdent(fk.Name), fromCols, p.QuoteIdent(fk.ToTable), toCols)
	if fk.OnDelete != "" {
		stmt += " ON DELETE " + fk.OnDelete
	}
	if fk.OnUpdate != "" {
		stmt += " ON UPDATE " + fk.OnUpdate
	}
	return []string{stmt + ";"}
}

func (p *Postgres) DropForeignKey(table, constraintName string) []string {
	return []string{fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, p.QuoteIdent(constraintName))}
}

func (p *Postgres) AddUnique(table string, u schema.UniqueConstraint) []string {
	cols := p.quoteIdents(u.Columns)
	return []string{fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", table, p.QuoteIdent(u.Name), cols)}
}

func (p *Postgres) DropUnique(table, constraintName string) []string {
	return []string{fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, p.QuoteIdent(constraintName))}
}

func (p *Postgres) AddCheck(table string, c schema.CheckConstraint) []string {
	return []string{fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s);", table, p.QuoteIdent(c.Name), c.Expression)}
}

func (p *Postgres) DropCheck(table, constraintName string) []string {
	return []string{fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, p.QuoteIdent(constraintName))}
}

func (p *Postgres) CreateIndex(idx schema.Index) []string {
	cols := p.quoteIdentsWithSort(idx.Columns, idx.Sort)
	if idx.Kind != "" {
		return []string{fmt.Sprintf("CREATE INDEX %s ON %s USING %s (%s);",
			p.QuoteIdent(idx.Name), idx.Table, idx.Kind, cols)}
	}
	return []string{fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
		p.QuoteIdent(idx.Name), idx.Table, cols)}
}

func (p *Postgres) DropIndex(idx schema.Index) []string {
	return []string{fmt.Sprintf("DROP INDEX IF EXISTS %s;", p.QuoteIdent(idx.Name))}
}

func (p *Postgres) WrapIdempotent(stmts []string) []string {
	var wrapped []string
	for _, stmt := range stmts {
		upper := strings.ToUpper(strings.TrimSpace(stmt))
		switch {
		case strings.HasPrefix(upper, "ALTER TABLE") && strings.Contains(upper, "ADD COLUMN"):
			wrapped = append(wrapped, fmt.Sprintf("DO $$ BEGIN\n  %s\nEXCEPTION WHEN duplicate_column THEN NULL;\nEND $$;", stmt))
		case strings.HasPrefix(upper, "CREATE INDEX"):
			wrapped = append(wrapped, strings.Replace(stmt, "CREATE INDEX", "CREATE INDEX IF NOT EXISTS", 1))
		case strings.HasPrefix(upper, "ALTER TABLE") && strings.Contains(upper, "ADD CONSTRAINT"):
			wrapped = append(wrapped, fmt.Sprintf("DO $$ BEGIN\n  %s\nEXCEPTION WHEN duplicate_object THEN NULL;\nEND $$;", stmt))
		case strings.HasPrefix(upper, "CREATE TRIGGER"):
			wrapped = append(wrapped, fmt.Sprintf("DO $$ BEGIN\n  %s\nEXCEPTION WHEN duplicate_object THEN NULL;\nEND $$;", stmt))
		case strings.HasPrefix(upper, "CREATE POLICY"):
			wrapped = append(wrapped, fmt.Sprintf("DO $$ BEGIN\n  %s\nEXCEPTION WHEN duplicate_object THEN NULL;\nEND $$;", stmt))
		default:
			wrapped = append(wrapped, stmt)
		}
	}
	return wrapped
}

func (p *Postgres) SetTableComment(table, comment string) string {
	return fmt.Sprintf("COMMENT ON TABLE %s IS %s;", table, p.quoteLiteral(comment))
}

func (p *Postgres) SetColumnComment(table, col, comment string) string {
	return fmt.Sprintf("COMMENT ON COLUMN %s.%s IS %s;", table, p.QuoteIdent(col), p.quoteLiteral(comment))
}

func (p *Postgres) UpsertSuffix(conflictCols, setCols []string) string {
	cols := p.quoteIdents(conflictCols)
	sets := make([]string, len(setCols))
	for i, c := range setCols {
		sets[i] = fmt.Sprintf("%s = EXCLUDED.%s", p.QuoteIdent(c), p.QuoteIdent(c))
	}
	return fmt.Sprintf("ON CONFLICT (%s) DO UPDATE SET %s", cols, strings.Join(sets, ", "))
}

func (p *Postgres) Placeholder(index int) string {
	return fmt.Sprintf("$%d", index)
}

func (p *Postgres) Begin() string    { return "BEGIN;" }
func (p *Postgres) Commit() string   { return "COMMIT;" }
func (p *Postgres) Rollback() string { return "ROLLBACK;" }

func (p *Postgres) CreateTrackerSQL() string {
	return `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    bigint        NOT NULL,
    name       varchar(255)  NOT NULL,
    applied_at timestamptz   NOT NULL DEFAULT now(),
    state      varchar(32)   NOT NULL DEFAULT 'applied',
    checksum   varchar(128),
    snapshot   jsonb,
    CONSTRAINT pk_schema_migrations PRIMARY KEY (version)
);
`
}

func (p *Postgres) GetAppliedSQL() string {
	return "SELECT version FROM schema_migrations WHERE state='applied'"
}

func (p *Postgres) GetAppliedChecksumsSQL() string {
	return "SELECT version, checksum FROM schema_migrations WHERE state='applied'"
}

func (p *Postgres) GetRecentAppliedSQL(n int) string {
	return fmt.Sprintf("SELECT version, name FROM schema_migrations WHERE state='applied' ORDER BY version DESC LIMIT %d", n)
}

func (p *Postgres) RecordAppliedSQL() string {
	return `INSERT INTO schema_migrations (version, name, applied_at, state, checksum, snapshot)
		VALUES ($1, $2, NOW(), 'applied', $3, $4)
		ON CONFLICT (version) DO UPDATE SET
			name       = EXCLUDED.name,
			applied_at = NOW(),
			state      = 'applied',
			checksum   = EXCLUDED.checksum`
	// Deliberately not touching `snapshot` here: if a row for this version
	// already exists (created by a prior `generate --db` via SaveSnapshotSQL
	// below), it already holds the real snapshot and must not be
	// overwritten with the nil the runner package always passes for it.
}

func (p *Postgres) RecordFailedSQL() string {
	return "INSERT INTO schema_migrations (version, name, applied_at, state) VALUES ($1, $2, NOW(), 'failed')"
}

func (p *Postgres) RecordRolledBackSQL() string {
	return "UPDATE schema_migrations SET state='rolled_back' WHERE version=$1"
}

func (p *Postgres) ListMigrationsSQL() string {
	return "SELECT version, name, applied_at, state FROM schema_migrations ORDER BY version"
}

func (p *Postgres) ResetMigrationsSQL() string {
	return "TRUNCATE schema_migrations"
}

// identityBaseType maps a serial pseudo-type to the base integer type
// Postgres requires when a column is also declared GENERATED ... AS
// IDENTITY -- serial and IDENTITY are two different, mutually exclusive
// auto-increment mechanisms, and combining them ("bigserial GENERATED
// ALWAYS AS IDENTITY") is rejected by Postgres with "both default and
// identity specified for column". Non-serial types are returned unchanged.
func identityBaseType(sqlType string) string {
	switch strings.ToLower(strings.TrimSpace(sqlType)) {
	case "serial", "serial4":
		return "integer"
	case "bigserial", "serial8":
		return "bigint"
	case "smallserial", "serial2":
		return "smallint"
	default:
		return sqlType
	}
}

func (p *Postgres) columnDef(c *schema.Column) string {
	dataType := p.DataType(*c)
	if c.Identity {
		dataType = identityBaseType(dataType)
	}
	parts := []string{p.QuoteIdent(c.Name), dataType}

	if !c.Nullable {
		parts = append(parts, "NOT NULL")
	}

	if c.Identity {
		parts = append(parts, "GENERATED BY DEFAULT AS IDENTITY")
	} else if c.Default != "" {
		parts = append(parts, "DEFAULT", p.ConvertDefaultForType(c.Default, c.SQLType))
	}

	if c.Collate != "" {
		parts = append(parts, "COLLATE", c.Collate)
	}

	return strings.Join(parts, " ")
}

func (p *Postgres) quoteIdents(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = p.QuoteIdent(c)
	}
	return strings.Join(quoted, ", ")
}

func (p *Postgres) quoteIdentsWithSort(cols []string, sort string) string {
	upperSort := strings.ToUpper(strings.TrimSpace(sort))
	if upperSort != "ASC" && upperSort != "DESC" {
		upperSort = ""
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		if upperSort != "" {
			quoted[i] = p.QuoteIdent(c) + " " + upperSort
		} else {
			quoted[i] = p.QuoteIdent(c)
		}
	}
	return strings.Join(quoted, ", ")
}

func (p *Postgres) SaveSnapshotSQL() string {
	return `INSERT INTO schema_migrations (version, name, applied_at, state, snapshot)
		VALUES ($1, $3, NOW(), 'pending', $2)
		ON CONFLICT (version) DO UPDATE SET snapshot = EXCLUDED.snapshot`
}

func (p *Postgres) LoadSnapshotSQL() string {
	return "SELECT snapshot FROM schema_migrations WHERE state='applied' AND snapshot IS NOT NULL ORDER BY version DESC LIMIT 1"
}

func (p *Postgres) UpgradeTrackerSQL() []string {
	return []string{
		`DO $$ BEGIN
			ALTER TABLE schema_migrations ADD COLUMN snapshot jsonb;
		EXCEPTION WHEN duplicate_column THEN NULL;
		END $$;`,
		// checksum was originally varchar(64), but internal/checksum.Compute
		// produces "sha256:" + 64 hex characters = 71 characters -- always
		// 7 too long. Widening a varchar column is a fast, non-rewriting
		// metadata-only change in Postgres and is safe to run every time
		// EnsureTracker runs (a no-op once already widened).
		`ALTER TABLE schema_migrations ALTER COLUMN checksum TYPE varchar(128);`,
	}
}

// --- Registered Objects ---

func (p *Postgres) AlterExtensionVersion(name, version string) string {
	return fmt.Sprintf("ALTER EXTENSION %s UPDATE TO %s;", p.QuoteIdent(name), p.quoteLiteral(version))
}

func (p *Postgres) CreateFunction(fn schema.Function) []string {
	var args string
	if len(fn.Arguments) > 0 {
		parts := make([]string, len(fn.Arguments))
		for i, arg := range fn.Arguments {
			mode := ""
			if arg.Mode != "" && arg.Mode != "IN" {
				mode = arg.Mode + " "
			}
			parts[i] = mode + p.QuoteIdent(arg.Name) + " " + p.QuoteIdent(arg.Type)
		}
		args = strings.Join(parts, ", ")
	}

	volatility := "VOLATILE"
	if fn.Volatility != "" {
		volatility = strings.ToUpper(fn.Volatility)
	}

	security := "DEFINER"
	if fn.Security != "" {
		security = strings.ToUpper(fn.Security)
	}

	retType := "void"
	if fn.ReturnType != "" {
		retType = fn.ReturnType
	}

	tag := "func_" + dollarTag(fn.Body)
	stmt := fmt.Sprintf(
		`CREATE OR REPLACE FUNCTION %s(%s)
    RETURNS %s
    LANGUAGE %s
    %s
    SECURITY %s
AS $%s$
%s
$%s$;`,
		p.QuoteIdent(fn.SQLName()), args,
		retType,
		fn.Language,
		volatility,
		security,
		tag, fn.Body, tag,
	)

	var stmts []string
	stmts = append(stmts, stmt)
	if fn.Description != "" {
		stmts = append(stmts, fmt.Sprintf("COMMENT ON FUNCTION %s IS %s;",
			p.QuoteIdent(fn.SQLName()), p.quoteLiteral(fn.Description)))
	}
	return stmts
}

func (p *Postgres) DropFunction(fn schema.Function) []string {
	argTypes := make([]string, len(fn.Arguments))
	for i, arg := range fn.Arguments {
		argTypes[i] = arg.Type
	}
	args := strings.Join(argTypes, ", ")
	return []string{fmt.Sprintf("DROP FUNCTION IF EXISTS %s(%s);", p.QuoteIdent(fn.SQLName()), args)}
}

func (p *Postgres) CreateView(v schema.View) []string {
	stmt := fmt.Sprintf("CREATE OR REPLACE VIEW %s AS\n%s;", p.QuoteIdent(v.SQLName()), v.Query)
	var stmts []string
	stmts = append(stmts, stmt)
	if v.Description != "" {
		stmts = append(stmts, fmt.Sprintf("COMMENT ON VIEW %s IS %s;",
			p.QuoteIdent(v.SQLName()), p.quoteLiteral(v.Description)))
	}
	return stmts
}

func (p *Postgres) DropView(v schema.View) []string {
	return []string{fmt.Sprintf("DROP VIEW IF EXISTS %s;", p.QuoteIdent(v.SQLName()))}
}

func (p *Postgres) CreateMaterializedView(mv schema.MaterializedView) []string {
	stmt := fmt.Sprintf("CREATE MATERIALIZED VIEW IF NOT EXISTS %s AS\n%s;",
		p.QuoteIdent(mv.SQLName()), mv.Query)
	var stmts []string
	stmts = append(stmts, stmt)
	if mv.Description != "" {
		stmts = append(stmts, fmt.Sprintf("COMMENT ON MATERIALIZED VIEW %s IS %s;",
			p.QuoteIdent(mv.SQLName()), p.quoteLiteral(mv.Description)))
	}
	return stmts
}

func (p *Postgres) DropMaterializedView(mv schema.MaterializedView) []string {
	return []string{fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s;", p.QuoteIdent(mv.SQLName()))}
}

func (p *Postgres) CreateTrigger(t schema.Trigger) []string {
	if len(t.Events) == 0 {
		return []string{"-- cannot create trigger: at least one event (INSERT, UPDATE, DELETE, TRUNCATE) is required"}
	}
	events := strings.Join(t.Events, " OR ")
	constraint := ""
	if t.Constraint != "" {
		constraint = " " + t.Constraint
	}
	stmt := fmt.Sprintf(
		"CREATE TRIGGER %s\n    %s %s ON %s\n    FOR EACH ROW%s\n    EXECUTE FUNCTION %s();",
		p.QuoteIdent(t.SQLName()),
		t.Timing, events, p.QuoteIdent(t.Table),
		constraint, p.QuoteIdent(t.Function),
	)
	var stmts []string
	stmts = append(stmts, stmt)
	if t.Description != "" {
		stmts = append(stmts, fmt.Sprintf("COMMENT ON TRIGGER %s ON %s IS %s;",
			p.QuoteIdent(t.SQLName()), p.QuoteIdent(t.Table), p.quoteLiteral(t.Description)))
	}
	return stmts
}

func (p *Postgres) DropTrigger(t schema.Trigger) []string {
	return []string{fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON %s;",
		p.QuoteIdent(t.SQLName()), p.QuoteIdent(t.Table))}
}

func (p *Postgres) CreateProcedure(proc schema.Procedure) []string {
	var args string
	if len(proc.Arguments) > 0 {
		parts := make([]string, len(proc.Arguments))
		for i, arg := range proc.Arguments {
			mode := ""
			if arg.Mode != "" && arg.Mode != "IN" {
				mode = arg.Mode + " "
			}
			parts[i] = mode + p.QuoteIdent(arg.Name) + " " + arg.Type
		}
		args = strings.Join(parts, ", ")
	}

	tag := "proc_" + dollarTag(proc.Body)
	stmt := fmt.Sprintf(
		`CREATE OR REPLACE PROCEDURE %s(%s)
    LANGUAGE %s
AS $%s$
%s
$%s$;`,
		p.QuoteIdent(proc.SQLName()), args,
		proc.Language,
		tag, proc.Body, tag,
	)

	var stmts []string
	stmts = append(stmts, stmt)
	if proc.Description != "" {
		stmts = append(stmts, fmt.Sprintf("COMMENT ON PROCEDURE %s IS %s;",
			p.QuoteIdent(proc.SQLName()), p.quoteLiteral(proc.Description)))
	}
	return stmts
}

func (p *Postgres) DropProcedure(proc schema.Procedure) []string {
	argTypes := make([]string, len(proc.Arguments))
	for i, arg := range proc.Arguments {
		argTypes[i] = arg.Type
	}
	args := strings.Join(argTypes, ", ")
	return []string{fmt.Sprintf("DROP PROCEDURE IF EXISTS %s(%s);", p.QuoteIdent(proc.SQLName()), args)}
}

func (p *Postgres) GrantSQL(g schema.Grant) string {
	if len(g.Privileges) == 0 || len(g.Roles) == 0 {
		return "-- cannot generate GRANT: privileges and roles are required"
	}
	objType := strings.ToUpper(strings.TrimSpace(g.ObjectType))
	switch objType {
	case "TABLE", "VIEW", "SEQUENCE", "FUNCTION", "PROCEDURE", "SCHEMA", "TYPE", "DATABASE":
		// valid
	default:
		return fmt.Sprintf("-- cannot generate GRANT: invalid object type %q", g.ObjectType)
	}
	privs := strings.Join(g.Privileges, ", ")
	quotedRoles := make([]string, len(g.Roles))
	for i, r := range g.Roles {
		quotedRoles[i] = p.QuoteIdent(r)
	}
	roles := strings.Join(quotedRoles, ", ")
	return fmt.Sprintf("GRANT %s ON %s %s TO %s;",
		privs, strings.ToUpper(g.ObjectType), p.QuoteIdent(g.ObjectName), roles)
}

func (p *Postgres) RevokeSQL(g schema.Grant) string {
	if len(g.Privileges) == 0 || len(g.Roles) == 0 {
		return "-- cannot generate REVOKE: privileges and roles are required"
	}
	objType := strings.ToUpper(strings.TrimSpace(g.ObjectType))
	switch objType {
	case "TABLE", "VIEW", "SEQUENCE", "FUNCTION", "PROCEDURE", "SCHEMA", "TYPE", "DATABASE":
		// valid
	default:
		return fmt.Sprintf("-- cannot generate REVOKE: invalid object type %q", g.ObjectType)
	}
	privs := strings.Join(g.Privileges, ", ")
	quotedRoles := make([]string, len(g.Roles))
	for i, r := range g.Roles {
		quotedRoles[i] = p.QuoteIdent(r)
	}
	roles := strings.Join(quotedRoles, ", ")
	return fmt.Sprintf("REVOKE %s ON %s %s FROM %s;",
		privs, strings.ToUpper(g.ObjectType), p.QuoteIdent(g.ObjectName), roles)
}

func (p *Postgres) CreatePolicy(pol schema.Policy) []string {
	using := ""
	if pol.Using != "" {
		using = fmt.Sprintf("\n    USING (%s)", pol.Using)
	}
	check := ""
	if pol.Check != "" {
		check = fmt.Sprintf("\n    WITH CHECK (%s)", pol.Check)
	} else if pol.Using != "" {
		check = fmt.Sprintf("\n    WITH CHECK (%s)", pol.Using)
	}
	permissive := "PERMISSIVE"
	if pol.Permissive != "" {
		permissive = strings.ToUpper(pol.Permissive)
	}
	var toClause string
	if len(pol.Roles) > 0 {
		quoted := make([]string, len(pol.Roles))
		for i, r := range pol.Roles {
			quoted[i] = p.QuoteIdent(r)
		}
		toClause = "\n    TO " + strings.Join(quoted, ", ")
	}
	stmt := fmt.Sprintf(
		"CREATE POLICY %s ON %s\n    AS %s\n    FOR %s%s%s%s;",
		p.QuoteIdent(pol.Name), p.QuoteIdent(pol.Table),
		permissive, pol.Command, toClause, using, check)
	return []string{stmt}
}

func (p *Postgres) DropPolicy(pol schema.Policy) []string {
	return []string{fmt.Sprintf("DROP POLICY IF EXISTS %s ON %s;",
		p.QuoteIdent(pol.Name), p.QuoteIdent(pol.Table))}
}
