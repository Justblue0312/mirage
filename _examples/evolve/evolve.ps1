#Requires -Version 5.1
param(
  [string]$DB = "postgres://test:test@localhost:5433/mirage_test?sslmode=disable",
  [int]$Port = 5433,
  [switch]$NoBuild,
  [switch]$SkipVerify,
  [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Resolve-Path "$PSScriptRoot\..\..").Path
$ExamplesRoot = (Resolve-Path "$PSScriptRoot\..").Path
$ModelsRoot = Join-Path $ExamplesRoot "models"
$MigrationsDir = Join-Path $ExamplesRoot "migrations"
$VersionsRoot = Join-Path $PSScriptRoot "versions"
$VerifyDir = Join-Path $PSScriptRoot "verify"
$BinDir = Join-Path $RepoRoot "bin"
$MirageBin = Join-Path $BinDir "mirage.exe"
if (-not (Test-Path $BinDir)) { New-Item -ItemType Directory -Path $BinDir | Out-Null }

function Write-Step($msg) { Write-Host "`n=== $msg ===" -ForegroundColor Cyan }
function Write-Ok($msg) { Write-Host "OK $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "WARN $msg" -ForegroundColor Yellow }

if (-not $NoBuild) {
  Write-Step "Building mirage"
  if (-not $DryRun) {
    & go build -o $MirageBin (Join-Path $RepoRoot "cmd/mirage")
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
  }
}

Write-Step "Checking podman"
if (-not $DryRun) { podman --version | Out-Null }

$ComposeFile = Join-Path $ExamplesRoot "docker-compose.yml"
$content = Get-Content $ComposeFile -Raw
if ($content -notmatch "5433:5432") {
  Write-Warn "Adding host port mapping 5433:5432"
  $content = $content -replace "postgres:16-alpine", "postgres:16-alpine`n    ports:`n      - '5433:5432'"
  if ($content -notmatch "5433:5432") {
    $content = $content -replace "healthcheck:", "    ports:`n      - '5433:5432'`n    healthcheck:"
  }
  Set-Content -Path $ComposeFile -Value $content
}

Write-Step "Starting postgres via podman"
if (-not $DryRun) {
  $oldPref = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  & podman compose -f $ComposeFile down -v *> $null
  & podman compose -f $ComposeFile up -d postgres *> $null
  $ErrorActionPreference = $oldPref
  Write-Host "Waiting for postgres health..."
  $retry=0
  while ($retry -lt 30) {
    $ErrorActionPreference = "Continue"
    $health = podman inspect --format "{{.State.Health.Status}}" examples-postgres-1 2>$null
    $ErrorActionPreference = "Stop"
    if ($health -eq "healthy") { break }
    Start-Sleep 2
    $retry++
    Write-Host "  waiting... $retry"
  }
  if ($retry -ge 30) { throw "postgres not healthy" }
  Write-Ok "postgres healthy on localhost:$Port"
  # create role and auth schema stub required by GRANT/POLICY (mirrors internal/integration test)
  $ErrorActionPreference = "Continue"
  & podman exec examples-postgres-1 psql -U test -d mirage_test -c "DO `$`$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='app_role') THEN CREATE ROLE app_role; END IF; END `$`$;" *> $null
  & podman exec examples-postgres-1 psql -U test -d mirage_test -c "CREATE SCHEMA IF NOT EXISTS auth; CREATE OR REPLACE FUNCTION auth.uid() RETURNS bigint AS `$`$ SELECT 0::bigint `$`$ LANGUAGE sql STABLE;" *> $null
  $ErrorActionPreference = "Stop"
  Write-Ok "ensured role app_role and auth.uid() stub"
}

Write-Step "Cleaning state"
if ($DryRun) {
  Write-Host "DryRun: would clean DB and archive migrations" -ForegroundColor DarkGray
} else {
  $oldPref = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  & podman exec examples-postgres-1 psql -U test -d mirage_test -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" *> $null
  & podman exec examples-postgres-1 psql -U test -d mirage_test -c "DROP SCHEMA IF EXISTS auth CASCADE; CREATE SCHEMA auth; CREATE OR REPLACE FUNCTION auth.uid() RETURNS bigint AS `$`$ SELECT 0::bigint `$`$ LANGUAGE sql STABLE;" *> $null
  & podman exec examples-postgres-1 psql -U test -d mirage_test -c "DO `$`$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='app_role') THEN CREATE ROLE app_role; END IF; END `$`$;" *> $null
  $ErrorActionPreference = $oldPref
  $migs = Get-ChildItem "$MigrationsDir\*.sql" -ErrorAction SilentlyContinue
  if ($migs) {
    Write-Host "Archiving existing migrations to evolve\artifacts"
    $art = Join-Path $PSScriptRoot "artifacts"
    if (-not (Test-Path $art)) { New-Item -ItemType Directory -Path $art | Out-Null }
    Copy-Item "$MigrationsDir\*.sql" $art -Force
    Remove-Item "$MigrationsDir\*.sql" -Force
    Write-Ok "cleaned migrations"
  }
}

$BackupModels = Join-Path $PSScriptRoot "backup_models"
if (Test-Path $BackupModels) { Remove-Item $BackupModels -Recurse -Force }
Copy-Item -Recurse $ModelsRoot $BackupModels
Write-Ok "backed up live models to $BackupModels"

function Sync-Version($ver) {
  $src = Join-Path $VersionsRoot $ver
  if (-not (Test-Path $src)) { throw "version $ver not found at $src" }
  Write-Step "Syncing models $ver -> live models"
  Get-ChildItem $ModelsRoot | Remove-Item -Recurse -Force
  $srcModels = Join-Path $src "models"
  if (Test-Path $srcModels) {
    Copy-Item -Recurse -Force "$srcModels\*" $ModelsRoot
  } elseif (Test-Path (Join-Path $src "auth")) {
    Copy-Item -Recurse -Force "$src\*" $ModelsRoot
  } else {
    Copy-Item -Recurse -Force "$src\*" $ModelsRoot
  }
  Write-Ok "synced $ver"
}

$versions = @("v1","v2","v3","v4","v5","v6")
foreach ($ver in $versions) {
  Sync-Version $ver
  Write-Step "Generate $ver"
  if (-not $DryRun) { & $MirageBin generate --source $ModelsRoot --recursive --migrations-dir $MigrationsDir -m "evolve $ver" --force --db $DB }
  $migCount = (Get-ChildItem $MigrationsDir -Filter "*.sql" -ErrorAction SilentlyContinue | Measure-Object).Count
  Write-Ok "migrations count: $migCount"

  Write-Step "Migrate $ver"
  if (-not $DryRun) { & $MirageBin migrate --db $DB --dir $MigrationsDir }

  Write-Step "Status $ver"
  if (-not $DryRun) {
    & $MirageBin status --db $DB --dir $MigrationsDir --format json
    & $MirageBin status --db $DB --dir $MigrationsDir
  }

  if (-not $SkipVerify) {
    Write-Step "Verify $ver"
    if (-not $DryRun) {
      $env:GOWORK="off"
      & go run -C $ExamplesRoot ./evolve/verify --db $DB --version $ver
      if ($LASTEXITCODE -ne 0) { throw "verify $ver failed" }
    }
  }
  Write-Ok "version $ver applied and verified"
}

Write-Step "Rollback 1 (v6 -> v5)"
if (-not $DryRun) { & $MirageBin rollback 1 --db $DB --dir $MigrationsDir --force }
if (-not $SkipVerify -and -not $DryRun) { $env:GOWORK="off"; & go run -C $ExamplesRoot ./evolve/verify --db $DB --version v5 }

Write-Step "Rollback 2 more (v5 -> v3)"
if (-not $DryRun) { & $MirageBin rollback 2 --db $DB --dir $MigrationsDir --force }
if (-not $SkipVerify -and -not $DryRun) { $env:GOWORK="off"; & go run -C $ExamplesRoot ./evolve/verify --db $DB --version v3 }

Write-Step "Re-migrate to v6"
if (-not $DryRun) { & $MirageBin migrate --db $DB --dir $MigrationsDir }
if (-not $SkipVerify -and -not $DryRun) { $env:GOWORK="off"; & go run -C $ExamplesRoot ./evolve/verify --db $DB --version v6 }

Write-Step "Restore live models"
Get-ChildItem $ModelsRoot | Remove-Item -Recurse -Force
Copy-Item -Recurse -Force "$BackupModels\*" $ModelsRoot
Write-Ok "live models restored"

Write-Host "`nAll evolve steps passed!" -ForegroundColor Green
Write-Host "To clean podman: podman compose -f $ComposeFile down -v" -ForegroundColor DarkGray
