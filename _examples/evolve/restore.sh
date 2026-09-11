#!/usr/bin/env bash
set -euo pipefail
DB=${DB:-postgres://test:test@localhost:5433/mirage_test?sslmode=disable}
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXAMPLES_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MODELS_ROOT="$EXAMPLES_ROOT/models"
BACKUP="$SCRIPT_DIR/backup_models"
VERSIONS_ROOT="$SCRIPT_DIR/versions"
MIGRATIONS_DIR="$EXAMPLES_ROOT/migrations"

if [ ! -d "$BACKUP" ]; then
  echo "No backup, restoring from v6"
  cp -r "$VERSIONS_ROOT/v6/models"/* "$MODELS_ROOT/" 2>/dev/null || cp -r "$VERSIONS_ROOT/v6"/* "$MODELS_ROOT/"
else
  rm -rf "$MODELS_ROOT"/*
  cp -r "$BACKUP"/* "$MODELS_ROOT/"
  echo "Restored live models from backup"
fi
# keep only canonical migration
for f in "$MIGRATIONS_DIR"/V*.sql; do
  [[ -e "$f" ]] || continue
  if [[ "$f" != *"V20260830161040"* ]]; then echo " removing $f"; rm -f "$f"; fi
done
echo "Restored. Run: mirage generate --source ./models --recursive --db \"$DB\" --migrations-dir ./migrations --force"
