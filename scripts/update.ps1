# Updates the AI Terminal Gateway binary and restarts the running background task.
# Works for both source developers (with Go) and end users (without Go, downloads compiled .exe).
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

Write-Host "========== AI Terminal Gateway Updater ==========" -ForegroundColor Cyan

$exePath = Join-Path $root "ai-gateway-api.exe"
$hasGo = Get-Command go -ErrorAction SilentlyContinue

if ($hasGo -and (Test-Path (Join-Path $root "main.go"))) {
    Write-Host "[1/3] Compiling latest binary from source..." -ForegroundColor Yellow
    if (Test-Path (Join-Path $root ".git")) {
        try { & git pull } catch { }
    }
    & go build -ldflags "-H windowsgui" -o $exePath .
    if ($LASTEXITCODE -ne 0) { throw "Build failed." }
} else {
    Write-Host "[1/3] Downloading latest compiled ai-gateway-api.exe from GitHub..." -ForegroundColor Yellow
    $url = "https://github.com/stonehappi/ai-terminal-gateway/releases/latest/download/ai-gateway-api.exe"
    $tempExe = Join-Path $env:TEMP "ai-gateway-api-latest.exe"
    try {
        Invoke-WebRequest -Uri $url -OutFile $tempExe -UseBasicParsing
        $exePath = $tempExe
    } catch {
        throw "Failed to download update from $url : $_"
    }
}

Write-Host "[2/3] Stopping running instance & updating executable..." -ForegroundColor Yellow
Stop-ScheduledTask -TaskName "AITerminalGateway" -ErrorAction SilentlyContinue
Stop-Process -Name "ai-gateway-api" -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

# Copy to installed location
$appDataFolder = "$env:LOCALAPPDATA\AI Terminal Gateway"
if (Test-Path $appDataFolder) {
    Copy-Item -Path $exePath -Destination (Join-Path $appDataFolder "ai-gateway-api.exe") -Force
}
$localRootExe = Join-Path $root "ai-gateway-api.exe"
if ((Test-Path $localRootExe) -and ($exePath -ne $localRootExe)) {
    Copy-Item -Path $exePath -Destination $localRootExe -Force
}

Write-Host "[3/3] Restarting AI Terminal Gateway..." -ForegroundColor Yellow
if (Get-ScheduledTask -TaskName "AITerminalGateway" -ErrorAction SilentlyContinue) {
    Start-ScheduledTask -TaskName "AITerminalGateway"
} else {
    $targetExe = Join-Path $appDataFolder "ai-gateway-api.exe"
    if (-not (Test-Path $targetExe)) { $targetExe = $localRootExe }
    Start-Process $targetExe
}

Write-Host ""
Write-Host "Done! AI Terminal Gateway has been updated." -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor Cyan
