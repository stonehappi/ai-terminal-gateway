# Builds the Windows installer (Setup.exe) for the AI Terminal Gateway.
# Requires: Go, and Inno Setup 6 (ISCC.exe). Install Inno Setup with:
#   winget install JRSoftware.InnoSetup
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

Write-Host '[1/2] Building ai-gateway-api.exe ...'
& go build -o (Join-Path $root 'ai-gateway-api.exe') .
if ($LASTEXITCODE -ne 0) { throw 'go build failed' }

# Locate the Inno Setup compiler.
$iscc = @(
    "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
    "${env:ProgramFiles}\Inno Setup 6\ISCC.exe",
    "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
) | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $iscc) { $iscc = (Get-Command ISCC.exe -ErrorAction SilentlyContinue).Source }
if (-not $iscc) {
    throw "Inno Setup (ISCC.exe) not found. Install it with:  winget install JRSoftware.InnoSetup"
}

Write-Host "[2/2] Compiling installer with $iscc ..."
& $iscc (Join-Path $root 'installer\ai-terminal-gateway.iss')
if ($LASTEXITCODE -ne 0) { throw 'ISCC failed' }

$out = Join-Path $root 'installer\Output\ai-terminal-gateway-setup.exe'
Write-Host ''
Write-Host "Done. Installer: $out"
