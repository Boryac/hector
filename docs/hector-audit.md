# docs/hector-audit.md — Hector 原仓库架构解剖（Phase 0 产出）

**审计日期：** 2026-11-03
**审计对象：** `https://github.com/verikod/hector`
**Fork 时 commit：** `abe472b6deb12b3dbba290dbec44cd3be27e163d`
**commit 时间：** 2026-05-06 23:47:01 +0200
**commit 摘要：** `fixed agent tool calling/streaming`
**上游分支：** `main`（另有 `origin/builder` @ `153608b`）
**本地工作副本：** `hector-researchos-codex-pack/upstream/hector/`
**二次开发分支：** `feature/hector-researchos`（自 `main` 创建，未推送）
**许可证：** MIT（Copyright 2026 Verikod），允许二次开发与商用

## 1. 仓库规模与目录树

受跟踪文件 488 个，Go module 名 `github.com/verikod/hector`，`go 1.24.4`（toolchain 1.24.8）。

```
hector/
├── cmd/hector/            3 文件   main.go / app.go / serve.go —— 单二进制 CLI 入口
├── pkg/                 277 文件   33 个顶级包（详见第 3 节）
├── studio/              122 文件   Vite + React + Tailwind 管理前端（非 Next.js）
├── ui/                    2 文件   embed.go（//go:embed dist）+ dist/.gitkeep 占位
├── docs/                 36 文件   mkdocs 站点（getting-started / guides / reference / blog）
├── configs/               6 文件   YAML 配置样例
├── deploy/                7 文件   docker-compose / fly / heroku / railway / render
├── deployments/           5 文件   部署清单
├── scripts/               3 文件   发布/安装脚本
├── grafana/               6 文件   仪表盘与数据源
├── .github/                        CI（lint / test / release）
├── Dockerfile / Makefile / .goreleaser.yml / mkdocs.yml
└── README.md（610 行）/ LICENSE（MIT）/ doc.go / version.go
```

## 2. 运行时架构（现状）

- **单二进制**：`cmd/hector` 编译出带内嵌 Studio 的 `hector` 可执行文件；`hector serve` 起 HTTP 服务（默认 8080）。
- **协议层**：A2A v0.3.0（`github.com/a2aproject/a2a-go`）JSON-RPC + SSE 状态流；MCP（`mark3labs/mcp-go`）接工具。
- **执行层**：`pkg/runner` 驱动 `pkg/agent`（llmagent / workflowagent / remoteagent / runneragent）→ `pkg/execution/native`（SQL 队列 + Worker Pool + 指标）。
- **状态层**：`pkg/session`（会话事件 + user_states）、`pkg/task`（A2A 任务）、`pkg/checkpoint`（检查点与启动恢复）全部走 SQL；方言支持 PostgreSQL（pgx）与 MySQL，代码内字符串切换 `?` / `$1`。
- **接入层**：`pkg/server`（A2A handler、admin handler、动态 agent 路由、事件翻译）。
- **安全层**：`pkg/auth`（JWT/JWKS/OIDC、共享密钥、多 issuer、角色/租户中间件、a2a-go 拦截器）+ `pkg/guardrails`（输入注入检测、PII 脱敏、工具授权、内容审核）。
- **模型层**：`pkg/model/{openai,anthropic,gemini,ollama}` 可插拔。
- **检索层**：`pkg/rag` + `pkg/vector/{chroma,chromem,milvus,pinecone,qdrant,weaviate}` + `pkg/embedder`。

## 3. 原模块能力映射（逐项结论）

