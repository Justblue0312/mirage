#!/usr/bin/env bash
set -euo pipefail
DB=${DB:-postgres://test:test@localhost:5433/mirage_test?sslmode=disable}
PORT=${PORT:-5433}
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXAMPLES_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$EXAMPLES_ROOT/.." && pwd)"
MODELS_ROOT="$EXAMPLES_ROOT/models"
MIGRATIONS_DIR="$EXAMPLES_ROOT/migrations"
VERSIONS_ROOT="$SCRIPT_DIR/versions"
VERIFY_DIR="$SCRIPT_DIR/verify"
MIRAGE_BIN="$REPO_ROOT/bin/mirage"

if [ ! -f "$MIRAGE_BIN" ]; then
  echo "=== Building mirage ==="
  go build -o "$MIRAGE_BIN" "$REPO_ROOT/cmd/mirage"
fi

podman --version

# Ensure port mapping
if ! grep -q "5433:5432" "$EXAMPLES_ROOT/docker-compose.yml"; then
  echo "Adding host port 5433:5432"
  # simple sed injection after postgres image
  sed -i.bak "s|postgres:16-alpine|postgres:16-alpine\n    ports:\n      - \"$PORT:5432\"|" "$EXAMPLES_ROOT/docker-compose.yml"
fi

echo "=== Starting postgres ==="
podman compose -f "$EXAMPLES_ROOT/docker-compose.yml" down -v || true
podman compose -f "$EXAMPLES_ROOT/docker-compose.yml" up -d postgres
echo "Waiting for postgres health..."
for i in $(seq 1 30); do
  h=$(podman inspect --format "{{.State.Health.Status}}" examples-postgres-1 2>/dev/null || echo "starting")
  if [ "$h" = "healthy" ]; then break; fi
  sleep 2
  echo " waiting... $i $h"
done
podman exec examples-postgres-1 psql -U test -d mirage_test -c "DO \$\$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='app_role') THEN CREATE ROLE app_role; END IF; END \$\$;" || true
podman exec examples-postgres-1 psql -U test -d mirage_test -c "CREATE SCHEMA IF NOT EXISTS auth; CREATE OR REPLACE FUNCTION auth.uid() RETURNS bigint AS \$\$ SELECT 0::bigint \$\$ LANGUAGE sql STABLE;" || true
echo "ensured role app_role and auth.uid() stub"

echo "=== Cleaning state ==="
podman exec examples-postgres-1 psql -U test -d mirage_test -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" || true
podman exec examples-postgres-1 psql -U test -d mirage_test -c "DROP SCHEMA IF EXISTS auth CASCADE; CREATE SCHEMA auth; CREATE OR REPLACE FUNCTION auth.uid() RETURNS bigint AS \$\$ SELECT 0::bigint \$\$ LANGUAGE sql STABLE;" || true
podman exec examples-postgres-1 psql -U test -d mirage_test -c "DO \$\$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='app_role') THEN CREATE ROLE app_role; END IF; END \$\$;" || true
mkdir -p "$SCRIPT_DIR/artifacts"
if ls "$MIGRATIONS_DIR"/*.sql 1>/dev/null 2>&1; then
  cp "$MIGRATIONS_DIR"/*.sql "$SCRIPT_DIR/artifacts/"
  rm -f "$MIGRATIONS_DIR"/*.sql
fi
BACKUP="$SCRIPT_DIR/backup_models"
rm -rf "$BACKUP"
cp -r "$MODELS_ROOT" "$BACKUP"

sync_version() {
  ver=$1
  echo "=== Syncing $ver ==="
  rm -rf "$MODELS_ROOT"/*
  if [ -d "$VERSIONS_ROOT/$ver/models" ]; then
    cp -r "$VERSIONS_ROOT/$ver/models"/* "$MODELS_ROOT/"
  elif [ -d "$VERSIONS_ROOT/$ver/auth" ]; then
    cp -r "$VERSIONS_ROOT/$ver"/* "$MODELS_ROOT/"
  else
    cp -r "$VERSIONS_ROOT/$ver"/* "$MODELS_ROOT/" 2>/dev/null || cp -r "$VERSIONS_ROOT/$ver/models"/* "$MODELS_ROOT/"
  fi
  # fallback
  if [ ! -d "$MODELS_ROOT/auth" ] && [ -d "$VERSIONS_ROOT/$ver" ]; then
    cp -r "$VERSIONS_ROOT/$ver"/* "$MODELS_ROOT/" || true
  fi
}

for ver in v1 v2 v3 v4 v5 v6; do
  sync_version $ver
  echo "=== Generate $ver ==="
  "$MIRAGE_BIN" generate --source "$MODELS_ROOT" --recursive --migrations-dir "$MIGRATIONS_DIR" -m "evolve $ver" --force --db "$DB"
  echo "=== Migrate $ver ==="
  "$MIRAGE_BIN" migrate --db "$DB" --dir "$MIGRATIONS_DIR"
  echo "=== Status $ver ==="
  "$MIRAGE_BIN" status --db "$DB" --dir "$MIGRATIONS_DIR" --format json
  echo "=== Verify $ver ==="
  GOWORK=off go run -C "$EXAMPLES_ROOT" ./evolve/verify --db "$DB" --version "$ver"
done

echo "=== Rollback 1 (v6->v5) ==="
"$MIRAGE_BIN" rollback 1 --db "$DB" --dir "$MIGRATIONS_DIR" --force
GOWORK=off go run -C "$EXAMPLES_ROOT" ./evolve/verify --db "$DB" --version v5

echo "=== Rollback 2 (v5->v3) ==="
"$MIRAGE_BIN" rollback 2 --db "$DB" --dir "$MIGRATIONS_DIR" --force
GOWORK=off go run -C "$EXAMPLES_ROOT" ./evolve/verify --db "$DB" --version v3

echo "=== Re-migrate to v6 ==="
"$MIRAGE_BIN" migrate --db "$DB" --dir "$MIGRATIONS_DIR"
GOWORK=off go run -C "$EXAMPLES_ROOT" ./evolve/verify --db "$DB" --version v6

echo "=== Restore live models ==="
rm -rf "$MODELS_ROOT"/*
cp -r "$BACKUP"/* "$MODELS_ROOT/"
echo "All evolve steps passed!"
echo "To clean: podman compose -f $EXAMPLES_ROOT/docker-compose.yml down -v"
