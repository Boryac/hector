# restart.ps1 — 重启后台服务。
$ErrorActionPreference = "Stop"
& "$PSScriptRoot\stop.ps1"
& "$PSScriptRoot\start.ps1"
exit $LASTEXITCODE