| 原模块 | 能力 | 结论 | 备注 |
|---|---|---|---|
| `pkg/agent` + `pkg/agent/llmagent` | Agent 定义、LLM 循环、工具调用、审批流转 | **复用** | 6 角色用同一 Runtime 多实例化；Researcher 动态多实例天然支持 |
| `pkg/agent/workflowagent` | sequential / parallel / loop / conditional 组合 | **部分复用** | 有并行与条件，无 DAG 依赖图、无环检测、无拓扑排序；DAG 编译需新增 |
| `pkg/agent/remoteagent` | A2A 客户端、agent card 解析、流式事件转换 | **修改** | `propagateMetadata` 只擦除 `agent_id`/`author`，身份类字段透传需收敛 |
| `pkg/execution/native` | SQL 队列（ack/nack/重试/死信/RecoverStale）+ Worker Pool + 指标 | **复用** | 已是 Scheduler + Worker Pool 雏形；并发数可配；支撑 T2.3 |
| `pkg/checkpoint` | 检查点状态、存储、启动恢复（WORKING 自动续跑 / 过期置 FAILED） | **复用** | 会话级粒度；缺节点级 Checkpoint，T2.5 需扩展 |
| `pkg/session` | 会话事件、sequence 递增、user_states、SQL 存储 | **复用** | 事件已有 `sequence_num`，是 EVENT_PROTOCOL 的落点；缺全局 `event_id` 幂等键 |
| `pkg/task` | A2A 任务服务与 SQL 存储（Get / List / CancelAllChildren） | **复用 + 修改** | Get 按 `(app_name, task_id)`、List 按 `(app_name, context_id)`，**无 user 维度过滤** |
| `pkg/server` | A2A JSON-RPC、事件翻译、admin API、动态 agent 路由 | **修改 + 扩展** | 需新增 Research REST + SSE 网关；`events.go` 身份绑定有缺陷（见 security-review） |
| `pkg/auth` | JWT 校验、共享密钥、角色/租户、a2a 拦截器 | **复用 + 修改** | 认证骨架完整；共享密钥把全部调用方折叠成单一身份；无吊销/scope 模型 |
| `pkg/guardrails` | Prompt Injection 检测、PII、工具授权、审核 | **复用** | 直接支撑 SECURITY_REVIEW 第 3 项，省一轮自研 |
| `pkg/tool/*`（filetool / commandtool / webtool / searchtool / mcptoolset / approvaltool） | 文件、命令沙箱、HTTP、搜索、MCP、审批 | **复用** | Tool Hub 的底座；`approvaltool` 支撑高风险审批（T4.5） |
| `pkg/tool/searchtool` + `webtool` | 搜索与抓取 | **复用 + 扩展** | 作为 Search Provider 接口的现实起点；需补 URL 规范化、去重、快照 |
| `pkg/rag` + `pkg/vector` + `pkg/embedder` | 文档切分、向量检索、嵌入 | **复用** | Analyst 的 RAG 能力来源 |
| `pkg/model/*` | 4 家 LLM Provider | **复用** | 满足 Provider 可切换；Settings 需补 Key 掩码与加密存储 |
| `pkg/config` + `pkg/builder` | YAML 配置、fluent builder、配置生成 | **复用** | 声明式配置底座；产品侧 Settings API 需另建（YAML ≠ 用户可改设置） |
| `pkg/observability` | OTel Trace / Metrics / Logs | **复用** | 满足可观测性要求 |
| `pkg/ratelimit` + `pkg/trigger` | 限流、webhook / 调度触发 | **复用** | 触发可用于定时研究任务（V1 不强制） |
| `studio/` | Vite + React 管理界面：Flow Builder、Chat、配置 | **部分复用（待决策）** | 技术栈与 DECISIONS「Next.js + React Flow」不一致；建议保留不动，ResearchOS 产品界面另建，避免重写 |
| `ui/embed.go` | 静态资源内嵌与 SPA 回退 | **复用** | 新前端可沿用同一内嵌模式 |
| `docs/` | mkdocs 文档站 | **保留** | 新增 ResearchOS 文档走 `docs/researchos/`，不覆盖原站 |
| `cmd/hector` | 单二进制入口 | **保留** | 新增 `cmd/api`、`cmd/worker` 独立进程，不替换原 CLI |

## 4. 任务队列 / Checkpoint / 恢复逻辑现状

**已有机制**
- SQL 队列：`pkg/execution/native/queue.go` 定义 ack / nack、指数退避重试、最大重试、死信、`RecoverStale(staleThreshold)`。
- Worker Pool：`worker.go` 固定 worker 数并发消费；`ShutdownTimeout` 默认 30s，`StaleRecoveryInterval` 默认 1m；启动与周期跑 `RecoverStale`；暴露 `Stats()` 指标。
- Checkpoint：`pkg/checkpoint` 具备 state / storage / manager / recovery / callbacks；启动时 `RecoverPendingTasks` 扫描，WORKING 可自动续跑，INPUT_REQUIRED 等人工，过期置 FAILED；状态按 `(app_name, user_id, session_id)` 定位。
- 会话事件：`session_events` 有 `sequence_num`，支持按序追加与按 `after_event_id` 过滤。

