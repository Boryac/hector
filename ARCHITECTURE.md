# ARCHITECTURE.md — 已实现架构

> 本文描述**当前代码里真实存在的结构**，不是规划稿。
> 规划阶段的取舍理由留在 `DECISIONS.md`，审计结论留在 `docs/hector-audit.md`。
> 代码位于 `upstream/hector`（分支 `feature/hector-researchos`），
> 研究域相关代码全部在 `internal/researchos/` 下，与上游 `pkg/` 隔离。

---

## 1. 总体形态：模块化单体 + 独立 Worker

单进程 `researchos all` 或双进程 `serve` / `worker` 都能跑，二者共用同一套
Store 与 Domain。没有微服务、没有消息队列中间件——任务队列就在 PostgreSQL 里。

```
浏览器
  │  REST + SSE（同源，Token 只走 Authorization 头）
  ▼
┌─────────────────────────────────────────────┐
│ internal/researchos/api      路由 / 鉴权 / SSE │
│ internal/researchos/planner  目标 → 计划       │
│ internal/researchos/dag      依赖就绪 / 认领    │
│ internal/researchos/exec     节点执行           │
│ internal/researchos/report   报告渲染           │
└─────────────────────────────────────────────┘
  │ sqlc + pgx
  ▼
PostgreSQL 16（13 张表，见 docs/DATA_MODEL.md）
  ▲
  │ SELECT ... FOR UPDATE SKIP LOCKED
┌─────────────────────────────────────────────┐
│ internal/researchos/worker   N 个并发认领循环  │
└─────────────────────────────────────────────┘
  │
  ▼ 外部世界（全部真实调用，无 Mock）
search: github / hackernews / wikipedia / bing
fetch : HTTP + robots + SSRF 守卫
llm   : OpenAI 兼容接口（未配置时降级为本地规则）
```

**为什么不做独立调度进程**：任务状态、节点状态、事件、租约全在一张事务边界内，
把调度拆出去就要引入跨进程一致性。单机单用户场景下，
`FOR UPDATE SKIP LOCKED` 已经能让 N 个 Worker 安全并发认领。

---

## 2. 代码地图（实测行数）

| 包 | 文件 | 行数 | 职责 | 不负责 |
|---|---|---|---|---|
| `api` | 5 | 1434 | 路由、鉴权、SSE、DTO 组装 | 业务执行 |
| `db` | 10 | 2798 | sqlc 生成代码 + 迁移 SQL | 手写业务查询 |
| `store` | 2 | 1235 | 事务封装：任务、节点、事件、资料、证据、报告 | HTTP 语义 |
| `exec` | 6 | 1758 | 节点执行器（search/fetch/analyze/verify/synthesize） | 调度与认领 |
| `domain` | 3 | 522 | 实体、状态机、事件类型常量 | 持久化细节 |
| `planner` | 3 | 713 | 启发式计划生成、计划归一化、检索式构造 | 调用模型执行 |
| `search` | 4 | 1050 | Provider 链、去重、降级、trace | 正文清洗 |
| `fetch` | 2 | 553 | 抓取、robots、SSRF 守卫、JSON→正文 | 结论抽取 |
| `report` | 3 | 767 | Markdown 渲染、引用编号、导出 | 数据查询 |
| `textutil` | 2 | 601 | 正文清洗、模板剥离、URL 规范化、字数统计 | 业务判断 |
| `worker` | 1 | 323 | 认领循环、心跳、租约续期、优雅退出 | 节点内部逻辑 |
| `config` | 3 | 471 | 环境变量、`.env` 加载、默认值 | 密钥管理（只读 env） |
| `dag` | 2 | 275 | 就绪判定、层级计算、环检测工具 | 状态写入 |
| `llm` | 1 | 219 | OpenAI 兼容薄客户端 | 提示词构造 |

`cmd/researchos/main.go` 提供 6 个子命令：`migrate` / `serve` / `worker` / `all` / `seed` / `version`。

---

## 3. 一次研究的完整链路

