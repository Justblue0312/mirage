//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/justblue/mirage/internal/dialect/postgres"
	"github.com/justblue/mirage/internal/diff"
	"github.com/justblue/mirage/internal/generator"
	"github.com/justblue/mirage/internal/scanner"
	"github.com/justblue/mirage/internal/schema"
)

const (
	composeDir = "../../_examples"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dsn := os.Getenv("MIRAGE_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://test:test@localhost:15432/mirage_test?sslmode=disable"
	}

	// If the env var is set (CI), skip podman and just connect.
	if os.Getenv("MIRAGE_TEST_DATABASE_URL") != "" {
		t.Log("Using MIRAGE_TEST_DATABASE_URL from environment")
	} else {
		composePath, err := filepath.Abs(composeDir)
		if err != nil {
			t.Fatalf("resolving compose path: %v", err)
		}

		t.Log("Starting postgres via podman compose...")
		cmd := exec.CommandContext(ctx, "podman", "compose",
			"-f", filepath.Join(composePath, "docker-compose.yml"),
			"up", "-d", "--wait")
		cmd.Dir = composePath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("starting postgres: %v", err)
		}

		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "podman", "compose",
				"-f", filepath.Join(composePath, "docker-compose.yml"),
				"down", "-v")
			cmd.Dir = composePath
			_ = cmd.Run()
		})
	}

	var db *sql.DB
	var err error
	connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
	defer connectCancel()
	for i := 0; i < 30; i++ {
		db, err = sql.Open("pgx", dsn)
		if err == nil {
			err = db.PingContext(connectCtx)
			if err == nil {
				t.Logf("Connected to test DB after %d attempts", i+1)
				break
			}
			db.Close()
		}
		select {
		case <-connectCtx.Done():
			t.Fatalf("timed out connecting to test DB after %d attempts: %v", i+1, err)
		case <-time.After(time.Second):
		}
	}
	if err != nil {
		t.Fatalf("connecting to test DB: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	// Every test that reaches this point is about to scan the example
	// models and apply the full schema from scratch, as if starting from
	// an empty database. Without a reset here, only the first test in a
	// given `go test` run to successfully apply the schema can pass --
	// every test after it hits "type/table already exists" on its very
	// first statement, since nothing tears down what a previous test
	// left behind. This is what actually made a combined `go test
	// -tags=integration ./...` run (exactly what CI does) impossible to
	// pass even with a fully correct schema.
	if _, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public; DROP SCHEMA IF EXISTS auth CASCADE;`); err != nil {
		t.Fatalf("resetting public schema: %v", err)
	}

	// Create the app_role role required by the generated GRANT statement
	// in the migration schema. Roles are cluster-level objects, not
	// schema-level, so they survive the DROP SCHEMA above and only need
	// to be created once per database.
	if _, err := db.ExecContext(ctx, `DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'app_role') THEN CREATE ROLE app_role; END IF; END $$;`); err != nil {
		t.Fatalf("creating app_role: %v", err)
	}

	// The example models include a Supabase-style RLS policy that
	// references auth.uid(). Create the auth schema with a stub uid()
	// function so the generated CREATE POLICY succeeds.
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS auth; CREATE OR REPLACE FUNCTION auth.uid() RETURNS bigint AS $$ SELECT 0::bigint $$ LANGUAGE sql STABLE;`); err != nil {
		t.Fatalf("creating auth.uid() stub: %v", err)
	}

	return db
}

func scanModels(t *testing.T) *schema.Package {
	t.Helper()
	dirs := []string{
		"../../_examples/models/auth",
		"../../_examples/models/content",
		"../../_examples/models/catalog",
		"../../_examples/models/metrics",
	}
	s := &scanner.Scanner{SourceDirs: dirs, Recursive: true}
	pkg, err := s.Scan()
	if err != nil {
		t.Fatalf("scanning models: %v", err)
	}
	t.Logf("Scanned %d tables, %d enums", len(pkg.Tables), len(pkg.Enums))
	return pkg
}

func generateSQL(t *testing.T, oldPkg, newPkg *schema.Package) *generator.MigrationFile {
	t.Helper()
	events := diff.Diff(oldPkg, newPkg)
	if len(events) == 0 {
		t.Fatal("no diff events generated")
	}
	t.Logf("Generated %d diff events", len(events))

	p := postgres.New()
	gen := generator.New(p, events, oldPkg, newPkg)
	mf, err := gen.Generate()
	if err != nil {
		t.Fatalf("generating migration: %v", err)
	}
	t.Logf("Generated %d up statements, %d down statements", len(mf.UpStatements), len(mf.DownStatements))
	return mf
}

