package scanner

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"github.com/justblue/mirage/internal/schema"
)

type DeclKind int

const (
	DeclTable DeclKind = iota
	DeclEnum
	DeclEmbeddable
	DeclExtension
	DeclFunction
	DeclView
	DeclMatView
	DeclTrigger
	DeclProcedure
	DeclGrant
	DeclPolicy
)

type RawDecl struct {
	FilePath      string
	Line          int
	Kind          DeclKind
	GoName        string
	TableAttrs    Attrs
	Fields        []RawField
	EnumValues    []string
	EmbeddedTypes []string

	Function         *schema.Function
	View             *schema.View
	MaterializedView *schema.MaterializedView
	Trigger          *schema.Trigger
	Procedure        *schema.Procedure
	Grant            *schema.Grant
	Policy           *schema.Policy
	Extension        *schema.Extension
}

type RawField struct {
	GoName string
	GoType string
	Line   int
	Attrs  Attrs
}

func ParseFile(fset *token.FileSet, node *ast.File, filePath string) ([]RawDecl, error) {
	var decls []RawDecl

	for _, decl := range node.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}

		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			pos := fset.Position(ts.Pos())

			if isEnumType(node, ts) {
				enumValues := extractEnumValues(node, fset, ts)
				decls = append(decls, RawDecl{
					FilePath:   filePath,
					Line:       pos.Line,
					Kind:       DeclEnum,
					GoName:     ts.Name.Name,
					EnumValues: enumValues,
				})
				continue
			}

			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}

			tableTag := extractTableTag(st)
			if tableTag != "" {
				tableName, opts, args := ParseTableTag(tableTag)
				fields, embeddedTypes := extractStructFields(fset, st, filePath)

				tableAttrs := Attrs{}
				if tableName != "" {
					tableAttrs["name"] = AttrValue{Value: tableName}
				}
				for k, v := range opts {
					if k != "name" {
						tableAttrs[k] = AttrValue{Value: v}
					}
				}
				for k, v := range args {
					tableAttrs[k] = AttrValue{Args: v}
				}

				decls = append(decls, RawDecl{
					FilePath:      filePath,
					Line:          pos.Line,
					Kind:          DeclTable,
					GoName:        ts.Name.Name,
					TableAttrs:    tableAttrs,
					Fields:        fields,
					EmbeddedTypes: embeddedTypes,
				})
				continue
			}

			if hasStructFields(st) {
				fields, embeddedTypes := extractStructFields(fset, st, filePath)
				decls = append(decls, RawDecl{
					FilePath:      filePath,
					Line:          pos.Line,
					Kind:          DeclEmbeddable,
					GoName:        ts.Name.Name,
					TableAttrs:    Attrs{},
					Fields:        fields,
					EmbeddedTypes: embeddedTypes,
				})
			}
		}
	}

	// Scan for mirage.Register() calls
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == "init" && d.Body != nil {
				extractRegisterCalls(fset, d.Body.List, filePath, &decls)
			}
		case *ast.GenDecl:
			if d.Tok == token.VAR {
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok || len(vs.Values) == 0 {
						continue
					}
					if call, ok := vs.Values[0].(*ast.CallExpr); ok && isMirageRegisterCall(call) {
						for _, arg := range call.Args {
							extractRegisteredObject(fset, arg, filePath, &decls)
						}
					}
				}
			}
		}
	}

	return decls, nil
}

func extractTableTag(st *ast.StructType) string {
	if st.Fields == nil {
		return ""
	}
	for _, f := range st.Fields.List {
		if len(f.Names) == 1 && f.Names[0].Name == "_" {
			if tag := getStructTag(f, "db"); tag != "" {
				return tag
			}
		}
	}
	return ""
}

func getStructTag(field *ast.Field, tag string) string {
	if field.Tag == nil {
		return ""
	}
	raw := field.Tag.Value
	if len(raw) < 2 {
		return ""
	}
	raw = raw[1 : len(raw)-1]

	for raw != "" {
		i := 0
		for i < len(raw) && raw[i] == ' ' {
			i++
		}
		raw = raw[i:]
		if raw == "" {
			break
		}

		i = 0
		for i < len(raw) && raw[i] != ':' && raw[i] != ' ' {
			i++
		}
		if i >= len(raw) {
			break
		}
		name := raw[:i]
		raw = raw[i:]

		if len(raw) == 0 || raw[0] != ':' {
			continue
		}
		raw = raw[1:]

		i = 0
		for i < len(raw) && raw[i] == ' ' {
			i++
		}
		raw = raw[i:]

		if len(raw) == 0 || raw[0] != '"' {
			continue
		}
		raw = raw[1:]

		i = 0
		for i < len(raw) && raw[i] != '"' {
			if raw[i] == '\\' {
				i++
			}
			i++
		}
		value := raw[:i]
		if i < len(raw) {
			raw = raw[i+1:]
		} else {
			raw = ""
		}

		if name == tag {
			return value
		}
	}
	return ""
}

