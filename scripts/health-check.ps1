# health-check.ps1 — 后台服务健康检查（进程 + HTTP）。退出码 0=健康 1=不健康。
$ErrorActionPreference = "Continue"
. "$PSScriptRoot\common.ps1"

$cfg = Get-AppConfig
$addr = if ($cfg.addr) { $cfg.addr } else { "127.0.0.1:8080" }
$pidFile = Join-Path $cfg.dataDir "run\backend.pid"

$procOk = $false
if (Test-Path $pidFile) {
    $rec = Get-Content $pidFile -Raw | ConvertFrom-Json
    if (Test-BackendAlive -PidToCheck $rec.pid) { $procOk = $true; Write-Output "进程: 运行中 (PID $($rec.pid))" }
}
if (-not $procOk) { Write-Output "进程: 未运行" }

$httpOk = $false
try {
    $r = Invoke-WebRequest -Uri "http://$addr/api/v1/health" -UseBasicParsing -TimeoutSec 5
    $httpOk = ($r.StatusCode -eq 200)
    Write-Output "HTTP /api/v1/health: $($r.StatusCode)"
} catch { Write-Output "HTTP /api/v1/health: 不可达" }

if ($procOk -and $httpOk) { Write-Output "结论: 健康"; exit 0 }
Write-Output "结论: 不健康"
exit 1
