package diff

import (
	"encoding/json"
	"reflect"
	"sort"

	"github.com/justblue/mirage/internal/schema"
)

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func PackagesEqual(a, b *schema.Package) bool {
	return reflect.DeepEqual(a, b)
}

func PackagesEqualJSON(a, b *schema.Package) (bool, string, string) {
	ja, err := json.Marshal(a)
	if err != nil {
		return false, "", ""
	}
	jb, err := json.Marshal(b)
	if err != nil {
		return false, "", ""
	}
	eq := string(ja) == string(jb)
	return eq, string(ja), string(jb)
}

type EventKind string

const (
	TableAdded              EventKind = "table_added"
	TableDropped            EventKind = "table_dropped"
	ColumnAdded             EventKind = "column_added"
	ColumnDropped           EventKind = "column_dropped"
	ColumnRenamed           EventKind = "column_renamed"
	ColumnTypeChanged       EventKind = "column_type_changed"
	ColumnNullChanged       EventKind = "column_null_changed"
	ColumnDefaultChanged    EventKind = "column_default_changed"
	IndexAdded              EventKind = "index_added"
	IndexDropped            EventKind = "index_dropped"
	UniqueAdded             EventKind = "unique_added"
	UniqueDropped           EventKind = "unique_dropped"
	FKAdded                 EventKind = "fk_added"
	FKDropped               EventKind = "fk_dropped"
	CheckAdded              EventKind = "check_added"
	CheckDropped            EventKind = "check_dropped"
	PKChanged               EventKind = "pk_changed"
	EnumAdded               EventKind = "enum_added"
	EnumDropped             EventKind = "enum_dropped"
	EnumValueAdded          EventKind = "enum_value_added"
	EnumValueDropped        EventKind = "enum_value_dropped"
	ExtensionAdded          EventKind = "extension_added"
	ExtensionDropped        EventKind = "extension_dropped"
	ExtensionVersionChanged EventKind = "extension_version_changed"
	CommentChanged          EventKind = "comment_changed"

	FunctionAdded                EventKind = "function_added"
	FunctionDropped              EventKind = "function_dropped"
	FunctionBodyChanged          EventKind = "function_body_changed"
	ViewAdded                    EventKind = "view_added"
	ViewDropped                  EventKind = "view_dropped"
	ViewQueryChanged             EventKind = "view_query_changed"
	MaterializedViewAdded        EventKind = "materialized_view_added"
	MaterializedViewDropped      EventKind = "materialized_view_dropped"
	MaterializedViewQueryChanged EventKind = "materialized_view_query_changed"
	TriggerAdded                 EventKind = "trigger_added"
	TriggerDropped               EventKind = "trigger_dropped"
	TriggerChanged               EventKind = "trigger_changed"
	ProcedureAdded               EventKind = "procedure_added"
	ProcedureDropped             EventKind = "procedure_dropped"
	ProcedureBodyChanged         EventKind = "procedure_body_changed"
	GrantAdded                   EventKind = "grant_added"
	GrantDropped                 EventKind = "grant_dropped"
	PolicyAdded                  EventKind = "policy_added"
	PolicyDropped                EventKind = "policy_dropped"
	PolicyChanged                EventKind = "policy_changed"
)

type Severity int

const (
	Info Severity = iota
	Warning
	Destructive
)

type DeltaEvent struct {
	Kind           EventKind
	Table          string
	Enum           string
	Column         string
	ConstraintName string
	OldValue       string
	NewValue       string
	Severity       Severity
	OldTable       *schema.Table
	NewTable       *schema.Table
	OldColumn      *schema.Column
	NewColumn      *schema.Column

	OldFunction         *schema.Function
	NewFunction         *schema.Function
	OldView             *schema.View
	NewView             *schema.View
	OldMaterializedView *schema.MaterializedView
	NewMaterializedView *schema.MaterializedView
	OldTrigger          *schema.Trigger
	NewTrigger          *schema.Trigger
	OldProcedure        *schema.Procedure
	NewProcedure        *schema.Procedure
	OldGrant            *schema.Grant
	NewGrant            *schema.Grant
	OldPolicy           *schema.Policy
	NewPolicy           *schema.Policy
	OldExtension        *schema.Extension
	NewExtension        *schema.Extension
}

