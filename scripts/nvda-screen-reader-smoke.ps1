$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$serverProcess = $null
$nvdaProcess = $null
$edgeProcess = $null
$script:winappPath = $null
$script:speechWindowHandle = $null

function Invoke-WinApp {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  $output = & $script:winappPath @Arguments 2>&1
  $exitCode = $LASTEXITCODE
  $text = ($output | Out-String).Trim()
  if ($exitCode -ne 0) {
    throw ("winapp " + ($Arguments -join " ") + " failed with exit code " + $exitCode + "." + [Environment]::NewLine + $text)
  }
  return $text
}

function Get-SpeechText {
  foreach ($controlType in @("Edit", "Document")) {
    try {
      $raw = Invoke-WinApp -Arguments @(
        "ui", "get-value", "", "-w", $script:speechWindowHandle,
        "--type", $controlType, "--json"
      )
      $result = $raw | ConvertFrom-Json
      if ($null -ne $result.text) {
        return [string]$result.text
      }
    } catch {
      # wx.TextCtrl is normally exposed as Edit; retry Document for provider variants.
    }
  }
  $tree = Invoke-WinApp -Arguments @("ui", "inspect", "-w", $script:speechWindowHandle)
  throw ("Could not read NVDA Speech Viewer text through UI Automation." + [Environment]::NewLine + $tree)
}

function Get-SpeechDelta([string]$Before, [string]$After) {
  if ($After.Length -ge $Before.Length) {
    return $After.Substring($Before.Length).Trim()
  }
  return $After.Trim()
}

function Wait-SpeechDelta {
  param(
    [Parameter(Mandatory = $true)][string]$Before,
    [Parameter(Mandatory = $true)][string]$ExpectedPattern,
    [int]$TimeoutSeconds = 10
  )

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $latest = ""
  $delta = ""
  do {
    Start-Sleep -Milliseconds 300
    $latest = Get-SpeechText
    $delta = Get-SpeechDelta -Before $Before -After $latest
    if ($delta -match $ExpectedPattern) {
      return $delta
    }
    if ($delta -match "(?i)No next heading") {
      return $delta
    }
  } while ((Get-Date) -lt $deadline)

  return $delta
}

