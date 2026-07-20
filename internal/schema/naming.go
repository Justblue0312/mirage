package schema

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// PKName generates a primary key constraint name.
func PKName(table string) string {
	return Truncate("pk_" + table)
}

// FKName generates a foreign key constraint name.
func FKName(fromTable string, fromCols []string) string {
	var s strings.Builder
	s.WriteString("fk_" + fromTable)
	for _, c := range fromCols {
		s.WriteString("_" + c)
	}
	return Truncate(s.String())
}

// UQName generates a unique constraint name.
func UQName(table string, cols []string) string {
	var s strings.Builder
	s.WriteString("uq_" + table)
	for _, c := range cols {
		s.WriteString("_" + c)
	}
	return Truncate(s.String())
}

// IdxName generates an index name.
func IdxName(table string, cols []string) string {
	var s strings.Builder
	s.WriteString("idx_" + table)
	for _, c := range cols {
		s.WriteString("_" + c)
	}
	return Truncate(s.String())
}

// ChkName generates a check constraint name.
func ChkName(table string, seq int) string {
	return Truncate(fmt.Sprintf("chk_%s_%d", table, seq))
}

// Truncate truncates a name to 63 characters (PostgreSQL limit), preserving a hash suffix.
func Truncate(name string) string {
	if len(name) <= 63 {
		return name
	}
	hash := sha256.Sum256([]byte(name))
	suffix := fmt.Sprintf("%x", hash[:4])

	idx := strings.Index(name, "_")
	if idx == -1 {
		prefix := name[:55]
		return prefix + suffix
	}

	var prefix string
	tableEnd := strings.Index(name[idx+1:], "_")
	if tableEnd == -1 {
		prefix = name
	} else {
		prefix = name[:idx+1+tableEnd+1]
	}

	if len(prefix)+len(suffix) > 63 {
		maxPrefix := max(63-len(suffix), 1)
		prefix = name[:maxPrefix]
	}

	return prefix + suffix
}
