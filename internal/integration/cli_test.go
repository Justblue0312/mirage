//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	cli "github.com/justblue/mirage/internal/cli"
)

const cliTestTableModel = `package models

type Widget struct {
	_ struct{} ` + "`db:\"name=widgets\"`" + `

	ID   int64  ` + "`db:\"pk,identity,type=bigint\"`" + `
	Name string ` + "`db:\"name=name,type=text,notnull\"`" + `
}
`

// writeCLITestModel creates a single-file model directory. A different dir
// per generation step is used (rather than editing one file in place) so
// each `generate` call sees an unambiguous, complete set of models.
func writeCLITestModel(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(content), 0644); err != nil {
		t.Fatalf("writing model: %v", err)
	}
	return dir
}

// TestGenerate_DestructiveChangeNonInteractive is the CLI-level regression
// test for the non-TTY destructive-change fail-closed gate. It needs a
// real, persisted --db snapshot to produce an actual destructive diff
// (dropping the widgets table) -- a DB-free scan alone has no prior state
// to be destructive relative to, which is why this lives here rather than
// in the internal/cli unit test suite. go test's own process has no TTY
// attached to stdin, which is exactly the condition this gate exists for.
func TestGenerate_DestructiveChangeNonInteractive(t *testing.T) {
	dsn := testMirageDSN(t)
	migrationsDir := filepath.Join(t.TempDir(), "migrations")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatalf("creating migrations dir: %v", err)
	}

	run := func(args ...string) error {
		return cli.Run(t.Context(), append([]string{"mirage"}, args...))
	}

	// 1. Ensure the tracker table exists (Migrate creates it unconditionally
	// before applying anything, even with zero migration files present).
	if err := run("migrate", "--db", dsn, "--dir", migrationsDir); err != nil {
		t.Fatalf("initial migrate (tracker setup): %v", err)
	}

	// 2. Generate and apply the initial schema (one table).
	modelsWithTable := writeCLITestModel(t, cliTestTableModel)
	if err := run("generate", "--source", modelsWithTable, "--migrations-dir", migrationsDir, "--db", dsn, "--message", "init"); err != nil {
		t.Fatalf("initial generate: %v", err)
	}
	if err := run("migrate", "--db", dsn, "--dir", migrationsDir); err != nil {
		t.Fatalf("applying initial migration: %v", err)
	}

	entriesBefore, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}

	// 3. Point --source at an empty models directory -- from the persisted
	// snapshot's perspective, the widgets table was just deleted. This is
	// exactly the destructive-diff scenario the gate exists for.
	emptyModels := t.TempDir()
	err = run("generate", "--source", emptyModels, "--migrations-dir", migrationsDir, "--db", dsn, "--verbose")
	if err == nil {
		t.Fatal("expected generate to fail closed on a destructive change with no TTY and no --force, got nil error")
	}
	t.Logf("got expected error: %v", err)

	entriesAfter, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	if len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("a failed-closed destructive generate must not write a migration file: had %d files, now have %d",
			len(entriesBefore), len(entriesAfter))
	}

	// 4. The same destructive change with --force must succeed -- the gate
	// blocks by default, it doesn't remove the escape hatch.
	if err := run("generate", "--source", emptyModels, "--migrations-dir", migrationsDir, "--db", dsn, "--force"); err != nil {
		t.Fatalf("generate with --force should succeed even non-interactively, got: %v", err)
	}

	entriesFinal, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	if len(entriesFinal) <= len(entriesBefore) {
		t.Fatalf("expected --force to write a new migration file, had %d, now have %d", len(entriesBefore), len(entriesFinal))
	}
}
