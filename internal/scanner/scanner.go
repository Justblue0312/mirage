package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/justblue/mirage/internal/schema"
)

type Scanner struct {
	SourceDirs []string
	Excludes   []string
	Recursive  bool
}

func (s *Scanner) Scan() (*schema.Package, error) {
	if len(s.Excludes) == 0 {
		s.Excludes = []string{"vendor", ".git", "node_modules"}
	}

	type scanResult struct {
		decls []RawDecl
		errs  []error
	}

	results := make(chan scanResult, len(s.SourceDirs))
	var wg sync.WaitGroup

	for _, dir := range s.SourceDirs {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			decls, errs := s.scanDir(d)
			results <- scanResult{decls: decls, errs: errs}
		}(dir)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var allDecls []RawDecl
	var allErrors []error
	for r := range results {
		allDecls = append(allDecls, r.decls...)
		allErrors = append(allErrors, r.errs...)
	}

	if len(allErrors) > 0 {
		return nil, joinErrors(allErrors)
	}

	pkg, errs := resolveDecls(allDecls)
	if len(errs) > 0 {
		return nil, joinErrors(errs)
	}

	return pkg, nil
}

func (s *Scanner) scanDir(dir string) ([]RawDecl, []error) {
	if !s.Recursive {
		return s.scanDirFlat(dir)
	}

	var allDecls []RawDecl
	var allErrors []error
	var mu sync.Mutex

	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	g, ctx := errgroup.WithContext(context.Background())

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if slices.Contains(s.Excludes, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}

		g.Go(func() error {
			defer func() { <-sem }()

			decls, err := ParseFileFromPath(path)
			if err != nil {
				mu.Lock()
				allErrors = append(allErrors, err)
				mu.Unlock()
				return nil
			}

			mu.Lock()
			allDecls = append(allDecls, decls...)
			mu.Unlock()
			return nil
		})

		return nil
	})

	err := g.Wait()
	if err != nil {
		return nil, []error{err}
	}

	if walkErr != nil {
		return nil, []error{walkErr}
	}

	if len(allErrors) > 0 {
		return allDecls, allErrors
	}

	return allDecls, nil
}

func (s *Scanner) scanDirFlat(dir string) ([]RawDecl, []error) {
	var allDecls []RawDecl
	var allErrors []error

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{err}
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		decls, err := ParseFileFromPath(path)
		if err != nil {
			allErrors = append(allErrors, err)
			continue
		}
		allDecls = append(allDecls, decls...)
	}

	if len(allErrors) > 0 {
		return allDecls, allErrors
	}

	return allDecls, nil
}

func findDeclByGoName(decls []RawDecl, goName string) *RawDecl {
	for i := range decls {
		if decls[i].GoName == goName {
			return &decls[i]
		}
	}
	shortName := goName
	if idx := strings.LastIndex(goName, "."); idx != -1 {
		shortName = goName[idx+1:]
	}
	for i := range decls {
		if decls[i].GoName == shortName {
			return &decls[i]
		}
	}
	return nil
}

func resolveDecls(decls []RawDecl) (*schema.Package, []error) {
	var errs []error

	enumMap, enumByGoName := resolveEnums(decls)

	tableMap, tableOrder := buildTableShells(decls)

	for _, d := range decls {
		if d.Kind != DeclTable || d.TableAttrs.Has("ignore") {
			continue
		}
		sqlName := d.TableAttrs.String("name", SnakeCase(d.GoName))
		t := tableMap[sqlName]
		errs = append(errs, resolveTable(d, t, decls, enumByGoName)...)
	}

	var pkg schema.Package
	for _, name := range tableOrder {
		pkg.Tables = append(pkg.Tables, *tableMap[name])
	}
	for _, e := range enumMap {
		pkg.Enums = append(pkg.Enums, e)
	}

	collectRegisteredObjects(decls, &pkg)
	sortPackage(&pkg)

	return &pkg, errs
}

