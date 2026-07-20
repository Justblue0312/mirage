package schema

import (
	"strings"
	"unicode"

	"github.com/gertd/go-pluralize"
)

var p = pluralize.NewClient()

// ToStructName converts a table name to a Go struct name (PascalCase singular).
var ToStructName = func(tableName string) string {
	return PascalCase(Singular(tableName))
}

// ToTableName converts a Go struct name to a table name (snake_case plural).
// This is the inverse of ToStructName and is used to auto-derive a table
// name from a Go type when the type does not implement TableNamer.
var ToTableName = func(structName string) string {
	return Plural(SnakeCase(structName))
}

// ToStructFieldName converts a column name to a Go struct field name (PascalCase).
var ToStructFieldName = PascalCase

// Singular returns the singular form of a word.
func Singular(s string) string {
	if s == "data" {
		return "data"
	}
	return p.Singular(s)
}

// Plural returns the plural form of a word.
func Plural(s string) string {
	if s == "data" {
		return "data"
	}
	return p.Plural(s)
}

// PascalCase converts a snake_case string to PascalCase.
func PascalCase(snake string) string {
	if snake == "" {
		return ""
	}
	parts := strings.Split(snake, "_")
	var result strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		result.WriteString(string(runes))
	}
	s := result.String()
	s = strings.ReplaceAll(s, "Id", "ID")
	s = strings.ReplaceAll(s, "Api", "API")
	s = strings.ReplaceAll(s, "Url", "URL")
	s = strings.ReplaceAll(s, "Json", "JSON")
	s = strings.ReplaceAll(s, "Sql", "SQL")
	s = strings.ReplaceAll(s, "Http", "HTTP")
	s = strings.ReplaceAll(s, "Tcp", "TCP")
	s = strings.ReplaceAll(s, "Udp", "UDP")
	return s
}

// SnakeCase converts a PascalCase or camelCase string to snake_case.
func SnakeCase(camel string) string {
	if camel == "" {
		return ""
	}

	var result []rune
	for i, r := range camel {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := rune(camel[i-1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					result = append(result, '_')
				} else if i+1 < len(camel) {
					next := rune(camel[i+1])
					if unicode.IsLower(next) {
						result = append(result, '_')
					}
				}
			}
			result = append(result, unicode.ToLower(r))
		} else {
			result = append(result, r)
		}
	}

	s := string(result)
	if len(s) > 3 && s[:2] == "id" && s[2] != '_' && unicode.IsLower(rune(s[2])) {
		s = "id_" + s[2:]
	}

	return strings.ToLower(s)
}