**相对 PRD 的缺口**
1. 无节点级 Checkpoint（现为会话级），T2.5「单节点失败不拖垮整任务 + 可重试」需新增节点快照结构。
2. 无研究任务生命周期状态机（Created→Planning→Awaiting Approval→Running→Synthesizing→Completed），现仅有 A2A 任务状态。
3. 事件缺全局 `event_id` 幂等键与「状态变更 + 事件同事务 / Outbox」，EVENT_PROTOCOL 的幂等与重放要求需补齐。
4. 无任务级暂停 / 恢复 API（只有取消与 checkpoint 自动恢复），T2.6 需在 Scheduler 层加暂停闸门。
5. 无研究域表结构（ResearchTask / WorkflowDefinition / Evidence / Claim / Report）。
6. 队列无「单任务 Agent 并发上限」概念（现为全局 worker 数），PRD 要求按任务限流。

## 5. A2A / 身份 / 用户上下文现状

调用链与结论详见 `docs/security-review.md`，此处只记事实：

```
客户端
  → auth.Middleware           校验 JWT，Claims 写入 context
  → a2a-go JSON-RPC handler
  → auth.Interceptor.Before   Claims → CallContext.User
  → server.Executor.Execute
  → toInvocationMeta(ctx, reqCtx)   ← 身份在此定稿
  → Session / Task / Checkpoint 均以 userID 为键
```

关键位点 `pkg/server/events.go:48-65`：
1. 先取 `auth.ClaimsFromContext(ctx).Subject`（可信）；
2. 再被 `reqCtx.Message.Metadata["user_id"]` **无条件覆盖**（不可信）；
3. 空则回落 `"default"`。

`reqCtx.ContextID` 直接作为 sessionID，来自客户端 `context_id`。即 userID 与 sessionID 两个键都可被调用方影响。

**结论：隔离不成立。** 详见 security-review 与可运行证据测试 `pkg/server/identity_isolation_test.go`。

## 6. 构建与测试基线（实测）

| 项 | 命令 | 结果 |
|---|---|---|
| 全量构建 | `go build ./...` | 通过（exit 0） |
| 核心包测试 | `go test ./pkg/server/... ./pkg/auth/... ./pkg/session/... ./pkg/task/... ./pkg/checkpoint/...` | 5 包全 ok |
| 身份隔离证据测试 | `go test ./pkg/server/ -run TestToInvocationMeta -v` | 4/4 PASS（证明漏洞存在） |
| 开发包骨架校验 | `bash scripts/verify.sh`（pack 根） | openapi 解析通过；go/web 检查跳过（骨架期预期） |
| 测试文件总量 | `git ls-files \| grep _test.go \| wc -l` | 37 个测试文件 / 488 文件 |

## 7. 环境现状

| 工具 | 版本 | 状态 |
|---|---|---|
| Go | 1.27.3 | 可用（上游要求 1.24+） |
| Node.js | v22.22.2 | 可用 |
| Python | 3.13.14 | 可用（verify.sh 的 yaml 校验） |
| Git | 2.55.0 | 可用 |
| Docker | 未安装 | 按决策推迟到 Phase 5 前安装 |
| PostgreSQL | 未安装（本地） | 开发期用 Testcontainers 或远程实例；Docker 就绪后走 compose |

## 8. 审计结论摘要

1. Hector 是成熟、可复用的 Agent 运行时，**9 类关键能力可直接复用**（Agent Runtime、执行队列与 Worker、Checkpoint、会话事件、工具 / MCP、Guardrails、多模型、可观测性、RAG）。
2. 真正需要新建的只有**研究域**：研究任务生命周期、DAG 编译、Planner 结构化校验、来源 / 证据 / 报告模型与 API、Research SSE 网关。
3. 唯一阻断级问题是 **A2A 身份隔离**：`metadata.user_id` 可伪造，且任务 / 会话读接口缺 user 维度，必须在多用户功能前修复；默认保持单用户本地模式。
4. 上游前端为 Vite + React，与 DECISIONS 的 Next.js + React Flow 存在技术栈分歧，建议保留 Studio 不动、另建产品前端，需人工确认。