// resolveEnums extracts enum declarations, returning two lookups: by SQL name
// and by the originating Go type name (used to resolve enum-typed columns).
func resolveEnums(decls []RawDecl) (bySQLName, byGoName map[string]schema.Enum) {
	bySQLName = make(map[string]schema.Enum)
	byGoName = make(map[string]schema.Enum)
	for _, d := range decls {
		if d.Kind != DeclEnum {
			continue
		}
		sqlName := d.TableAttrs.String("name", SnakeCase(d.GoName))
		e := schema.Enum{
			StructName:  d.GoName,
			SearchPath:  d.TableAttrs.String("schema", "public"),
			Name:        sqlName,
			Values:      d.EnumValues,
			Description: d.TableAttrs.String("comment", ""),
		}
		bySQLName[sqlName] = e
		byGoName[d.GoName] = e
	}
	return bySQLName, byGoName
}

// buildTableShells creates the bare *schema.Table for each non-ignored table
// declaration (name, search path, comment, options, type, partitioning) without
// resolving columns or constraints yet. tableOrder preserves declaration order.
func buildTableShells(decls []RawDecl) (tableMap map[string]*schema.Table, tableOrder []string) {
	tableMap = make(map[string]*schema.Table)
	for _, d := range decls {
		if d.Kind != DeclTable || d.TableAttrs.Has("ignore") {
			continue
		}

		sqlName := d.TableAttrs.String("name", SnakeCase(d.GoName))
		t := &schema.Table{
			StructName:  d.GoName,
			SearchPath:  d.TableAttrs.String("schema", "public"),
			Name:        sqlName,
			Description: d.TableAttrs.String("comment", ""),
			Options:     d.TableAttrs.String("options", ""),
			Type:        schema.ParseTableType(d.TableAttrs.String("type", "")),
		}

		if part := d.TableAttrs.Args("partitioned"); len(part) >= 2 {
			t.Partitioned = &schema.Partition{
				Strategy: strings.ToUpper(part[0]),
				Column:   part[1],
			}
		}

		tableMap[sqlName] = t
		tableOrder = append(tableOrder, sqlName)
	}
	return tableMap, tableOrder
}

// flattenFields resolves the field list for a table declaration, flattening
// embedded structs (depth-first, de-duplicated). It returns the combined field
// slice and the count of leading fields that came from embedded types, which
// callers use to apply per-override rules only to embedded-derived columns.
func flattenFields(d RawDecl, decls []RawDecl) (allFields []RawField, embeddedCount int) {
	seenEmbedded := make(map[string]bool)
	var resolveEmbedded func(embeddedTypes []string)
	resolveEmbedded = func(embeddedTypes []string) {
		for _, embeddedType := range embeddedTypes {
			if seenEmbedded[embeddedType] {
				continue
			}
			seenEmbedded[embeddedType] = true

			embeddedDecl := findDeclByGoName(decls, embeddedType)
			if embeddedDecl == nil {
				continue
			}
			resolveEmbedded(embeddedDecl.EmbeddedTypes)
			allFields = append(allFields, embeddedDecl.Fields...)
		}
	}
	resolveEmbedded(d.EmbeddedTypes)
	embeddedCount = len(allFields)
	allFields = append(allFields, d.Fields...)
	return allFields, embeddedCount
}

// resolveTable populates a table's columns and all its constraints (PK, unique,
// index, FK, check) from the declaration's fields and table-level tags. It
// returns any errors encountered (e.g. untyped columns, conflicting PKs).
func resolveTable(d RawDecl, t *schema.Table, decls []RawDecl, enumByGoName map[string]schema.Enum) []error {
	var errs []error
	sqlName := t.Name

	tableLevelPK := d.TableAttrs.Args("pk")
	allFields, embeddedCount := flattenFields(d, decls)

	overrides := parseOverrides(d)

	// When two fields map to the same column name, the earlier one is dropped.
	seenCols := make(map[string]int)
	for i, f := range allFields {
		colName := f.Attrs.String("name", SnakeCase(f.GoName))
		if prevIdx, exists := seenCols[colName]; exists {
			allFields[prevIdx] = RawField{}
		}
		seenCols[colName] = i
	}

	var pkCols []string
	for i, f := range allFields {
		if f.GoName == "" || f.GoName == "_" || f.Attrs.Has("ignore") {
			continue
		}

		isEmbedded := i < embeddedCount
		col, err := resolveColumn(f, i, isEmbedded, overrides, enumByGoName, sqlName)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if col.PrimaryKey {
			pkCols = append(pkCols, col.Name)
		}
		t.Columns = append(t.Columns, col)
	}

	resolveColumnDerivedConstraints(t)

	sort.SliceStable(t.Columns, func(i, j int) bool {
		return t.Columns[i].OrdinalPosition < t.Columns[j].OrdinalPosition
	})

	if len(tableLevelPK) > 0 {
		if len(pkCols) > 0 {
			errs = append(errs, fmt.Errorf("table %q: conflicting PK sources (field-level pk and table-level pk)", sqlName))
		}
		t.PrimaryKey = &schema.PrimaryKey{Name: schema.PKName(sqlName), Columns: tableLevelPK}
	} else if len(pkCols) > 0 {
		t.PrimaryKey = &schema.PrimaryKey{Name: schema.PKName(sqlName), Columns: pkCols}
	}

	resolveTableLevelConstraints(d, t)
	return errs
}

