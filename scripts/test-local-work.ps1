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
if ([string]::IsNullOrWhiteSpace($humanToken) -or $humanToken.Length -lt 32) {
  throw ".env.local-work does not contain a valid ORBIT_LOCAL_HUMAN_TOKEN."
}

Write-Host "[1/5] Targeted Go tests and PostgreSQL bootstrap regression..."
docker compose --env-file $envFile -f docker-compose.yml -f docker-compose.local-work.yml --profile local-work-test run --rm local-work-test
if ($LASTEXITCODE -ne 0) { throw "Targeted Local Work Go tests failed." }

Write-Host "[2/5] Web TypeScript typecheck..."
npm run typecheck
if ($LASTEXITCODE -ne 0) { throw "Web typecheck failed." }

Write-Host "[3/5] API readiness..."
$ready = $false
for ($attempt = 0; $attempt -lt 60; $attempt++) {
  try {
    $response = Invoke-RestMethod -Method Get -Uri "http://127.0.0.1:8080/health/ready" -TimeoutSec 5
    if ($response.status -eq "ready") {
      $ready = $true
      break
    }
  } catch {
    Start-Sleep -Seconds 2
  }
}
if (-not $ready) { throw "ORBIT API did not become ready at http://127.0.0.1:8080/health/ready." }

Write-Host "[4/5] Authenticated workspace data smoke..."
$headers = @{ Authorization = "Bearer $humanToken" }
$workspaces = (Invoke-RestMethod -Headers $headers -Uri "http://127.0.0.1:8080/api/v1/workspaces").data
$workspace = $workspaces | Where-Object { $_.slug -eq "orbit-local-work" } | Select-Object -First 1
if (-not $workspace) { throw "The authenticated local owner cannot see orbit-local-work." }

$base = "http://127.0.0.1:8080/api/v1/workspaces/$($workspace.id)"
$services = (Invoke-RestMethod -Headers $headers -Uri "$base/services").data
$incidents = (Invoke-RestMethod -Headers $headers -Uri "$base/incidents").data
if (@($services).Count -lt 6) { throw "Expected at least six workspace services; received $(@($services).Count)." }
if (@($incidents).Count -ne 2) { throw "Expected two seeded workspace incidents; received $(@($incidents).Count)." }

Write-Host "[5/5] Authenticated OTLP write and workspace-scoped Metrics/Logs/Traces queries..."
$compose = @("--env-file", $envFile, "-f", "docker-compose.yml", "-f", "docker-compose.local-work.yml", "--profile", "observability")
for ($attempt = 0; $attempt -lt 3; $attempt++) {
  docker compose @compose exec -T telemetry-demo wget -qO- http://127.0.0.1:8081/emit | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "The telemetry demo could not submit a request through its authenticated OTLP gateway." }
  Start-Sleep -Seconds 2
}

$telemetryReady = $false
$lastTelemetryError = ""
$deadline = (Get-Date).AddSeconds(90)
while ((Get-Date) -lt $deadline) {
  try {
    $metrics = Invoke-RestMethod -Headers $headers -Uri "$base/telemetry/metrics?window=1h&service=orbit-telemetry-demo&metric=request_rate&group_by=none" -TimeoutSec 10
    $logs = Invoke-RestMethod -Headers $headers -Uri "$base/telemetry/logs?window=1h&service=orbit-telemetry-demo&limit=20" -TimeoutSec 10
    $traces = Invoke-RestMethod -Headers $headers -Uri "$base/telemetry/traces?window=1h&service=orbit-telemetry-demo&limit=20" -TimeoutSec 10
    if (@($metrics.data).Count -gt 0 -and @($logs.data).Count -gt 0 -and @($traces.data).Count -gt 0) {
      $telemetryReady = $true
      break
    }
  } catch {
    $lastTelemetryError = $_.Exception.Message
  }
  Start-Sleep -Seconds 3
}
if (-not $telemetryReady) {
  throw "Workspace-scoped telemetry did not become queryable. Last error: $lastTelemetryError"
}

Write-Host "Local Work checks passed: $(@($services).Count) services, $(@($incidents).Count) incidents; Metrics=$(@($metrics.data).Count), Logs=$(@($logs.data).Count), Traces=$(@($traces.data).Count)."
