package postgres

import (
	"strings"
	"testing"

	"github.com/justblue/mirage/internal/schema"
)

func TestCapabilityFlags(t *testing.T) {
	p := New()
	if !p.SupportsEnum() {
		t.Error("SupportsEnum = false, want true")
	}
	if !p.SupportsReturning() {
		t.Error("SupportsReturning = false, want true")
	}
	if !p.SupportsILike() {
		t.Error("SupportsILike = false, want true")
	}
	if !p.SupportsTransactionalDDL() {
		t.Error("SupportsTransactionalDDL = false, want true")
	}
	if !p.SupportsIfNotExists() {
		t.Error("SupportsIfNotExists = false, want true")
	}
	if !p.SupportsAddEnumValueInTransaction() {
		t.Error("SupportsAddEnumValueInTransaction = false, want true")
	}
}

func TestQuoteIdent(t *testing.T) {
	p := New()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple identifier", "user", `"user"`},
		{"identifier with double quote", `user"name`, `"user""name"`},
		{"empty string", "", `""`},
		{"already quoted", `"foo"`, `"""foo"""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.QuoteIdent(tt.input); got != tt.want {
				t.Errorf("QuoteIdent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDataType(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		col  schema.Column
		want string
	}{
		{"integer", schema.Column{SQLType: "integer"}, "integer"},
		{"varchar", schema.Column{SQLType: "varchar(255)"}, "varchar(255)"},
		{"timestamp", schema.Column{SQLType: "timestamp with time zone"}, "timestamp with time zone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.DataType(tt.col); got != tt.want {
				t.Errorf("DataType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertDefault(t *testing.T) {
	p := New()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"null", "NULL", "NULL"},
		{"now()", "now()", "now()"},
		{"string literal", "'hello'", "'hello'"},
		{"numeric", "0", "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.ConvertDefault(tt.input); got != tt.want {
				t.Errorf("ConvertDefault(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertDefaultForType(t *testing.T) {
	p := New()

	tests := []struct {
		name    string
		val     string
		sqlType string
		want    string
	}{
		{"jsonb empty object", "{}", "jsonb", "'{}'"},
		{"jsonb null string", "null", "jsonb", "null"},
		{"jsonb NULL keyword", "NULL", "jsonb", "NULL"},
		{"text literal", "hello", "text", "'hello'"},
		{"varchar literal", "world", "varchar(10)", "'world'"},
		{"uuid literal", "550e8400-e29b-41d4-a716-446655440000", "uuid", "'550e8400-e29b-41d4-a716-446655440000'"},
		{"int literal", "123", "integer", "123"},
		{"bool true", "true", "boolean", "true"},
		{"bool false", "false", "boolean", "false"},
		{"now function", "now()", "timestamp", "now()"},
		{"date literal", "2023-01-01", "date", "'2023-01-01'"},
		{"custom enum", "ACTIVE", "user_status", "'ACTIVE'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.ConvertDefaultForType(tt.val, tt.sqlType); got != tt.want {
				t.Errorf("ConvertDefaultForType(%q, %q) = %q, want %q", tt.val, tt.sqlType, got, tt.want)
			}
		})
	}
}

func TestCreateEnumSQL(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		enum *schema.Enum
		want string
	}{
		{
			"simple enum",
			&schema.Enum{Name: "status", Values: []string{"active", "inactive"}},
			"CREATE TYPE status AS ENUM ('active', 'inactive');",
		},
		{
			"schema-qualified enum",
			&schema.Enum{SearchPath: "app", Name: "role", Values: []string{"admin", "user"}},
			"CREATE TYPE app.role AS ENUM ('admin', 'user');",
		},
		{
			"single value enum",
			&schema.Enum{Name: "color", Values: []string{"red"}},
			"CREATE TYPE color AS ENUM ('red');",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.CreateEnumSQL(tt.enum); got != tt.want {
				t.Errorf("CreateEnumSQL() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestDropEnumSQL(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		enum *schema.Enum
		want string
	}{
		{"simple enum", &schema.Enum{Name: "status"}, "DROP TYPE IF EXISTS status;"},
		{"schema-qualified", &schema.Enum{SearchPath: "app", Name: "role"}, "DROP TYPE IF EXISTS app.role;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.DropEnumSQL(tt.enum); got != tt.want {
				t.Errorf("DropEnumSQL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAlterEnumAddValue(t *testing.T) {
	p := New()
	enum := &schema.Enum{Name: "status"}
	got := p.AlterEnumAddValue(enum, "pending")

	if strings.Contains(got, "-- requires autocommit") {
		t.Error("AlterEnumAddValue() should not contain autocommit comment")
	}
	if !strings.Contains(got, "ALTER TYPE status ADD VALUE IF NOT EXISTS 'pending';") {
		t.Errorf("AlterEnumAddValue() missing ALTER TYPE statement: %q", got)
	}
}

func TestCreateTable(t *testing.T) {
	p := New()

	t.Run("two-column table with IF NOT EXISTS", func(t *testing.T) {
		table := schema.Table{
			Name: "users",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
				{Name: "name", SQLType: "varchar(100)", Nullable: false},
			},
		}
		stmts := p.CreateTable(table)
		if len(stmts) != 1 {
			t.Fatalf("expected 1 statement, got %d", len(stmts))
		}
		if !strings.Contains(stmts[0], "CREATE TABLE IF NOT EXISTS users") {
			t.Errorf("missing IF NOT EXISTS: %s", stmts[0])
		}
		if !strings.Contains(stmts[0], `"id" integer NOT NULL`) {
			t.Errorf("missing id column: %s", stmts[0])
		}
		if !strings.Contains(stmts[0], `"name" varchar(100) NOT NULL`) {
			t.Errorf("missing name column: %s", stmts[0])
		}
	})

	t.Run("column ordering respects OrdinalPosition", func(t *testing.T) {
		table := schema.Table{
			Name: "t",
			Columns: []*schema.Column{
				{Name: "last_col", SQLType: "text", OrdinalPosition: 100},
				{Name: "first_col", SQLType: "integer", OrdinalPosition: 10},
				{Name: "mid_col", SQLType: "text", OrdinalPosition: 50},
			},
		}
		stmts := p.CreateTable(table)
		stmt := stmts[0]

		firstPos := strings.Index(stmt, `"first_col"`)
		midPos := strings.Index(stmt, `"mid_col"`)
		lastPos := strings.Index(stmt, `"last_col"`)

		if firstPos == -1 || midPos == -1 || lastPos == -1 {
			t.Fatal("missing columns in SQL")
		}

		if firstPos >= midPos {
			t.Error("first_col should appear before mid_col")
		}
		if midPos >= lastPos {
			t.Error("mid_col should appear before last_col")
		}
	})

	t.Run("includes CONSTRAINT for PK", func(t *testing.T) {
		table := schema.Table{
			Name: "t",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
			},
			PrimaryKey: &schema.PrimaryKey{Name: "t_pkey", Columns: []string{"id"}},
		}
		stmts := p.CreateTable(table)
		if !strings.Contains(stmts[0], `CONSTRAINT "t_pkey" PRIMARY KEY ("id")`) {
			t.Errorf("missing PK constraint: %s", stmts[0])
		}
	})

	t.Run("includes CONSTRAINT for UQ", func(t *testing.T) {
		table := schema.Table{
			Name: "t",
			Columns: []*schema.Column{
				{Name: "email", SQLType: "varchar(255)", Nullable: false},
			},
			Uniques: []schema.UniqueConstraint{
				{Name: "t_email_key", Columns: []string{"email"}},
			},
		}
		stmts := p.CreateTable(table)
		if !strings.Contains(stmts[0], `CONSTRAINT "t_email_key" UNIQUE ("email")`) {
			t.Errorf("missing UQ constraint: %s", stmts[0])
		}
	})

	t.Run("includes CONSTRAINT for FK", func(t *testing.T) {
		table := schema.Table{
			Name: "orders",
			Columns: []*schema.Column{
				{Name: "user_id", SQLType: "integer", Nullable: false},
			},
			ForeignKeys: []schema.ForeignKey{
				{
					Name:        "orders_user_id_fkey",
					FromColumns: []string{"user_id"},
					ToTable:     "users",
					ToColumns:   []string{"id"},
					OnDelete:    "CASCADE",
					OnUpdate:    "NO ACTION",
				},
			},
		}
		stmts := p.CreateTable(table)
		stmt := stmts[0]
		if !strings.Contains(stmt, `CONSTRAINT "orders_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id")`) {
			t.Errorf("missing FK constraint: %s", stmt)
		}
		if !strings.Contains(stmt, "ON DELETE CASCADE") {
			t.Errorf("missing ON DELETE: %s", stmt)
		}
		if !strings.Contains(stmt, "ON UPDATE NO ACTION") {
			t.Errorf("missing ON UPDATE: %s", stmt)
		}
	})

	t.Run("includes CONSTRAINT for CHECK", func(t *testing.T) {
		table := schema.Table{
			Name: "products",
			Columns: []*schema.Column{
				{Name: "price", SQLType: "numeric", Nullable: false},
			},
			Checks: []schema.CheckConstraint{
				{Name: "price_positive", Expression: "price > 0"},
			},
		}
		stmts := p.CreateTable(table)
		if !strings.Contains(stmts[0], `CONSTRAINT "price_positive" CHECK (price > 0)`) {
			t.Errorf("missing CHECK constraint: %s", stmts[0])
		}
	})

	t.Run("with comment emits COMMENT ON TABLE", func(t *testing.T) {
		table := schema.Table{
			Name:        "users",
			Description: "User accounts",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
			},
		}
		stmts := p.CreateTable(table)
		if len(stmts) != 2 {
			t.Fatalf("expected 2 statements, got %d", len(stmts))
		}
		if !strings.Contains(stmts[1], "COMMENT ON TABLE users IS 'User accounts';") {
			t.Errorf("missing table comment: %s", stmts[1])
		}
	})

	t.Run("with partition", func(t *testing.T) {
		table := schema.Table{
			Name: "logs",
			Columns: []*schema.Column{
				{Name: "id", SQLType: "integer", Nullable: false},
				{Name: "created_at", SQLType: "timestamp", Nullable: false},
			},
			Partitioned: &schema.Partition{Strategy: "RANGE", Column: "created_at"},
		}
		stmts := p.CreateTable(table)
		if !strings.Contains(stmts[0], "PARTITION BY RANGE (\"created_at\")") {
			t.Errorf("missing partition clause: %s", stmts[0])
		}
	})
}

func TestDropTable(t *testing.T) {
	p := New()

	table := schema.Table{Name: "users"}
	stmts := p.DropTable(table)

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := "DROP TABLE IF EXISTS users;"
	if stmts[0] != want {
		t.Errorf("DropTable() = %q, want %q", stmts[0], want)
	}
}

func TestAddColumn(t *testing.T) {
	p := New()

	col := schema.Column{Name: "email", SQLType: "varchar(255)", Nullable: false}
	stmts := p.AddColumn("users", col)

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE users ADD COLUMN "email" varchar(255) NOT NULL;`
	if stmts[0] != want {
		t.Errorf("AddColumn() = %q, want %q", stmts[0], want)
	}
}