// parseOverrides reads the table-level `override` tag into a set that marks
// which embedded-derived constraint tags (pk/uq/idx) should be suppressed.
func parseOverrides(d RawDecl) map[string]bool {
	overrides := make(map[string]bool)
	for _, o := range d.TableAttrs.Args("override") {
		switch o {
		case "pk":
			overrides["pk"] = true
		case "uq", "unique":
			overrides["uq"] = true
			overrides["unique"] = true
		case "idx", "index":
			overrides["idx"] = true
			overrides["index"] = true
		}
	}
	return overrides
}

// resolveColumn builds a single *schema.Column from a field's tags, resolving
// enum types, index/FK/check attributes, generated columns, and ordinal
// position. index i is the field's position in the flattened field slice.
func resolveColumn(f RawField, i int, isEmbedded bool, overrides map[string]bool, enumByGoName map[string]schema.Enum, sqlName string) (*schema.Column, error) {
	col := schema.Column{
		FieldName:       f.GoName,
		Name:            f.Attrs.String("name", SnakeCase(f.GoName)),
		SQLType:         f.Attrs.String("type", ""),
		Nullable:        !f.Attrs.Has("notnull"),
		Default:         f.Attrs.String("default", ""),
		Unique:          (f.Attrs.Has("unique") || f.Attrs.Has("uq")) && (!isEmbedded || (!overrides["uq"] && !overrides["unique"])),
		PrimaryKey:      f.Attrs.Has("pk") && (!isEmbedded || !overrides["pk"]),
		Description:     f.Attrs.String("comment", ""),
		Collate:         f.Attrs.String("collate", ""),
		Options:         f.Attrs.String("options", ""),
		TypeChangeUsing: f.Attrs.String("using", ""),
	}

	if enum, ok := enumByGoName[f.GoType]; ok {
		if col.SQLType == "" || strings.HasPrefix(col.SQLType, "varchar") || col.SQLType == "text" {
			col.SQLType = enum.SQLName()
		}
	}

	if col.SQLType == "" {
		return nil, fmt.Errorf("field %q in table %q missing type and could not be resolved as enum", f.GoName, sqlName)
	}

	if order := f.Attrs.String("sort_order", ""); order != "" {
		if n, err := strconv.Atoi(order); err == nil {
			col.OrdinalPosition = n
		}
	}
	if col.OrdinalPosition == 0 {
		col.OrdinalPosition = (i + 1) * 10
	}

	if f.Attrs.Has("null") {
		col.Nullable = true
	}

	if gen := f.Attrs.String("generated", ""); gen != "" {
		switch strings.ToLower(gen) {
		case "always":
			col.AutoGenerated = true
			col.GeneratedExpression = "always"
		case "by_default", "identity":
			col.AutoGenerated = true
			col.Identity = true
		}
	}

	hasIndex := len(f.Attrs.Args("index")) > 0 || f.Attrs.Has("index") || len(f.Attrs.Args("idx")) > 0 || f.Attrs.Has("idx")
	skipIndex := isEmbedded && (overrides["idx"] || overrides["index"])
	if hasIndex && !skipIndex {
		kind := ""
		sortOrder := "ASC"
		if v := f.Attrs.String("index", ""); v != "" {
			kind = v
		} else if v := f.Attrs.String("idx", ""); v != "" {
			kind = v
		}
		if s := f.Attrs.String("sort", ""); s != "" {
			sortOrder = strings.ToUpper(s)
		}
		col.Index = schema.ParseIndexType(kind)
		col.IndexSort = sortOrder
	}

	fkArgs := f.Attrs.Args("fk")
	if len(fkArgs) == 0 {
		fkArgs = f.Attrs.Args("ref")
	}
	if len(fkArgs) > 0 {
		ref := fkArgs[0]
		refParts := strings.SplitN(ref, " ", 2)
		refTarget := refParts[0]
		onDelete := ""
		onUpdate := ""
		if len(refParts) > 1 {
			clause := strings.ToUpper(refParts[1])
			if idx := strings.Index(clause, "ON DELETE "); idx != -1 {
				onDelete = strings.TrimSpace(clause[idx+len("ON DELETE "):])
			}
			if idx := strings.Index(clause, "ON UPDATE "); idx != -1 {
				onUpdate = strings.TrimSpace(clause[idx+len("ON UPDATE "):])
			}
		}
		parts := strings.SplitN(refTarget, ".", 2)
		if len(parts) == 2 {
			col.ReferenceTableName = parts[0]
			col.ReferenceColumnName = parts[1]
			col.ReferenceOnDelete = onDelete
			col.ReferenceOnUpdate = onUpdate
		}
	}

	if chk := f.Attrs.String("check", ""); chk != "" {
		col.CheckConstraint = chk
	}

	return &col, nil
}