```
POST /api/v1/research
  └─ 建 research_tasks（status=created）+ research.events(created)
  └─ 后台 goroutine：Planner 生成计划
        → 写 workflow_nodes（含 depends_on）
        → status=awaiting_approval（RESEARCHOS_REQUIRE_APPROVAL=true）

POST /api/v1/research/{id}/approve
  └─ TransitionTask(awaiting_approval → running)
  └─ 首个无依赖节点变为 ready

Worker 循环（N 并发）
  └─ ClaimReadyNodes：SELECT ... WHERE status='ready' AND depends_on 全 completed
                      ORDER BY priority FOR UPDATE OF n SKIP LOCKED
  └─ 置 running + lease_expires_at + attempts+1
  └─ exec.Engine.Run(node)
        search     → Provider 链检索 + 首批抓取
        fetch      → 补抓正文（robots / SSRF / 大小上限）
        analyze    → 抽结论 + 证据边（source_id, quote）
        verify     → 跨源聚类 + 冲突判定
        synthesize → 报告落库
  └─ 写 node 结果 + 事件 + BumpSequence
  └─ 依赖它的节点在下一轮变为 ready

全部节点终态 → TransitionTask(running → synthesizing → completed)
任一节点 failed 且无重试额度 → TransitionTask(running → failed)
```

### 节点类型与产出

| 类型 | 输入 | 产出 | 失败后果 |
|---|---|---|---|
| `search` | query | 资料元数据 + 首批正文 | 重试 `max_attempts` 后退避 |
| `fetch` | source_ids | 补抓正文 | 同上 |
| `analyze` | question + 依赖资料 | claims + evidence | **无资料/无结论时返回 `skipped` 而非 error** |
| `verify` | claims | 聚类、支持度、冲突 | 无结论时 `verified=0` 而非 error |
| `synthesize` | 全部 claims + 证据 | `reports` 一行 | 0 结论时失败（不产出空报告） |

`analyze` 与 `verify` 的「软失败」是刻意设计：单个子问题覆盖不足不应该让整份
研究崩掉，覆盖缺口会写进报告摘要与争议章节。但 `synthesize` 拿不到任何结论时
必须失败——一份没有证据的报告比一条失败记录更容易误导人。

---

## 4. 状态机

任务状态白名单与跃迁表在 `internal/researchos/domain/transitions.go`，
唯一写入入口是 `store.TransitionTask`：

- 同状态跃迁幂等，直接返回；
- 非法跃迁写审计事件 `research.transition_rejected` 并返回 `ErrInvalidTransition`，
  API 层映射为 **409 `invalid_transition`**；
- 合法跃迁带 `WHERE status = <from>` 条件更新，防并发覆盖。

9 个状态 × 8 个目标状态的完整矩阵见 `docs/DATA_MODEL.md` §2。

---

## 5. 事件与 SSE

| 关注点 | 实现 |
|---|---|
| 幂等 | 每条事件有 `event_id`（UUID），客户端可去重 |
| 顺序 | `research_tasks.latest_sequence` 单调自增，任务内严格递增 |
| 断线续接 | 标准 `Last-Event-ID` 头，或 `?after_sequence=N` |
| 传输 | `text/event-stream`，15s 心跳注释帧；任务终态时主动 `stream.end` 收尾 |
| 鉴权（首选） | `Authorization: Bearer <API Token>`，命令行与脚本走这条 |
| 鉴权（浏览器） | `EventSource` 不能设头，改用一次性 ticket：`POST /api/v1/research/{id}/events/ticket` 用 Bearer 换取，再用 `?ticket=` 建流 |

ticket 的实现落在 `api/ticket.go`：进程内表，key 是 `sha256(ticket)`（不存明文），
默认 60s 过期，绑定单一 `task_id`，校验通过即删除；单任务未消费上限 16 张，防止签发接口刷内存。
`withAuth` 里只有 `eventsTaskID(path)` 命中的那一个路由接受 ticket，ticket 不能替代 Bearer 调其他接口。
`?token=` 通道已删除——长期凭据进 URL 会沉淀到浏览器历史、`Referer` 与代理日志，这一点写在
`docs/security-review.md` §1.8。

LLM 模式下 search 节点的检索式优先取规划器给出的 `subtask.query`（`planner.NodeSearchQuery`），
只有它为空时才回退到「目标 + 维度词」模板——否则模型拆出来的子问题对检索毫无影响。

事件类型见 `docs/DATA_MODEL.md` §3。前端 `web/lib/event-map.ts` 做类型 → 文案/图标映射，
未识别的类型降级为原始 type 名显示，不丢事件。

