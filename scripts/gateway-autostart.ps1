# Launcher for the AI Terminal Gateway, started by the auto-start scheduled task.
# Loads .env (if present), builds the binary if needed, then runs it.
$ErrorActionPreference = 'Stop'

# Project root is the parent of this scripts/ directory.
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# Load .env: KEY=VALUE lines, ignoring blanks and # comments.
$envFile = Join-Path $root '.env'
if (Test-Path $envFile) {
    Get-Content $envFile | Where-Object { $_ -and $_ -notmatch '^\s*#' } | ForEach-Object {
        $k, $v = $_ -split '=', 2
        if ($k) { [Environment]::SetEnvironmentVariable($k.Trim(), $v) }
    }
}

# Build the binary once if it's missing (keeps startup fast on later logons).
# -H windowsgui => GUI subsystem, so no console window ever appears (the user
# can't close it by accident). Logs go to GATEWAY_LOG_FILE from .env.
$exe = Join-Path $root 'ai-gateway-api.exe'
if (-not (Test-Path $exe)) {
    & go build -ldflags '-H windowsgui' -o $exe .
}

# Run in the foreground; the scheduled task owns this process's lifetime.
& $exe