func TestDropColumn(t *testing.T) {
	p := New()

	stmts := p.DropColumn("users", "email")

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE users DROP COLUMN "email";`
	if stmts[0] != want {
		t.Errorf("DropColumn() = %q, want %q", stmts[0], want)
	}
}

func TestAlterColumnType(t *testing.T) {
	p := New()

	col := schema.Column{Name: "status", SQLType: "varchar(50)"}
	stmts := p.AlterColumnType("users", col)

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE users ALTER COLUMN "status" TYPE varchar(50) USING "status"::varchar(50);`
	if stmts[0] != want {
		t.Errorf("AlterColumnType() = %q, want %q", stmts[0], want)
	}
	if !strings.Contains(stmts[0], "USING") {
		t.Error("AlterColumnType() must include USING clause")
	}
}

func TestAlterColumnNullability(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		col  schema.Column
		want string
	}{
		{
			"set not null",
			schema.Column{Name: "email", SQLType: "varchar(255)", Nullable: false},
			`ALTER TABLE users ALTER COLUMN "email" SET NOT NULL;`,
		},
		{
			"drop not null",
			schema.Column{Name: "email", SQLType: "varchar(255)", Nullable: true},
			`ALTER TABLE users ALTER COLUMN "email" DROP NOT NULL;`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := p.AlterColumnNullability("users", tt.col)
			if len(stmts) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(stmts))
			}
			if stmts[0] != tt.want {
				t.Errorf("AlterColumnNullability() = %q, want %q", stmts[0], tt.want)
			}
		})
	}
}