func isEnumType(node *ast.File, ts *ast.TypeSpec) bool {
	baseType, ok := ts.Type.(*ast.Ident)
	if !ok {
		return false
	}
	if baseType.Name != "string" {
		return false
	}

	typeName := ts.Name.Name
	for _, decl := range node.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if vs.Type != nil {
				if ident, ok := vs.Type.(*ast.Ident); ok && ident.Name == typeName {
					return true
				}
			}
		}
	}
	return false
}

func extractEnumValues(file *ast.File, fset *token.FileSet, ts *ast.TypeSpec) []string {
	var values []string
	typeName := ts.Name.Name

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}

		genPos := fset.Position(gen.Pos())
		typeEnd := fset.Position(ts.End())

		isProximityCandidate := genPos.Line > typeEnd.Line && genPos.Line <= typeEnd.Line+5

		var lastExplicitType string
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			if vs.Type != nil {
				if ident, ok := vs.Type.(*ast.Ident); ok {
					lastExplicitType = ident.Name
				} else {
					lastExplicitType = ""
				}
			} else if len(vs.Values) > 0 {
				lastExplicitType = ""
			}

			match := lastExplicitType == typeName
			if !match && vs.Type == nil && lastExplicitType == "" && isProximityCandidate {
				match = true
			}

			if !match {
				continue
			}

			if len(vs.Values) > 0 {
				for i, val := range vs.Values {
					if lit, ok := val.(*ast.BasicLit); ok {
						switch lit.Kind {
						case token.STRING:
							s, _ := strconv.Unquote(lit.Value)
							values = append(values, s)
						case token.INT:
							values = append(values, lit.Value)
						}
					} else if _, ok := val.(*ast.Ident); ok {
						nameIdx := i
						if nameIdx >= len(vs.Names) {
							nameIdx = len(vs.Names) - 1
						}
						values = append(values, SnakeCase(vs.Names[nameIdx].Name))
					}
				}
			} else {
				for _, name := range vs.Names {
					values = append(values, SnakeCase(name.Name))
				}
			}
		}
	}

	return values
}

func extractEmbeddedTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			return pkg.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	case *ast.StarExpr:
		return extractEmbeddedTypeName(t.X)
	}
	return ""
}

func extractTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			return pkg.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	case *ast.StarExpr:
		return extractTypeName(t.X)
	case *ast.IndexExpr:
		return extractTypeName(t.Index)
	case *ast.ArrayType:
		return extractTypeName(t.Elt) + "[]"
	}
	return ""
}

func extractStructFields(fset *token.FileSet, st *ast.StructType, _ string) ([]RawField, []string) {
	var fields []RawField
	var embeddedTypes []string

	if st.Fields != nil {
		for _, f := range st.Fields.List {
			if len(f.Names) == 0 {
				typeName := extractEmbeddedTypeName(f.Type)
				if typeName != "" {
					embeddedTypes = append(embeddedTypes, typeName)
				}
				continue
			}
			name := f.Names[0].Name
			if name == "_" || (len(name) > 0 && name[0] >= 'a' && name[0] <= 'z') {
				continue
			}

			fieldTag := getStructTag(f, "db")
			if fieldTag == "" {
				continue
			}

			parsed := ParseColumnTag(fieldTag)

			fpos := fset.Position(f.Pos())
			goType := extractTypeName(f.Type)

			attrs := Attrs{}
			if parsed.Name != "" {
				attrs["name"] = AttrValue{Value: parsed.Name}
			}
			if parsed.SQLType != "" {
				attrs["type"] = AttrValue{Value: parsed.SQLType}
			}
			if parsed.PK {
				attrs["pk"] = AttrValue{Flag: true}
			}
			if parsed.NotNull {
				attrs["notnull"] = AttrValue{Flag: true}
			}
			if parsed.Unique {
				attrs["unique"] = AttrValue{Flag: true}
			}
			if parsed.Default != "" {
				attrs["default"] = AttrValue{Value: parsed.Default}
			}
			if parsed.Index != "" {
				attrs["index"] = AttrValue{Value: parsed.IndexType}
			}
			if parsed.UniqueIndex != "" {
				attrs["unique_index"] = AttrValue{Value: parsed.UniqueIndex}
			}
			if parsed.FK != "" {
				attrs["fk"] = AttrValue{Args: []string{parsed.FK}}
			}
			if parsed.OnDelete != "" {
				attrs["ondelete"] = AttrValue{Value: parsed.OnDelete}
			}
			if parsed.OnUpdate != "" {
				attrs["onupdate"] = AttrValue{Value: parsed.OnUpdate}
			}
			if parsed.Check != "" {
				attrs["check"] = AttrValue{Value: parsed.Check}
			}
			if parsed.Generated != "" {
				attrs["generated"] = AttrValue{Value: parsed.Generated}
			}
			if parsed.Comment != "" {
				attrs["comment"] = AttrValue{Value: parsed.Comment}
			}
			if parsed.Collate != "" {
				attrs["collate"] = AttrValue{Value: parsed.Collate}
			}
			if parsed.Using != "" {
				attrs["using"] = AttrValue{Value: parsed.Using}
			}
			if parsed.Ignore {
				attrs["ignore"] = AttrValue{Flag: true}
			}
			if parsed.Identity {
				attrs["generated"] = AttrValue{Value: "by_default"}
			}

			if parsed.SortOrder > 0 {
				attrs["sort_order"] = AttrValue{Value: strconv.Itoa(parsed.SortOrder)}
			}

			fields = append(fields, RawField{
				GoName: name,
				GoType: goType,
				Line:   fpos.Line,
				Attrs:  attrs,
			})
		}
	}

	return fields, embeddedTypes
}

