package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/justblue/mirage/internal/dialect/postgres"
	"github.com/justblue/mirage/internal/diff"
	"github.com/justblue/mirage/internal/generator"
	"github.com/justblue/mirage/internal/scanner"
	"github.com/justblue/mirage/internal/schema"
)

func main() {
	dirs := []string{
		"models/auth",
		"models/content",
		"models/catalog",
		"models/metrics",
	}

	// Debug: list files in each dir
	for _, d := range dirs {
		abs, _ := filepath.Abs(d)
		fmt.Printf("Dir: %s (abs: %s)\n", d, abs)
		entries, err := os.ReadDir(d)
		if err != nil {
			fmt.Printf("  ERROR reading dir: %v\n", err)
			continue
		}
		for _, e := range entries {
			fmt.Printf("  %s (isDir=%v)\n", e.Name(), e.IsDir())
		}
	}

	fmt.Println("\n=== Scanning models ===")
	s := &scanner.Scanner{SourceDirs: dirs, Recursive: true}
	pkg, err := s.Scan()
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Scanned %d tables, %d enums\n\n", len(pkg.Tables), len(pkg.Enums))

	for _, t := range pkg.Tables {
		fmt.Printf("  Table: %s (%d columns)\n", t.Name, len(t.Columns))
	}
	for _, e := range pkg.Enums {
		fmt.Printf("  Enum:  %s (%v)\n", e.SQLName(), e.Values)
	}

	fmt.Println("\n=== Diffing (empty -> full schema) ===")
	empty := &schema.Package{}
	events := diff.Diff(empty, pkg)
	fmt.Printf("Generated %d events\n\n", len(events))
	for i, e := range events {
		extra := ""
		if e.Column != "" {
			extra = " col=" + e.Column
		}
		fmt.Printf("  [%d] %s table=%s%s\n", i+1, e.Kind, e.Table, extra)
	}

	fmt.Println("\n=== Generating migration SQL ===")
	p := postgres.New()
	gen := generator.New(p, events, empty, pkg)
	mf, err := gen.Generate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Version: %s\n", mf.Version)
	fmt.Printf("Up statements:   %d\n", len(mf.UpStatements))
	fmt.Printf("Down statements: %d\n", len(mf.DownStatements))
	fmt.Printf("HasDestructive:  %v\n", mf.HasDestructive)
	fmt.Printf("Checksum:        %s\n", mf.Checksum)

	os.MkdirAll("migrations", 0755)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("-- Mirage migration V%s\n", mf.Version))
	sb.WriteString(fmt.Sprintf("-- Checksum: %s\n\n", mf.Checksum))

	sb.WriteString("-- +migrate Up\n\n")
	for _, stmt := range mf.UpStatements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if !strings.HasSuffix(stmt, ";") {
			stmt += ";"
		}
		sb.WriteString(stmt)
		sb.WriteString("\n\n")
	}

	sb.WriteString("-- +migrate Down\n\n")
	for _, stmt := range mf.DownStatements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if !strings.HasSuffix(stmt, ";") {
			stmt += ";"
		}
		sb.WriteString(stmt)
		sb.WriteString("\n\n")
	}

	filename := fmt.Sprintf("migrations/V%s_%s.sql", mf.Version, mf.Description)
	if err := os.WriteFile(filename, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Migration written to: %s ===\n", filename)
	fmt.Println("\n--- SQL Preview ---")
	preview := sb.String()
	if len(preview) > 4000 {
		preview = preview[:4000] + "\n... (truncated)"
	}
	fmt.Println(preview)
}