func TestAlterColumnDefault(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		col  schema.Column
		want string
	}{
		{
			"set default",
			schema.Column{Name: "created_at", SQLType: "timestamp", Default: "now()"},
			`ALTER TABLE users ALTER COLUMN "created_at" SET DEFAULT now();`,
		},
		{
			"drop default",
			schema.Column{Name: "created_at", SQLType: "timestamp", Default: ""},
			`ALTER TABLE users ALTER COLUMN "created_at" DROP DEFAULT;`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := p.AlterColumnDefault("users", tt.col)
			if len(stmts) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(stmts))
			}
			if stmts[0] != tt.want {
				t.Errorf("AlterColumnDefault() = %q, want %q", stmts[0], tt.want)
			}
		})
	}
}

func TestAddPrimaryKey(t *testing.T) {
	p := New()

	pk := schema.PrimaryKey{Name: "users_pkey", Columns: []string{"id"}}
	stmts := p.AddPrimaryKey("users", pk)

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE users ADD CONSTRAINT "users_pkey" PRIMARY KEY ("id");`
	if stmts[0] != want {
		t.Errorf("AddPrimaryKey() = %q, want %q", stmts[0], want)
	}
}

func TestDropPrimaryKey(t *testing.T) {
	p := New()

	pk := schema.PrimaryKey{Name: "users_pkey"}
	stmts := p.DropPrimaryKey("users", pk)

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE users DROP CONSTRAINT "users_pkey";`
	if stmts[0] != want {
		t.Errorf("DropPrimaryKey() = %q, want %q", stmts[0], want)
	}
}

