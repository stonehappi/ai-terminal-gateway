# Registers a per-user scheduled task that auto-starts the AI Terminal Gateway
# at logon. Run once: powershell -ExecutionPolicy Bypass -File scripts\install-autostart.ps1
# No admin rights needed (the task runs as the current user, so the generation
# CLI login and Docker Desktop are available).
$ErrorActionPreference = 'Stop'

$taskName = 'AITerminalGateway'
$launcher = Join-Path $PSScriptRoot 'gateway-autostart.ps1'
$psArgs   = '-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "' + $launcher + '"'

$action   = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $psArgs
$trigger  = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME

# Start when available, don't stop on battery, restart a few times if it dies,
# and never time out (it's a long-running server).
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)

Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Description 'Auto-start the AI Terminal Gateway at logon' -Force | Out-Null

Write-Host "Registered scheduled task '$taskName'. The gateway will start at your next logon."
Write-Host "Start it now without logging out:  Start-ScheduledTask -TaskName $taskName"
Write-Host "Remove it later:                   powershell -File scripts\uninstall-autostart.ps1"