// resolveColumnDerivedConstraints promotes per-column unique/index/check flags
// into table-level constraint entries.
func resolveColumnDerivedConstraints(t *schema.Table) {
	sqlName := t.Name
	for _, col := range t.Columns {
		if col.Unique {
			t.Uniques = append(t.Uniques, schema.UniqueConstraint{
				Name:    schema.UQName(sqlName, []string{col.Name}),
				Columns: []string{col.Name},
			})
		}
		if col.Index != schema.InvalidIndex {
			t.Indexes = append(t.Indexes, schema.Index{
				Name:    schema.IdxName(sqlName, []string{col.Name}),
				Table:   sqlName,
				Columns: []string{col.Name},
				Kind:    col.Index.String(),
				Sort:    col.IndexSort,
			})
		}
	}
	for _, col := range t.Columns {
		if col.CheckConstraint != "" {
			t.Checks = append(t.Checks, schema.CheckConstraint{
				Name:       schema.ChkName(sqlName, len(t.Checks)+1),
				Expression: col.CheckConstraint,
			})
		}
	}
}

// resolveTableLevelConstraints parses the table-level fk/ref/uq/unique/index/
// idx/check tags into table constraints.
func resolveTableLevelConstraints(d RawDecl, t *schema.Table) {
	sqlName := t.Name

	appendFKs := func(tag string) {
		for _, fkSet := range d.TableAttrs.AllArgs(tag) {
			for _, fkArg := range fkSet {
				parts := strings.SplitN(fkArg, ":", 2)
				if len(parts) != 2 {
					continue
				}
				fromCol := parts[0]
				toParts := strings.SplitN(parts[1], ".", 2)
				if len(toParts) != 2 {
					continue
				}
				fk := schema.ForeignKey{
					FromTable:   sqlName,
					FromColumns: []string{fromCol},
					ToTable:     toParts[0],
					ToColumns:   []string{toParts[1]},
				}
				fk.Name = schema.FKName(sqlName, []string{fromCol})
				t.ForeignKeys = append(t.ForeignKeys, fk)
			}
		}
	}
	appendFKs("fk")
	appendFKs("ref")

	appendUniques := func(tag string) {
		for _, uqArgs := range d.TableAttrs.AllArgs(tag) {
			t.Uniques = append(t.Uniques, schema.UniqueConstraint{
				Name:    schema.UQName(sqlName, uqArgs),
				Columns: uqArgs,
			})
		}
	}
	appendUniques("uq")
	appendUniques("unique")

	appendIndexes := func(tag string) {
		for _, idxArgs := range d.TableAttrs.AllArgs(tag) {
			t.Indexes = append(t.Indexes, schema.Index{
				Name:    schema.IdxName(sqlName, idxArgs),
				Table:   sqlName,
				Columns: idxArgs,
				Kind:    "",
				Sort:    "ASC",
			})
		}
	}
	appendIndexes("index")
	appendIndexes("idx")

	for _, chkSet := range d.TableAttrs.AllArgs("check") {
		for _, chkArg := range chkSet {
			t.Checks = append(t.Checks, schema.CheckConstraint{
				Name:       schema.ChkName(sqlName, len(t.Checks)+1),
				Expression: chkArg,
			})
		}
	}
}

