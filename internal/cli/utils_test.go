package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justblue/mirage/internal/generator"
	"github.com/justblue/mirage/internal/runner"
	"github.com/justblue/mirage/internal/validate"
)

func writeMigrationFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("-- +migrate Up\n"), 0644); err != nil {
		t.Fatalf("writing fixture %s: %v", name, err)
	}
}

func TestScanMigrations(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "V00002_add_users.sql")
	writeMigrationFile(t, dir, "V00001_init.sql")
	writeMigrationFile(t, dir, "V00010_late.sql")
	// Should be ignored: wrong extension, no version prefix, malformed name.
	writeMigrationFile(t, dir, "README.md")
	writeMigrationFile(t, dir, "notes.sql")
	writeMigrationFile(t, dir, "Vbadversion.sql")

	files, err := runner.ScanMigrations(dir)
	if err != nil {
		t.Fatalf("ScanMigrations: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 valid migration files, got %d: %+v", len(files), files)
	}

	// ScanMigrations sorts by version string; confirm ordering, and that
	// "00010" doesn't accidentally sort before "00002" as a plain string
	// comparison would only be correct because all versions here share
	// the same zero-padded width.
	wantOrder := []string{"00001", "00002", "00010"}
	for i, f := range files {
		if f.Version != wantOrder[i] {
			t.Errorf("position %d: got version %q, want %q", i, f.Version, wantOrder[i])
		}
	}
	if files[0].Name != "init" || files[1].Name != "add_users" || files[2].Name != "late" {
		t.Errorf("unexpected names: %+v", files)
	}
}

func TestScanMigrations_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := runner.ScanMigrations(dir)
	if err != nil {
		t.Fatalf("ScanMigrations on empty dir: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected no files, got %d", len(files))
	}
}

func TestScanMigrations_NonexistentDir(t *testing.T) {
	if _, err := runner.ScanMigrations(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected an error scanning a nonexistent directory, got nil")
	}
}

func TestNextSequentialVersion(t *testing.T) {
	dir := t.TempDir()
	if got := nextSequentialVersion(dir); got != "00001" {
		t.Errorf("empty dir: got %q, want %q", got, "00001")
	}

	writeMigrationFile(t, dir, "V00001_init.sql")
	writeMigrationFile(t, dir, "V00007_seven.sql")
	writeMigrationFile(t, dir, "V00003_three.sql")

	if got := nextSequentialVersion(dir); got != "00008" {
		t.Errorf("got %q, want %q (max existing version + 1)", got, "00008")
	}
}

func TestNextSequentialVersion_IgnoresMalformedNames(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "V00005_ok.sql")
	writeMigrationFile(t, dir, "Vnotanumber_bad.sql")
	writeMigrationFile(t, dir, "not-a-migration.sql")

	if got := nextSequentialVersion(dir); got != "00006" {
		t.Errorf("got %q, want %q", got, "00006")
	}
}

func TestFormatMigrationFile(t *testing.T) {
	mf := &generator.MigrationFile{
		Version:      "00001",
		Description:  "add_users",
		Checksum:     "sha256:deadbeef",
		Message:      "adds the users table",
		UpStatements: []string{`CREATE TABLE "users" (id bigserial PRIMARY KEY);`},
		DownStatements: []string{
			`DROP TABLE "users";`,
		},
		HasDestructive: false,
	}

	out := formatMigrationFile(mf)

	for _, want := range []string{
		"-- Migration: V00001_add_users",
		"-- Checksum: sha256:deadbeef",
		"-- Message: adds the users table",
		"-- +migrate Up",
		`CREATE TABLE "users"`,
		"-- +migrate Down",
		`DROP TABLE "users";`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("formatted migration missing expected substring %q\n--- got ---\n%s", want, out)
		}
	}

	if strings.Contains(out, "WARNING: destructive") {
		t.Errorf("non-destructive migration should not include the destructive-change warning banner")
	}
}

func TestFormatMigrationFile_DestructiveWarning(t *testing.T) {
	mf := &generator.MigrationFile{
		Version:        "00002",
		Description:    "drop_legacy",
		UpStatements:   []string{`DROP TABLE "legacy";`},
		HasDestructive: true,
	}

	out := formatMigrationFile(mf)
	if !strings.Contains(out, "WARNING: destructive") {
		t.Errorf("destructive migration should include the warning banner, got:\n%s", out)
	}
}

func TestFormatMigrationFile_AutocommitStatements(t *testing.T) {
	mf := &generator.MigrationFile{
		Version:              "00003",
		Description:          "add_index_concurrently",
		UpStatements:         []string{`SELECT 1;`},
		AutocommitStatements: []string{`CREATE INDEX CONCURRENTLY idx_foo ON foo(bar);`},
	}

	out := formatMigrationFile(mf)
	if !strings.Contains(out, "Autocommit statements") {
		t.Errorf("expected autocommit section header, got:\n%s", out)
	}
	if !strings.Contains(out, "CREATE INDEX CONCURRENTLY") {
		t.Errorf("expected autocommit statement content, got:\n%s", out)
	}
}

func TestCountErrors(t *testing.T) {
	errs := []validate.ValidationError{
		{Code: validate.ErrNotNullNoDefault}, // warning-class per severity table
	}
	if n := countErrors(errs); n != 0 {
		t.Errorf("expected 0 hard errors for a warning-only slice, got %d", n)
	}
}

func TestCountErrors_Empty(t *testing.T) {
	if n := countErrors(nil); n != 0 {
		t.Errorf("expected 0 for nil input, got %d", n)
	}
}