func hasStructFields(st *ast.StructType) bool {
	if st.Fields == nil {
		return false
	}
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			return true
		}
		name := f.Names[0].Name
		if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
			continue
		}
		if getStructTag(f, "db") != "" {
			return true
		}
	}
	return false
}

func SnakeCase(s string) string {
	return ToDelimited(s, '_')
}

func ToDelimited(s string, delimiter uint8) string {
	return ToScreamingDelimited(s, delimiter, "", false)
}

func ToScreamingDelimited(s string, delimiter uint8, ignore string, screaming bool) string {
	s = strings.TrimSpace(s)
	n := strings.Builder{}
	n.Grow(len(s) + 2)
	for i, v := range []byte(s) {
		vIsCap := v >= 'A' && v <= 'Z'
		vIsLow := v >= 'a' && v <= 'z'
		if vIsLow && screaming {
			v += 'A'
			v -= 'a'
		} else if vIsCap && !screaming {
			v += 'a'
			v -= 'A'
		}

		if i+1 < len(s) {
			next := s[i+1]
			vIsNum := v >= '0' && v <= '9'
			nextIsCap := next >= 'A' && next <= 'Z'
			nextIsLow := next >= 'a' && next <= 'z'
			nextIsNum := next >= '0' && next <= '9'
			if (vIsCap && (nextIsLow || nextIsNum)) || (vIsLow && (nextIsCap || nextIsNum)) || (vIsNum && (nextIsCap || nextIsLow)) {
				prevIgnore := ignore != "" && i > 0 && strings.ContainsAny(string(s[i-1]), ignore)
				if !prevIgnore {
					if vIsCap && nextIsLow {
						if prevIsCap := i > 0 && s[i-1] >= 'A' && s[i-1] <= 'Z'; prevIsCap {
							n.WriteByte(delimiter)
						}
					}
					n.WriteByte(v)
					if vIsLow || vIsNum || nextIsNum {
						n.WriteByte(delimiter)
					}
					continue
				}
			}
		}

		if (v == ' ' || v == '_' || v == '-' || v == '.') && !strings.ContainsAny(string(v), ignore) {
			n.WriteByte(delimiter)
		} else {
			n.WriteByte(v)
		}
	}

	return n.String()
}

func ParseFileFromPath(filePath string) ([]RawDecl, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}
	return ParseFile(fset, node, filePath)
}

func parseExtensionLiteral(cl *ast.CompositeLit) *schema.Extension {
	e := &schema.Extension{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "SearchPath":
			e.SearchPath = extractStringLit(kv.Value)
		case "Name":
			e.Name = extractStringLit(kv.Value)
		case "Schema":
			e.Schema = extractStringLit(kv.Value)
		case "Version":
			e.Version = extractStringLit(kv.Value)
		case "IfNotExists":
			e.IfNotExists = extractBoolLit(kv.Value)
		case "Cascade":
			e.Cascade = extractBoolLit(kv.Value)
		}
	}
	return e
}

