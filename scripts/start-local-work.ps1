$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $root

function Require-Command([string]$Name) {
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "Required command '$Name' was not found. Install Docker Desktop, Node.js 22+, and npm."
  }
}

Require-Command "docker"
Require-Command "node"
Require-Command "npm"

$nodeVersion = (& node --version).Trim()
if ($LASTEXITCODE -ne 0 -or $nodeVersion -notmatch '^v?(\d+)\.') {
  throw "Could not determine the Node.js version. Install Node.js 22+ and reopen PowerShell."
}
$nodeMajor = [int]$Matches[1]
if ($nodeMajor -lt 22) {
  throw "Node.js 22+ is required; detected $nodeVersion. Upgrade Node.js and rerun this script."
}

$dockerInfo = docker info 2>&1
if ($LASTEXITCODE -ne 0) {
  throw "Docker Desktop is not running. Start Docker Desktop and rerun this script."
}
& docker compose version *> $null
if ($LASTEXITCODE -ne 0) {
  throw "Docker Compose is unavailable. Update Docker Desktop or install the Docker Compose plugin."
}

$envFile = Join-Path $root ".env.local-work"
if (-not (Test-Path -LiteralPath $envFile)) {
  $humanBytes = New-Object byte[] 36
  $workloadBytes = New-Object byte[] 36
  $random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
  try {
    $random.GetBytes($humanBytes)
    $random.GetBytes($workloadBytes)
  } finally {
    $random.Dispose()
  }
  $humanToken = [Convert]::ToBase64String($humanBytes)
  $workloadToken = [Convert]::ToBase64String($workloadBytes)
  @(
    "ORBIT_LOCAL_HUMAN_TOKEN=$humanToken"
    "ORBIT_LOCAL_WORKLOAD_TOKEN=$workloadToken"
  ) | Set-Content -LiteralPath $envFile -Encoding ascii
  Write-Host "Created private local credentials in .env.local-work."
  Write-Host "Do not share this file. It is excluded from Git."
}

$localValues = @{}
Get-Content -LiteralPath $envFile | ForEach-Object {
  if ($_ -match '^\s*([^#=]+)=(.*)$') {
    $localValues[$matches[1].Trim()] = $matches[2]
  }
}
$humanToken = [string]$localValues["ORBIT_LOCAL_HUMAN_TOKEN"]
$workloadToken = [string]$localValues["ORBIT_LOCAL_WORKLOAD_TOKEN"]
if ($humanToken.Length -lt 32 -or $workloadToken.Length -lt 32) {
  throw ".env.local-work must contain two distinct credentials of at least 32 characters. Back up the file, repair both ORBIT_LOCAL_*_TOKEN entries, and rerun this script."
}
if ($humanToken -eq $workloadToken) {
  throw "The local human and workload credentials must be different. Replace one token in .env.local-work and rerun this script."
}

$composeArgs = @(
  "--env-file", $envFile,
  "-f", "docker-compose.yml",
  "-f", "docker-compose.local-work.yml",
  "--profile", "observability"
)
& docker compose @composeArgs config --quiet
if ($LASTEXITCODE -ne 0) {
  throw "Local Work Docker Compose configuration is invalid. Check .env.local-work and the Compose files."
}

Write-Host "Checking the locked web dependencies before starting Docker services..."
& npm ls --depth=0 *> $null
if ($LASTEXITCODE -ne 0) {
  Write-Host "Installing web dependencies from the committed lockfile..."
  & npm ci --no-audit --no-fund
  if ($LASTEXITCODE -ne 0) { throw "npm ci failed. Docker services have not been started." }
}

Write-Host "Starting PostgreSQL, ORBIT API, Collector, Prometheus, Loki, Tempo, and the authenticated telemetry demo..."
& docker compose @composeArgs up -d --build postgres api prometheus loki tempo otel-collector telemetry-demo
if ($LASTEXITCODE -ne 0) { throw "Docker Compose failed to start the Local Work services." }

Write-Host "Waiting for the local API to become ready..."
$ready = $false
$readinessDeadline = (Get-Date).AddSeconds(120)
while ((Get-Date) -lt $readinessDeadline) {
  try {
    $health = Invoke-RestMethod -Method Get -Uri "http://127.0.0.1:8080/health/ready" -TimeoutSec 2
    if ($health.status -eq "ready") {
      $ready = $true
      break
    }
  } catch {
    # The API may still be migrating PostgreSQL or bootstrapping the local owner.
  }
  Start-Sleep -Seconds 2
}
if (-not $ready) {
  Write-Host "Local Work did not become ready. Recent container status and logs:"
  & docker compose @composeArgs ps --all
  & docker compose @composeArgs logs --no-color --tail=100 api postgres otel-collector prometheus loki tempo
  throw "The API did not become ready within 120 seconds. Review the diagnostics above, fix the reported issue, and rerun the launcher."
}

$env:NEXT_PUBLIC_ORBIT_LOCAL_WORK = "true"
$env:NEXT_PUBLIC_ORBIT_API_URL = "http://localhost:8080"
$env:NEXT_PUBLIC_ORBIT_BASE_PATH = ""

Write-Host ""
Write-Host "ORBIT Local Work is starting at http://localhost:3000"
Write-Host "API: http://localhost:8080/health/ready"
Write-Host "Sign in with ORBIT_LOCAL_HUMAN_TOKEN from .env.local-work."
Write-Host "Telemetry workload credentials remain inside the private Docker network."
Write-Host "Press Ctrl+C to stop the web UI; run the documented compose down command to stop the backend."
Write-Host ""

npm run dev -- --hostname 127.0.0.1 --port 3000
