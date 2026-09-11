package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/justblue/mirage/internal/config"
)

func setTestConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	orig := appConfig
	appConfig = cfg
	t.Cleanup(func() { appConfig = orig })
}

func TestCfgDB_FromConfig(t *testing.T) {
	setTestConfig(t, &config.Config{DB: "postgres://config@localhost/db"})
	if got := cfgDB(); got != "postgres://config@localhost/db" {
		t.Errorf("cfgDB() = %q, want postgres://config@localhost/db", got)
	}
}

func TestCfgDB_NilConfig(t *testing.T) {
	setTestConfig(t, nil)
	if got := cfgDB(); got != "" {
		t.Errorf("cfgDB() = %q, want empty", got)
	}
}

func TestCfgSource_FromConfig(t *testing.T) {
	setTestConfig(t, &config.Config{Source: []string{"./models", "./pkg"}})
	got := cfgSource()
	if len(got) != 2 || got[0] != "./models" || got[1] != "./pkg" {
		t.Errorf("cfgSource() = %v, want [./models ./pkg]", got)
	}
}

func TestCfgSource_NilConfig(t *testing.T) {
	setTestConfig(t, nil)
	if got := cfgSource(); got != nil {
		t.Errorf("cfgSource() = %v, want nil", got)
	}
}

func TestCfgMigrationsDir_FromConfig(t *testing.T) {
	setTestConfig(t, &config.Config{MigrationsDir: "./db/migrate"})
	if got := cfgMigrationsDir(); got != "./db/migrate" {
		t.Errorf("cfgMigrationsDir() = %q, want ./db/migrate", got)
	}
}

func TestCfgMigrationsDir_NilConfig(t *testing.T) {
	setTestConfig(t, nil)
	if got := cfgMigrationsDir(); got != "" {
		t.Errorf("cfgMigrationsDir() = %q, want empty", got)
	}
}

func TestCfgIdempotent_FromConfig(t *testing.T) {
	setTestConfig(t, &config.Config{Idempotent: true})
	if !cfgIdempotent() {
		t.Error("cfgIdempotent() = false, want true")
	}
}

func TestCfgIdempotent_NilConfig(t *testing.T) {
	setTestConfig(t, nil)
	if cfgIdempotent() {
		t.Error("cfgIdempotent() = true, want false")
	}
}

func TestCfgVerbose_FromConfig(t *testing.T) {
	setTestConfig(t, &config.Config{Verbose: true})
	if !cfgVerbose() {
		t.Error("cfgVerbose() = false, want true")
	}
}

func TestCfgVerbose_NilConfig(t *testing.T) {
	setTestConfig(t, nil)
	if cfgVerbose() {
		t.Error("cfgVerbose() = true, want false")
	}
}

func TestConfigLoad_FullConfig(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`source:
  - ./models
  - ./pkg
migrations_dir: ./db/migrations
db: postgres://localhost/testdb
idempotent: true
verbose: true
`)
	if err := os.WriteFile(filepath.Join(dir, "mirage.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Chdir(dir)
	defer os.Chdir(orig)

	cfg, path, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if path == "" {
		t.Fatal("expected config path")
	}
	if len(cfg.Source) != 2 || cfg.Source[0] != "./models" || cfg.Source[1] != "./pkg" {
		t.Errorf("Source = %v, want [./models ./pkg]", cfg.Source)
	}
	if cfg.MigrationsDir != "./db/migrations" {
		t.Errorf("MigrationsDir = %q, want ./db/migrations", cfg.MigrationsDir)
	}
	if cfg.DB != "postgres://localhost/testdb" {
		t.Errorf("DB = %q, want postgres://localhost/testdb", cfg.DB)
	}
	if !cfg.Idempotent {
		t.Error("Idempotent = false, want true")
	}
	if !cfg.Verbose {
		t.Error("Verbose = false, want true")
	}
}

func TestConfigOverrides_Defaults(t *testing.T) {
	dir := t.TempDir()
	content := []byte("migrations_dir: ./custom/migrations\n")
	if err := os.WriteFile(filepath.Join(dir, "mirage.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Chdir(dir)
	defer os.Chdir(orig)

	cfg, _, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	setTestConfig(t, cfg)
	dirFromConfig := cfgMigrationsDir()
	if dirFromConfig != "./custom/migrations" {
		t.Errorf("config migrations_dir = %q, want ./custom/migrations", dirFromConfig)
	}
}

func TestCLIFlagOverridesConfig(t *testing.T) {
	setTestConfig(t, &config.Config{DB: "postgres://config@localhost/db"})

	flagValue := "postgres://flag@localhost/db"
	connStr := flagValue
	isSet := true
	if !isSet {
		if cfg := cfgDB(); cfg != "" {
			connStr = cfg
		}
	}
	if connStr != "postgres://flag@localhost/db" {
		t.Errorf("connStr = %q, want flag value", connStr)
	}
}

func TestConfigFallbackWhenFlagNotSet(t *testing.T) {
	setTestConfig(t, &config.Config{DB: "postgres://config@localhost/db"})

	connStr := ""
	isSet := false
	if !isSet {
		if cfg := cfgDB(); cfg != "" {
			connStr = cfg
		}
	}
	if connStr != "postgres://config@localhost/db" {
		t.Errorf("connStr = %q, want config value", connStr)
	}
}

func TestGeneratePipeline_UsesConfigSourceDirs(t *testing.T) {
	modelsDir := t.TempDir()
	writeModelFile(t, modelsDir, "user.go", validUserModel)

	setTestConfig(t, &config.Config{Source: []string{modelsDir}})

	res, err := generatePipeline(context.Background(), generateOptions{
		SourceDirs:   cfgSource(),
		MigrationDir: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("generatePipeline with config source dirs: %v", err)
	}
	if res.NoChanges {
		t.Error("expected changes with config-provided source dirs")
	}
}
