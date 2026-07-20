package generator

import (
	"fmt"
	"strings"
	"time"

	"github.com/justblue/mirage/internal/checksum"
	"github.com/justblue/mirage/internal/dialect"
	"github.com/justblue/mirage/internal/diff"
	"github.com/justblue/mirage/internal/graph"
	"github.com/justblue/mirage/internal/schema"
)

type Generator struct {
	dialect dialect.DDLDialect
	events  []diff.DeltaEvent
	oldPkg  *schema.Package
	newPkg  *schema.Package
}

type MigrationFile struct {
	Version              string
	Description          string
	Message              string
	UpStatements         []string
	DownStatements       []string
	AutocommitStatements []string
	HasDestructive       bool
	Checksum             string
}

func New(d dialect.DDLDialect, events []diff.DeltaEvent, old, new *schema.Package) *Generator {
	return &Generator{
		dialect: d,
		events:  events,
		oldPkg:  old,
		newPkg:  new,
	}
}

func (g *Generator) Generate() (*MigrationFile, error) {
	var upStmts []string
	var downStmts []string
	var autocommitStmts []string
	hasDestructive := false

	tableOrder, err := graph.Build(g.newPkg.Tables).Sort()
	if err != nil {
		return nil, fmt.Errorf("building dependency graph: %w", err)
	}
	tableOrderSet := make(map[string]int)
	for i, t := range tableOrder {
		tableOrderSet[t] = i
	}

	enumAdds := g.filterEvents(diff.EnumAdded)
	tableAdds := g.filterEvents(diff.TableAdded)
	columnAdds := g.filterEvents(diff.ColumnAdded)
	typeChanges := g.filterEvents(diff.ColumnTypeChanged)
	nullChanges := g.filterEvents(diff.ColumnNullChanged)
	defaultChanges := g.filterEvents(diff.ColumnDefaultChanged)
	pkChanges := g.filterEvents(diff.PKChanged)
	fkAdds := g.filterEvents(diff.FKAdded)
	uqAdds := g.filterEvents(diff.UniqueAdded)
	checkAdds := g.filterEvents(diff.CheckAdded)
	indexAdds := g.filterEvents(diff.IndexAdded)
	columnDrops := g.filterEvents(diff.ColumnDropped)
	columnRenames := g.filterEvents(diff.ColumnRenamed)
	tableDrops := g.filterEvents(diff.TableDropped)
	enumDrops := g.filterEvents(diff.EnumDropped)
	enumValueAdds := g.filterEvents(diff.EnumValueAdded)
	enumValueDrops := g.filterEvents(diff.EnumValueDropped)
	comments := g.filterEvents(diff.CommentChanged)
	fkDrops := g.filterEvents(diff.FKDropped)
	uqDrops := g.filterEvents(diff.UniqueDropped)
	checkDrops := g.filterEvents(diff.CheckDropped)
	indexDrops := g.filterEvents(diff.IndexDropped)

	funcAdds := g.filterEvents(diff.FunctionAdded)
	funcChanges := g.filterEvents(diff.FunctionBodyChanged)
	funcDrops := g.filterEvents(diff.FunctionDropped)
	procAdds := g.filterEvents(diff.ProcedureAdded)
	procChanges := g.filterEvents(diff.ProcedureBodyChanged)
	procDrops := g.filterEvents(diff.ProcedureDropped)
	viewAdds := g.filterEvents(diff.ViewAdded)
	viewChanges := g.filterEvents(diff.ViewQueryChanged)
	viewDrops := g.filterEvents(diff.ViewDropped)
	matViewAdds := g.filterEvents(diff.MaterializedViewAdded)
	matViewChanges := g.filterEvents(diff.MaterializedViewQueryChanged)
	matViewDrops := g.filterEvents(diff.MaterializedViewDropped)
	triggerAdds := g.filterEvents(diff.TriggerAdded)
	triggerChanges := g.filterEvents(diff.TriggerChanged)
	triggerDrops := g.filterEvents(diff.TriggerDropped)
	grantAdds := g.filterEvents(diff.GrantAdded)
	grantDrops := g.filterEvents(diff.GrantDropped)
	policyAdds := g.filterEvents(diff.PolicyAdded)
	policyChanges := g.filterEvents(diff.PolicyChanged)
	policyDrops := g.filterEvents(diff.PolicyDropped)

	extensionAdds := g.filterEvents(diff.ExtensionAdded)
	extensionDrops := g.filterEvents(diff.ExtensionDropped)
	extensionVersionChanges := g.filterEvents(diff.ExtensionVersionChanged)

	for _, e := range g.events {
		if e.Severity == diff.Destructive {
			hasDestructive = true
		}
	}

	// Extensions must exist before any dependent types, tables, or functions,
	// so they are emitted first.
	for _, e := range extensionAdds {
		if e.NewExtension != nil {
			upStmts = append(upStmts, g.dialect.CreateExtension(*e.NewExtension))
		}
	}
	for _, e := range extensionDrops {
		if e.OldExtension != nil {
			upStmts = append(upStmts, g.dialect.DropExtension(*e.OldExtension))
		}
	}
	for _, e := range extensionVersionChanges {
		if e.NewExtension != nil {
			upStmts = append(upStmts, g.dialect.AlterExtensionVersion(e.NewExtension.Name, e.NewExtension.Version))
		}
	}

	for _, e := range enumAdds {
		tbl := g.findTable(e.Table)
		if tbl != nil {
			for _, en := range g.newPkg.Enums {
				if en.SQLName() == e.Table {
					upStmts = append(upStmts, g.dialect.CreateEnumSQL(&en))
					break
				}
			}
		} else {
			for _, en := range g.newPkg.Enums {
				if en.SQLName() == e.Enum {
					upStmts = append(upStmts, g.dialect.CreateEnumSQL(&en))
					break
				}
			}
		}
	}

	for _, name := range tableOrder {
		for _, e := range tableAdds {
			if e.NewTable != nil && e.NewTable.SQLName() == name {
				tbl := *e.NewTable
				if !g.dialect.SupportsEnum() {
					tbl = g.injectEnumChecks(tbl)
				}
				upStmts = append(upStmts, g.dialect.CreateTable(tbl)...)
			}
		}
	}

	for _, name := range tableOrder {
		for _, e := range columnAdds {
			if e.Table == name {
				tbl := g.findTable(name)
				if tbl != nil {
					for _, c := range tbl.Columns {
						if c.Name == e.Column {
							col := *c
							if !g.dialect.SupportsEnum() {
								col = g.injectColumnEnumCheck(name, col)
							}
							upStmts = append(upStmts, g.dialect.AddColumn(name, col)...)
							break
						}
					}
				}
			}
		}
	}

	for _, e := range columnRenames {
		if e.OldValue != "" && e.NewValue != "" {
			upStmts = append(upStmts, g.dialect.RenameColumn(e.Table, e.OldValue, e.NewValue))
		}
	}

	for _, e := range typeChanges {
		if e.NewColumn != nil {
			upStmts = append(upStmts, g.dialect.AlterColumnType(e.Table, *e.NewColumn)...)
		}
	}

	for _, e := range nullChanges {
		if e.NewColumn != nil {
			upStmts = append(upStmts, g.dialect.AlterColumnNullability(e.Table, *e.NewColumn)...)
		}
	}

	for _, e := range defaultChanges {
		if e.NewColumn != nil {
			upStmts = append(upStmts, g.dialect.AlterColumnDefault(e.Table, *e.NewColumn)...)
		}
	}

	for _, e := range pkChanges {
		tbl := g.findTable(e.Table)
		if tbl != nil && tbl.PrimaryKey != nil {
			if oldTbl := g.findOldTable(e.Table); oldTbl != nil && oldTbl.PrimaryKey != nil {
				upStmts = append(upStmts, g.dialect.DropPrimaryKey(e.Table, *oldTbl.PrimaryKey)...)
			}
			upStmts = append(upStmts, g.dialect.AddPrimaryKey(e.Table, *tbl.PrimaryKey)...)
		}
	}

	for _, e := range fkAdds {
		tbl := g.findTable(e.Table)
		if tbl != nil {
			for _, fk := range tbl.ForeignKeys {
				if fk.Name == e.ConstraintName {
					upStmts = append(upStmts, g.dialect.AddForeignKey(e.Table, fk)...)
					break
				}
			}
		}
	}

	for _, e := range uqAdds {
		tbl := g.findTable(e.Table)
		if tbl != nil {
			for _, uq := range tbl.Uniques {
				if uq.Name == e.ConstraintName {
					upStmts = append(upStmts, g.dialect.AddUnique(e.Table, uq)...)
					break
				}
			}
		}
	}

	for _, e := range checkAdds {
		tbl := g.findTable(e.Table)
		if tbl != nil {
			for _, chk := range tbl.Checks {
				if chk.Name == e.ConstraintName {
					upStmts = append(upStmts, g.dialect.AddCheck(e.Table, chk)...)
					break
				}
			}
		}
	}

	for _, e := range indexAdds {
		tbl := g.findTable(e.Table)
		if tbl != nil {
			for _, idx := range tbl.Indexes {
				if idx.Name == e.ConstraintName {
					upStmts = append(upStmts, g.dialect.CreateIndex(idx)...)
					break
				}
			}
		}
	}

	// Functions and Procedures (after tables - may reference table columns)
	for _, e := range funcAdds {
		if e.NewFunction != nil {
			upStmts = append(upStmts, g.dialect.CreateFunction(*e.NewFunction)...)
		}
	}
	for _, e := range funcChanges {
		if e.OldFunction != nil {
			upStmts = append(upStmts, g.dialect.DropFunction(*e.OldFunction)...)
		}
		if e.NewFunction != nil {
			upStmts = append(upStmts, g.dialect.CreateFunction(*e.NewFunction)...)
		}
	}
	for _, e := range procAdds {
		if e.NewProcedure != nil {
			upStmts = append(upStmts, g.dialect.CreateProcedure(*e.NewProcedure)...)
		}
	}
	for _, e := range procChanges {
		if e.OldProcedure != nil {
			upStmts = append(upStmts, g.dialect.DropProcedure(*e.OldProcedure)...)
		}
		if e.NewProcedure != nil {
			upStmts = append(upStmts, g.dialect.CreateProcedure(*e.NewProcedure)...)
		}
	}

	// Triggers (immediately after functions)
	for _, e := range triggerAdds {
		if e.NewTrigger != nil {
			upStmts = append(upStmts, g.dialect.CreateTrigger(*e.NewTrigger)...)
		}
	}
	for _, e := range triggerChanges {
		if e.OldTrigger != nil {
			upStmts = append(upStmts, g.dialect.DropTrigger(*e.OldTrigger)...)
		}
		if e.NewTrigger != nil {
			upStmts = append(upStmts, g.dialect.CreateTrigger(*e.NewTrigger)...)
		}
	}

	// Views and Materialized Views (after tables, functions, and triggers)
	for _, e := range viewAdds {
		if e.NewView != nil {
			upStmts = append(upStmts, g.dialect.CreateView(*e.NewView)...)
		}
	}
	for _, e := range viewChanges {
		if e.NewView != nil {
			upStmts = append(upStmts, g.dialect.CreateView(*e.NewView)...)
		}
	}
	for _, e := range matViewAdds {
		if e.NewMaterializedView != nil {
			upStmts = append(upStmts, g.dialect.CreateMaterializedView(*e.NewMaterializedView)...)
		}
	}
	for _, e := range matViewChanges {
		if e.OldMaterializedView != nil {
			upStmts = append(upStmts, g.dialect.DropMaterializedView(*e.OldMaterializedView)...)
		}
		if e.NewMaterializedView != nil {
			upStmts = append(upStmts, g.dialect.CreateMaterializedView(*e.NewMaterializedView)...)
		}
	}

	// Grants (after tables/views exist)
	for _, e := range grantAdds {
		if e.NewGrant != nil {
			upStmts = append(upStmts, g.dialect.GrantSQL(*e.NewGrant))
		}
	}

	// Policies (after tables exist)
	for _, e := range policyAdds {
		if e.NewPolicy != nil {
			upStmts = append(upStmts, g.dialect.CreatePolicy(*e.NewPolicy)...)
		}
	}
	for _, e := range policyChanges {
		if e.OldPolicy != nil {
			upStmts = append(upStmts, g.dialect.DropPolicy(*e.OldPolicy)...)
		}
		if e.NewPolicy != nil {
			upStmts = append(upStmts, g.dialect.CreatePolicy(*e.NewPolicy)...)
		}
	}

	for _, e := range columnDrops {
		upStmts = append(upStmts, g.dialect.DropColumn(e.Table, e.Column)...)
	}

	for _, e := range fkDrops {
		upStmts = append(upStmts, g.dialect.DropForeignKey(e.Table, e.ConstraintName)...)
	}
	for _, e := range uqDrops {
		upStmts = append(upStmts, g.dialect.DropUnique(e.Table, e.ConstraintName)...)
	}
	for _, e := range checkDrops {
		upStmts = append(upStmts, g.dialect.DropCheck(e.Table, e.ConstraintName)...)
	}
	for _, e := range indexDrops {
		idx := schema.Index{Name: e.ConstraintName, Table: e.Table}
		upStmts = append(upStmts, g.dialect.DropIndex(idx)...)
	}

	// Drop tables in reverse dependency order (dependents before their
	// dependencies) so FK constraints are never violated mid-migration.
	oldTableOrder, err := graph.Build(g.oldPkg.Tables).Sort()
	if err != nil {
		return nil, fmt.Errorf("building dependency graph for table drops: %w", err)
	}
	dropTableStmts := make(map[string][]string)
	for _, e := range tableDrops {
		if e.OldTable != nil {
			dropTableStmts[e.OldTable.SQLName()] = g.dialect.DropTable(*e.OldTable)
		}
	}
	for i := len(oldTableOrder) - 1; i >= 0; i-- {
		if ds, ok := dropTableStmts[oldTableOrder[i]]; ok {
			upStmts = append(upStmts, ds...)
		}
	}

	// Drop registered objects in reverse dependency order
	for _, e := range policyDrops {
		if e.OldPolicy != nil {
			upStmts = append(upStmts, g.dialect.DropPolicy(*e.OldPolicy)...)
		}
	}
	for _, e := range grantDrops {
		if e.OldGrant != nil {
			upStmts = append(upStmts, g.dialect.RevokeSQL(*e.OldGrant))
		}
	}
	for _, e := range triggerDrops {
		if e.OldTrigger != nil {
			upStmts = append(upStmts, g.dialect.DropTrigger(*e.OldTrigger)...)
		}
	}
	for _, e := range matViewDrops {
		if e.OldMaterializedView != nil {
			upStmts = append(upStmts, g.dialect.DropMaterializedView(*e.OldMaterializedView)...)
		}
	}
	for _, e := range viewDrops {
		if e.OldView != nil {
			upStmts = append(upStmts, g.dialect.DropView(*e.OldView)...)
		}
	}
	for _, e := range procDrops {
		if e.OldProcedure != nil {
			upStmts = append(upStmts, g.dialect.DropProcedure(*e.OldProcedure)...)
		}
	}
	for _, e := range funcDrops {
		if e.OldFunction != nil {
			upStmts = append(upStmts, g.dialect.DropFunction(*e.OldFunction)...)
		}
	}

	for _, e := range enumDrops {
		for _, en := range g.oldPkg.Enums {
			if en.SQLName() == e.Enum {
				upStmts = append(upStmts, g.dialect.DropEnumSQL(&en))
				break
			}
		}
	}

	for _, e := range enumValueAdds {
		for _, en := range g.newPkg.Enums {
			if en.SQLName() == e.Enum {
				stmt := g.dialect.AlterEnumAddValue(&en, e.NewValue)
				if strings.HasPrefix(stmt, "-- requires autocommit") {
					autocommitStmts = append(autocommitStmts, stmt)
				} else {
					upStmts = append(upStmts, stmt)
				}
				break
			}
		}
	}

	for _, e := range enumValueDrops {
		found := false
		for _, en := range g.newPkg.Enums {
			if en.SQLName() == e.Enum {
				found = true
				break
			}
		}
		if !found {
			for _, en := range g.oldPkg.Enums {
				if en.SQLName() == e.Enum {
					found = true
					break
				}
			}
		}
		if found {
			if g.dialect.SupportsAddEnumValueInTransaction() {
				upStmts = append(upStmts, fmt.Sprintf("-- cannot reverse: enum value %q cannot be removed", e.OldValue))
			}
		}
	}

	for _, e := range comments {
		upStmts = append(upStmts, g.dialect.SetTableComment(e.Table, e.NewValue))
	}

	downStmts, err = g.generateDown()
	if err != nil {
		return nil, err
	}

	version := time.Now().UTC().Format("20060102150405")
	description := g.describe()

	checksum := checksum.Compute(upStmts)

	return &MigrationFile{
		Version:              version,
		Description:          description,
		UpStatements:         upStmts,
		DownStatements:       downStmts,
		AutocommitStatements: autocommitStmts,
		HasDestructive:       hasDestructive,
		Checksum:             checksum,
	}, nil
}