try {
  Write-Host "=== Prepare static ORBIT site ==="
  $siteRoot = Join-Path $env:RUNNER_TEMP "orbit-nvda-site"
  $orbitRoot = Join-Path $siteRoot "orbit"
  New-Item -ItemType Directory -Path $orbitRoot -Force | Out-Null
  Copy-Item -Path (Join-Path $PWD "out\*") -Destination $orbitRoot -Recurse -Force

  $serverProcess = Start-Process -FilePath "python" -ArgumentList @(
    "-m", "http.server", "4173", "--directory", $siteRoot
  ) -PassThru -WindowStyle Hidden

  $siteUrl = "http://127.0.0.1:4173/orbit/"
  $serverReady = $false
  for ($attempt = 0; $attempt -lt 30; $attempt++) {
    try {
      $response = Invoke-WebRequest -Uri $siteUrl -TimeoutSec 3 -UseBasicParsing
      if ($response.StatusCode -eq 200) {
        $serverReady = $true
        break
      }
    } catch {
      Start-Sleep -Seconds 1
    }
  }
  if (-not $serverReady) {
    throw ("The local ORBIT static site did not become ready at " + $siteUrl)
  }
  Write-Host ("Static site ready: " + $siteUrl)

  Write-Host "=== Install NVDA and Windows UI Automation CLI ==="
  winget install --id NVAccess.NVDA --exact --silent --accept-package-agreements --accept-source-agreements --disable-interactivity
  if ($LASTEXITCODE -ne 0) {
    throw ("NVDA installation failed with exit code " + $LASTEXITCODE)
  }

  winget install --id Microsoft.WinAppCli --exact --silent --accept-package-agreements --accept-source-agreements --disable-interactivity
  if ($LASTEXITCODE -ne 0) {
    throw ("WinApp CLI installation failed with exit code " + $LASTEXITCODE)
  }

  $winappCommand = Get-Command "winapp.exe" -ErrorAction SilentlyContinue
  if ($null -eq $winappCommand) {
    $aliasPath = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\winapp.exe"
    if (Test-Path $aliasPath) {
      $script:winappPath = $aliasPath
    } else {
      throw "winapp.exe was not added to PATH after installing Microsoft.WinAppCli"
    }
  } else {
    $script:winappPath = $winappCommand.Source
  }
  Write-Host ("WinApp CLI: " + $script:winappPath)

  $programFilesX86 = [Environment]::GetEnvironmentVariable("ProgramFiles(x86)")
  $nvdaCandidates = @(
    (Join-Path $env:ProgramFiles "NVDA\nvda.exe"),
    (Join-Path $programFilesX86 "NVDA\nvda.exe"),
    (Join-Path $env:LOCALAPPDATA "Programs\NVDA\nvda.exe")
  )
  $nvdaExe = $nvdaCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
  if (-not $nvdaExe) {
    $nvdaCommand = Get-Command "nvda.exe" -ErrorAction SilentlyContinue
    if ($nvdaCommand) {
      $nvdaExe = $nvdaCommand.Source
    }
  }
  if (-not $nvdaExe) {
    throw "NVDA installed through WinGet, but nvda.exe was not found in the standard install locations"
  }
  Write-Host ("NVDA executable: " + $nvdaExe)
  Write-Host ("NVDA file version: " + (Get-Item $nvdaExe).VersionInfo.ProductVersion)

  Write-Host "=== Configure Speech Viewer and spoken browse-mode announcements ==="
  $nvdaConfigDirectory = Join-Path $env:APPDATA "nvda"
  New-Item -ItemType Directory -Path $nvdaConfigDirectory -Force | Out-Null
  $nvdaConfigPath = Join-Path $nvdaConfigDirectory "nvda.ini"
  $nvdaConfig = @"
[general]
language = en

[speechViewer]
showSpeechViewerAtStartup = True
autoPositionWindow = True

[virtualBuffers]
passThroughAudioIndication = False
enableOnPageLoad = True
"@
  [System.IO.File]::WriteAllText(
    $nvdaConfigPath,
    $nvdaConfig,
    [System.Text.UTF8Encoding]::new($false)
  )

  Get-Process -Name "nvda" -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep -Seconds 1
  $nvdaLog = Join-Path $env:RUNNER_TEMP "nvda-screen-reader.log"
  $nvdaProcess = Start-Process -FilePath $nvdaExe -ArgumentList @(
    "--disable-addons", "--log-level=20", "--log-file=$nvdaLog"
  ) -PassThru

  Write-Host "=== Wait for the real NVDA Speech Viewer window ==="
  $speechViewerOutput = ""
  $speechWindowHandle = $null
  $windowDeadline = (Get-Date).AddSeconds(45)
  do {
    $speechViewerOutput = Invoke-WinApp -Arguments @(
      "ui", "list-windows", "-a", "$($nvdaProcess.Id)"
    )
    if ($speechViewerOutput -match "NVDA Speech Viewer") {
      $handleMatch = [regex]::Match($speechViewerOutput, "(?i)HWND\s+(\d+)")
      if ($handleMatch.Success) {
        $speechWindowHandle = $handleMatch.Groups[1].Value
        break
      }
    }
    Start-Sleep -Seconds 1
  } while ((Get-Date) -lt $windowDeadline)

  if (-not $speechWindowHandle) {
    throw ("NVDA Speech Viewer did not appear. NVDA windows:" + [Environment]::NewLine + $speechViewerOutput)
  }
  $script:speechWindowHandle = $speechWindowHandle
  Write-Host ("NVDA Speech Viewer HWND: " + $speechWindowHandle)

  Write-Host "=== Open ORBIT in Microsoft Edge ==="
  $edgeCandidates = @(
    (Join-Path $programFilesX86 "Microsoft\Edge\Application\msedge.exe"),
    (Join-Path $env:ProgramFiles "Microsoft\Edge\Application\msedge.exe")
  )
  $edgeExe = $edgeCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
  if (-not $edgeExe) {
    $edgeCommand = Get-Command "msedge.exe" -ErrorAction SilentlyContinue
    if ($edgeCommand) { $edgeExe = $edgeCommand.Source }
  }
  if (-not $edgeExe) {
    throw "Microsoft Edge executable was not found on the Windows runner"
  }

  $edgeProfile = Join-Path $env:RUNNER_TEMP "orbit-nvda-edge-profile"
  $edgeProcess = Start-Process -FilePath $edgeExe -ArgumentList @(
    "--no-first-run",
    "--disable-default-apps",
    "--user-data-dir=$edgeProfile",
    "--app=$siteUrl"
  ) -PassThru

  Start-Sleep -Seconds 5
  $edgeWindows = Invoke-WinApp -Arguments @("ui", "list-windows", "-a", "msedge")
  if ([string]::IsNullOrWhiteSpace($edgeWindows)) {
    throw "Microsoft Edge did not expose a UI Automation window"
  }
  Write-Host $edgeWindows

  Invoke-WinApp -Arguments @("ui", "focus", "-a", "msedge") | Write-Host
  Start-Sleep -Seconds 1

  Write-Host "=== Move focus into the web document ==="
  $beforeTab = Get-SpeechText
  Invoke-WinApp -Arguments @("ui", "send-keys", "tab", "-a", "msedge", "--via", "send-input") | Write-Host
  $tabSpeech = Wait-SpeechDelta -Before $beforeTab -ExpectedPattern "(?i)Skip to main content" -TimeoutSeconds 12
  Write-Host ("NVDA Tab announcement: " + $tabSpeech)

  $beforeEnter = Get-SpeechText
  Invoke-WinApp -Arguments @("ui", "send-keys", "enter", "-a", "msedge", "--via", "send-input") | Write-Host
  $enterSpeech = Wait-SpeechDelta -Before $beforeEnter -ExpectedPattern "(?i)(main|content)" -TimeoutSeconds 8
  Write-Host ("NVDA skip-link announcement: " + $enterSpeech)

  Write-Host "=== Ensure NVDA Browse Mode is active ==="
  $beforeMode = Get-SpeechText
  Invoke-WinApp -Arguments @("ui", "send-keys", "insert+space", "-a", "msedge", "--via", "send-input") | Write-Host
  $modeSpeech = Wait-SpeechDelta -Before $beforeMode -ExpectedPattern "(?i)(Browse mode|Focus mode)" -TimeoutSeconds 8
  Write-Host ("NVDA mode announcement: " + $modeSpeech)

  if ($modeSpeech -match "(?i)Focus mode") {
    $beforeMode = Get-SpeechText
    Invoke-WinApp -Arguments @("ui", "send-keys", "insert+space", "-a", "msedge", "--via", "send-input") | Write-Host
    $modeSpeech = Wait-SpeechDelta -Before $beforeMode -ExpectedPattern "(?i)Browse mode" -TimeoutSeconds 8
    Write-Host ("NVDA mode switch: " + $modeSpeech)
  }
  if ($modeSpeech -notmatch "(?i)Browse mode") {
    throw ("Could not establish NVDA Browse Mode. Latest mode announcement: " + $modeSpeech)
  }

  Write-Host "=== Exercise NVDA H quick-navigation against the live browser ==="
  Invoke-WinApp -Arguments @("ui", "send-keys", "ctrl+home", "-a", "msedge", "--via", "send-input") | Write-Host
  Start-Sleep -Milliseconds 500

  $headingTargets = @("OBSERVE", "INVESTIGATE", "DELIVER")
  foreach ($heading in $headingTargets) {
    $beforeHeading = Get-SpeechText
    Invoke-WinApp -Arguments @("ui", "send-keys", "h", "-a", "msedge", "--via", "send-input") | Write-Host
    $headingSpeech = Wait-SpeechDelta -Before $beforeHeading -ExpectedPattern ([regex]::Escape($heading)) -TimeoutSeconds 8
    Write-Host ("NVDA H -> " + $heading + " : " + $headingSpeech)

    if ($headingSpeech -notmatch [regex]::Escape($heading)) {
      throw ("NVDA H did not reach '" + $heading + "'. Speech Viewer reported: " + $headingSpeech)
    }
    if ($headingSpeech -notmatch "(?i)heading") {
      throw ("NVDA announced '" + $heading + "' without identifying it as a heading: " + $headingSpeech)
    }
  }

  Write-Host "=== NVDA screen-reader smoke test passed ==="
  Write-Host "NVDA Browse Mode single-letter H navigation announced OBSERVE, INVESTIGATE, and DELIVER as headings."
  Write-Host ("Full NVDA log: " + $nvdaLog)
}
catch {
  Write-Host ("::error::NVDA screen-reader smoke test failed: " + $_.Exception.Message)
  if ($script:speechWindowHandle) {
    try {
      Write-Host "Last NVDA Speech Viewer transcript:"
      Write-Host (Get-SpeechText)
    } catch {
      Write-Host ("Speech Viewer transcript unavailable: " + $_.Exception.Message)
    }
  }
  $failedLog = Join-Path $env:RUNNER_TEMP "nvda-screen-reader.log"
  if (Test-Path $failedLog) {
    Write-Host "Last NVDA log lines:"
    Get-Content $failedLog -Tail 80
  }
  throw
}
finally {
  if ($edgeProcess -and -not $edgeProcess.HasExited) {
    Stop-Process -Id $edgeProcess.Id -Force -ErrorAction SilentlyContinue
  }
  if ($nvdaProcess -and -not $nvdaProcess.HasExited) {
    Stop-Process -Id $nvdaProcess.Id -Force -ErrorAction SilentlyContinue
  }
  if ($serverProcess -and -not $serverProcess.HasExited) {
    Stop-Process -Id $serverProcess.Id -Force -ErrorAction SilentlyContinue
  }
}