func extractRegisterCalls(fset *token.FileSet, stmts []ast.Stmt, filePath string, decls *[]RawDecl) {
	for _, stmt := range stmts {
		exprStmt, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		callExpr, ok := exprStmt.X.(*ast.CallExpr)
		if !ok || !isMirageRegisterCall(callExpr) {
			continue
		}
		for _, arg := range callExpr.Args {
			extractRegisteredObject(fset, arg, filePath, decls)
		}
	}
}

func isMirageRegisterCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name != "Register" {
		return false
	}
	if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "mirage" {
		return true
	}
	return false
}

func extractRegisteredObject(fset *token.FileSet, arg ast.Expr, filePath string, decls *[]RawDecl) {
	cl, ok := arg.(*ast.CompositeLit)
	if !ok {
		return
	}
	typeName := extractTypeName(cl.Type)
	line := fset.Position(cl.Pos()).Line

	switch {
	case strings.HasSuffix(typeName, "Extension"):
		e := parseExtensionLiteral(cl)
		*decls = append(*decls, RawDecl{
			FilePath: filePath, Line: line, Kind: DeclExtension,
			GoName: e.Name, Extension: e,
		})
	case strings.HasSuffix(typeName, "Function"):
		fn := parseFunctionLiteral(cl)
		*decls = append(*decls, RawDecl{
			FilePath: filePath, Line: line, Kind: DeclFunction,
			GoName: fn.Name, Function: fn,
		})
	case strings.HasSuffix(typeName, "MaterializedView"):
		mv := parseMaterializedViewLiteral(cl)
		*decls = append(*decls, RawDecl{
			FilePath: filePath, Line: line, Kind: DeclMatView,
			GoName: mv.Name, MaterializedView: mv,
		})
	case typeName == "View" || strings.HasSuffix(typeName, "View"):
		v := parseViewLiteral(cl)
		*decls = append(*decls, RawDecl{
			FilePath: filePath, Line: line, Kind: DeclView,
			GoName: v.Name, View: v,
		})
	case strings.HasSuffix(typeName, "Trigger"):
		t := parseTriggerLiteral(cl)
		*decls = append(*decls, RawDecl{
			FilePath: filePath, Line: line, Kind: DeclTrigger,
			GoName: t.Name, Trigger: t,
		})
	case strings.HasSuffix(typeName, "Procedure"):
		p := parseProcedureLiteral(cl)
		*decls = append(*decls, RawDecl{
			FilePath: filePath, Line: line, Kind: DeclProcedure,
			GoName: p.Name, Procedure: p,
		})
	case strings.HasSuffix(typeName, "Grant"):
		g := parseGrantLiteral(cl)
		*decls = append(*decls, RawDecl{
			FilePath: filePath, Line: line, Kind: DeclGrant,
			GoName: g.ObjectName, Grant: g,
		})
	case strings.HasSuffix(typeName, "Policy"):
		p := parsePolicyLiteral(cl)
		*decls = append(*decls, RawDecl{
			FilePath: filePath, Line: line, Kind: DeclPolicy,
			GoName: p.Name, Policy: p,
		})
	}
}

func parseFunctionLiteral(cl *ast.CompositeLit) *schema.Function {
	fn := &schema.Function{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "SearchPath":
			fn.SearchPath = extractStringLit(kv.Value)
		case "Name":
			fn.Name = extractStringLit(kv.Value)
		case "Description":
			fn.Description = extractStringLit(kv.Value)
		case "Language":
			fn.Language = extractStringLit(kv.Value)
		case "ReturnType":
			fn.ReturnType = extractStringLit(kv.Value)
		case "Volatility":
			fn.Volatility = extractStringLit(kv.Value)
		case "Security":
			fn.Security = extractStringLit(kv.Value)
		case "Body":
			fn.Body = extractStringLit(kv.Value)
		case "Arguments":
			fn.Arguments = parseFunctionArgumentList(kv.Value)
		}
	}
	return fn
}

func parseViewLiteral(cl *ast.CompositeLit) *schema.View {
	v := &schema.View{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "SearchPath":
			v.SearchPath = extractStringLit(kv.Value)
		case "Name":
			v.Name = extractStringLit(kv.Value)
		case "Description":
			v.Description = extractStringLit(kv.Value)
		case "Query":
			v.Query = extractStringLit(kv.Value)
		}
	}
	return v
}

func parseMaterializedViewLiteral(cl *ast.CompositeLit) *schema.MaterializedView {
	mv := &schema.MaterializedView{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "SearchPath":
			mv.SearchPath = extractStringLit(kv.Value)
		case "Name":
			mv.Name = extractStringLit(kv.Value)
		case "Description":
			mv.Description = extractStringLit(kv.Value)
		case "Query":
			mv.Query = extractStringLit(kv.Value)
		}
	}
	return mv
}