func (g *Generator) generateDown() ([]string, error) {
	var stmts []string
	var enumDropStmts []string
	var extensionDropStmts []string
	tableDropStmts := make(map[string][]string)

	tableOrder, err := graph.Build(g.newPkg.Tables).Sort()
	if err != nil {
		return nil, fmt.Errorf("building dependency graph for down migration: %w", err)
	}

	reversed := make([]diff.DeltaEvent, len(g.events))
	copy(reversed, g.events)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}

	for _, e := range reversed {
		switch e.Kind {
		case diff.TableAdded:
			if e.NewTable != nil {
				tableDropStmts[e.NewTable.SQLName()] = g.dialect.DropTable(*e.NewTable)
			}
		case diff.TableDropped:
			if e.OldTable != nil {
				tbl := *e.OldTable
				if !g.dialect.SupportsEnum() {
					tbl = g.injectEnumChecks(tbl)
				}
				stmts = append(stmts, g.dialect.CreateTable(tbl)...)
			}
		case diff.ColumnAdded:
			stmts = append(stmts, g.dialect.DropColumn(e.Table, e.Column)...)
		case diff.ColumnDropped:
			if e.OldColumn != nil {
				col := *e.OldColumn
				if !g.dialect.SupportsEnum() {
					col = g.injectColumnEnumCheck(e.Table, col)
				}
				stmts = append(stmts, g.dialect.AddColumn(e.Table, col)...)
			}
		case diff.ColumnRenamed:
			if e.OldValue != "" && e.NewValue != "" {
				stmts = append(stmts, g.dialect.RenameColumn(e.Table, e.NewValue, e.OldValue))
			}
		case diff.ColumnTypeChanged:
			if e.OldColumn != nil {
				stmts = append(stmts, g.dialect.AlterColumnType(e.Table, *e.OldColumn)...)
			}
		case diff.ColumnNullChanged:
			if e.OldColumn != nil {
				stmts = append(stmts, g.dialect.AlterColumnNullability(e.Table, *e.OldColumn)...)
			}
		case diff.ColumnDefaultChanged:
			if e.OldColumn != nil {
				stmts = append(stmts, g.dialect.AlterColumnDefault(e.Table, *e.OldColumn)...)
			}
		case diff.FKAdded:
			if e.ConstraintName != "" {
				stmts = append(stmts, g.dialect.DropForeignKey(e.Table, e.ConstraintName)...)
			}
		case diff.UniqueAdded:
			if e.ConstraintName != "" {
				stmts = append(stmts, g.dialect.DropUnique(e.Table, e.ConstraintName)...)
			}
		case diff.IndexAdded:
			if e.ConstraintName != "" {
				idx := schema.Index{Name: e.ConstraintName, Table: e.Table}
				stmts = append(stmts, g.dialect.DropIndex(idx)...)
			}
		case diff.CheckAdded:
			if e.ConstraintName != "" {
				stmts = append(stmts, g.dialect.DropCheck(e.Table, e.ConstraintName)...)
			}
		case diff.PKChanged:
			if newTbl := g.findTable(e.Table); newTbl != nil && newTbl.PrimaryKey != nil {
				stmts = append(stmts, g.dialect.DropPrimaryKey(e.Table, *newTbl.PrimaryKey)...)
			}
			if oldTbl := g.findOldTable(e.Table); oldTbl != nil && oldTbl.PrimaryKey != nil {
				stmts = append(stmts, g.dialect.AddPrimaryKey(e.Table, *oldTbl.PrimaryKey)...)
			}
		case diff.EnumAdded:
			for _, en := range g.newPkg.Enums {
				if en.SQLName() == e.Enum {
					enumDropStmts = append(enumDropStmts, g.dialect.DropEnumSQL(&en))
					break
				}
			}
		case diff.EnumDropped:
			for _, en := range g.oldPkg.Enums {
				if en.SQLName() == e.Enum {
					stmts = append(stmts, g.dialect.CreateEnumSQL(&en))
					break
				}
			}
		case diff.FunctionAdded:
			if e.NewFunction != nil {
				stmts = append(stmts, g.dialect.DropFunction(*e.NewFunction)...)
			}
		case diff.FunctionDropped:
			if e.OldFunction != nil {
				stmts = append(stmts, g.dialect.CreateFunction(*e.OldFunction)...)
			}
		case diff.FunctionBodyChanged:
			if e.OldFunction != nil {
				stmts = append(stmts, g.dialect.CreateFunction(*e.OldFunction)...)
			}
		case diff.ViewAdded:
			if e.NewView != nil {
				stmts = append(stmts, g.dialect.DropView(*e.NewView)...)
			}
		case diff.ViewDropped:
			if e.OldView != nil {
				stmts = append(stmts, g.dialect.CreateView(*e.OldView)...)
			}
		case diff.ViewQueryChanged:
			if e.OldView != nil {
				stmts = append(stmts, g.dialect.CreateView(*e.OldView)...)
			}
		case diff.MaterializedViewAdded:
			if e.NewMaterializedView != nil {
				stmts = append(stmts, g.dialect.DropMaterializedView(*e.NewMaterializedView)...)
			}
		case diff.MaterializedViewDropped:
			if e.OldMaterializedView != nil {
				stmts = append(stmts, g.dialect.CreateMaterializedView(*e.OldMaterializedView)...)
			}
		case diff.MaterializedViewQueryChanged:
			if e.OldMaterializedView != nil {
				stmts = append(stmts, g.dialect.CreateMaterializedView(*e.OldMaterializedView)...)
			}
		case diff.TriggerAdded:
			if e.NewTrigger != nil {
				stmts = append(stmts, g.dialect.DropTrigger(*e.NewTrigger)...)
			}
		case diff.TriggerDropped:
			if e.OldTrigger != nil {
				stmts = append(stmts, g.dialect.CreateTrigger(*e.OldTrigger)...)
			}
		case diff.TriggerChanged:
			if e.OldTrigger != nil {
				stmts = append(stmts, g.dialect.CreateTrigger(*e.OldTrigger)...)
			}
		case diff.ProcedureAdded:
			if e.NewProcedure != nil {
				stmts = append(stmts, g.dialect.DropProcedure(*e.NewProcedure)...)
			}
		case diff.ProcedureDropped:
			if e.OldProcedure != nil {
				stmts = append(stmts, g.dialect.CreateProcedure(*e.OldProcedure)...)
			}
		case diff.ProcedureBodyChanged:
			if e.OldProcedure != nil {
				stmts = append(stmts, g.dialect.CreateProcedure(*e.OldProcedure)...)
			}
		case diff.GrantAdded:
			if e.NewGrant != nil {
				stmts = append(stmts, g.dialect.RevokeSQL(*e.NewGrant))
			}
		case diff.GrantDropped:
			if e.OldGrant != nil {
				stmts = append(stmts, g.dialect.GrantSQL(*e.OldGrant))
			}
		case diff.PolicyAdded:
			if e.NewPolicy != nil {
				stmts = append(stmts, g.dialect.DropPolicy(*e.NewPolicy)...)
			}
		case diff.PolicyDropped:
			if e.OldPolicy != nil {
				stmts = append(stmts, g.dialect.CreatePolicy(*e.OldPolicy)...)
			}
		case diff.PolicyChanged:
			if e.OldPolicy != nil {
				stmts = append(stmts, g.dialect.CreatePolicy(*e.OldPolicy)...)
			}
		case diff.ExtensionAdded:
			if e.NewExtension != nil {
				stmts = append(stmts, g.dialect.DropExtension(*e.NewExtension))
			}
		case diff.ExtensionDropped:
			if e.OldExtension != nil {
				stmts = append(stmts, g.dialect.CreateExtension(*e.OldExtension))
			}
		case diff.ExtensionVersionChanged:
			if e.OldExtension != nil {
				stmts = append(stmts, g.dialect.AlterExtensionVersion(e.OldExtension.Name, e.OldExtension.Version))
			}
		}
	}

	// Tables must be dropped in the exact reverse of their creation order
	// (dependents before what they depend on), not in whatever order the
	// alphabetically-sorted diff events happened to reverse to.
	for i := len(tableOrder) - 1; i >= 0; i-- {
		if ds, ok := tableDropStmts[tableOrder[i]]; ok {
			stmts = append(stmts, ds...)
		}
	}

	// Enum types can only be dropped once nothing references them anymore,
	// i.e. after every table drop above has run.
	stmts = append(stmts, enumDropStmts...)

	// Extensions are dropped last: by the time we get here every dependent
	// object (tables, types, functions) has already been removed.
	for _, e := range extensionDropStmts {
		stmts = append(stmts, e)
	}

	return stmts, nil
}

