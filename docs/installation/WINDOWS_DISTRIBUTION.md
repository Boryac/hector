# Windows 发行说明（WINDOWS_DISTRIBUTION）

> 产物：`Hector-ResearchOS-v1.0.0-rc.1-windows-x64.zip`（便携式，免安装）。
> 构建来源：仓库 `main` 分支 tag `v1.0.0-rc.1`；构建方式见 `.github/workflows/release-windows.yml` 或本地 `scripts/package-release.ps1`。

## 目录结构

```
Hector-ResearchOS-v1.0.0-rc.1-windows-x64/
├── researchos-tray.exe        托盘管理程序（Wails + WebView2 + 自研 win32 托盘）
├── START_HERE.md              入门指引
├── LICENSE / CHANGELOG.md
├── bin/researchos.exe         后端（API + Worker 单二进制；内嵌前端/迁移/中文字体）
├── web/                       前端静态导出（后端内嵌资源的同源备份，便于排查）
├── config/                    配置说明（实际配置在 %LOCALAPPDATA%\Hector ResearchOS\）
├── database/migrations/       SQL 迁移文件（已内嵌二进制，此处供 DBA 审阅）
├── assets/fonts/              中文字体（Noto Sans SC，OFL 许可，见 assets/licenses/）
├── scripts/                   install/start/stop/restart/health-check/diagnostics.ps1
└── docs/                      安装、配置、排障、发行说明
```

## 资源定位原则

后端以自身 exe 所在目录为根解析 `web/.build`、迁移（已内嵌）、字体（已内嵌）；托盘以自身 exe 所在目录的 `bin\researchos.exe` 启动后端；用户数据一律在数据目录。**不依赖开发机绝对路径**。

## 构建方法（本地复现）

```powershell
# 前置：Go 1.24+、Node 20+、wails CLI
npm --prefix web ci && npm --prefix web run build     # 产出 web/.build
go build -trimpath -ldflags "-s -w -X main.Version=v1.0.0-rc.1" -o dist/bin/researchos.exe ./cmd/researchos
cd desktop/traymanager && wails build                 # 产出 researchos-tray.exe
pwsh scripts/package-release.ps1                      # 组装 + ZIP + SHA256
```

## 已知平台限制

- 仅 Windows x64；macOS/Linux 未打包（可用源码 + Docker 方式运行）。
- 不含 MSI 安装器与自动更新（RC1 决策，见 KNOWN_LIMITATIONS）。
- PostgreSQL 16 为外置依赖，ZIP 不捆绑。
- 托盘程序在极少数远端桌面/Server Core 环境可能无系统托盘区可用——此时用 `scripts\start.ps1` 系列直接管理后台。