func parseTriggerLiteral(cl *ast.CompositeLit) *schema.Trigger {
	t := &schema.Trigger{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "SearchPath":
			t.SearchPath = extractStringLit(kv.Value)
		case "Name":
			t.Name = extractStringLit(kv.Value)
		case "Description":
			t.Description = extractStringLit(kv.Value)
		case "Table":
			t.Table = extractStringLit(kv.Value)
		case "Timing":
			t.Timing = extractStringLit(kv.Value)
		case "Function":
			t.Function = extractStringLit(kv.Value)
		case "Constraint":
			t.Constraint = extractStringLit(kv.Value)
		case "Events":
			t.Events = parseStringSlice(kv.Value)
		}
	}
	return t
}

func parseProcedureLiteral(cl *ast.CompositeLit) *schema.Procedure {
	p := &schema.Procedure{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "SearchPath":
			p.SearchPath = extractStringLit(kv.Value)
		case "Name":
			p.Name = extractStringLit(kv.Value)
		case "Description":
			p.Description = extractStringLit(kv.Value)
		case "Language":
			p.Language = extractStringLit(kv.Value)
		case "Body":
			p.Body = extractStringLit(kv.Value)
		case "Arguments":
			p.Arguments = parseProcedureArgumentList(kv.Value)
		}
	}
	return p
}

func parseGrantLiteral(cl *ast.CompositeLit) *schema.Grant {
	g := &schema.Grant{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "SearchPath":
			g.SearchPath = extractStringLit(kv.Value)
		case "ObjectType":
			g.ObjectType = extractStringLit(kv.Value)
		case "ObjectName":
			g.ObjectName = extractStringLit(kv.Value)
		case "Privileges":
			g.Privileges = parseStringSlice(kv.Value)
		case "Roles":
			g.Roles = parseStringSlice(kv.Value)
		}
	}
	return g
}

func parsePolicyLiteral(cl *ast.CompositeLit) *schema.Policy {
	p := &schema.Policy{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "SearchPath":
			p.SearchPath = extractStringLit(kv.Value)
		case "Name":
			p.Name = extractStringLit(kv.Value)
		case "Table":
			p.Table = extractStringLit(kv.Value)
		case "Command":
			p.Command = extractStringLit(kv.Value)
		case "Using":
			p.Using = extractStringLit(kv.Value)
		case "Check":
			p.Check = extractStringLit(kv.Value)
		case "Permissive":
			p.Permissive = extractStringLit(kv.Value)
		case "Roles":
			p.Roles = parseStringSlice(kv.Value)
		}
	}
	return p
}

func parseFunctionArgumentList(expr ast.Expr) []schema.FunctionArgument {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var args []schema.FunctionArgument
	for _, elt := range cl.Elts {
		argCl, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}
		arg := schema.FunctionArgument{}
		for _, argElt := range argCl.Elts {
			kv, ok := argElt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyIdent, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			key := keyIdent.Name
			val := extractStringLit(kv.Value)
			switch key {
			case "Name":
				arg.Name = val
			case "Type":
				arg.Type = val
			case "Mode":
				arg.Mode = val
			}
		}
		args = append(args, arg)
	}
	return args
}

func parseProcedureArgumentList(expr ast.Expr) []schema.ProcedureArgument {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var args []schema.ProcedureArgument
	for _, elt := range cl.Elts {
		argCl, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}
		arg := schema.ProcedureArgument{}
		for _, argElt := range argCl.Elts {
			kv, ok := argElt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyIdent, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			key := keyIdent.Name
			val := extractStringLit(kv.Value)
			switch key {
			case "Name":
				arg.Name = val
			case "Type":
				arg.Type = val
			case "Mode":
				arg.Mode = val
			}
		}
		args = append(args, arg)
	}
	return args
}

func parseStringSlice(expr ast.Expr) []string {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var result []string
	for _, elt := range cl.Elts {
		if s := extractStringLit(elt); s != "" {
			result = append(result, s)
		}
	}
	return result
}

func extractBoolLit(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	return id.Name == "true"
}

func extractStringLit(expr ast.Expr) string {
	switch lit := expr.(type) {
	case *ast.BasicLit:
		if lit.Kind == token.STRING {
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return ""
			}
			return s
		}
		// Non-string literal (e.g. an int); leave it for callers that handle
		// non-string values.
		return lit.Value
	case *ast.Ident:
		// An identifier here means the user referenced a Go constant/var
		// rather than a string literal. We cannot resolve its value at scan
		// time, so return empty rather than the identifier *name*, which
		// would produce incorrect SQL.
		return ""
	}
	return ""
}