func Diff(old, new *schema.Package) []DeltaEvent {
	if PackagesEqual(old, new) {
		return nil
	}

	if old == nil {
		old = &schema.Package{}
	}
	if new == nil {
		new = &schema.Package{}
	}

	var events []DeltaEvent

	oldTables := make(map[string]*schema.Table)
	for i := range old.Tables {
		t := &old.Tables[i]
		oldTables[t.SQLName()] = t
	}

	newTables := make(map[string]*schema.Table)
	for i := range new.Tables {
		t := &new.Tables[i]
		newTables[t.SQLName()] = t
	}

	for _, name := range sortedKeys(newTables) {
		nt := newTables[name]
		ot, ok := oldTables[name]
		if !ok {
			events = append(events, DeltaEvent{
				Kind:     TableAdded,
				Table:    name,
				Severity: Info,
				NewTable: nt,
			})
			// Indexes, unique constraints, FK, and check constraints are
			// emitted as part of the CREATE TABLE statement by the generator,
			// so we must NOT also emit separate IndexAdded/UniqueAdded/...
			// events here (that would create them twice).
			continue
		}
		events = append(events, diffTables(ot, nt)...)
	}

	for _, name := range sortedKeys(oldTables) {
		ot := oldTables[name]
		if _, ok := newTables[name]; !ok {
			events = append(events, DeltaEvent{
				Kind:     TableDropped,
				Table:    name,
				Severity: Destructive,
				OldTable: ot,
			})
		}
	}

	oldEnums := make(map[string]*schema.Enum)
	for i := range old.Enums {
		e := &old.Enums[i]
		oldEnums[e.SQLName()] = e
	}

	newEnums := make(map[string]*schema.Enum)
	for i := range new.Enums {
		e := &new.Enums[i]
		newEnums[e.SQLName()] = e
	}

	for _, name := range sortedKeys(newEnums) {
		ne := newEnums[name]
		oe, ok := oldEnums[name]
		if !ok {
			events = append(events, DeltaEvent{
				Kind:     EnumAdded,
				Enum:     name,
				Severity: Info,
			})
			continue
		}
		events = append(events, diffEnums(oe, ne)...)
	}

	for _, name := range sortedKeys(oldEnums) {
		if _, ok := newEnums[name]; !ok {
			events = append(events, DeltaEvent{
				Kind:     EnumDropped,
				Enum:     name,
				Severity: Destructive,
			})
		}
	}

	events = append(events, diffRegisteredObjects(old, new)...)

	return events
}

func diffTables(old, new *schema.Table) []DeltaEvent {
	var events []DeltaEvent
	name := old.SQLName()

	oldCols := make(map[string]*schema.Column)
	for _, c := range old.Columns {
		oldCols[c.Name] = c
	}

	newCols := make(map[string]*schema.Column)
	for _, c := range new.Columns {
		newCols[c.Name] = c
	}

	for _, cname := range sortedKeys(newCols) {
		nc := newCols[cname]
		oc, ok := oldCols[cname]
		if !ok {
			events = append(events, DeltaEvent{
				Kind:      ColumnAdded,
				Table:     name,
				Column:    cname,
				Severity:  Info,
				NewColumn: nc,
			})
			continue
		}
		if oc.SQLType != nc.SQLType {
			events = append(events, DeltaEvent{
				Kind:      ColumnTypeChanged,
				Table:     name,
				Column:    cname,
				OldValue:  oc.SQLType,
				NewValue:  nc.SQLType,
				Severity:  Warning,
				OldColumn: oc,
				NewColumn: nc,
			})
		}
		if oc.Nullable != nc.Nullable {
			events = append(events, DeltaEvent{
				Kind:      ColumnNullChanged,
				Table:     name,
				Column:    cname,
				OldValue:  boolStr(oc.Nullable),
				NewValue:  boolStr(nc.Nullable),
				Severity:  Warning,
				OldColumn: oc,
				NewColumn: nc,
			})
		}
		if oc.Default != nc.Default {
			events = append(events, DeltaEvent{
				Kind:      ColumnDefaultChanged,
				Table:     name,
				Column:    cname,
				OldValue:  oc.Default,
				NewValue:  nc.Default,
				Severity:  Info,
				OldColumn: oc,
				NewColumn: nc,
			})
		}
	}

	for _, cname := range sortedKeys(oldCols) {
		oc := oldCols[cname]
		if _, ok := newCols[cname]; !ok {
			events = append(events, DeltaEvent{
				Kind:      ColumnDropped,
				Table:     name,
				Column:    cname,
				Severity:  Destructive,
				OldColumn: oc,
			})
		}
	}

	if !pkEqual(old.PrimaryKey, new.PrimaryKey) {
		events = append(events, DeltaEvent{
			Kind:     PKChanged,
			Table:    name,
			Severity: Warning,
		})
	}

	diffConstraints(name, "fk", old.ForeignKeys, new.ForeignKeys, &events, func(fk schema.ForeignKey) string { return fk.Name })
	diffConstraints(name, "uq", old.Uniques, new.Uniques, &events, func(u schema.UniqueConstraint) string { return u.Name })
	diffConstraints(name, "idx", old.Indexes, new.Indexes, &events, func(idx schema.Index) string { return idx.Name })
	diffConstraints(name, "chk", old.Checks, new.Checks, &events, func(c schema.CheckConstraint) string { return c.Name })

	if old.Description != new.Description {
		events = append(events, DeltaEvent{
			Kind:     CommentChanged,
			Table:    name,
			OldValue: old.Description,
			NewValue: new.Description,
			Severity: Info,
		})
	}

	return events
}

