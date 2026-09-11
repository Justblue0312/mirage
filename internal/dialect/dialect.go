package dialect

import "github.com/justblue/mirage/internal/schema"

// Capabilities reports which optional SQL features a dialect supports. These
// flags let feature-agnostic code (the generator, runner) branch on behavior
// without hard-coding a specific dialect.
type Capabilities interface {
	Name() string

	SupportsEnum() bool
	SupportsReturning() bool
	SupportsILike() bool
	SupportsTransactionalDDL() bool
	SupportsAddEnumValueInTransaction() bool
	SupportsIfNotExists() bool
}

// QuoteDialect handles identifier quoting.
type QuoteDialect interface {
	QuoteIdent(name string) string
}

// DDLDialect generates the schema-mutation SQL (types, tables, columns,
// constraints, indexes, comments, and registered database objects) that the
// migration generator emits. This is the core surface a new dialect must
// implement to participate in migration generation.
type DDLDialect interface {
	Capabilities
	QuoteDialect

	DataType(col schema.Column) string
	ConvertDefault(defaultVal string) string
	AutoIncrement() string

	EnumSQL(e *schema.Enum, col schema.Column) string
	CreateEnumSQL(e *schema.Enum) string
	DropEnumSQL(e *schema.Enum) string
	AlterEnumAddValue(e *schema.Enum, newValue string) string

	CreateTable(t schema.Table) []string
	DropTable(t schema.Table) []string

	AddColumn(table string, col schema.Column) []string
	DropColumn(table, col string) []string
	AlterColumnType(table string, col schema.Column) []string
	AlterColumnNullability(table string, col schema.Column) []string
	AlterColumnDefault(table string, col schema.Column) []string
	RenameColumn(table, oldName, newName string) string

	AddPrimaryKey(table string, pk schema.PrimaryKey) []string
	DropPrimaryKey(table string, pk schema.PrimaryKey) []string
	AddForeignKey(table string, fk schema.ForeignKey) []string
	DropForeignKey(table, constraintName string) []string
	AddUnique(table string, u schema.UniqueConstraint) []string
	DropUnique(table, constraintName string) []string
	AddCheck(table string, c schema.CheckConstraint) []string
	DropCheck(table, constraintName string) []string

	CreateIndex(idx schema.Index) []string
	DropIndex(idx schema.Index) []string

	WrapIdempotent(stmts []string) []string

	SetTableComment(table, comment string) string
	SetColumnComment(table, col, comment string) string

	UpsertSuffix(conflictCols, setCols []string) string

	Placeholder(index int) string

	Begin() string
	Commit() string
	Rollback() string

	// Registered objects.
	CreateExtension(e schema.Extension) string
	DropExtension(e schema.Extension) string
	AlterExtensionVersion(name, version string) string
	CreateFunction(fn schema.Function) []string
	DropFunction(fn schema.Function) []string
	CreateView(v schema.View) []string
	DropView(v schema.View) []string
	CreateMaterializedView(mv schema.MaterializedView) []string
	DropMaterializedView(mv schema.MaterializedView) []string
	CreateTrigger(t schema.Trigger) []string
	DropTrigger(t schema.Trigger) []string
	CreateProcedure(p schema.Procedure) []string
	DropProcedure(p schema.Procedure) []string
	GrantSQL(g schema.Grant) string
	RevokeSQL(g schema.Grant) string
	CreatePolicy(p schema.Policy) []string
	DropPolicy(p schema.Policy) []string
}

// TrackerDialect generates the SQL the migration runner uses to bookkeep
// applied/failed/rolled-back migrations and to persist schema snapshots.
type TrackerDialect interface {
	Capabilities

	CreateTrackerSQL() string
	GetAppliedSQL() string
	GetAppliedChecksumsSQL() string
	GetRecentAppliedSQL(n int) string
	RecordAppliedSQL() string
	RecordFailedSQL() string
	RecordRolledBackSQL() string
	ListMigrationsSQL() string
	ResetMigrationsSQL() string
	SaveSnapshotSQL() string
	LoadSnapshotSQL() string
	UpgradeTrackerSQL() []string
}

// Dialect is the full surface implemented by a concrete dialect (e.g.
// postgres.Postgres). It composes the narrower interfaces above so existing
// callers can keep depending on the whole thing, while new code can depend on
// only the slice it needs.
type Dialect interface {
	DDLDialect
	TrackerDialect
}
