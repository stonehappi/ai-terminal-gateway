# Removes the AI Terminal Gateway auto-start scheduled task and stops it if running.
$ErrorActionPreference = 'Stop'
$taskName = 'AITerminalGateway'

if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) {
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
    Write-Host "Removed scheduled task '$taskName'."
} else {
    Write-Host "Scheduled task '$taskName' is not registered."
}
