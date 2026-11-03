# ARCHITECTURE.md — 技术架构设计

## 1. 总体架构：模块化单体 + 独立 Worker

12 周内不做微服务。微服务会引入服务发现、分布式事务、部署编排等额外复杂度，
且不能直接提升产品展示效果。但核心执行能力必须有清晰模块边界，后续可独立拆分。

```
Presentation: Next.js + React + TypeScript
  Workspace / Workflow Canvas / Report Center
        │  REST API + SSE
Go API Server: Authentication · API · Event Gateway
Application: Research Service / Planner Service / Workflow Service / Report Service / Evidence Service / Tool Registry
Execution: DAG Scheduler（依赖管理）/ Agent Runtime（生命周期）/ Worker Pool（并发）/ Tool Router（MCP 路由）
State: Events · State · Checkpoints → PostgreSQL（主存储）/ Redis（可选，缓存与临时协调）/ Object Storage（报告、附件、网页快照）/ OpenTelemetry
```

**PostgreSQL 是正式版本主要持久化存储。Redis 不应成为任务状态唯一来源。**

## 2. 技术选型

| 层级 | 方案 | 用途 |
|---|---|---|
| 后端 | Go | API、调度、执行 |
| 前端 | Next.js + TypeScript | Web 产品 |
| UI | Tailwind CSS + shadcn/ui | 组件系统 |
| 工作流 | React Flow / @xyflow/react | DAG 画布，禁止自研图形编辑器 |
| 数据请求 | TanStack Query | 服务端状态 |
| 实时通信 | SSE | 服务端向客户端单向推送，第一版不引入 WebSocket |
| API | REST + OpenAPI | 前后端接口，见 api/openapi.yaml |
| 数据库 | PostgreSQL | 核心数据 |
| 向量检索 | pgvector | 语义检索，不能代替全文搜索与结构化查询 |
| 缓存 | Redis | 可选 |
| ORM | GORM 或 sqlc（二选一，Phase 0 锁定，不得混用） | 数据访问 |
| 日志 | slog / Zap | 结构化日志 |
| 可观测性 | OpenTelemetry | Trace、Metrics、Logs |
| 部署 | Docker Compose | 一键启动 |

决策说明：
- React Flow：时间应用于节点交互、状态同步、执行可视化，而非重复造图形库。
- SSE：研究状态主要是单向推送，无需双向复杂度。
- 优先沿用 Hector 已有 Agent Runtime、MCP、A2A、持久化执行机制，具体接口以源码审查为准。

## 3. 模块边界与职责（Codex 必遵）

| 模块 | 负责什么 | 不负责什么 |
|---|---|---|
| API | 接口、参数校验、认证 | 业务执行 |
| Research | 研究任务生命周期 | 底层并发调度 |
| Planner | 生成研究计划 | 直接执行所有任务 |
| Workflow | DAG 定义、依赖管理 | 前端画布渲染 |
| Scheduler | 节点调度、并发控制 | 模型提示词设计 |
| Agent Runtime | Agent 生命周期和执行 | 页面展示 |
| Search | 搜索、抓取、解析 | 研究结论判定 |
| Evidence | 来源与证据关系 | 生成最终报告 |
| MCP | 工具注册、发现、调用 | 研究任务拆解 |
| Report | 报告结构与导出 | 网页搜索 |
| Observability | 日志、指标、Tracing | 业务状态唯一存储 |

**尤其防止：把所有业务逻辑都写进 Agent Runtime。Agent 是执行单元，不是系统业务中心。**

## 4. 核心业务执行流程

输入示例：分析全球 AI Agent 市场，研究 10 个代表性开源项目，比较架构、社区、商业化，给出未来三年判断。

Pipeline：Research Planning（Planner Agent，结构化计划）→ Task Decomposition（DAG Scheduler，依赖图与并行）
→ Parallel Research（多个 Researcher）→ Evidence Collection（Search + Web Reader，来源索引）
→ Cross Verification（Verifier，多源验证、标记冲突与缺口）→ Synthesis（Synthesizer，统一分析）
→ Report Generation（Reporter，结构化报告、对比表、引用、结论）。

### Agent 角色（V1 先做 6 种，不要预设十几个）

| Agent | 职责 | 主要工具 |
|---|---|---|
| Planner | 规划和拆解 | LLM |
| Researcher | 专项研究，可动态多实例 | Search、Web |
| Analyst | 分析和比较资料 | RAG、LLM |
| Verifier | 验证结论、识别冲突 | Search、Evidence |
| Synthesizer | 整合成果 | Evidence、LLM |
| Reporter | 生成最终报告 | Markdown、Export |

例如 Researcher A 开源项目 / B 行业发展 / C 商业模式 / D 技术趋势，比固定十几个 Agent 更灵活。

## 5. Go 后端目标目录结构（目标，不是第一天搬迁令）

```
hector-researchos/
├── cmd/api/main.go  cmd/worker/main.go  cmd/migrate/main.go
├── internal/api/{handler,middleware,router,dto}
├── internal/{research,planner,workflow,scheduler,agent,search,evidence,tools,report,events,persistence,observability,config}
├── web/{app,components,features/{workspace,workflow,agents,sources,evidence,reports,settings},hooks,lib,types}
├── api/openapi.yaml  deployments/{docker,compose.yaml}  docs/{architecture,development,deployment,api}.md
├── scripts/  tests/  Dockerfile  Makefile  README.md
```

说明：cmd/api 与 cmd/worker 分离，可独立运行；internal 不对外暴露；api/openapi.yaml 是前后端契约；
web/features 按产品功能组织；tests 放集成与 E2E。
**这是一份目标结构，不是要求把 Hector 当前文件机械迁移。原项目包结构与依赖方向先审查，再决定复用。**

## 6. 前端产品结构（Next.js App Router）

```
web/app/{page.tsx, research/{new,[id]}, reports, tools, settings}
web/features/{workspace,workflow,execution,sources,evidence,reports}
web/components/{layout,navigation,charts,ui}  web/hooks  web/lib  web/types
```

页面：Dashboard（新建、最近任务、状态、最近报告、系统概览）；
New Research（目标、深度、范围、最大时间、最大并发、是否交叉验证，均给合理默认值）；
Research Workspace（三栏：左任务与步骤 / 中画布 / 右节点详情+Agent输出+日志，下方实时事件时间线）；
Report Viewer（目录导航、Markdown、引用脚注、来源弹窗、表格、导出）；
Tool Hub（MCP Server、工具列表、状态、调用记录、权限）。