func TestAddForeignKey(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		fk   schema.ForeignKey
		want string
	}{
		{
			"with on delete and on update",
			schema.ForeignKey{
				Name:        "orders_user_id_fkey",
				FromColumns: []string{"user_id"},
				ToTable:     "users",
				ToColumns:   []string{"id"},
				OnDelete:    "CASCADE",
				OnUpdate:    "CASCADE",
			},
			`ALTER TABLE orders ADD CONSTRAINT "orders_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE ON UPDATE CASCADE;`,
		},
		{
			"without on delete or on update",
			schema.ForeignKey{
				Name:        "orders_user_id_fkey",
				FromColumns: []string{"user_id"},
				ToTable:     "users",
				ToColumns:   []string{"id"},
			},
			`ALTER TABLE orders ADD CONSTRAINT "orders_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id");`,
		},
		{
			"composite key",
			schema.ForeignKey{
				Name:        "fk_composite",
				FromColumns: []string{"a", "b"},
				ToTable:     "other",
				ToColumns:   []string{"x", "y"},
			},
			`ALTER TABLE orders ADD CONSTRAINT "fk_composite" FOREIGN KEY ("a", "b") REFERENCES "other" ("x", "y");`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := p.AddForeignKey("orders", tt.fk)
			if len(stmts) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(stmts))
			}
			if stmts[0] != tt.want {
				t.Errorf("AddForeignKey() =\n  %q\nwant:\n  %q", stmts[0], tt.want)
			}
		})
	}
}

func TestDropForeignKey(t *testing.T) {
	p := New()

	stmts := p.DropForeignKey("orders", "orders_user_id_fkey")

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE orders DROP CONSTRAINT "orders_user_id_fkey";`
	if stmts[0] != want {
		t.Errorf("DropForeignKey() = %q, want %q", stmts[0], want)
	}
}

func TestAddUnique(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		u    schema.UniqueConstraint
		want string
	}{
		{
			"single column",
			schema.UniqueConstraint{Name: "users_email_key", Columns: []string{"email"}},
			`ALTER TABLE users ADD CONSTRAINT "users_email_key" UNIQUE ("email");`,
		},
		{
			"composite",
			schema.UniqueConstraint{Name: "uq_composite", Columns: []string{"a", "b"}},
			`ALTER TABLE users ADD CONSTRAINT "uq_composite" UNIQUE ("a", "b");`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := p.AddUnique("users", tt.u)
			if len(stmts) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(stmts))
			}
			if stmts[0] != tt.want {
				t.Errorf("AddUnique() = %q, want %q", stmts[0], tt.want)
			}
		})
	}
}

func TestDropUnique(t *testing.T) {
	p := New()

	stmts := p.DropUnique("users", "users_email_key")

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE users DROP CONSTRAINT "users_email_key";`
	if stmts[0] != want {
		t.Errorf("DropUnique() = %q, want %q", stmts[0], want)
	}
}

func TestAddCheck(t *testing.T) {
	p := New()

	c := schema.CheckConstraint{Name: "price_positive", Expression: "price > 0"}
	stmts := p.AddCheck("products", c)

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE products ADD CONSTRAINT "price_positive" CHECK (price > 0);`
	if stmts[0] != want {
		t.Errorf("AddCheck() = %q, want %q", stmts[0], want)
	}
}

func TestDropCheck(t *testing.T) {
	p := New()

	stmts := p.DropCheck("products", "price_positive")

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE products DROP CONSTRAINT "price_positive";`
	if stmts[0] != want {
		t.Errorf("DropCheck() = %q, want %q", stmts[0], want)
	}
}

