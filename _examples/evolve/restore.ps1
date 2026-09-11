param([string]$DB = "postgres://test:test@localhost:5433/mirage_test?sslmode=disable")
$ErrorActionPreference="Stop"
$ExamplesRoot = (Resolve-Path "$PSScriptRoot\..").Path
$ModelsRoot = Join-Path $ExamplesRoot "models"
$Backup = Join-Path $PSScriptRoot "backup_models"
$VersionsRoot = Join-Path $PSScriptRoot "versions"
$MigrationsDir = Join-Path $ExamplesRoot "migrations"

if (-not (Test-Path $Backup)) {
  Write-Host "No backup found, restoring from v6 snapshot"
  $Backup = Join-Path $VersionsRoot "v6"
  Copy-Item -Recurse -Force "$Backup\models\*" $ModelsRoot -ErrorAction SilentlyContinue
  if (-not (Test-Path "$ModelsRoot\auth")) { Copy-Item -Recurse -Force "$Backup\*" $ModelsRoot }
} else {
  Get-ChildItem $ModelsRoot | Remove-Item -Recurse -Force
  Copy-Item -Recurse -Force "$Backup\*" $ModelsRoot
  Write-Host "Restored live models from backup"
}

Write-Host "Cleaning evolve migrations (keeping last canonical)"
Get-ChildItem "$MigrationsDir\*.sql" | Where-Object { $_.Name -notlike "V20260830161040*" } | ForEach-Object { Remove-Item $_.FullName -Force; Write-Host " removed $($_.Name)" }

Write-Host "Restored. Run: mirage generate --source ./models --recursive --db `"$DB`" --migrations-dir ./migrations --force"