func (g *Generator) filterEvents(kind diff.EventKind) []diff.DeltaEvent {
	var result []diff.DeltaEvent
	for _, e := range g.events {
		if e.Kind == kind {
			result = append(result, e)
		}
	}
	return result
}

func (g *Generator) findTable(name string) *schema.Table {
	for i := range g.newPkg.Tables {
		if g.newPkg.Tables[i].SQLName() == name {
			return &g.newPkg.Tables[i]
		}
	}
	return nil
}

func (g *Generator) findOldTable(name string) *schema.Table {
	for i := range g.oldPkg.Tables {
		if g.oldPkg.Tables[i].SQLName() == name {
			return &g.oldPkg.Tables[i]
		}
	}
	return nil
}

func (g *Generator) describe() string {
	tableEvents := make(map[string][]diff.DeltaEvent)
	for _, e := range g.events {
		if e.Table != "" {
			tableEvents[e.Table] = append(tableEvents[e.Table], e)
		}
	}

	if len(g.events) == 1 {
		e := g.events[0]
		switch e.Kind {
		case diff.TableAdded:
			return "create_" + e.Table
		case diff.TableDropped:
			return "drop_table_" + e.Table
		case diff.ColumnAdded:
			return "add_" + e.Column + "_to_" + e.Table
		case diff.ColumnDropped:
			return "drop_" + e.Column + "_from_" + e.Table
		case diff.ColumnRenamed:
			return "rename_" + e.OldValue + "_to_" + e.NewValue + "_in_" + e.Table
		}
	}

	if len(g.events) == 2 {
		var parts []string
		for _, e := range g.events {
			switch e.Kind {
			case diff.ColumnAdded:
				parts = append(parts, "add_"+e.Column)
			case diff.ColumnDropped:
				parts = append(parts, "drop_"+e.Column)
			}
		}
		if len(parts) == 2 {
			table := ""
			for _, e := range g.events {
				if e.Table != "" {
					table = e.Table
					break
				}
			}
			return strings.Join(parts, "_") + "_" + table
		}
	}

	return fmt.Sprintf("batch_%d_changes", len(g.events))
}

