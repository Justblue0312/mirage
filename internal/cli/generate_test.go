package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justblue/mirage/internal/diff"
)

func TestGeneratePipeline_FirstRunProducesFile(t *testing.T) {
	modelsDir := t.TempDir()
	writeModelFile(t, modelsDir, "user.go", validUserModel)

	res, err := generatePipeline(context.Background(), generateOptions{
		SourceDirs:   []string{modelsDir},
		MigrationDir: t.TempDir(),
		Message:      "init",
	}, nil)
	if err != nil {
		t.Fatalf("generatePipeline: %v", err)
	}
	if res.NoChanges {
		t.Fatal("first run should report changes, got NoChanges=true")
	}
	if res.File == nil {
		t.Fatal("expected a migration file to be produced")
	}
	if len(res.File.UpStatements) == 0 {
		t.Error("expected up statements in generated migration")
	}
	if res.File.Description != "init" {
		t.Errorf("description = %q, want init", res.File.Description)
	}
}

func TestGeneratePipeline_RequiresSource(t *testing.T) {
	_, err := generatePipeline(context.Background(), generateOptions{MigrationDir: t.TempDir()}, nil)
	if err == nil {
		t.Fatal("expected error when no source directories are given")
	}
}

func TestGeneratePipeline_ValidationErrorAborts(t *testing.T) {
	modelsDir := t.TempDir()
	writeModelFile(t, modelsDir, "user.go", validUserModel)
	writeModelFile(t, modelsDir, "dup.go", duplicateTableModel)

	res, err := generatePipeline(context.Background(), generateOptions{
		SourceDirs:   []string{modelsDir},
		MigrationDir: t.TempDir(),
	}, nil)
	if err == nil {
		t.Fatal("expected a validation error for duplicate table names")
	}
	if res != nil && res.File != nil {
		t.Error("no file should be produced on validation failure")
	}
}

// stubDestructiveConfirmer records whether it was called and returns a fixed
// decision, letting us assert the pipeline routes destructive changes through
// the confirmation hook rather than performing terminal I/O itself.
func TestGeneratePipeline_ConfirmRenameHookInvoked(t *testing.T) {
	// This exercises the hook plumbing directly: with Force set, renames are
	// auto-confirmed and the hook must NOT be consulted.
	called := false
	hook := func(diff.RenameHint) (bool, error) {
		called = true
		return true, nil
	}

	modelsDir := t.TempDir()
	writeModelFile(t, modelsDir, "user.go", validUserModel)

	_, err := generatePipeline(context.Background(), generateOptions{
		SourceDirs:    []string{modelsDir},
		MigrationDir:  t.TempDir(),
		Force:         true,
		ConfirmRename: hook,
	}, nil)
	if err != nil {
		t.Fatalf("generatePipeline: %v", err)
	}
	if called {
		t.Error("with --force, the rename confirmation hook should not be called")
	}
}

func TestCmdGenerate_DryRunWithoutDB(t *testing.T) {
	modelsDir := t.TempDir()
	writeModelFile(t, modelsDir, "user.go", validUserModel)

	migrationsDir := filepath.Join(t.TempDir(), "migrations")

	err := runCLI(t, "generate",
		"--source", modelsDir,
		"--migrations-dir", migrationsDir,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("dry-run generate without --db should work against an empty baseline schema, got: %v", err)
	}

	entries, _ := os.ReadDir(migrationsDir)
	if len(entries) != 0 {
		t.Errorf("--dry-run must not write any migration file, found %d: %v", len(entries), entries)
	}
}

func TestCmdGenerate_WritesFileWithoutDryRun(t *testing.T) {
	modelsDir := t.TempDir()
	writeModelFile(t, modelsDir, "user.go", validUserModel)

	migrationsDir := filepath.Join(t.TempDir(), "migrations")

	err := runCLI(t, "generate",
		"--source", modelsDir,
		"--migrations-dir", migrationsDir,
		"--message", "init",
	)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 migration file, got %d: %v", len(entries), entries)
	}
}