func execMigrations(t *testing.T, db *sql.DB, mf *generator.MigrationFile) {
	t.Helper()
	ctx := context.Background()
	for i, stmt := range mf.UpStatements {
		stmt = trimSQL(stmt)
		if stmt == "" {
			continue
		}
		t.Logf("Executing up statement %d: %s", i+1, truncate(stmt, 100))
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("executing up statement %d: %v\nSQL: %s", i+1, err, stmt)
		}
	}
}

func execDownMigrations(t *testing.T, db *sql.DB, mf *generator.MigrationFile) {
	t.Helper()
	ctx := context.Background()
	for i, stmt := range mf.DownStatements {
		stmt = trimSQL(stmt)
		if stmt == "" {
			continue
		}
		t.Logf("Executing down statement %d: %s", i+1, truncate(stmt, 100))
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("executing down statement %d: %v\nSQL: %s", i+1, err, stmt)
		}
	}
}

func trimSQL(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, name).Scan(&count)
	return count > 0
}

func columnExists(t *testing.T, db *sql.DB, table, col string) bool {
	t.Helper()
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, table, col).Scan(&count)
	return count > 0
}

func enumExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_type WHERE typname=$1)`, name).Scan(&exists)
	return exists
}

func rowCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
	return count
}

// ---- Tests ----

func TestScanModels(t *testing.T) {
	pkg := scanModels(t)

	if len(pkg.Tables) < 8 {
		t.Errorf("expected at least 8 tables, got %d", len(pkg.Tables))
	}
	if len(pkg.Enums) < 2 {
		t.Errorf("expected at least 2 enums, got %d", len(pkg.Enums))
	}

	// Verify specific tables exist
	tableNames := make(map[string]bool)
	for _, tbl := range pkg.Tables {
		tableNames[tbl.Name] = true
		expected := []string{"users", "posts", "comments", "tags", "post_tags", "categories", "products", "product_images", "product_variants", "events"}
		for _, e := range expected {
			if !tableNames[e] {
				// continue checking
			}
		}
	}
	for _, name := range []string{"users", "posts", "comments", "tags", "post_tags", "categories", "products", "product_images", "product_variants", "events"} {
		if !tableNames[name] {
			t.Errorf("expected table %q not found", name)
		}
	}

	// Verify enums
	enumNames := make(map[string]bool)
	for _, e := range pkg.Enums {
		enumNames[e.Name] = true
	}
	for _, name := range []string{"user_role", "post_status"} {
		if !enumNames[name] {
			t.Errorf("expected enum %q not found", name)
		}
	}
}

func TestGenerateMigration(t *testing.T) {
	pkg := scanModels(t)
	empty := &schema.Package{}
	mf := generateSQL(t, empty, pkg)

	if len(mf.UpStatements) == 0 {
		t.Fatal("expected up statements")
	}

	// Verify CREATE TYPE for enums
	foundEnum := false
	for _, stmt := range mf.UpStatements {
		if contains(stmt, "CREATE TYPE") && contains(stmt, "AS ENUM") {
			foundEnum = true
			break
		}
	}
	if !foundEnum {
		t.Error("expected CREATE TYPE ... AS ENUM statement")
	}

	// Verify CREATE TABLE for users
	foundUsers := false
	for _, stmt := range mf.UpStatements {
		if contains(stmt, "CREATE TABLE") && contains(stmt, "users") {
			foundUsers = true
			break
		}
	}
	if !foundUsers {
		t.Error("expected CREATE TABLE users statement")
	}
}

func TestApplyMigration(t *testing.T) {
	db := setupTestDB(t)
	pkg := scanModels(t)
	empty := &schema.Package{}
	mf := generateSQL(t, empty, pkg)

	execMigrations(t, db, mf)

	// Verify tables created
	for _, name := range []string{"users", "posts", "comments", "tags", "post_tags", "categories", "products", "product_images", "product_variants", "events"} {
		if !tableExists(t, db, name) {
			t.Errorf("table %q should exist after migration", name)
		}
	}

	// Verify enums created
	for _, name := range []string{"user_role", "post_status"} {
		if !enumExists(t, db, name) {
			t.Errorf("enum %q should exist after migration", name)
		}
	}

	// Verify specific columns
	if !columnExists(t, db, "users", "username") {
		t.Error("users.username should exist")
	}
	if !columnExists(t, db, "users", "email") {
		t.Error("users.email should exist")
	}
	if !columnExists(t, db, "posts", "body") {
		t.Error("posts.body should exist")
	}
}

func TestCRUD_Users(t *testing.T) {
	db := setupTestDB(t)
	pkg := scanModels(t)
	empty := &schema.Package{}
	mf := generateSQL(t, empty, pkg)
	execMigrations(t, db, mf)

	ctx := context.Background()

	// INSERT user
	var id int64
	err := db.QueryRowContext(ctx,
		`INSERT INTO users (username, email, password, role, status)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		"testuser", "test@example.com", "hashed_password", "member", "active").Scan(&id)
	if err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	t.Logf("Inserted user with ID: %d", id)

	// SELECT user
	var username, email, role string
	err = db.QueryRowContext(ctx,
		`SELECT username, email, role FROM users WHERE id = $1`, id).
		Scan(&username, &email, &role)
	if err != nil {
		t.Fatalf("selecting user: %v", err)
	}
	if username != "testuser" {
		t.Errorf("expected username 'testuser', got %q", username)
	}
	if email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %q", email)
	}
	if role != "member" {
		t.Errorf("expected role 'member', got %q", role)
	}

	// UPDATE user
	_, err = db.ExecContext(ctx,
		`UPDATE users SET first_name = $1, updated_at = NOW() WHERE id = $2`,
		"Test", id)
	if err != nil {
		t.Fatalf("updating user: %v", err)
	}

	var firstName string
	err = db.QueryRowContext(ctx,
		`SELECT first_name FROM users WHERE id = $1`, id).
		Scan(&firstName)
	if err != nil {
		t.Fatalf("selecting updated user: %v", err)
	}
	if firstName != "Test" {
		t.Errorf("expected first_name 'Test', got %q", firstName)
	}

	// Verify count
	count := rowCount(t, db, "users")
	if count != 1 {
		t.Errorf("expected 1 user, got %d", count)
	}
}