func diffEnums(old, new *schema.Enum) []DeltaEvent {
	var events []DeltaEvent
	name := old.SQLName()

	oldVals := make(map[string]bool)
	for _, v := range old.Values {
		oldVals[v] = true
	}

	newVals := make(map[string]bool)
	for _, v := range new.Values {
		newVals[v] = true
	}

	for _, v := range new.Values {
		if !oldVals[v] {
			events = append(events, DeltaEvent{
				Kind:     EnumValueAdded,
				Enum:     name,
				NewValue: v,
				Severity: Info,
			})
		}
	}

	for _, v := range old.Values {
		if !newVals[v] {
			events = append(events, DeltaEvent{
				Kind:     EnumValueDropped,
				Enum:     name,
				OldValue: v,
				Severity: Destructive,
			})
		}
	}

	return events
}

func diffConstraints[T any](table, kind string, oldList, newList []T, events *[]DeltaEvent, nameFn func(T) string) {
	oldMap := make(map[string]T)
	for _, o := range oldList {
		oldMap[nameFn(o)] = o
	}

	newMap := make(map[string]T)
	for _, n := range newList {
		newMap[nameFn(n)] = n
	}

	for _, name := range sortedKeys(newMap) {
		if _, ok := oldMap[name]; !ok {
			switch kind {
			case "fk":
				*events = append(*events, DeltaEvent{Kind: FKAdded, Table: table, ConstraintName: name, Severity: Info})
			case "uq":
				*events = append(*events, DeltaEvent{Kind: UniqueAdded, Table: table, ConstraintName: name, Severity: Info})
			case "idx":
				*events = append(*events, DeltaEvent{Kind: IndexAdded, Table: table, ConstraintName: name, Severity: Info})
			case "chk":
				*events = append(*events, DeltaEvent{Kind: CheckAdded, Table: table, ConstraintName: name, Severity: Info})
			}
		}
	}

	for _, name := range sortedKeys(oldMap) {
		if _, ok := newMap[name]; !ok {
			switch kind {
			case "fk":
				*events = append(*events, DeltaEvent{Kind: FKDropped, Table: table, ConstraintName: name, Severity: Warning})
			case "uq":
				*events = append(*events, DeltaEvent{Kind: UniqueDropped, Table: table, ConstraintName: name, Severity: Warning})
			case "idx":
				*events = append(*events, DeltaEvent{Kind: IndexDropped, Table: table, ConstraintName: name, Severity: Info})
			case "chk":
				*events = append(*events, DeltaEvent{Kind: CheckDropped, Table: table, ConstraintName: name, Severity: Warning})
			}
		}
	}
}

func pkEqual(a, b *schema.PrimaryKey) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a.Columns) != len(b.Columns) {
		return false
	}
	// Order matters: PRIMARY KEY (a, b) is not equivalent to (b, a) for
	// index structure and query planning, so compare positionally.
	for i := range a.Columns {
		if a.Columns[i] != b.Columns[i] {
			return false
		}
	}
	return true
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

type RenameHint struct {
	Table     string
	OldColumn string
	NewColumn string
}