// collectRegisteredObjects appends functions, views, materialized views,
// triggers, procedures, grants, and policies from the declaration list into pkg.
func collectRegisteredObjects(decls []RawDecl, pkg *schema.Package) {
	for _, d := range decls {
		switch d.Kind {
		case DeclExtension:
			if d.Extension != nil {
				pkg.Extensions = append(pkg.Extensions, *d.Extension)
			}
		case DeclFunction:
			if d.Function != nil {
				pkg.Functions = append(pkg.Functions, *d.Function)
			}
		case DeclView:
			if d.View != nil {
				pkg.Views = append(pkg.Views, *d.View)
			}
		case DeclMatView:
			if d.MaterializedView != nil {
				pkg.MaterializedViews = append(pkg.MaterializedViews, *d.MaterializedView)
			}
		case DeclTrigger:
			if d.Trigger != nil {
				pkg.Triggers = append(pkg.Triggers, *d.Trigger)
			}
		case DeclProcedure:
			if d.Procedure != nil {
				pkg.Procedures = append(pkg.Procedures, *d.Procedure)
			}
		case DeclGrant:
			if d.Grant != nil {
				pkg.Grants = append(pkg.Grants, *d.Grant)
			}
		case DeclPolicy:
			if d.Policy != nil {
				pkg.Policies = append(pkg.Policies, *d.Policy)
			}
		}
	}
}

// sortPackage sorts every collection in pkg by its stable SQL identity so that
// scanner output is deterministic regardless of goroutine/declaration order.
func sortPackage(pkg *schema.Package) {
	sort.Slice(pkg.Enums, func(i, j int) bool {
		return pkg.Enums[i].SQLName() < pkg.Enums[j].SQLName()
	})
	sort.Slice(pkg.Functions, func(i, j int) bool {
		return pkg.Functions[i].SQLName() < pkg.Functions[j].SQLName()
	})
	sort.Slice(pkg.Views, func(i, j int) bool {
		return pkg.Views[i].SQLName() < pkg.Views[j].SQLName()
	})
	sort.Slice(pkg.MaterializedViews, func(i, j int) bool {
		return pkg.MaterializedViews[i].SQLName() < pkg.MaterializedViews[j].SQLName()
	})
	sort.Slice(pkg.Triggers, func(i, j int) bool {
		return pkg.Triggers[i].SQLName() < pkg.Triggers[j].SQLName()
	})
	sort.Slice(pkg.Procedures, func(i, j int) bool {
		return pkg.Procedures[i].SQLName() < pkg.Procedures[j].SQLName()
	})
	sort.Slice(pkg.Tables, func(i, j int) bool {
		return pkg.Tables[i].SQLName() < pkg.Tables[j].SQLName()
	})
	sort.Slice(pkg.Extensions, func(i, j int) bool {
		return pkg.Extensions[i].SQLName() < pkg.Extensions[j].SQLName()
	})
	sort.Slice(pkg.Grants, func(i, j int) bool {
		return pkg.Grants[i].SortKey() < pkg.Grants[j].SortKey()
	})
	sort.Slice(pkg.Policies, func(i, j int) bool {
		return pkg.Policies[i].SortKey() < pkg.Policies[j].SortKey()
	})
}

func joinErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return fmt.Errorf("multiple errors:\n  - %s", strings.Join(msgs, "\n  - "))
}
