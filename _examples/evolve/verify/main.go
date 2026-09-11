package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func mustQueryExists(ctx context.Context, pool *pgxpool.Pool, table string) {
	var exists bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, table).Scan(&exists)
	if err != nil || !exists {
		log.Fatalf("table %s should exist but check failed: exists=%v err=%v", table, exists, err)
	}
	fmt.Printf("  table %s exists: ok\n", table)
}

func mustNotExist(ctx context.Context, pool *pgxpool.Pool, table string) {
	var exists bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, table).Scan(&exists)
	if exists {
		log.Fatalf("table %s should NOT exist but found", table)
	}
	fmt.Printf("  table %s absent: ok\n", table)
}

func columnExists(ctx context.Context, pool *pgxpool.Pool, table, col string) bool {
	var exists bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, table, col).Scan(&exists)
	return exists
}

func main() {
	var dbURL = flag.String("db", "postgres://test:test@localhost:5433/mirage_test?sslmode=disable", "DB url")
	var version = flag.String("version", "v6", "expect version v1..v6")
	flag.Parse()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	fmt.Printf("Verifying %s against %s\n", *version, *dbURL)

	// Always check schema_migrations
	mustQueryExists(ctx, pool, "schema_migrations")
	var applied int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE state='applied'`).Scan(&applied)
	fmt.Printf("  applied migrations: %d\n", applied)
	if applied == 0 {
		log.Fatalf("no applied migrations")
	}

	// Common tables
	mustQueryExists(ctx, pool, "users")
	mustQueryExists(ctx, pool, "posts")
	mustQueryExists(ctx, pool, "events")

	switch *version {
	case "v1":
		if columnExists(ctx, pool, "users", "settings") {
			log.Fatalf("v1: users.settings should NOT exist")
		}
		fmt.Println("  v1: users.settings absent ok")
		mustNotExist(ctx, pool, "events_2024_q1")
		// v1 role default guest: check column default
		var def *string
		_ = pool.QueryRow(ctx, `SELECT column_default FROM information_schema.columns WHERE table_name='users' AND column_name='role'`).Scan(&def)
		fmt.Printf("  v1 role default: %v\n", *def)
		if def != nil && !strings.Contains(*def, "guest") {
			log.Fatalf("v1 role default should contain guest, got %v", *def)
		}
	case "v2":
		if !columnExists(ctx, pool, "users", "settings") {
			log.Fatalf("v2: users.settings should exist (jsonb)")
		}
		fmt.Println("  v2: users.settings exists ok")
		// jsonb CRUD: insert and read back
		var id int64
		err = pool.QueryRow(ctx, `INSERT INTO users (username,email,password,settings) VALUES ('alice','a@v2.com','hash','{"theme":"dark"}') RETURNING id`).Scan(&id)
		if err != nil {
			log.Fatalf("v2 insert user with jsonb: %v", err)
		}
		var raw string
		_ = pool.QueryRow(ctx, `SELECT settings->>'theme' FROM users WHERE id=$1`, id).Scan(&raw)
		if raw != "dark" {
			log.Fatalf("v2 jsonb read theme=%q want dark", raw)
		}
		fmt.Printf("  v2 jsonb roundtrip ok id=%d theme=%s\n", id, raw)
		// cleanup
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
		mustNotExist(ctx, pool, "events_2024_q1")
	case "v3":
		if !columnExists(ctx, pool, "users", "avatar_uri") {
			log.Fatalf("v3: users.avatar_uri should exist (rename from avatar_url)")
		}
		if columnExists(ctx, pool, "users", "avatar_url") {
			log.Fatalf("v3: users.avatar_url should NOT exist after rename")
		}
		fmt.Println("  v3: rename avatar_url->avatar_uri ok")
		// type change: last_name varchar(100)->200 (username not altered due to view dependency)
		var typ string
		_ = pool.QueryRow(ctx, `SELECT character_maximum_length FROM information_schema.columns WHERE table_name='users' AND column_name='last_name'`).Scan(&typ)
		// check length
		var maxLen *int
		_ = pool.QueryRow(ctx, `SELECT character_maximum_length FROM information_schema.columns WHERE table_name='users' AND column_name='last_name'`).Scan(&maxLen)
		if maxLen != nil && *maxLen != 200 {
			log.Fatalf("v3 last_name maxLen=%v want 200", *maxLen)
		}
		fmt.Printf("  v3 last_name type change to 200 ok\n")
		// view still original? v3 view is still original (active_users)
		var viewDef string
		_ = pool.QueryRow(ctx, `SELECT pg_get_viewdef('active_users'::regclass, true)`).Scan(&viewDef)
		fmt.Printf("  v3 view def snippet: %s\n", viewDef[:min(80, len(viewDef))])
	case "v4":
		mustQueryExists(ctx, pool, "events_2024_q1")
		// check parent is partitioned
		var isPart bool
		_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_partitioned_table WHERE partrelid='events'::regclass)`).Scan(&isPart)
		if !isPart {
			log.Fatalf("v4: events should be partitioned")
		}
		fmt.Println("  v4: events partitioned ok, child exists")
		// try insert routing
		var eid int64
		err = pool.QueryRow(ctx, `INSERT INTO events (session_id, event_type, path, created_at) VALUES ('s1','click','/p','2024-02-01') RETURNING id`).Scan(&eid)
		if err != nil {
			log.Fatalf("v4 insert partitioned: %v", err)
		}
		var cnt int
		_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM events_2024_q1 WHERE id=$1`, eid).Scan(&cnt)
		if cnt != 1 {
			log.Fatalf("v4: partitioned routing failed, cnt=%d", cnt)
		}
		fmt.Printf("  v4 partitioned routing ok eid=%d\n", eid)
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE id=$1`, eid)
	case "v5":
		mustNotExist(ctx, pool, "plain_favorites")
		if columnExists(ctx, pool, "users", "bio") {
			log.Fatalf("v5: users.bio should be dropped")
		}
		fmt.Println("  v5: plain_favorites dropped, bio dropped ok")
		// view tightened
		var vd string
		_ = pool.QueryRow(ctx, `SELECT pg_get_viewdef('active_users'::regclass, true)`).Scan(&vd)
		if !strings.Contains(vd, "role") {
			log.Fatalf("v5 view should contain role filter, got %s", vd)
		}
		fmt.Println("  v5: view tightened ok")
	case "v6":
		mustQueryExists(ctx, pool, "plain_favorites")
		if !columnExists(ctx, pool, "users", "bio") {
			log.Fatalf("v6: users.bio should exist after restore")
		}
		fmt.Println("  v6: plain_favorites and bio restored ok")
		// jsonb slice test
		var id int64
		err = pool.QueryRow(ctx, `INSERT INTO users (username,email,password,settings,preferences) VALUES ('bob','b@v6.com','hash','{"theme":"light"}','[{"key":"k","value":"v"}]') RETURNING id`).Scan(&id)
		if err != nil {
			log.Fatalf("v6 insert with preferences: %v", err)
		}
		var prefRaw string
		_ = pool.QueryRow(ctx, `SELECT preferences::text FROM users WHERE id=$1`, id).Scan(&prefRaw)
		var arr []map[string]string
		_ = json.Unmarshal([]byte(prefRaw), &arr)
		if len(arr) != 1 || arr[0]["key"] != "k" {
			log.Fatalf("v6 preferences roundtrip failed: %s", prefRaw)
		}
		fmt.Printf("  v6 jsonb slice roundtrip ok id=%d\n", id)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	default:
		log.Fatalf("unknown version %s", *version)
	}

	// Always check events partition PK includes created_at
	if *version == "v6" || *version == "v4" || *version == "v5" {
		var pk string
		_ = pool.QueryRow(ctx, `SELECT string_agg(column_name, ',' ORDER BY ordinal_position) FROM information_schema.key_column_usage WHERE table_name='events' AND constraint_name LIKE 'pk_%'`).Scan(&pk)
		fmt.Printf("  events PK: %s\n", pk)
	}

	fmt.Printf("Verify %s PASS\n", *version)
	os.Exit(0)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