func DetectColumnRenames(oldTree, newTree *schema.Package) []RenameHint {
	var hints []RenameHint

	oldTables := make(map[string]*schema.Table, len(oldTree.Tables))
	for i := range oldTree.Tables {
		oldTables[oldTree.Tables[i].Name] = &oldTree.Tables[i]
	}
	newTables := make(map[string]*schema.Table, len(newTree.Tables))
	for i := range newTree.Tables {
		newTables[newTree.Tables[i].Name] = &newTree.Tables[i]
	}

	for _, nt := range newTree.Tables {
		ot, ok := oldTables[nt.Name]
		if !ok {
			continue
		}
		oldCols := make(map[string]bool, len(ot.Columns))
		for _, c := range ot.Columns {
			oldCols[c.Name] = true
		}
		newCols := make(map[string]bool, len(nt.Columns))
		for _, c := range nt.Columns {
			newCols[c.Name] = true
		}
		var dropped []string
		for _, c := range ot.Columns {
			if !newCols[c.Name] {
				dropped = append(dropped, c.Name)
			}
		}
		var added []string
		for _, c := range nt.Columns {
			if !oldCols[c.Name] {
				added = append(added, c.Name)
			}
		}
		for _, d := range dropped {
			bestDist := -1
			bestAdd := ""
			for _, a := range added {
				dist := editDistance(d, a)
				if bestDist < 0 || dist < bestDist {
					bestDist = dist
					bestAdd = a
				}
			}
			if bestDist >= 0 && bestDist <= 3 && bestDist < len(d)/2 {
				hints = append(hints, RenameHint{
					Table:     nt.Name,
					OldColumn: d,
					NewColumn: bestAdd,
				})
			}
		}
	}
	return hints
}