---

## 6. 并发与恢复

| 机制 | 参数 | 行为 |
|---|---|---|
| Worker 并发 | `RESEARCHOS_WORKERS`（默认 4） | N 个 goroutine 循环认领 |
| 租约 | `RESEARCHOS_STALE_LEASE`（默认 3m） | 认领时写 `lease_expires_at`，运行中定期续期 |
| 崩溃恢复 | 同上 | 租约过期后节点回到 `ready`，**已完成节点不动** |
| 重试 | `max_attempts` / `next_attempt_at` | 退避重试，用尽后置 `failed` |
| 暂停闸门 | `paused` 任务 | 认领 SQL 排除 paused 任务的节点 |

故障注入实测见 `docs/benchmark.md` 与 `scripts/fault-injection.sh`。

---

## 7. 检索与抓取

```
SearchQueryFor(goal, index)
  ├─ 目标含 ASCII 技术词 → "<技术词...> <英文维度>"
  └─ 纯中文目标        → "<目标清洗后> <中文维度>"
        ↓
Engine.SearchAll（各 Provider 并行，单点失败只记 trace）
  ├─ 语言路由：纯中文检索式跳过 github / hackernews
  ├─ Bing：按检索式语种选 mkt（zh-CN / en-US）
  ├─ 去重：规范化 URL，非 http(s) 一律丢弃
  └─ 落库：snippet 截断到 600 rune
        ↓
fetch：robots.txt → DNS 解析 → Dial 阶段校验真实 IP → 大小上限 → 正文清洗
        ↓
textutil：剥脚本/样式/nav/footer、剥 wiki 模板与表格、剥维护提示、去引用残渣
```

SSRF 防护是三层：host 白名单、解析后 IP 校验、**Dial 阶段再校验一次**
（挡 DNS rebinding 与 `169.254.169.254` 一类元数据地址）。

---

## 8. 前端

Next.js 14 App Router + static export（`output: 'export'`，产物 `web/.build`），
由 Go 进程同源托管：静态资源免鉴权，`/api/` 前缀强制 Bearer。

```
web/app/page.tsx      Dashboard：新建任务、任务列表、状态、报告入口
web/app/task/page.tsx 工作台：节点时间线、事件流、资料来源、报告与导出
web/lib/api.ts        fetch 封装，Token 从内存/输入读取，不进 URL
web/lib/event-map.ts  事件类型 → 展示语义
web/lib/types.ts      与 api/openapi.yaml 对应的 TS 类型
```

React Flow 在工作台里渲染节点依赖图。`downloadReport()` 用带
`Authorization` 头的 fetch 取 Blob 再触发下载，**不用 `<a href="...?token=">`**。

---

## 9. 身份与安全边界

| 项 | 现状 |
|---|---|
| 部署形态 | 单用户本地，`auth.mode=single-user-local` |
| API 鉴权 | `/api/` 前缀一律要求 `Authorization: Bearer <RESEARCHOS_API_TOKEN>` |
| 静态资源 | 免鉴权（浏览器无法为导航请求加头） |
| 共享密钥 | 固定 `Subject`，因此**不做多用户**；多用户能力需重做身份模型 |
| admin key | `subtle.ConstantTimeCompare`，空密钥直接拒绝 |
| A2A 身份 | `pkg/server/events.go` 不再采信客户端 `metadata.user_id`，以 JWT claims 为准 |
| 密钥落盘 | 只从环境变量/`.env.researchos` 读，`.env.researchos` 在 `.gitignore` 里 |

Issue #2 的修复范围与回归测试见 `docs/security-review.md`。

---

## 10. 与上游 Hector 的关系

| 上游能力 | 复用方式 |
|---|---|
| Agent Runtime / WorkerPool / Checkpoint / MCP | 未改，继续服务上游 Agent 功能 |
| `pkg/server`（A2A 网关） | 修了 Issue #2 的身份覆盖与 admin key 比较 |
| `pkg/model` | **未复用**：它面向 ADK 多轮工具调用，形状与研究域不匹配 |
| `task_queue` | **未复用**：FIFO 无法表达 DAG 依赖，改用 `workflow_nodes` |

研究域是新增的 `internal/researchos/`，与 `pkg/` 无循环依赖；
`pkg/server` 的改动只有身份校验，没有引入研究域概念。
