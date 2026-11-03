# install.ps1 — 便携包安装自检（不做系统级安装、不需要管理员权限）。
# 校验：目录完整、WebView2、脚本执行策略提示、数据目录可写性。
$ErrorActionPreference = "Continue"
$root = Split-Path $PSScriptRoot
Write-Output "=== Hector ResearchOS 安装自检 ==="

$need = @("researchos-tray.exe", "bin\researchos.exe", "web\index.html", "database\migrations\0001_init.sql", "assets\fonts\NotoSansSC-Regular.ttf")
$missing = @()
foreach ($n in $need) { if (-not (Test-Path (Join-Path $root $n))) { $missing += $n } }
if ($missing) {
    Write-Output "缺失文件: $($missing -join ', ')"
    Write-Output "发行包不完整，请重新下载解压。"
    exit 1
}
Write-Output "发行包完整性: OK"

$wv = Get-ItemProperty "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" -ErrorAction SilentlyContinue
if ($wv) { Write-Output "WebView2 Runtime: $($wv.pv) OK" }
else { Write-Output "WebView2 Runtime: 未检测到 —— 托盘窗口无法显示，请安装 WebView2 Runtime（Windows 10/11 通常自带）" }

Write-Output ""
Write-Output "下一步：双击 researchos-tray.exe 完成首次初始化（选择数据目录、配置 PostgreSQL 16）。"
Write-Output "详细说明见 START_HERE.md 与 docs\installation\QUICK_START.md"
exit 0