// ApplyRenameHints rewrites a slice of diff events, collapsing each
// ColumnDropped + ColumnAdded pair identified by a confirmed RenameHint into a
// single ColumnRenamed event. Events that do not correspond to a confirmed
// hint are left untouched. The ColumnRenamed event carries the old column name
// in OldValue and the new column name in NewValue.
func ApplyRenameHints(events []DeltaEvent, hints []RenameHint) []DeltaEvent {
	if len(hints) == 0 {
		return events
	}

	type renameKey struct {
		table string
		old   string
		new   string
	}
	confirmed := make(map[renameKey]bool, len(hints))
	for _, h := range hints {
		confirmed[renameKey{table: h.Table, old: h.OldColumn, new: h.NewColumn}] = true
	}

	// Track which (table, column) drops and adds are consumed by a rename so we
	// can skip the originals and emit the rename in place of the drop.
	droppedConsumed := make(map[[2]string]bool)
	addedConsumed := make(map[[2]string]bool)
	for k := range confirmed {
		droppedConsumed[[2]string{k.table, k.old}] = false
		addedConsumed[[2]string{k.table, k.new}] = false
	}

	result := make([]DeltaEvent, 0, len(events))
	for _, e := range events {
		switch e.Kind {
		case ColumnDropped:
			// Does this drop participate in a confirmed rename?
			var matched *renameKey
			for k := range confirmed {
				if k.table == e.Table && k.old == e.Column {
					kk := k
					matched = &kk
					break
				}
			}
			if matched != nil {
				result = append(result, DeltaEvent{
					Kind:      ColumnRenamed,
					Table:     matched.table,
					Column:    matched.new,
					OldValue:  matched.old,
					NewValue:  matched.new,
					Severity:  Info,
					OldColumn: e.OldColumn,
				})
				continue
			}
			result = append(result, e)
		case ColumnAdded:
			// Drop the matching add; the rename already covers it.
			consumed := false
			for k := range confirmed {
				if k.table == e.Table && k.new == e.Column {
					consumed = true
					break
				}
			}
			if consumed {
				continue
			}
			result = append(result, e)
		default:
			result = append(result, e)
		}
	}
	return result
}

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func diffRegisteredObjects(old, new *schema.Package) []DeltaEvent {
	var events []DeltaEvent

	// Extensions
	oldExts := make(map[string]*schema.Extension)
	for i := range old.Extensions {
		e := &old.Extensions[i]
		oldExts[e.SQLName()] = e
	}
	newExts := make(map[string]*schema.Extension)
	for i := range new.Extensions {
		e := &new.Extensions[i]
		newExts[e.SQLName()] = e
	}
	for _, name := range sortedKeys(newExts) {
		ne := newExts[name]
		oe, ok := oldExts[name]
		if !ok {
			events = append(events, DeltaEvent{Kind: ExtensionAdded, Severity: Info, NewExtension: ne})
			continue
		}
		if oe.Name != ne.Name || oe.Schema != ne.Schema {
			events = append(events, DeltaEvent{Kind: ExtensionDropped, Severity: Destructive, OldExtension: oe})
			events = append(events, DeltaEvent{Kind: ExtensionAdded, Severity: Info, NewExtension: ne})
		} else if oe.Version != ne.Version {
			events = append(events, DeltaEvent{Kind: ExtensionVersionChanged, Severity: Info, OldExtension: oe, NewExtension: ne})
		}
	}
	for _, name := range sortedKeys(oldExts) {
		if _, ok := newExts[name]; !ok {
			events = append(events, DeltaEvent{Kind: ExtensionDropped, Severity: Destructive, OldExtension: oldExts[name]})
		}
	}

	// Functions
	oldFuncs := make(map[string]*schema.Function)
	for i := range old.Functions {
		f := &old.Functions[i]
		oldFuncs[f.SQLName()] = f
	}
	newFuncs := make(map[string]*schema.Function)
	for i := range new.Functions {
		f := &new.Functions[i]
		newFuncs[f.SQLName()] = f
	}
	for _, name := range sortedKeys(newFuncs) {
		nf := newFuncs[name]
		of, ok := oldFuncs[name]
		if !ok {
			events = append(events, DeltaEvent{Kind: FunctionAdded, Severity: Info, NewFunction: nf})
			continue
		}
		if of.HashBody() != nf.HashBody() || of.Description != nf.Description {
			events = append(events, DeltaEvent{Kind: FunctionBodyChanged, Severity: Info, OldFunction: of, NewFunction: nf})
		}
	}
	for _, name := range sortedKeys(oldFuncs) {
		if _, ok := newFuncs[name]; !ok {
			events = append(events, DeltaEvent{Kind: FunctionDropped, Severity: Warning, OldFunction: oldFuncs[name]})
		}
	}

	// Views
	oldViews := make(map[string]*schema.View)
	for i := range old.Views {
		v := &old.Views[i]
		oldViews[v.SQLName()] = v
	}
	newViews := make(map[string]*schema.View)
	for i := range new.Views {
		v := &new.Views[i]
		newViews[v.SQLName()] = v
	}
	for _, name := range sortedKeys(newViews) {
		nv := newViews[name]
		ov, ok := oldViews[name]
		if !ok {
			events = append(events, DeltaEvent{Kind: ViewAdded, Severity: Info, NewView: nv})
			continue
		}
		if ov.Query != nv.Query || ov.Description != nv.Description {
			events = append(events, DeltaEvent{Kind: ViewQueryChanged, Severity: Info, OldView: ov, NewView: nv})
		}
	}
	for _, name := range sortedKeys(oldViews) {
		if _, ok := newViews[name]; !ok {
			events = append(events, DeltaEvent{Kind: ViewDropped, Severity: Warning, OldView: oldViews[name]})
		}
	}

	// Materialized Views
	oldMatViews := make(map[string]*schema.MaterializedView)
	for i := range old.MaterializedViews {
		mv := &old.MaterializedViews[i]
		oldMatViews[mv.SQLName()] = mv
	}
	newMatViews := make(map[string]*schema.MaterializedView)
	for i := range new.MaterializedViews {
		mv := &new.MaterializedViews[i]
		newMatViews[mv.SQLName()] = mv
	}
	for _, name := range sortedKeys(newMatViews) {
		nmv := newMatViews[name]
		omv, ok := oldMatViews[name]
		if !ok {
			events = append(events, DeltaEvent{Kind: MaterializedViewAdded, Severity: Info, NewMaterializedView: nmv})
			continue
		}
		if omv.Query != nmv.Query || omv.Description != nmv.Description {
			events = append(events, DeltaEvent{Kind: MaterializedViewQueryChanged, Severity: Info, OldMaterializedView: omv, NewMaterializedView: nmv})
		}
	}
	for _, name := range sortedKeys(oldMatViews) {
		if _, ok := newMatViews[name]; !ok {
			events = append(events, DeltaEvent{Kind: MaterializedViewDropped, Severity: Warning, OldMaterializedView: oldMatViews[name]})
		}
	}

	// Triggers
	oldTriggers := make(map[string]*schema.Trigger)
	for i := range old.Triggers {
		t := &old.Triggers[i]
		oldTriggers[t.SQLName()] = t
	}
	newTriggers := make(map[string]*schema.Trigger)
	for i := range new.Triggers {
		t := &new.Triggers[i]
		newTriggers[t.SQLName()] = t
	}
	for _, name := range sortedKeys(newTriggers) {
		nt := newTriggers[name]
		ot, ok := oldTriggers[name]
		if !ok {
			events = append(events, DeltaEvent{Kind: TriggerAdded, Severity: Info, NewTrigger: nt})
			continue
		}
		if ot.HashBody() != nt.HashBody() || ot.Description != nt.Description {
			events = append(events, DeltaEvent{Kind: TriggerChanged, Severity: Info, OldTrigger: ot, NewTrigger: nt})
		}
	}
	for _, name := range sortedKeys(oldTriggers) {
		if _, ok := newTriggers[name]; !ok {
			events = append(events, DeltaEvent{Kind: TriggerDropped, Severity: Warning, OldTrigger: oldTriggers[name]})
		}
	}

	// Procedures
	oldProcs := make(map[string]*schema.Procedure)
	for i := range old.Procedures {
		p := &old.Procedures[i]
		oldProcs[p.SQLName()] = p
	}
	newProcs := make(map[string]*schema.Procedure)
	for i := range new.Procedures {
		p := &new.Procedures[i]
		newProcs[p.SQLName()] = p
	}
	for _, name := range sortedKeys(newProcs) {
		np := newProcs[name]
		op, ok := oldProcs[name]
		if !ok {
			events = append(events, DeltaEvent{Kind: ProcedureAdded, Severity: Info, NewProcedure: np})
			continue
		}
		if op.HashBody() != np.HashBody() || op.Description != np.Description {
			events = append(events, DeltaEvent{Kind: ProcedureBodyChanged, Severity: Info, OldProcedure: op, NewProcedure: np})
		}
	}
	for _, name := range sortedKeys(oldProcs) {
		if _, ok := newProcs[name]; !ok {
			events = append(events, DeltaEvent{Kind: ProcedureDropped, Severity: Warning, OldProcedure: oldProcs[name]})
		}
	}

	// Grants (identified by ObjectType+ObjectName+Roles tuple)
	oldGrants := make(map[string]*schema.Grant)
	for i := range old.Grants {
		g := &old.Grants[i]
		oldGrants[grantKey(g)] = g
	}
	newGrants := make(map[string]*schema.Grant)
	for i := range new.Grants {
		g := &new.Grants[i]
		newGrants[grantKey(g)] = g
	}
	for _, key := range sortedKeys(newGrants) {
		ng := newGrants[key]
		if _, ok := oldGrants[key]; !ok {
			events = append(events, DeltaEvent{Kind: GrantAdded, Severity: Info, NewGrant: ng})
		}
	}
	for _, key := range sortedKeys(oldGrants) {
		if _, ok := newGrants[key]; !ok {
			events = append(events, DeltaEvent{Kind: GrantDropped, Severity: Warning, OldGrant: oldGrants[key]})
		}
	}

	// Policies (identified by Table+Name)
	oldPolicies := make(map[string]*schema.Policy)
	for i := range old.Policies {
		p := &old.Policies[i]
		oldPolicies[policyKey(p)] = p
	}
	newPolicies := make(map[string]*schema.Policy)
	for i := range new.Policies {
		p := &new.Policies[i]
		newPolicies[policyKey(p)] = p
	}
	for _, key := range sortedKeys(newPolicies) {
		np := newPolicies[key]
		op, ok := oldPolicies[key]
		if !ok {
			events = append(events, DeltaEvent{Kind: PolicyAdded, Severity: Info, NewPolicy: np})
			continue
		}
		if op.Command != np.Command || op.Using != np.Using || op.Check != np.Check ||
			op.Permissive != np.Permissive || !stringSliceEqual(op.Roles, np.Roles) {
			events = append(events, DeltaEvent{Kind: PolicyChanged, Severity: Info, OldPolicy: op, NewPolicy: np})
		}
	}
	for _, key := range sortedKeys(oldPolicies) {
		if _, ok := newPolicies[key]; !ok {
			events = append(events, DeltaEvent{Kind: PolicyDropped, Severity: Warning, OldPolicy: oldPolicies[key]})
		}
	}

	return events
}

func grantKey(g *schema.Grant) string {
	return g.SortKey()
}

func policyKey(p *schema.Policy) string {
	return p.SortKey()
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := make([]string, len(a))
	copy(sa, a)
	sort.Strings(sa)
	sb := make([]string, len(b))
	copy(sb, b)
	sort.Strings(sb)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}