func (g *Generator) injectEnumChecks(t schema.Table) schema.Table {
	newTable := t
	for i, col := range t.Columns {
		for _, en := range g.newPkg.Enums {
			if col.SQLType == en.Name || col.SQLType == en.SQLName() {
				newTable.Columns[i].SQLType = g.dialect.EnumSQL(&en, *col)
				vals := make([]string, len(en.Values))
				for j, v := range en.Values {
					vals[j] = "'" + v + "'"
				}
				expr := fmt.Sprintf("%s IN (%s)", g.dialect.QuoteIdent(col.Name), strings.Join(vals, ", "))
				newTable.Checks = append(newTable.Checks, schema.CheckConstraint{
					Name:       fmt.Sprintf("chk_%s_%s_enum", t.Name, col.Name),
					Expression: expr,
				})
			}
		}
	}
	return newTable
}

func (g *Generator) injectColumnEnumCheck(_ string, col schema.Column) schema.Column {
	newCol := col
	for _, en := range g.newPkg.Enums {
		if col.SQLType == en.Name || col.SQLType == en.SQLName() {
			newCol.SQLType = g.dialect.EnumSQL(&en, col)
			vals := make([]string, len(en.Values))
			for j, v := range en.Values {
				vals[j] = "'" + v + "'"
			}
			newCol.CheckConstraint = fmt.Sprintf("%s IN (%s)", g.dialect.QuoteIdent(col.Name), strings.Join(vals, ", "))
		}
	}
	return newCol
}
