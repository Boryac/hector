# package-release.ps1 — 组装 Windows x64 便携 ZIP（本地或 CI 均可用）。
# 用法: powershell -File scripts\package-release.ps1 [-Version v1.0.0-rc.1]
param([string]$Version = "v1.0.0-rc.1")

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot
$stage = Join-Path $env:TEMP ("Hector-ResearchOS-" + $Version + "-windows-x64")
if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }

Write-Output "[1/5] 前端构建（web/.build）"
Push-Location (Join-Path $root "web")
if (-not (Test-Path node_modules)) { npm ci --silent }
$env:CODEBUDDY_SAFE_DELETE_ENABLED = "0"
npm run build | Out-Null
Pop-Location

Write-Output "[2/5] 后端构建（内嵌前端/迁移/字体）"
$env:CGO_ENABLED = "0"
Push-Location $root
go build -trimpath -ldflags "-s -w -X main.Version=$Version" -o (Join-Path $stage "bin\researchos.exe") ".\cmd\researchos"
$ok = ($LASTEXITCODE -eq 0)
Pop-Location
if (-not $ok) { throw "后端构建失败" }

Write-Output "[3/5] 托盘构建（Wails）"
Push-Location (Join-Path $root "desktop\traymanager")
wails build | Out-Null
Copy-Item "build\bin\researchos-tray.exe" $stage -Force
Pop-Location

Write-Output "[4/5] 组装资源"
# 前端必须位于 bin\web\.build：托盘以后端 exe 所在目录（bin\）为工作目录注入 RESEARCHOS_WEB_DIR，
# 后端据此挂载静态 UI；放错层级会导致界面 404（RC1 发行后实测发现）。
New-Item -ItemType Directory -Force -Path (Join-Path $stage "bin\web\.build"), (Join-Path $stage "config"), (Join-Path $stage "database\migrations"), (Join-Path $stage "assets\fonts"), (Join-Path $stage "assets\licenses"), (Join-Path $stage "scripts"), (Join-Path $stage "docs\release"), (Join-Path $stage "docs\installation") | Out-Null
Copy-Item (Join-Path $root "web\.build\*") (Join-Path $stage "bin\web\.build") -Recurse -Force
Copy-Item (Join-Path $root "internal\researchos\db\migrations\*.sql") (Join-Path $stage "database\migrations")
Copy-Item (Join-Path $root "internal\researchos\report\fonts\NotoSansSC-Regular.ttf") (Join-Path $stage "assets\fonts")
Copy-Item (Join-Path $root "internal\researchos\report\fonts\OFL.txt") (Join-Path $stage "assets\licenses\NotoSansSC-OFL.txt")
Copy-Item (Join-Path $root "LICENSE"), (Join-Path $root "CHANGELOG.md") $stage
Copy-Item (Join-Path $root "docs\installation\QUICK_START.md"), (Join-Path $root "docs\installation\CONFIGURATION.md"), (Join-Path $root "docs\installation\TROUBLESHOOTING.md"), (Join-Path $root "docs\installation\WINDOWS_DISTRIBUTION.md") (Join-Path $stage "docs\installation")
Copy-Item (Join-Path $root "docs\release\RC1_RELEASE_NOTES.md"), (Join-Path $root "docs\release\KNOWN_LIMITATIONS.md") (Join-Path $stage "docs\release")
Copy-Item (Join-Path $root "scripts\*.ps1") (Join-Path $stage "scripts")
# START_HERE
Copy-Item (Join-Path $root "docs\installation\QUICK_START.md") (Join-Path $stage "START_HERE.md")

Write-Output "[5/5] 打包 ZIP + SHA256 + BUILD_INFO"
$zip = "$stage.zip"
if (Test-Path $zip) { Remove-Item $zip }
Compress-Archive -Path $stage -DestinationPath $zip

$hash = (Get-FileHash $zip -Algorithm SHA256).Hash
$commit = git -C $root rev-parse HEAD
$buildInfo = @"
artifact: $(Split-Path $zip -Leaf)
version: $Version
commit: $commit
built_at: $((Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"))
os: windows amd64
go: $(go version)
sha256: $hash
"@
Set-Content (Join-Path $env:TEMP "BUILD_INFO.txt") $buildInfo -Encoding UTF8

Write-Output ""
Write-Output "ZIP : $zip"
Write-Output "SHA256: $hash"
Write-Output "BUILD_INFO: $(Join-Path $env:TEMP 'BUILD_INFO.txt')"