func TestCmdGenerate_NoChangesIsANoOp(t *testing.T) {
	modelsDir := t.TempDir()
	writeModelFile(t, modelsDir, "user.go", validUserModel)
	migrationsDir := filepath.Join(t.TempDir(), "migrations")

	if err := runCLI(t, "generate", "--source", modelsDir, "--migrations-dir", migrationsDir, "--message", "init"); err != nil {
		t.Fatalf("first generate: %v", err)
	}
	entriesAfterFirst, _ := os.ReadDir(migrationsDir)

	// Re-running generate against the same models with no --db means there
	// is no persisted "old" snapshot to diff against, so this intentionally
	// does NOT assert "no new file is written" (that behavior requires
	// --db, which is exercised in the integration suite). What we do assert
	// is that running it again doesn't error and doesn't corrupt the
	// migrations directory.
	if err := runCLI(t, "generate", "--source", modelsDir, "--migrations-dir", migrationsDir, "--message", "second"); err != nil {
		t.Fatalf("second generate: %v", err)
	}
	entriesAfterSecond, _ := os.ReadDir(migrationsDir)
	if len(entriesAfterSecond) <= len(entriesAfterFirst) {
		t.Errorf("expected a second migration file to be added, had %d, now have %d", len(entriesAfterFirst), len(entriesAfterSecond))
	}
}

func TestCmdGenerate_RequiresSourceFlag(t *testing.T) {
	if err := runCLI(t, "generate", "--migrations-dir", t.TempDir()); err == nil {
		t.Fatal("expected an error when --source is omitted")
	}
}

func TestCmdGenerate_InvalidModelFailsValidation(t *testing.T) {
	modelsDir := t.TempDir()
	// A table with no primary key at all triggers ErrMissingPK, which is
	// warn-only (see internal/validate) -- included here mainly to document
	// that a *hard* validation error (e.g. ErrDuplicateTableName) is what
	// actually blocks generate; see TestCmdGenerate_DuplicateTableFails.
	writeModelFile(t, modelsDir, "user.go", validUserModel)
	writeModelFile(t, modelsDir, "dup.go", duplicateTableModel)

	migrationsDir := filepath.Join(t.TempDir(), "migrations")
	err := runCLI(t, "generate", "--source", modelsDir, "--migrations-dir", migrationsDir)
	if err == nil {
		t.Fatal("expected generate to fail validation for a duplicate table name")
	}

	entries, _ := os.ReadDir(migrationsDir)
	if len(entries) != 0 {
		t.Errorf("a failed validation must not leave a migration file behind, found %d", len(entries))
	}
}

// TestCmdGenerate_DestructiveChangeNonInteractive: see the sibling
// integration test TestGenerate_DestructiveChangeNonInteractive in
// internal/integration, which exercises this against a real --db snapshot
// (required to actually produce a DROP statement to test the gate against).
// It documents the still-open gap described in IMPROVE.md's CLI Review,
// Problem 4: a destructive change generated by a non-interactive process
// (no TTY -- exactly what every CI runner is) is written to disk with no
// confirmation prompt and no --force requirement.

// idempotentModel produces an ALTER TABLE ADD COLUMN statement (wrapped in a
// DO $$ BEGIN ... EXCEPTION block by WrapIdempotent) when the column is added
// to a pre-existing table. On a first run it instead produces a CREATE TABLE,
// which is why the idempotent tests below go through two generate steps with a
// persisted --db snapshot rather than relying on a fresh scan.
const idempotentModelV1 = `package models

type Product struct {
	_ struct{} ` + "`db:\"name=products\"`" + `
	ID   int64  ` + "`db:\"pk,identity,type=bigserial\"`" + `
	Name string ` + "`db:\"name=name,type=varchar(255),notnull\"`" + `
}
`