func TestCRUD_Posts(t *testing.T) {
	db := setupTestDB(t)
	pkg := scanModels(t)
	empty := &schema.Package{}
	mf := generateSQL(t, empty, pkg)
	execMigrations(t, db, mf)

	ctx := context.Background()

	// Create user first (FK dependency)
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (username, email, password, role, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		"author", "author@example.com", "hashed", "member", "active")
	if err != nil {
		t.Fatalf("inserting user: %v", err)
	}

	var userID int64
	_ = db.QueryRowContext(ctx, `SELECT id FROM users WHERE username='author'`).Scan(&userID)

	// Insert post
	var postID int64
	err = db.QueryRowContext(ctx,
		`INSERT INTO posts (user_id, title, slug, body, status)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, "Hello World", "hello-world", "# Hello\n\nThis is my first post.", "published").Scan(&postID)
	if err != nil {
		t.Fatalf("inserting post: %v", err)
	}

	// Insert comment
	_, err = db.ExecContext(ctx,
		`INSERT INTO comments (post_id, user_id, body)
		 VALUES ($1, $2, $3)`,
		postID, userID, "Great post!")
	if err != nil {
		t.Fatalf("inserting comment: %v", err)
	}

	// Insert tags
	_, err = db.ExecContext(ctx,
		`INSERT INTO tags (name, slug) VALUES ($1, $2)`,
		"golang", "golang")
	if err != nil {
		t.Fatalf("inserting tag: %v", err)
	}

	var tagID int64
	_ = db.QueryRowContext(ctx, `SELECT id FROM tags WHERE slug='golang'`).Scan(&tagID)

	// Link post to tag
	_, err = db.ExecContext(ctx,
		`INSERT INTO post_tags (post_id, tag_id, sort_order) VALUES ($1, $2, $3)`,
		postID, tagID, 1)
	if err != nil {
		t.Fatalf("inserting post_tag: %v", err)
	}

	// Verify post count
	if count := rowCount(t, db, "posts"); count != 1 {
		t.Errorf("expected 1 post, got %d", count)
	}
	if count := rowCount(t, db, "comments"); count != 1 {
		t.Errorf("expected 1 comment, got %d", count)
	}
	if count := rowCount(t, db, "tags"); count != 1 {
		t.Errorf("expected 1 tag, got %d", count)
	}
	if count := rowCount(t, db, "post_tags"); count != 1 {
		t.Errorf("expected 1 post_tag, got %d", count)
	}
}

func TestCRUD_Categories(t *testing.T) {
	db := setupTestDB(t)
	pkg := scanModels(t)
	empty := &schema.Package{}
	mf := generateSQL(t, empty, pkg)
	execMigrations(t, db, mf)

	ctx := context.Background()

	// Insert root category
	_, err := db.ExecContext(ctx,
		`INSERT INTO categories (name, slug, depth) VALUES ($1, $2, $3)`,
		"Electronics", "electronics", 0)
	if err != nil {
		t.Fatalf("inserting root category: %v", err)
	}

	var rootID int64
	_ = db.QueryRowContext(ctx, `SELECT id FROM categories WHERE slug='electronics'`).Scan(&rootID)

	// Insert child category
	_, err = db.ExecContext(ctx,
		`INSERT INTO categories (parent_id, name, slug, depth) VALUES ($1, $2, $3, $4)`,
		rootID, "Phones", "phones", 1)
	if err != nil {
		t.Fatalf("inserting child category: %v", err)
	}

	// Insert grandchild
	var childID int64
	_ = db.QueryRowContext(ctx, `SELECT id FROM categories WHERE slug='phones'`).Scan(&childID)

	_, err = db.ExecContext(ctx,
		`INSERT INTO categories (parent_id, name, slug, depth) VALUES ($1, $2, $3, $4)`,
		childID, "Smartphones", "smartphones", 2)
	if err != nil {
		t.Fatalf("inserting grandchild category: %v", err)
	}

	// Verify tree structure
	count := rowCount(t, db, "categories")
	if count != 3 {
		t.Errorf("expected 3 categories, got %d", count)
	}

	// Verify self-referencing FK works
	var parentCount int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM categories c1
		 JOIN categories c2 ON c1.parent_id = c2.id
		 WHERE c1.depth = 1 AND c2.depth = 0`).
		Scan(&parentCount)
	if parentCount != 1 {
		t.Errorf("expected 1 parent-child relationship, got %d", parentCount)
	}
}

