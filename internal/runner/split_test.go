package runner

import (
	"strings"
	"testing"
)

func TestSplitStatements_DollarQuotedBody(t *testing.T) {
	// A function body containing a semicolon must not be split at that
	// semicolon when it sits inside a dollar-quoted string.
	block := `CREATE OR REPLACE FUNCTION f() RETURNS int AS $func$
BEGIN
  RETURN 1; -- semicolon inside the body must NOT split here
END;
$func$ LANGUAGE sql;`

	stmts := splitStatements(block)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "RETURN 1") || !strings.Contains(stmts[0], "END") {
		t.Errorf("statement lost body content: %q", stmts[0])
	}
}

func TestSplitStatements_SimpleSemicolons(t *testing.T) {
	block := "SELECT 1; SELECT 2; SELECT 3;"
	stmts := splitStatements(block)
	if len(stmts) != 3 {
		t.Fatalf("expected 3 statements, got %d: %v", len(stmts), stmts)
	}
}

func TestSplitStatements_EmptyAndCommentLinesSkipped(t *testing.T) {
	block := "-- a comment\n\nSELECT 1;\n-- another\nSELECT 2;"
	stmts := splitStatements(block)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(stmts), stmts)
	}
}
