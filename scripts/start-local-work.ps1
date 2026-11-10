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

$dockerInfo = docker info 2>&1
if ($LASTEXITCODE -ne 0) {
  throw "Docker Desktop is not running. Start Docker Desktop and rerun this script."
}

$envFile = Join-Path $root ".env.local-work"
if (-not (Test-Path $envFile)) {
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
  ) | Set-Content -Path $envFile -Encoding ascii
  Write-Host "Created private local credentials in .env.local-work."
  Write-Host "Do not share this file. It is excluded from Git."
}

Write-Host "Starting PostgreSQL, ORBIT API, Collector, Prometheus, Loki, Tempo, and the authenticated telemetry demo..."
docker compose --env-file $envFile -f docker-compose.yml -f docker-compose.local-work.yml --profile observability up -d --build postgres api prometheus loki tempo otel-collector telemetry-demo
if ($LASTEXITCODE -ne 0) { throw "Docker Compose failed to start the Local Work services." }

$env:NEXT_PUBLIC_ORBIT_LOCAL_WORK = "true"
$env:NEXT_PUBLIC_ORBIT_API_URL = "http://localhost:8080"
$env:NEXT_PUBLIC_ORBIT_BASE_PATH = ""

if (-not (Test-Path (Join-Path $root "node_modules"))) {
  Write-Host "Installing web dependencies from the committed lockfile..."
  npm ci
  if ($LASTEXITCODE -ne 0) { throw "npm ci failed." }
}

Write-Host ""
Write-Host "ORBIT Local Work is starting at http://localhost:3000"
Write-Host "API: http://localhost:8080/health/ready"
Write-Host "Sign in with ORBIT_LOCAL_HUMAN_TOKEN from .env.local-work."
Write-Host "Telemetry workload credentials remain inside the private Docker network."
Write-Host "Press Ctrl+C to stop the web UI; run the documented compose down command to stop the backend."
Write-Host ""

npm run dev -- --hostname 127.0.0.1
