# 开发者指南（DEVELOPER GUIDE）

> 仓库：单一 Git 仓库（verikod/hector 本地工作副本）。产品 = Hector ResearchOS（上游 hector 平台之上叠加 `internal/researchos` 产品链路）。

## 目录速览

| 路径 | 内容 |
|---|---|
| `cmd/researchos` | 后端入口：`migrate` / `serve`（API）/ `worker` / `all`（API+Worker） |
| `internal/researchos/` | 产品域：api / config / dag / exec / fetch / llm / mcp / planner / report / search / secrets / settings / store / worker |
| `web/` | Next.js 14 静态导出前端（`npm run build` → `web/.build`） |
| `desktop/traymanager/` | 独立 Go 模块：Wails 托盘管理程序（win32 托盘 + 向导） |
| `scripts/` | verify.sh（统一校验）、check-*.py、rc1_*（探测/审计/压测）、package-release.ps1 |
| `docs/` | architecture / development / installation / release / audit / product |
| `deployments/` | compose 与配置模板 |

## 本地开发

```bash
# 依赖：Go 1.24+、Node 20+、PostgreSQL 16（外置）
cp .env.example .env.researchos   # 填 DSN 与 RESEARCHOS_API_TOKEN（不入库）
go build -o bin/researchos.exe ./cmd/researchos
./bin/researchos.exe migrate
RESEARCHOS_WEB_DIR=web/.build ./bin/researchos.exe all   # 前端先 npm run build

cd web && npm ci && npm run build       # 前端
cd desktop/traymanager && wails build   # 托盘（需 wails CLI）

bash scripts/verify.sh                  # 统一校验（14 项，可证伪）
make researchos-verify                  # 同上（Makefile 已接）
```

环境变量参考：docs/installation/CONFIGURATION.md。注意 `RESEARCHOS_SECRET_KEY` 与 `data\secret.key` 的优先级（文件 > 环境），保持稳定否则已存 API Key 失效。

## 托盘程序（desktop/traymanager）

- 独立 go.mod，不污染主模块依赖。
- 托盘 = 自研 win32 Shell_NotifyIcon（wails v2.17.1 无 SystemTray API，见 DECISIONS 42）；窗口 = Wails + WebView2；后端 = 独立 researchos.exe 子进程（PID+创建时间+镜像名三重身份校验；优雅关闭走 `POST /api/v1/admin/shutdown`）。
- 集成冒烟：`RESEARCHOS_BACKEND_EXE=... RESEARCHOS_SMOKE_DSN=... go test -run TestSmokeInitAndLifecycle .`

## 测试与验收脚本

| 脚本 | 用途 |
|---|---|
| `scripts/verify.sh` | 14 项统一校验（含负向自检），FAIL 非零退出 |
| `scripts/rc1_security_probe.py` | 13 项 API 安全探测 |
| `scripts/rc1_browser_audit.py` | 27 项 Playwright 浏览器断言 |
| `scripts/rc1_probe_model.py` | LLM Provider 能力探测（18 项） |
| `scripts/rc1_run_research.py` | 真实研究任务 A/B/C 全链路证据落盘 |
| `scripts/package-release.ps1` | Windows x64 便携 ZIP 组装 + SHA256 + BUILD_INFO |

## 发行

见 docs/installation/WINDOWS_DISTRIBUTION.md 与 `.github/workflows/release-windows.yml`（tag 触发，产物为 Workflow Artifacts，不自动建 Release）。

## Git 约定

- 主分支 `main`；历史含上游 hector（v1.21–v1.41 标签）与开发包两条来源线（merge `7d8c432`）。
- 提交遵循 conventional commits；不推远程时一切工作保留在本地（推送需用户授权）。
