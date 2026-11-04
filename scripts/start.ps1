# start.ps1 — 启动 ResearchOS 后台服务（不启动托盘；托盘请运行 researchos-tray.exe）。
# 用法: powershell -ExecutionPolicy Bypass -File scripts\start.ps1
$ErrorActionPreference = "Stop"
. "$PSScriptRoot\common.ps1"

$cfg = Get-AppConfig
$addr = if ($cfg.addr) { $cfg.addr } else { "127.0.0.1:8080" }
$runDir = Join-Path $cfg.dataDir "run"
New-Item -ItemType Directory -Force -Path $runDir, (Join-Path $cfg.dataDir "logs") | Out-Null
$pidFile = Join-Path $runDir "backend.pid"

# 已运行检查（pidfile + 进程身份）
if (Test-Path $pidFile) {
    $rec = Get-Content $pidFile -Raw | ConvertFrom-Json
    if (Test-BackendAlive -PidToCheck $rec.pid) {
        Write-Output "后台服务已在运行 (PID $($rec.pid))，地址 http://$addr"
        exit 0
    }
}

Remove-DuplicateCaseEnvVars

$exe = Get-BackendExe
$log = Join-Path $cfg.dataDir ("logs\backend-{0}.log" -f (Get-Date -Format "yyyyMMdd-HHmmss"))

# 子进程继承当前环境变量（PS 5.1 / 7 通用）
$env:RESEARCHOS_ADDR = $addr
$env:RESEARCHOS_DATABASE_URL = $cfg.pgDsn
$env:RESEARCHOS_API_TOKEN = $cfg.apiToken
$env:RESEARCHOS_SECRET_KEY = $cfg.secretKey
# 前端静态目录与托盘保持一致（bin\web\.build）；已显式设置时不覆盖
if (-not $env:RESEARCHOS_WEB_DIR) {
    $env:RESEARCHOS_WEB_DIR = Join-Path (Split-Path $exe) "web\.build"
}

$proc = Start-Process -FilePath $exe -ArgumentList "all" -WorkingDirectory (Split-Path $exe) `
    -WindowStyle Hidden -PassThru -RedirectStandardOutput $log -RedirectStandardError "$log.err"

@{ pid = $proc.Id } | ConvertTo-Json | Set-Content $pidFile -Encoding UTF8

if (Wait-Healthy -Addr $addr) {
    Write-Output "启动成功: PID $($proc.Id)，地址 http://$addr"
    exit 0
}
Write-Output "服务进程已启动 (PID $($proc.Id))，但 20s 内健康检查未通过；日志: $log"
exit 1
