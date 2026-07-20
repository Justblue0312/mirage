package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI invokes the real command tree exactly as os.Args would, and is the
// black-box pattern for testing internal/cli commands that don't yet have a
// service-layer split: no mocking of urfave/cli, no reaching into private
// Action closures -- just "run mirage <args> in a temp dir and check what
// happened on disk / what error came back".
func runCLI(t *testing.T, args ...string) error {
	t.Helper()
	return Run(context.Background(), append([]string{"mirage"}, args...))
}

func TestCmdCreate_WritesMigrationFile(t *testing.T) {
	dir := t.TempDir()

	if err := runCLI(t, "create", "--dir", dir, "--message", "add users table"); err != nil {
		t.Fatalf("create: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 file to be created, got %d: %v", len(entries), entries)
	}

	name := entries[0].Name()
	if !strings.HasPrefix(name, "V") || !strings.HasSuffix(name, "_add_users_table.sql") {
		t.Errorf("unexpected file name %q", name)
	}

	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("reading created file: %v", err)
	}
	for _, want := range []string{"-- +migrate Up", "-- +migrate Down", "-- Message: add users table"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("created migration missing expected content %q\n--- got ---\n%s", want, content)
		}
	}
}

func TestCmdCreate_WithoutMessage(t *testing.T) {
	dir := t.TempDir()

	if err := runCLI(t, "create", "--dir", dir); err != nil {
		t.Fatalf("create: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 file, got %d", len(entries))
	}
	content, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading created file: %v", err)
	}
	if strings.Contains(string(content), "-- Message:") {
		t.Errorf("migration created without --message should not include a Message header, got:\n%s", content)
	}
}

func TestCmdCreate_SequentialNaming(t *testing.T) {
	dir := t.TempDir()

	if err := runCLI(t, "create", "--dir", dir, "--sequential-naming", "--message", "first"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := runCLI(t, "create", "--dir", dir, "--sequential-naming", "--message", "second"); err != nil {
		t.Fatalf("second create: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(entries), entries)
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !strings.HasPrefix(names[0], "V00001_") && !strings.HasPrefix(names[1], "V00001_") {
		t.Errorf("expected one file to start with V00001_, got %v", names)
	}
	if !strings.HasPrefix(names[0], "V00002_") && !strings.HasPrefix(names[1], "V00002_") {
		t.Errorf("expected one file to start with V00002_ (sequential numbering across two calls), got %v", names)
	}
}

func TestCmdCreate_CreatesMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "migrations")

	if err := runCLI(t, "create", "--dir", dir, "--message", "init"); err != nil {
		t.Fatalf("create with nonexistent nested dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("expected migrations dir to be created: %v", err)
	}
}