func TestCRUD_Products(t *testing.T) {
	db := setupTestDB(t)
	pkg := scanModels(t)
	empty := &schema.Package{}
	mf := generateSQL(t, empty, pkg)
	execMigrations(t, db, mf)

	ctx := context.Background()

	// Create category first
	_, err := db.ExecContext(ctx,
		`INSERT INTO categories (name, slug, depth) VALUES ($1, $2, $3)`,
		"Widgets", "widgets", 0)
	if err != nil {
		t.Fatalf("inserting category: %v", err)
	}
	var catID int64
	_ = db.QueryRowContext(ctx, `SELECT id FROM categories WHERE slug='widgets'`).Scan(&catID)

	// Insert product
	var productID int64
	err = db.QueryRowContext(ctx,
		`INSERT INTO products (category_id, sku, name, slug, price_cents, currency_code, stock_quantity)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		catID, "WIDGET-001", "Super Widget", "super-widget", 2999, "USD", 100).Scan(&productID)
	if err != nil {
		t.Fatalf("inserting product: %v", err)
	}

	// Insert product images
	_, err = db.ExecContext(ctx,
		`INSERT INTO product_images (product_id, url, alt_text, sort_order, is_primary)
		 VALUES ($1, $2, $3, $4, $5)`,
		productID, "https://example.com/widget.jpg", "Super Widget photo", 0, true)
	if err != nil {
		t.Fatalf("inserting product image: %v", err)
	}

	// Insert product variant
	_, err = db.ExecContext(ctx,
		`INSERT INTO product_variants (product_id, sku, name, price_cents, stock_quantity, attributes_json)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		productID, "WIDGET-001-RED", "Red Widget", 3499, 50, `{"color":"red","size":"L"}`)
	if err != nil {
		t.Fatalf("inserting product variant: %v", err)
	}

	// Verify
	if count := rowCount(t, db, "products"); count != 1 {
		t.Errorf("expected 1 product, got %d", count)
	}
	if count := rowCount(t, db, "product_images"); count != 1 {
		t.Errorf("expected 1 product image, got %d", count)
	}
	if count := rowCount(t, db, "product_variants"); count != 1 {
		t.Errorf("expected 1 product variant, got %d", count)
	}

	// Verify CHECK constraint works
	_, err = db.ExecContext(ctx,
		`INSERT INTO products (category_id, sku, name, slug, price_cents, currency_code, stock_quantity)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		catID, "WIDGET-002", "Bad Widget", "bad-widget", -100, "USD", 10)
	if err == nil {
		t.Error("expected error for negative price_cents (CHECK constraint)")
	}
}

func TestCRUD_Events(t *testing.T) {
	db := setupTestDB(t)
	pkg := scanModels(t)
	empty := &schema.Package{}
	mf := generateSQL(t, empty, pkg)
	execMigrations(t, db, mf)

	ctx := context.Background()

	// events is PARTITION BY RANGE (created_at) with no partitions created
	// by the migration itself -- mirage generates the partitioned parent
	// table but doesn't manage partitions, same as most migration tools.
	// A real application needs at least one covering partition before it
	// can insert; a DEFAULT partition is the simplest one that works
	// regardless of what created_at's value ends up being.
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS events_default PARTITION OF events DEFAULT;`); err != nil {
		t.Fatalf("creating default partition for events: %v", err)
	}

	// Insert anonymous event
	_, err := db.ExecContext(ctx,
		`INSERT INTO events (session_id, event_type, path, user_agent)
		 VALUES ($1, $2, $3, $4)`,
		"sess_abc123", "page_view", "/", "Mozilla/5.0 Test Browser")
	if err != nil {
		t.Fatalf("inserting event: %v", err)
	}

	// Insert event with metadata
	_, err = db.ExecContext(ctx,
		`INSERT INTO events (session_id, event_type, resource_type, resource_id, path, metadata_json, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		"sess_abc123", "purchase", "product", 42, "/checkout",
		`{"total":2999,"items":1}`, 1500)
	if err != nil {
		t.Fatalf("inserting event with metadata: %v", err)
	}

	if count := rowCount(t, db, "events"); count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}

func TestRollback(t *testing.T) {
	db := setupTestDB(t)
	pkg := scanModels(t)
	empty := &schema.Package{}
	mf := generateSQL(t, empty, pkg)
	execMigrations(t, db, mf)

	ctx := context.Background()

	// Insert data
	_, _ = db.ExecContext(ctx,
		`INSERT INTO users (username, email, password, role, status)
		 VALUES ('rollback_user', 'rb@example.com', 'hash', 'guest', 'active')`)

	if count := rowCount(t, db, "users"); count != 1 {
		t.Fatalf("expected 1 user before rollback, got %d", count)
	}

	// Rollback down
	execDownMigrations(t, db, mf)

	// Verify tables dropped
	for _, name := range []string{"users", "posts", "comments"} {
		if tableExists(t, db, name) {
			t.Errorf("table %q should not exist after rollback", name)
		}
	}
}

func TestSchemaIntrospection(t *testing.T) {
	db := setupTestDB(t)
	pkg := scanModels(t)
	empty := &schema.Package{}
	mf := generateSQL(t, empty, pkg)
	execMigrations(t, db, mf)

	ctx := context.Background()

	// Verify users table has correct constraints
	var nullable string
	err := db.QueryRowContext(ctx,
		`SELECT is_nullable FROM information_schema.columns
		 WHERE table_schema='public' AND table_name='users' AND column_name='username'`).
		Scan(&nullable)
	if err != nil {
		t.Fatalf("querying column info: %v", err)
	}
	if nullable != "NO" {
		t.Errorf("users.username should be NOT NULL, got %q", nullable)
	}

	// Verify email is unique
	var constraintType string
	err = db.QueryRowContext(ctx,
		`SELECT constraint_type FROM information_schema.table_constraints
		 WHERE table_schema='public' AND table_name='users' AND constraint_type='UNIQUE'`).
		Scan(&constraintType)
	if err != nil {
		t.Fatalf("querying unique constraint: %v", err)
	}

	// Verify FK constraints exist
	var fkCount int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.table_constraints
		 WHERE table_schema='public' AND constraint_type='FOREIGN KEY'`).
		Scan(&fkCount)
	if fkCount < 5 {
		t.Errorf("expected at least 5 FK constraints, got %d", fkCount)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