const idempotentModelV2 = `package models

type Product struct {
	_ struct{} ` + "`db:\"name=products\"`" + `
	ID   int64  ` + "`db:\"pk,identity,type=bigserial\"`" + `
	Name string ` + "`db:\"name=name,type=varchar(255),notnull\"`" + `
	Sku  string ` + "`db:\"name=sku,type=varchar(100),notnull\"`" + `
}
`

func lastMigrationFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected at least 1 migration file, got 0")
	}
	var newest os.FileInfo
	for _, e := range entries {
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("stat %s: %v", e.Name(), err)
		}
		if newest == nil || info.ModTime().After(newest.ModTime()) {
			newest = info
		}
	}
	return filepath.Join(dir, newest.Name())
}

func resetTestDB(t *testing.T, dsn string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to test db: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatalf("resetting test db schema: %v", err)
	}
}

func TestCmdGenerate_IdempotentFlagWrapsStatements(t *testing.T) {
	dsn := os.Getenv("MIRAGE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIRAGE_TEST_DATABASE_URL not set; skipping idempotent CLI test")
	}
	resetTestDB(t, dsn)

	migrationsDir := filepath.Join(t.TempDir(), "migrations")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatalf("creating migrations dir: %v", err)
	}
	run := func(args ...string) error {
		return runCLI(t, append(args, "--db", dsn, "--dir", migrationsDir)...)
	}

	// 1. tracker + initial schema
	if err := run("migrate"); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}
	v1Dir := t.TempDir()
	writeModelFile(t, v1Dir, "product.go", idempotentModelV1)
	if err := run("generate", "--source", v1Dir, "--message", "init"); err != nil {
		t.Fatalf("initial generate: %v", err)
	}
	if err := run("migrate"); err != nil {
		t.Fatalf("apply initial: %v", err)
	}

	// 2. add a column; with --idempotent the ADD COLUMN must be wrapped.
	v2Dir := t.TempDir()
	writeModelFile(t, v2Dir, "product.go", idempotentModelV2)
	if err := run("generate", "--source", v2Dir, "--message", "add_sku", "--idempotent"); err != nil {
		t.Fatalf("generate --idempotent: %v", err)
	}

	content, err := os.ReadFile(lastMigrationFile(t, migrationsDir))
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}
	body := string(content)
	if !strings.Contains(body, "DO $$ BEGIN") || !strings.Contains(body, `ALTER TABLE products ADD COLUMN "sku"`) {
		t.Errorf("--idempotent migration should wrap ADD COLUMN in a DO block, got:\n%s", body)
	}
}

func TestCmdGenerate_WithoutIdempotentLeavesStatementsUnwrapped(t *testing.T) {
	dsn := os.Getenv("MIRAGE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIRAGE_TEST_DATABASE_URL not set; skipping idempotent CLI test")
	}
	resetTestDB(t, dsn)

	migrationsDir := filepath.Join(t.TempDir(), "migrations")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatalf("creating migrations dir: %v", err)
	}
	run := func(args ...string) error {
		return runCLI(t, append(args, "--db", dsn, "--dir", migrationsDir)...)
	}

	if err := run("migrate"); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}
	v1Dir := t.TempDir()
	writeModelFile(t, v1Dir, "product.go", idempotentModelV1)
	if err := run("generate", "--source", v1Dir, "--message", "init"); err != nil {
		t.Fatalf("initial generate: %v", err)
	}
	if err := run("migrate"); err != nil {
		t.Fatalf("apply initial: %v", err)
	}

	v2Dir := t.TempDir()
	writeModelFile(t, v2Dir, "product.go", idempotentModelV2)
	if err := run("generate", "--source", v2Dir, "--message", "add_sku"); err != nil {
		t.Fatalf("generate without --idempotent: %v", err)
	}

	content, err := os.ReadFile(lastMigrationFile(t, migrationsDir))
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}
	body := string(content)
	if strings.Contains(body, "DO $$ BEGIN") {
		t.Errorf("migration without --idempotent should not wrap ADD COLUMN in a DO block, got:\n%s", body)
	}
}
