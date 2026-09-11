# Evolve Harness — v1 → v6 incremental migration test

This harness proves `mirage` works **repeatedly**, not just on first `empty→full` generation.

## What it tests

| Version | DB change kind | Verifier checks |
|---|---|---|
| **v1** | Baseline `init` — 3 tables, `user_role` enum, `uuid-ossp`, partitioned `events` parent, `role` default `guest` | `users` no `settings`, no child partition |
| **v2** | **ADD** `jsonb` struct `Foo` (`settings` `UserSettings`, `preferences []UserPreferences`, `extra *UserSettings`) | `INSERT ... settings '{"theme":"dark"}'` roundtrip |
| **v3** | **ALTER** rename `avatar_url→avatar_uri`, type `varchar(100)→150/200`, `default 0→1` | `information_schema.columns` checks |
| **v4** | **PARTITION** child `events_2024_q1 PARTITION OF events` + routing insert → child table | `pg_partitioned_table`, `INSERT` routes |
| **v5** | **DROP** `plain_favorites` table + `users.bio` column, **REPLACE** view `active_users` tightened | `to_regclass` absent, view `pg_get_viewdef` contains `role` |
| **v6** | **RESTORE** equals live `models/` (14 tables) | `plain_favorites` + `bio` back, `preferences` `[]Foo` slice roundtrip |

Additional rollback checks: `rollback 1` (v6→v5), `rollback 2` (v5→v3), `migrate` back to v6.

## Layout

```
evolve/
  versions/v1…v6/models/...   # full snapshots (copy, not patch)
  verify/main.go              # pgx verifier per version
  evolve.ps1 / evolve.sh      # orchestrator (podman + mirage CLI)
  restore.ps1 / restore.sh    # restore live models + clean migrations
  artifacts/                  # archived migrations per run
  backup_models/              # auto-created on first evolve run
```

## Quick start (Windows, podman)

```powershell
# from _examples/
.\evolve\evolve.ps1            # builds mirage, starts postgres:5433, loops v1→v6, verifies, rollback, restore
# -DryRun to print without executing
# -SkipVerify to skip Go verifier
# -NoBuild to reuse bin\mirage.exe

# Manual
.\evolve\restore.ps1
podman compose -f docker-compose.yml down -v
```

Linux/macOS:
```bash
./evolve/evolve.sh
./evolve/restore.sh
```

## Manual steps (what evolve does)

```bash
podman compose -f docker-compose.yml up -d postgres  # healthcheck
go build -o ../bin/mirage ../cmd/mirage

# for ver in v1..v6:
cp -r evolve/versions/$ver/models/* models/
mirage generate --source ./models --recursive --db $DB --migrations-dir ./migrations -m "evolve $ver" --force
mirage migrate --db $DB --dir ./migrations
mirage status --db $DB --dir ./migrations --format json
go run ./evolve/verify --db $DB --version $ver

mirage rollback 1 --db $DB --dir ./migrations --force
mirage rollback 2 --db $DB --dir ./migrations --force
mirage migrate --db $DB --dir ./migrations
```

## Adding a new version

1. Copy `versions/v6` → `versions/v7`
2. Edit Go structs (add `type=jsonb`, `partition`, `check`, etc.)
3. Add case to `verify/main.go` `switch *version`
4. Extend `versions` array in `evolve.ps1` / `evolve.sh`

## Why full snapshots?

`mirage generate --source` scans a directory tree. Patches are fragile on Windows. Full copies are explicit and work with `Recursive: true` scanner (`internal/scanner/scanner.go:675`) and `validate` (`internal/validate/validate.go`).

