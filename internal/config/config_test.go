package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FindsMirageYaml(t *testing.T) {
	dir := t.TempDir()
	content := []byte("source:\n  - ./models\nmigrations_dir: ./db/migrations\ndb: postgres://localhost/test\n")
	if err := os.WriteFile(filepath.Join(dir, "mirage.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	// Change to the temp dir and restore on cleanup.
	orig, _ := os.Getwd()
	t.Chdir(dir)
	defer os.Chdir(orig)

	cfg, path, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if path == "" {
		t.Fatal("Load() returned empty path")
	}
	if len(cfg.Source) != 1 || cfg.Source[0] != "./models" {
		t.Errorf("Source = %v, want [./models]", cfg.Source)
	}
	if cfg.MigrationsDir != "./db/migrations" {
		t.Errorf("MigrationsDir = %q, want %q", cfg.MigrationsDir, "./db/migrations")
	}
	if cfg.DB != "postgres://localhost/test" {
		t.Errorf("DB = %q, want %q", cfg.DB, "postgres://localhost/test")
	}
}

func TestLoad_FindsDotMirageYaml(t *testing.T) {
	dir := t.TempDir()
	content := []byte("source:\n  - ./pkg\n")
	if err := os.WriteFile(filepath.Join(dir, ".mirage.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Chdir(dir)
	defer os.Chdir(orig)

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Source) != 1 || cfg.Source[0] != "./pkg" {
		t.Errorf("Source = %v, want [./pkg]", cfg.Source)
	}
}

func TestLoad_FindsInParentDirectory(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "subdir")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte("source:\n  - ./parent-models\n")
	if err := os.WriteFile(filepath.Join(parent, "mirage.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Chdir(child)
	defer os.Chdir(orig)

	cfg, path, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if filepath.Dir(path) != parent {
		t.Errorf("path dir = %q, want %q", filepath.Dir(path), parent)
	}
	if len(cfg.Source) != 1 || cfg.Source[0] != "./parent-models" {
		t.Errorf("Source = %v, want [./parent-models]", cfg.Source)
	}
}

func TestLoad_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Chdir(dir)
	defer os.Chdir(orig)

	cfg, path, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if path != "" {
		t.Errorf("path = %q, want empty", path)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
	if len(cfg.Source) != 0 {
		t.Errorf("Source = %v, want empty", cfg.Source)
	}
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	content := []byte("db: ${TEST_DB_URL}\n")
	if err := os.WriteFile(filepath.Join(dir, "mirage.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_DB_URL", "postgres://expanded@localhost/db")

	orig, _ := os.Getwd()
	t.Chdir(dir)
	defer os.Chdir(orig)

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.DB != "postgres://expanded@localhost/db" {
		t.Errorf("DB = %q, want %q", cfg.DB, "postgres://expanded@localhost/db")
	}
}

func TestLoad_MissingEnvVar(t *testing.T) {
	dir := t.TempDir()
	content := []byte("db: ${NONEXISTENT_VAR_12345}\n")
	if err := os.WriteFile(filepath.Join(dir, "mirage.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Chdir(dir)
	defer os.Chdir(orig)

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	// Missing env var keeps the ${...} reference as-is.
	if cfg.DB != "${NONEXISTENT_VAR_12345}" {
		t.Errorf("DB = %q, want %q", cfg.DB, "${NONEXISTENT_VAR_12345}")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"first non-empty", []string{"", "b", "c"}, "b"},
		{"all empty", []string{"", "", ""}, ""},
		{"first is non-empty", []string{"a", "b"}, "a"},
		{"empty input", []string{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirstNonEmpty(tt.values...)
			if got != tt.want {
				t.Errorf("FirstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}
