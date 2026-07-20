package scanner

import (
	"strconv"
	"strings"
)

type TagOptions struct {
	TableName string
	TableOpts map[string]string
	TableArgs map[string][]string
	Columns   []TagColumn
	EnumDefs  []TagEnum
}

type TagColumn struct {
	GoName      string
	GoType      string
	Name        string
	SQLType     string
	PK          bool
	PKArgs      []string
	NotNull     bool
	Unique      bool
	UniqueArgs  []string
	Default     string
	Index       string
	IndexType   string
	UniqueIndex string
	FK          string
	OnDelete    string
	OnUpdate    string
	Check       string
	Generated   string
	Comment     string
	Collate     string
	Using       string
	Ignore      bool
	Identity    bool
	SortOrder   int
}

type TagEnum struct {
	GoName string
	Name   string
	Values []string
}

func ParseTag(raw string) (flags []string, kv map[string]string, args map[string][]string) {
	kv = make(map[string]string)
	args = make(map[string][]string)

	parts := splitTagParts(raw)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if eqIdx := strings.IndexByte(part, '='); eqIdx != -1 {
			key := part[:eqIdx]
			value := part[eqIdx+1:]
			kv[key] = value
		} else if idx := strings.IndexByte(part, '('); idx != -1 {
			key := part[:idx]
			rest := part[idx+1:]
			rest = strings.TrimSuffix(rest, ")")
			argList := splitTagArgs(rest)
			args[key] = append(args[key], argList...)
			if _, exists := kv[key]; !exists {
				kv[key] = strings.Join(argList, ",")
			}
		} else {
			flags = append(flags, part)
		}
	}

	return
}

func splitTagParts(raw string) []string {
	var parts []string
	var current strings.Builder
	inParen := false

	for _, ch := range raw {
		switch ch {
		case '(':
			inParen = true
			current.WriteRune(ch)
		case ')':
			inParen = false
			current.WriteRune(ch)
		case ',':
			if inParen {
				current.WriteRune(ch)
			} else {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func splitTagArgs(rest string) []string {
	var args []string
	var current strings.Builder
	inParen := false

	for _, ch := range rest {
		switch ch {
		case '(':
			inParen = true
			current.WriteRune(ch)
		case ')':
			inParen = false
			current.WriteRune(ch)
		case ',':
			if inParen {
				current.WriteRune(ch)
			} else {
				args = append(args, strings.TrimSpace(current.String()))
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		args = append(args, strings.TrimSpace(current.String()))
	}

	return args
}

func ParseTableTag(raw string) (tableName string, opts map[string]string, args map[string][]string) {
	flags, kv, a := ParseTag(raw)
	opts = kv
	args = a

	if name, ok := kv["name"]; ok {
		tableName = name
	}

	_ = flags
	return
}

func ParseColumnTag(raw string) TagColumn {
	flags, kv, args := ParseTag(raw)

	col := TagColumn{}

	if name, ok := kv["name"]; ok {
		col.Name = name
	}

	if t, ok := kv["type"]; ok {
		col.SQLType = t
	}

	if d, ok := kv["default"]; ok {
		col.Default = d
	}

	if c, ok := kv["comment"]; ok {
		col.Comment = c
	}

	if c, ok := kv["collate"]; ok {
		col.Collate = c
	}

	if u, ok := kv["using"]; ok {
		col.Using = u
	}

	if idx, ok := kv["index"]; ok {
		col.Index = "true"
		col.IndexType = idx
	}

	if ui, ok := kv["unique_index"]; ok {
		col.UniqueIndex = ui
	}

	if fk, ok := kv["ref"]; ok {
		col.FK = fk
	} else if fk, ok := kv["fk"]; ok {
		col.FK = fk
	}

	if od, ok := kv["ondelete"]; ok {
		col.OnDelete = strings.ToUpper(od)
	}

	if ou, ok := kv["onupdate"]; ok {
		col.OnUpdate = strings.ToUpper(ou)
	}

	if chk, ok := kv["check"]; ok {
		col.Check = chk
	}

	if gen, ok := kv["generated"]; ok {
		col.Generated = gen
	}

	for _, flag := range flags {
		switch flag {
		case "pk", "primary":
			col.PK = true
			if pkArgs, ok := args["pk"]; ok {
				col.PKArgs = pkArgs
			}
		case "unique":
			col.Unique = true
			if uqArgs, ok := args["unique"]; ok {
				col.UniqueArgs = uqArgs
			}
		case "notnull", "not_null":
			col.NotNull = true
		case "ignore":
			col.Ignore = true
		case "identity":
			col.Identity = true
		}
	}

	if _, ok := args["pk"]; ok && !col.PK {
		col.PK = true
		col.PKArgs = args["pk"]
	}
	if _, ok := args["unique"]; ok && !col.Unique {
		col.Unique = true
		col.UniqueArgs = args["unique"]
	}

	if v, ok := kv["sort_order"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			col.SortOrder = n
		}
	}

	return col
}