func TestCreateIndex(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		idx  schema.Index
		want string
	}{
		{
			"index without kind uses default",
			schema.Index{Name: "idx_users_email", Table: "users", Columns: []string{"email"}, Kind: ""},
			`CREATE INDEX "idx_users_email" ON users ("email");`,
		},
		{
			"gin index",
			schema.Index{Name: "idx_data", Table: "docs", Columns: []string{"body"}, Kind: "gin"},
			`CREATE INDEX "idx_data" ON docs USING gin ("body");`,
		},
		{
			"hash index",
			schema.Index{Name: "idx_token", Table: "users", Columns: []string{"token"}, Kind: "hash"},
			`CREATE INDEX "idx_token" ON users USING hash ("token");`,
		},
		{
			"index with sort and no kind",
			schema.Index{Name: "idx_created", Table: "logs", Columns: []string{"created_at"}, Kind: "", Sort: "DESC"},
			`CREATE INDEX "idx_created" ON logs ("created_at" DESC);`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := p.CreateIndex(tt.idx)
			if len(stmts) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(stmts))
			}
			if stmts[0] != tt.want {
				t.Errorf("CreateIndex() = %q, want %q", stmts[0], tt.want)
			}
		})
	}
}

func TestDropIndex(t *testing.T) {
	p := New()

	idx := schema.Index{Name: "idx_users_email"}
	stmts := p.DropIndex(idx)

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `DROP INDEX IF EXISTS "idx_users_email";`
	if stmts[0] != want {
		t.Errorf("DropIndex() = %q, want %q", stmts[0], want)
	}
}

