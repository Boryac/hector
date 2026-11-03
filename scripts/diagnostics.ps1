# diagnostics.ps1 — 环境与运行诊断（只读，不修改任何系统配置；输出已脱敏）。
$ErrorActionPreference = "Continue"
. "$PSScriptRoot\common.ps1"

function Line { param($k, $v) "{0,-28} {1}" -f $k, $v }
Write-Output "=== Hector ResearchOS 诊断 ==="
Write-Output (Line "Windows" ([System.Environment]::OSVersion.VersionString))
Write-Output (Line "PowerShell" $PSVersionTable.PSVersion.ToString())
Write-Output (Line "64 位进程" ([Environment]::Is64BitProcess))

# WebView2
$wv = Get-ItemProperty "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" -ErrorAction SilentlyContinue
Write-Output (Line "WebView2 Runtime" $(if ($wv) { $wv.pv } else { "未检测到（托盘窗口需要）" }))

# 后端与配置
try {
    $cfg = Get-AppConfig
    Write-Output (Line "配置" "已初始化")
    Write-Output (Line "数据目录" $cfg.dataDir)
    Write-Output (Line "监听地址" $(if ($cfg.addr) { $cfg.addr } else { "127.0.0.1:8080" }))
    Write-Output (Line "DSN" ($cfg.pgDsn -replace ":[^:@]+@", ":***@"))
} catch { Write-Output (Line "配置" $_.Exception.Message) }

# 数据目录空间
try {
    $cfg2 = Get-AppConfig
    $drive = Get-PSDrive ($cfg2.dataDir.Substring(0, 1))
    Write-Output (Line "数据盘剩余空间" ("{0:N1} GB" -f ($drive.Free / 1GB)))
} catch {}

# 端口占用
$addr = "127.0.0.1:8080"
try {
    $c = Get-AppConfig
    if ($c.addr) { $addr = $c.addr }
} catch {}
$port = [int]($addr.Split(":")[1])
$conn = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
if ($conn) {
    $p = Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue
    Write-Output (Line "端口 $port" ("被占用: PID {0} ({1})" -f $conn.OwningProcess, $p.ProcessName))
} else {
    Write-Output (Line "端口 $port" "空闲")
}

# Web 静态资源
$webDir = Join-Path $PSScriptRoot "..\web"
Write-Output (Line "Web 静态资源" $(if (Test-Path (Join-Path $webDir "index.html")) { "完整" } else { "缺失（重装发行包）" }))

# 数据库连通
try {
    $c3 = Get-AppConfig
    $uri = "http://$addr/api/v1/health"
    $r = Invoke-WebRequest -Uri $uri -UseBasicParsing -TimeoutSec 5
    Write-Output (Line "后端健康" ("HTTP " + $r.StatusCode))
} catch { Write-Output (Line "后端健康" "不可达（服务未启动或配置有误）") }

# LLM / Search 配置状态（经设置中心，掩码输出）
try {
    $c4 = Get-AppConfig
    $h = @{ Authorization = "Bearer $($c4.apiToken)" }
    $s = Invoke-RestMethod -Uri "http://$addr/api/v1/settings" -Headers $h -TimeoutSec 5
    foreach ($sec in $s.sections) {
        if ($sec.id -eq "model") {
            $prov = ($sec.fields | Where-Object key -eq "model.provider").value
            $keySet = ($sec.fields | Where-Object key -eq "model.api_key").configured
            Write-Output (Line "LLM" ("provider={0}, key={1}" -f $prov, $(if ($keySet) { "已配置" } else { "未配置" })))
        }
        if ($sec.id -eq "search") {
            $sp = ($sec.fields | Where-Object key -eq "search.provider").value
            Write-Output (Line "搜索" ("provider={0}" -f $sp))
        }
    }
} catch { Write-Output (Line "LLM/搜索状态" "无法读取（服务未运行）") }

# 最近的启动错误（日志尾部，脱敏）
try {
    $c5 = Get-AppConfig
    $logs = Get-ChildItem (Join-Path $c5.dataDir "logs") -Filter "backend-*.log" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if ($logs) {
        Write-Output "最近日志: $($logs.Name)"
        Get-Content $logs.FullName -Tail 8 | ForEach-Object {
            ($_ -replace "(postgres(ql)?://[^:\s]+:)[^@\s]+@", '$1***@') -replace "(Bearer\s+)\S+", '$1***' -replace "(sk-)[A-Za-z0-9_-]{6,}", '$1***'
        }
    } else { Write-Output "最近日志: 无" }
} catch { Write-Output "最近日志: 读取失败" }

Write-Output "=== 诊断结束（本脚本只读，不修改系统配置） ==="
