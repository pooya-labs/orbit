$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $root

$envFile = Join-Path $root ".env.local-work"
if (-not (Test-Path $envFile)) {
  throw "Local credentials are missing. Start Local Work first with scripts/start-local-work.ps1."
}

$values = @{}
Get-Content $envFile | ForEach-Object {
  if ($_ -match '^\s*([^#=]+)=(.*)$') { $values[$matches[1]] = $matches[2] }
}
$humanToken = $values["ORBIT_LOCAL_HUMAN_TOKEN"]
if ([string]::IsNullOrWhiteSpace($humanToken)) {
  throw ".env.local-work does not contain ORBIT_LOCAL_HUMAN_TOKEN."
}

Write-Host "[1/4] Targeted Go tests and PostgreSQL bootstrap regression..."
docker compose --env-file $envFile -f docker-compose.yml -f docker-compose.local-work.yml --profile local-work-test run --rm local-work-test
if ($LASTEXITCODE -ne 0) { throw "Targeted Local Work Go tests failed." }

Write-Host "[2/4] Web TypeScript typecheck..."
npm run typecheck
if ($LASTEXITCODE -ne 0) { throw "Web typecheck failed." }

Write-Host "[3/4] API readiness..."
$ready = Invoke-RestMethod -Method Get -Uri "http://localhost:8080/health/ready"
if ($ready.status -ne "ready") { throw "ORBIT API did not report ready." }

Write-Host "[4/4] Authenticated workspace data smoke..."
$headers = @{ Authorization = "Bearer $humanToken" }
$workspaces = (Invoke-RestMethod -Headers $headers -Uri "http://localhost:8080/api/v1/workspaces").data
$workspace = $workspaces | Where-Object { $_.slug -eq "orbit-local-work" } | Select-Object -First 1
if (-not $workspace) { throw "The authenticated local owner cannot see orbit-local-work." }

$base = "http://localhost:8080/api/v1/workspaces/$($workspace.id)"
$services = (Invoke-RestMethod -Headers $headers -Uri "$base/services").data
$incidents = (Invoke-RestMethod -Headers $headers -Uri "$base/incidents").data
if ($services.Count -lt 6) { throw "Expected at least six workspace services; received $($services.Count)." }
if ($incidents.Count -ne 2) { throw "Expected two seeded workspace incidents; received $($incidents.Count)." }

Write-Host "Local Work checks passed: $($services.Count) services, $($incidents.Count) incidents, authenticated owner access."
