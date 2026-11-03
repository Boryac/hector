# Hector ResearchOS

把一句研究目标变成**带来源、可追溯、可复核**的研究报告：规划 → 检索 → 抓取 → 抽取结论 → 交叉验证 → 生成报告，全过程在界面上实时可见。

- **基础**：Fork 自 [verikod/hector](https://github.com/verikod/hector)，保留上游 Agent Runtime / MCP / RAG 能力，产品链路另建 `internal/researchos`
- **技术栈**：Go 1.24 · PostgreSQL（sqlc + pgx） · Next.js 14 + React 18 + React Flow · SSE
- **形态**：模块化单体 + 独立 Worker，单用户本地部署（多用户能力未开放，见 [安全边界](#安全边界)）

---

## 当前状态（先看这一节，避免误判）

| 能力 | 状态 | 证据 |
|---|---|---|
| Go 构建 / 全量单测 | ✅ 通过 | `go build ./...`；`go test ./...` 全绿（新增 10 个测试文件） |
| 数据库迁移 | ✅ 通过 | PostgreSQL 16.2 实跑，`schema_migrations` 记 `0001_init.sql` |
| 端到端研究任务 | ✅ 通过 | 6/6 节点、报告生成、引用可回溯（真实外部检索） |
| SSE 实时事件 | ✅ 通过 | `sequence` 连续；`Last-Event-ID` / `?after_sequence=` 可续接 |
| 前端构建 | ✅ 通过 | Next.js 静态导出 3 个路由：`/`、`/task/`、`/_not-found` |
| 前端页面视觉 | ⚠️ **未截图验证** | 本机无法启动 Chrome/Edge（连 `--version` 都无输出），需按 `scripts/screenshot.sh` 人工截图 |
| 真实 LLM 调用 | ⚠️ **未验证** | 未配 API Key，当前走本地确定性流程；UI 与报告标注 `heuristic` |
| 真实检索 | ✅ 已接通 | GitHub / Hacker News / Wikipedia / Bing 免密钥可用；Tavily·SerpAPI·Brave 需 Key |
| 并发 5 任务 | ✅ 实测 | 数据见 [`docs/benchmark.md`](docs/benchmark.md) |
| Docker Compose | ⚠️ **未在本机验证** | 本机未装 Docker；`docker-compose.yaml` 与 `Dockerfile.researchos` 已交付 |
| 多用户隔离 | 🚫 **禁用** | A2A Issue #2 核心链路已修，会话归属校验未做，按决策保持单用户 |

> 表格中带 ⚠️ 的项**尚未真实验证**，不得当作已完成。

---

## 快速开始

### 方式 A：本机已有 PostgreSQL

```bash
cd upstream/hector

# 1. 配置（.env.researchos 已被 .gitignore 忽略）
cp .env.example .env.researchos
#    至少设置 RESEARCHOS_DATABASE_URL 与 RESEARCHOS_API_TOKEN

# 2. 前端静态产物（可选，仅构建界面时需要）
cd web && npm ci && npx next build && cd ..

# 3. 迁移并启动（API + Worker 同进程）
go run ./cmd/researchos all
```

打开 <http://127.0.0.1:8080/>。

### 方式 B：Docker Compose

```bash
cd hector-researchos-codex-pack
export RESEARCHOS_API_TOKEN=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
docker compose up -d --build
```

> **未在本机验证**（开发环境无 Docker）。首次使用请留意 `db` 健康检查与迁移时机。

### 方式 C：分进程（调试推荐）

```bash
./researchos migrate     # 只跑迁移
./researchos serve       # API + 静态前端
./researchos worker      # DAG 执行器（可多开，靠租约互斥）
./researchos seed        # 创建演示任务并跑到完成
```

---

## 界面

| 页面 | 内容 |
|---|---|
| `/` Dashboard | 健康状态（DB / LLM / 检索 Provider）、创建任务、任务列表 |
| `/task/?id=...` 任务详情 | React Flow 工作流 DAG、节点状态与重试、事件流（SSE）、资料、证据、Agent、报告 |

报告可导出 `.md` / `.html`（带 `Authorization` 的下载，Token 不出现在 URL 里）。

---

## 工作流长什么样

规划器产出的是一张 DAG，节点类型决定执行器：

```
search ──► analyze ──┐
search ──► analyze ──┼──► verify ──► synthesize
                     ┘
```

- **search**：多 Provider 并发检索，URL 规范化 + 内容指纹去重后落库
- **analyze**：选资料（站点多样性优先）→ 抓取正文 → 逐字校验引文后才产结论
- **verify**：结论聚类 → 独立站点数 + 数值一致性 → `supported / partially_supported / conflicting`
- **synthesize**：生成结构化报告，引用编号稳定映射到真实 URL

节点依赖写在 `workflow_nodes.depends_on`，认领用 `FOR UPDATE … SKIP LOCKED`，租约超时自动回收。
单个子问题覆盖不足时该分析节点降级跳过，不拖垮整份研究；只有「一条结论都没有」才判定任务失败。
细节见 [`docs/DATA_MODEL.md`](docs/DATA_MODEL.md)。

---

## 目录结构（真实路径）

```
hector-researchos-codex-pack/
├── upstream/hector/                 # 上游仓库（分支 feature/hector-researchos）
│   ├── internal/researchos/         # 产品链路（本项目主要代码）
│   │   ├── api/                     # REST + SSE + 静态托管
│   │   ├── domain/                  # 领域模型 + 状态机
│   │   ├── store/                   # 事务性状态变更 + 事件写入
│   │   ├── db/                      # 迁移、sqlc 生成代码
│   │   ├── planner/ dag/            # 规划与图校验
│   │   ├── search/ fetch/           # 真实检索与抓取（含 SSRF 防护）
│   │   ├── textutil/                # 正文抽取、URL 规范化、关键词
│   │   ├── exec/                    # 节点执行器（含交叉验证）
│   │   ├── report/                  # 报告生成与渲染
│   │   └── worker/                  # 调度循环、重试、收尾
│   ├── cmd/researchos/              # 入口：migrate/serve/worker/all/seed
│   └── web/                         # 产品前端（Next.js 静态导出）
├── api/openapi.yaml                 # API 契约（OpenAPI 3.1）
├── docs/                            # 审计、数据模型、安全、基准
├── scripts/                         # 验证 / 压测 / 故障注入 / 截图
└── docker-compose.yaml
```

---

## 配置

全部走环境变量，完整注释见 `upstream/hector/.env.example`：

| 变量 | 说明 | 默认 |
|---|---|---|
| `RESEARCHOS_DATABASE_URL` | 必填，PostgreSQL DSN | — |
| `RESEARCHOS_API_TOKEN` | 必填（`serve`/`all` 强制），Bearer Token | — |
| `RESEARCHOS_LOCAL_USER` | 服务端唯一身份，客户端不可指定 | `local-user` |
| `RESEARCHOS_LLM_PROVIDER` / `_API_KEY` / `_MODEL` | 留空则走启发式流程 | `none` |
| `RESEARCHOS_SEARCH_PROVIDERS` | `github,hackernews,wikipedia,bing,tavily,serpapi,brave` | `bing,github,wikipedia` |
| `RESEARCHOS_WORKERS` / `MAX_AGENTS` | 并发 Worker 数 / 单任务 Agent 上限 | `4` / `4` |
| `RESEARCHOS_REQUIRE_APPROVAL` | 规划后是否需人工批准 | `true` |
| `RESEARCHOS_WEB_DIR` | 前端静态产物目录 | 空（不托管 UI） |

`.env.researchos` 在启动时自动加载，且**不覆盖**已存在的 shell 变量。

---

## 测试与验收

```bash
cd upstream/hector
go build ./...
go test ./...                                  # 全量单测（含上游）
go test -cover ./internal/researchos/...       # 覆盖率（数字见 docs/benchmark.md）

cd ../../..
bash scripts/verify.sh                         # 契约与结构检查
bash scripts/fault-injection.sh                # 杀 Worker → 租约恢复验证
bash scripts/load-test.sh 5 quick              # 5 任务并发压测
bash scripts/screenshot.sh                     # 生成界面截图（需可用桌面）
```

已知环境问题：本机 `go test -cover` 对**无测试文件的包**（`db`/`llm`/`store`/`worker`）会报
`cmd/internal/cov is not in std`，属 Go 工具链问题，不影响 `go test ./...`。

---

## 安全边界

- 身份**只认服务端配置**：请求体 / 查询串 / 请求头里的 `user_id` 一律忽略（A2A Issue #2，已修复并回归）
- 所有 `/api/` 路径必须带 Bearer Token；静态资源免鉴权，但不含任何任务数据
- Token 用 `subtle.ConstantTimeCompare` 比较；SSE 支持 `?token=`，**仅限** `/events`
- 抓取侧：遵循 robots.txt、Dial 阶段校验真实 IP（挡 SSRF / DNS rebinding / 云元数据地址）、限大小与超时
- 报告渲染：HTML 转义 + 仅允许 `http(s)` 链接，越界引用标记会被剔除

未完成项（因此**多用户能力保持禁用**）：会话归属校验（`context_id → owner`）、上游 `pkg/task` 的 user 过滤。
详见 [`docs/security-review.md`](docs/security-review.md) 第 1.7 节。

---

## 已知限制

1. 未配 LLM 时结论由「关键词句抽取 + 跨源比对」产生，深度弱于模型综述，报告中已标注 `heuristic`。
2. GitHub 匿名配额会 403（配 `RESEARCHOS_GITHUB_TOKEN` 可解）；Bing 中文结果质量不稳定。
3. PDF 导出未实现（`.md` / `.html` 可用）。
4. 前端页面未经真实截图验证（当前环境无法启动浏览器）。
5. Docker 路径未在本机跑过。

---

## 文档

| 文档 | 内容 |
|---|---|
| [`docs/DATA_MODEL.md`](docs/DATA_MODEL.md) | 表结构、状态机全矩阵、队列并发语义、恢复路径 |
| [`api/openapi.yaml`](api/openapi.yaml) | REST + SSE 契约（含身份与错误语义） |
| [`docs/hector-audit.md`](docs/hector-audit.md) | 上游源码审计与复用结论 |
| [`docs/mapping-table.md`](docs/mapping-table.md) | 模块映射：保留 / 修改 / 新增 / 替换 |
| [`docs/security-review.md`](docs/security-review.md) | 安全审查结论与修复状态 |
| [`docs/benchmark.md`](docs/benchmark.md) | 覆盖率与并发实测数据 |
| [`DECISIONS.md`](DECISIONS.md) · [`TASKS.md`](TASKS.md) · [`PROGRESS.md`](PROGRESS.md) | 决策记录 / 任务清单 / 迭代账本 |
| [`AGENTS.md`](AGENTS.md) · [`CODEX.md`](CODEX.md) | 面向接续开发者（含 AI Agent）的工作约定 |
