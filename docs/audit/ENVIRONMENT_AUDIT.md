# 环境审计（ENVIRONMENT AUDIT）

> 审计时间：2026-11-04 02:20–02:30 (GMT+8)。本文件为仓库整合、RC1 冻结与 Windows 发行前的环境实测记录。所有凭据均已脱敏，本文不含任何真实密钥。

## 1. 操作系统与运行时

| 项 | 实测值 | 对本任务的影响 |
|---|---|---|
| 操作系统 | Windows 11（内核 10.0.26300）x64 | 发行目标平台本体 |
| PowerShell | 5.1.26100.9711 | 发行脚本按 PS 5.1 兼容语法编写 |
| Git | 2.55.0.windows.3（PortableGit） | bundle/subtree/merge 均可用 |
| Go | go1.27.3 windows/amd64 | 后端构建可用 |
| Node.js / npm | v22.22.2 / 10.9.7 | 前端构建可用 |
| Docker | **未安装**（command not found） | Docker 实机验证继续 **BLOCKED** |
| GitHub CLI | gh 2.102.0，已登录 Boryac，scope 含 `repo`、`workflow` | 可读取仓库信息、校验 workflow 语法 |
| Wails CLI | **未安装**（`wails: command not found`） | Phase 5 需先 `go install` |
| WebView2 Runtime | 155.0.4283.62（已随 Edge 安装） | Wails 前置条件满足 |

## 2. 数据库与本地服务

| 项 | 实测值 |
|---|---|
| PostgreSQL 16 | 本机 55432 端口监听中（便携实例），产品 DSN 经 `.env.researchos` 注入（内容不入库不入文档） |
| ResearchOS 服务 | 127.0.0.1:8080（此前 RC1 验证轮保留的进程，本轮整合测试用独立端口复测） |
| 本地模型中继 | 127.0.0.1:18326（DeepSeek Harness）；Relay Key 仅从 `~/.dsh/our-free-model/settings.json` 运行时读取，任何输出路径均已脱敏 |

## 3. 磁盘与目录

- C 盘剩余 72.8 GB / 197.4 GB（满足构建 + 发行包 + 备份需求）。
- 目录布局（本任务开始时）：
  - `C:\Users\22334\Desktop\hector-researchos-codex-pack` — 开发包仓库（85 个跟踪文件：ResearchOS 文档/脚本/账本）+ `upstream/hector/`（上游 git 工作副本，被本仓 .gitignore 排除）
  - `C:\Users\22334\Desktop\hector-researchos-codex-pack\upstream\hector` — 上游仓库（origin = github.com/verikod/hector，PUBLIC，main 为默认分支）

## 4. 仓库盘点（Phase 0/1 实测）

| 项 | 开发包仓库 | upstream（verikod/hector） |
|---|---|---|
| Remote | 无 | https://github.com/verikod/hector.git |
| 可见性 | 仅本地 | PUBLIC（gh API 实测） |
| 默认分支 | main | main |
| 工作分支 | main @ `2d5d4d9` | feature/hector-researchos @ `b3c6ac3` |
| 提交数（全 refs） | 15 | 33（含 builder 分支） |
| 标签 | 无 | 17 个（v1.21.0 – v1.41.2，均为上游历史版本） |
| Releases / Issues / PR | 无 | Releases 3+（最新 v1.41.2）、open issues 1 |
| main↔feature 关系 | — | **main 是 feature 的祖先**（快进安全） |
| 历史敏感信息扫描 | 文件名级：仅 `.env.example`；内容级 `sk-` 模式 0 命中 | 同左，0 命中；`secrets.go` 等为源码文件名，非凭据 |

结论：**无已提交的有效凭据**，无需历史重写或凭据轮换。

## 5. 安全注意（本轮已执行）

- 凭据仅从既有安全配置运行时读取（`.env.researchos`、`~/.dsh/.../settings.json`），未复制到任何新文件、Git 对象、发行包或报告。
- 本报告及后续所有报告输出前均经模式扫描（`sk-[A-Za-z0-9]{20,}`、`ofm-`、Bearer 值）确认无泄漏。
- 环境变量中存在重复大小写的 `HTTP(S)_PROXY`（127.0.0.1:62655 本地代理），已知的 curl/IWR 走代理问题在本轮测试中统一以 `--noproxy '*'` 规避。

## 6. 结论

- 可执行：Git 整合、Go/前端构建与测试、Wails 评估与托盘程序开发、ZIP 打包、GitHub Actions 工作流编写与（本地）验证。
- BLOCKED：Docker 实机验证（无 Docker）。
- 需安装：Wails CLI（Phase 5 处理）。