func TestWrapIdempotent(t *testing.T) {
	p := New()

	tests := []struct {
		name  string
		input []string
		check func(t *testing.T, got []string)
	}{
		{
			"ADD COLUMN wrapped in DO block",
			[]string{`ALTER TABLE users ADD COLUMN "email" varchar(255);`},
			func(t *testing.T, got []string) {
				if len(got) != 1 {
					t.Fatalf("expected 1 statement, got %d", len(got))
				}
				if !strings.HasPrefix(got[0], "DO $$ BEGIN") {
					t.Errorf("expected DO block, got: %s", got[0])
				}
				if !strings.Contains(got[0], "EXCEPTION WHEN duplicate_column THEN NULL") {
					t.Errorf("expected duplicate_column exception, got: %s", got[0])
				}
			},
		},
		{
			"CREATE INDEX replaced with IF NOT EXISTS",
			[]string{`CREATE INDEX "idx_email" ON users USING btree ("email");`},
			func(t *testing.T, got []string) {
				if len(got) != 1 {
					t.Fatalf("expected 1 statement, got %d", len(got))
				}
				if !strings.Contains(got[0], "CREATE INDEX IF NOT EXISTS") {
					t.Errorf("expected IF NOT EXISTS, got: %s", got[0])
				}
			},
		},
		{
			"ADD CONSTRAINT wrapped in DO block",
			[]string{`ALTER TABLE users ADD CONSTRAINT "users_email_key" UNIQUE ("email");`},
			func(t *testing.T, got []string) {
				if len(got) != 1 {
					t.Fatalf("expected 1 statement, got %d", len(got))
				}
				if !strings.HasPrefix(got[0], "DO $$ BEGIN") {
					t.Errorf("expected DO block, got: %s", got[0])
				}
				if !strings.Contains(got[0], "EXCEPTION WHEN duplicate_object THEN NULL") {
					t.Errorf("expected duplicate_object exception, got: %s", got[0])
				}
			},
		},
		{
			"unmatched statement passed through",
			[]string{`DROP TABLE IF EXISTS users;`},
			func(t *testing.T, got []string) {
				if len(got) != 1 {
					t.Fatalf("expected 1 statement, got %d", len(got))
				}
				if got[0] != `DROP TABLE IF EXISTS users;` {
					t.Errorf("expected passthrough, got: %s", got[0])
				}
			},
		},
		{
			"multiple statements wrapped individually",
			[]string{
				`ALTER TABLE users ADD COLUMN "email" varchar(255);`,
				`CREATE INDEX "idx_email" ON users USING btree ("email");`,
			},
			func(t *testing.T, got []string) {
				if len(got) != 2 {
					t.Fatalf("expected 2 statements, got %d", len(got))
				}
				if !strings.HasPrefix(got[0], "DO $$ BEGIN") {
					t.Errorf("first statement should be DO block, got: %s", got[0])
				}
				if !strings.Contains(got[1], "CREATE INDEX IF NOT EXISTS") {
					t.Errorf("second statement should have IF NOT EXISTS, got: %s", got[1])
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.WrapIdempotent(tt.input)
			tt.check(t, got)
		})
	}
}

func TestSetTableComment(t *testing.T) {
	p := New()

	got := p.SetTableComment("users", "User accounts")
	want := "COMMENT ON TABLE users IS 'User accounts';"
	if got != want {
		t.Errorf("SetTableComment() = %q, want %q", got, want)
	}
}

func TestSetColumnComment(t *testing.T) {
	p := New()

	got := p.SetColumnComment("users", "email", "User email address")
	want := `COMMENT ON COLUMN users."email" IS 'User email address';`
	if got != want {
		t.Errorf("SetColumnComment() = %q, want %q", got, want)
	}
}

func TestBeginCommitRollback(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Begin", p.Begin(), "BEGIN;"},
		{"Commit", p.Commit(), "COMMIT;"},
		{"Rollback", p.Rollback(), "ROLLBACK;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestName(t *testing.T) {
	p := New()
	if got := p.Name(); got != "postgres" {
		t.Errorf("Name() = %q, want %q", got, "postgres")
	}
}

func TestAutoIncrement(t *testing.T) {
	p := New()
	if got := p.AutoIncrement(); got != "" {
		t.Errorf("AutoIncrement() = %q, want empty string", got)
	}
}

func TestRenameColumn(t *testing.T) {
	p := New()

	got := p.RenameColumn("users", "old_name", "new_name")
	want := `ALTER TABLE users RENAME COLUMN "old_name" TO "new_name";`
	if got != want {
		t.Errorf("RenameColumn() = %q, want %q", got, want)
	}
}

func TestEnumSQL(t *testing.T) {
	p := New()

	enum := &schema.Enum{Name: "status"}
	col := schema.Column{SQLType: "status"}

	got := p.EnumSQL(enum, col)
	want := "status"
	if got != want {
		t.Errorf("EnumSQL() = %q, want %q", got, want)
	}
}

func TestUpsertSuffix(t *testing.T) {
	p := New()

	got := p.UpsertSuffix([]string{"id"}, []string{"name", "email"})
	want := `ON CONFLICT ("id") DO UPDATE SET "name" = EXCLUDED."name", "email" = EXCLUDED."email"`
	if got != want {
		t.Errorf("UpsertSuffix() = %q, want %q", got, want)
	}
}

func TestCreateTableWithTableType(t *testing.T) {
	p := New()

	table := schema.Table{
		Name: "temp_data",
		Type: schema.TableTypeView,
		Columns: []*schema.Column{
			{Name: "id", SQLType: "integer", Nullable: false},
		},
	}
	stmts := p.CreateTable(table)
	if !strings.Contains(stmts[0], " VIEW") {
		t.Errorf("expected VIEW table type: %s", stmts[0])
	}
}

func TestCreateTableWithOptions(t *testing.T) {
	p := New()

	table := schema.Table{
		Name:    "t",
		Options: "WITH (autovacuum_enabled = false)",
		Columns: []*schema.Column{
			{Name: "id", SQLType: "integer", Nullable: false},
		},
	}
	stmts := p.CreateTable(table)
	if !strings.Contains(stmts[0], "WITH (autovacuum_enabled = false)") {
		t.Errorf("expected options: %s", stmts[0])
	}
}

func TestCreateIndexWithCompositeColumns(t *testing.T) {
	p := New()

	idx := schema.Index{
		Name:    "idx_composite",
		Table:   "users",
		Columns: []string{"last_name", "first_name"},
		Kind:    "",
	}
	stmts := p.CreateIndex(idx)
	want := `CREATE INDEX "idx_composite" ON users ("last_name", "first_name");`
	if stmts[0] != want {
		t.Errorf("CreateIndex() = %q, want %q", stmts[0], want)
	}
}

func TestCreateTable_NoInlineUnique(t *testing.T) {
	p := New()
	tbl := schema.Table{
		StructName: "User",
		Name:       "users",
		Columns: []*schema.Column{
			{Name: "id", SQLType: "bigserial", PrimaryKey: true, OrdinalPosition: 0},
			{Name: "email", SQLType: "varchar(255)", Nullable: false, Unique: true, OrdinalPosition: 1},
		},
		Uniques: []schema.UniqueConstraint{
			{Name: "uq_users_email", Columns: []string{"email"}},
		},
	}

	stmts := p.CreateTable(tbl)
	createSQL := strings.Join(stmts, "\n")

	if strings.Contains(createSQL, `"email" varchar(255) NOT NULL UNIQUE`) {
		t.Error("column def should not contain inline UNIQUE; uniqueness is handled by table-level constraint")
	}
	if !strings.Contains(createSQL, `CONSTRAINT "uq_users_email" UNIQUE ("email")`) {
		t.Error("should have table-level UNIQUE constraint")
	}
	count := strings.Count(createSQL, "UNIQUE")
	if count != 1 {
		t.Errorf("expected 1 UNIQUE clause, got %d in:\n%s", count, createSQL)
	}
}

func TestSetTableComment_EscapesQuotes(t *testing.T) {
	p := New()
	got := p.SetTableComment("public.users", "user's account table")
	want := `COMMENT ON TABLE public.users IS 'user''s account table';`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestSetColumnComment_EscapesQuotes(t *testing.T) {
	p := New()
	got := p.SetColumnComment("public.users", "name", "the user's name")
	want := `COMMENT ON COLUMN public.users."name" IS 'the user''s name';`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestCreateEnumSQL_EscapesQuotes(t *testing.T) {
	p := New()
	en := &schema.Enum{
		Name:   "status",
		Values: []string{"active", "it's_complicated"},
	}
	got := p.CreateEnumSQL(en)
	want := `CREATE TYPE status AS ENUM ('active', 'it''s_complicated');`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestAlterEnumAddValue_EscapesQuotes(t *testing.T) {
	p := New()
	en := &schema.Enum{Name: "status"}
	got := p.AlterEnumAddValue(en, "don't_stop")
	want := `ALTER TYPE status ADD VALUE IF NOT EXISTS 'don''t_stop';`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestCreatePolicy_EmptyRolesOmitsToClause(t *testing.T) {
	p := New()
	pol := schema.Policy{Name: "p", Table: "users", Command: "SELECT", Using: "true"}
	got := p.CreatePolicy(pol)
	// No TO clause when roles is empty; WITH CHECK falls back to USING.
	if strings.Contains(got[0], "TO ") {
		t.Errorf("expected no TO clause with empty roles, got %q", got[0])
	}
	if !strings.Contains(got[0], "USING (true)") {
		t.Errorf("expected USING clause, got %q", got[0])
	}
}

func TestCreatePolicy_QuotesRoles(t *testing.T) {
	p := New()
	pol := schema.Policy{Name: "p", Table: "users", Command: "ALL", Roles: []string{"app role", "readonly"}}
	got := p.CreatePolicy(pol)
	if !strings.Contains(got[0], `TO "app role", "readonly"`) {
		t.Errorf("expected quoted roles, got %q", got[0])
	}
}

func TestGrantSQL_EmptyRolesOrPrivilegesIsSafe(t *testing.T) {
	p := New()
	if s := p.GrantSQL(schema.Grant{ObjectType: "table", ObjectName: "users"}); !strings.HasPrefix(s, "--") {
		t.Errorf("expected safe comment for empty grant, got %q", s)
	}
	if s := p.GrantSQL(schema.Grant{ObjectType: "table", ObjectName: "users", Privileges: []string{"SELECT"}}); !strings.HasPrefix(s, "--") {
		t.Errorf("expected safe comment for missing roles, got %q", s)
	}
}

func TestGrantSQL_InvalidObjectTypeIsSafe(t *testing.T) {
	p := New()
	s := p.GrantSQL(schema.Grant{ObjectType: "table; DROP", ObjectName: "users", Privileges: []string{"SELECT"}, Roles: []string{"r"}})
	if !strings.HasPrefix(s, "--") {
		t.Errorf("expected safe comment for invalid object type, got %q", s)
	}
}

func TestGrantSQL_QuotesRoles(t *testing.T) {
	p := New()
	s := p.GrantSQL(schema.Grant{ObjectType: "table", ObjectName: "users", Privileges: []string{"SELECT"}, Roles: []string{"app role"}})
	if !strings.Contains(s, `TO "app role";`) {
		t.Errorf("expected quoted role, got %q", s)
	}
}

func TestCreateTrigger_EmptyEventsIsSafe(t *testing.T) {
	p := New()
	got := p.CreateTrigger(schema.Trigger{Name: "t", Table: "users", Timing: "BEFORE", Function: "fn"})
	if !strings.HasPrefix(got[0], "--") {
		t.Errorf("expected safe comment for empty events, got %q", got[0])
	}
}

func TestConvertDefaultForType_EscapesSingleQuotes(t *testing.T) {
	p := New()
	got := p.ConvertDefaultForType("O'Brien", "varchar(255)")
	want := "'O''Brien'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestQuoteIdentsWithSort_ValidatesDirection(t *testing.T) {
	p := New()
	// "BOGUS" is not a valid direction -> treated as no sort.
	got := p.quoteIdentsWithSort([]string{"a", "b"}, "BOGUS")
	if strings.Contains(got, "BOGUS") {
		t.Errorf("invalid sort direction must be ignored, got %q", got)
	}
	// DESC must be preserved.
	got2 := p.quoteIdentsWithSort([]string{"a"}, "DESC")
	if !strings.Contains(got2, "DESC") {
		t.Errorf("valid DESC direction must be preserved, got %q", got2)
	}
}

func TestCreateFunction_QuotesTargetSchema(t *testing.T) {
	p := New()
	fn := schema.Function{SearchPath: "auth", Name: "hash_pw", Language: "sql", Body: "SELECT 1"}
	got := p.CreateFunction(fn)
	if !strings.Contains(got[0], `CREATE OR REPLACE FUNCTION "auth.hash_pw"`) {
		t.Errorf("function must be created in its schema, got %q", got[0])
	}
}

func TestCreateFunction_DollarTagAvoidsBodyCollision(t *testing.T) {
	p := New()
	// Body contains the literal "$function$" which would prematurely close a
	// naive dollar quote. The chosen tag must be distinct from the body's
	// content so the body is not misinterpreted as a closing tag.
	fn := schema.Function{Name: "f", Language: "sql", Body: "SELECT '$function$'"}
	got := p.CreateFunction(fn)[0]
	if !strings.Contains(got, "$func_") {
		t.Errorf("expected a generated dollar tag prefix, got %q", got)
	}
	if !strings.Contains(got, "SELECT '$function$'") {
		t.Errorf("body content should be preserved intact, got %q", got)
	}
}

func TestCreateExtension(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		ext  schema.Extension
		want string
	}{
		{"simple", schema.Extension{Name: "uuid-ossp"}, `CREATE EXTENSION "uuid-ossp";`},
		{"if not exists", schema.Extension{Name: "pgcrypto", IfNotExists: true}, `CREATE EXTENSION IF NOT EXISTS "pgcrypto";`},
		{"with schema", schema.Extension{Name: "hstore", Schema: "extensions"}, `CREATE EXTENSION "hstore" SCHEMA "extensions";`},
		{"with version", schema.Extension{Name: "postgis", Version: "3.0"}, `CREATE EXTENSION "postgis" VERSION '3.0';`},
		{"schema + if not exists", schema.Extension{Name: "hstore", Schema: "extensions", IfNotExists: true}, `CREATE EXTENSION IF NOT EXISTS "hstore" SCHEMA "extensions";`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.CreateExtension(tt.ext); got != tt.want {
				t.Errorf("CreateExtension() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDropExtension(t *testing.T) {
	p := New()

	tests := []struct {
		name string
		ext  schema.Extension
		want string
	}{
		{"simple", schema.Extension{Name: "uuid-ossp"}, `DROP EXTENSION "uuid-ossp";`},
		{"if exists", schema.Extension{Name: "pgcrypto", IfNotExists: true}, `DROP EXTENSION IF EXISTS "pgcrypto";`},
		{"cascade", schema.Extension{Name: "postgis", Cascade: true}, `DROP EXTENSION "postgis" CASCADE;`},
		{"if exists + cascade", schema.Extension{Name: "postgis", IfNotExists: true, Cascade: true}, `DROP EXTENSION IF EXISTS "postgis" CASCADE;`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.DropExtension(tt.ext); got != tt.want {
				t.Errorf("DropExtension() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAlterExtensionVersion(t *testing.T) {
	p := New()

	tests := []struct {
		name    string
		extName string
		version string
		want    string
	}{
		{"simple", "postgis", "3.1", `ALTER EXTENSION "postgis" UPDATE TO '3.1';`},
		{"schema-qualified", "ext.postgis", "3.1", `ALTER EXTENSION "ext.postgis" UPDATE TO '3.1';`},
		{"version with special chars", "myext", "1.0_beta", `ALTER EXTENSION "myext" UPDATE TO '1.0_beta';`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.AlterExtensionVersion(tt.extName, tt.version)
			if got != tt.want {
				t.Errorf("AlterExtensionVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
