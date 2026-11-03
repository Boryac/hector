# stop.ps1 — 优雅停止后台服务：先调 shutdown 端点，超时且身份确认后受控强制终止。
$ErrorActionPreference = "Stop"
. "$PSScriptRoot\common.ps1"

$cfg = Get-AppConfig
$addr = if ($cfg.addr) { $cfg.addr } else { "127.0.0.1:8080" }
$pidFile = Join-Path $cfg.dataDir "run\backend.pid"

if (-not (Test-Path $pidFile)) { Write-Output "后台服务未在运行（无 pid 记录）"; exit 0 }
$rec = Get-Content $pidFile -Raw | ConvertFrom-Json
if (-not (Test-BackendAlive -PidToCheck $rec.pid)) {
    Remove-Item $pidFile -Force
    Write-Output "pid 记录已失效，清理完成"
    exit 0
}

# 1) 优雅关闭（与 Ctrl+C 同路径：先停接入，再排空 worker）
try {
    Invoke-WebRequest -Uri "http://$addr/api/v1/admin/shutdown" -Method POST `
        -Headers @{ Authorization = "Bearer $($cfg.apiToken)" } -UseBasicParsing -TimeoutSec 5 | Out-Null
} catch { Write-Output "shutdown 端点未响应（可能未开鉴权匹配），转等待+强制路径" }

# 2) 最多等 15s
$deadline = (Get-Date).AddSeconds(15)
while ((Get-Date) -lt $deadline -and (Test-BackendAlive -PidToCheck $rec.pid)) {
    Start-Sleep -Milliseconds 500
}

# 3) 身份确认后受控强制终止
if (Test-BackendAlive -PidToCheck $rec.pid) {
    Stop-Process -Id $rec.pid -Force -Confirm:$false
    Write-Output "优雅停止超时，已强制终止 PID $($rec.pid)"
} else {
    Write-Output "已优雅停止 PID $($rec.pid)"
}
Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
exit 0
