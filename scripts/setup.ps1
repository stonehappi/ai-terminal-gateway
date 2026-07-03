# First-run setup for the AI Terminal Gateway. Invoked by the installer (or run
# manually). Generates .env with a secure key, auto-detects the sandbox backend,
# registers the logon auto-start task, and prints a summary.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\setup.ps1 -Provider codex
param(
    [ValidateSet('claude', 'agy', 'codex')]
    [string]$Provider = 'codex',
    [ValidateSet('auto', 'docker', 'local')]
    [string]$Backend = 'auto',
    [int]$Port = 8081,
    [switch]$LoginNow,
    [switch]$StartNow
)
$ErrorActionPreference = 'Stop'

$appRoot = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $appRoot '.env'

function New-ApiKey {
    $bytes = New-Object 'System.Byte[]' 24
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    return 'gw_' + (([Convert]::ToBase64String($bytes)) -replace '[+/=]', '').Substring(0, 28)
}

function Resolve-Backend([string]$choice) {
    if ($choice -ne 'auto') { return $choice }
    $docker = Get-Command docker -ErrorAction SilentlyContinue
    if ($docker) { return 'docker' }   # Docker CLI present -> use isolated backend
    return 'local'
}

$backendResolved = Resolve-Backend $Backend

# --- Write .env (only if missing, so re-running never overwrites a real config) ---
if (Test-Path $envFile) {
    Write-Host "Keeping existing .env (not overwritten)."
    $key = ((Get-Content $envFile | Where-Object { $_ -match '^GATEWAY_API_KEYS=' }) -split '=', 2)[1]
} else {
    $key = New-ApiKey
    @(
        '# Local runtime config for the AI Terminal Gateway. NOT committed.',
        "GATEWAY_API_KEYS=$key",
        "PORT=$Port",
        "LLM_PROVIDER=$Provider",
        'CLAUDE_BIN=claude',
        'CLAUDE_MODEL=',
        'AGY_BIN=agy',
        'AGY_MODEL=',
        'CODEX_BIN=codex',
        'CODEX_MODEL=',
        "SANDBOX_BACKEND=$backendResolved",
        'SANDBOX_TIMEOUT_SECONDS=30',
        'SANDBOX_PYTHON_IMAGE=python:3.12-slim',
        'SANDBOX_BASH_IMAGE=bash:5',
        'SANDBOX_MEMORY=256m',
        'SANDBOX_CPUS=1'
    ) | Set-Content -Path $envFile -Encoding ASCII
    Write-Host "Created .env (provider=$Provider, backend=$backendResolved)."
}

# --- Optional: log in to the chosen provider CLI (interactive, opens a browser) ---
if ($LoginNow) {
    $cli = $Provider
    if (Get-Command $cli -ErrorAction SilentlyContinue) {
        Write-Host "Launching '$cli' login..."
        if ($cli -eq 'codex') { & $cli login } else { & $cli }
    } else {
        Write-Warning "'$cli' is not on PATH. Install and log in before using provider '$Provider'."
    }
}

# --- Register the auto-start task ---
& (Join-Path $PSScriptRoot 'install-autostart.ps1')

if ($StartNow) {
    Start-ScheduledTask -TaskName 'AITerminalGateway'
    Start-Sleep -Seconds 3
    try { Start-Process "http://localhost:$Port" } catch { }   # open the web console
}

# --- Write the key to a readable file and open it so the user can copy it ---
$keyFile = Join-Path $appRoot 'YOUR-API-KEY.txt'
@(
    'AI Terminal Gateway - your access details',
    '==========================================',
    '',
    "  URL:      http://localhost:$Port",
    "  API key:  $key",
    "  Provider: $Provider",
    "  Sandbox:  $backendResolved",
    '',
    "EASIEST WAY TO TRY IT: open http://localhost:$Port in your web browser,",
    'paste the API key above once, type a request, and click Run.',
    '',
    "For API calls, send this header to http://localhost:$Port/v1/run :",
    '',
    "  Authorization: Bearer $key",
    '',
    'Example:',
    "  curl http://localhost:$Port/v1/run -H ""Authorization: Bearer $key"" \",
    "       -d '{""prompt"":""list the first 10 prime numbers""}'",
    '',
    'Keep this key private. To change it, edit GATEWAY_API_KEYS in:',
    "  $envFile",
    'The gateway starts automatically each time you log in.'
) | Set-Content -Path $keyFile -Encoding UTF8
try { Start-Process notepad.exe $keyFile } catch { }

# --- Summary (console) ---
Write-Host ''
Write-Host '======================================================'
Write-Host ' AI Terminal Gateway is set up.'
Write-Host "   URL:      http://localhost:$Port"
Write-Host "   API key:  $key"
Write-Host "   Provider: $Provider    Sandbox: $backendResolved"
if ($backendResolved -eq 'docker') {
    Write-Host '   Note: start Docker Desktop for code-execution requests.'
}
Write-Host "   Your key was saved to and opened from: $keyFile"
Write-Host '   It will start automatically each time you log in.'
Write-Host '======================================================'
