# docs/mapping-table.md — 二次开发差异清单（Phase 0 产出）

**基线 commit：** `abe472b`（verikod/hector main）
**审计文档：** 见 `docs/hector-audit.md`
**原则：** 能复用不重写；新增必须有明确代码边界与验收依据；不为了套用预设目录结构而重排上游代码。

## 1. 保留（直接沿用，不改代码）

| 项目 | 结论 | 依据 |
|---|---|---|
| 仓库整体结构（cmd/hector + pkg/33 包 + studio + docs + deploy） | 保留 | 上游结构清晰，重排只会制造回归风险 |
| `cmd/hector`（main.go / app.go / serve.go） | 保留 | 单二进制 CLI 是上游核心形态，不移除 |
| `pkg/execution/native` 队列与 Worker Pool | 保留 | 已具备 ack/nack、退避重试、死信、RecoverStale、指标，直接支撑 T2.3 |
| `pkg/checkpoint` 恢复机制 | 保留 | 启动扫描 + WORKING 自动续跑 + 过期置 FAILED，支撑 T5.3 故障注入基础 |
| `pkg/session` 事件与 user_states | 保留 | sequence_num 已是事件续接基础 |
| `pkg/guardrails`（input 注入检测 / output PII / tool 授权 / moderation） | 保留 | 直接满足 SECURITY_REVIEW 第 3 项，避免重复造轮子 |
| `pkg/model/{openai,anthropic,gemini,ollama}` | 保留 | 4 Provider 可切换，满足「模型 Provider 接口」要求 |
| `pkg/rag` + `pkg/vector` + `pkg/embedder` | 保留 | Analyst 的 RAG 能力底座 |
| `pkg/observability`（OTel） | 保留 | 满足 Trace / Metrics / Logs 要求 |
| `pkg/tool/*`（file / command / web / search / mcp / approval / todo / memory） | 保留 | Tool Hub 与审批机制底座 |
| `pkg/config` + `pkg/builder`（YAML 声明式配置 + fluent API） | 保留 | 配置驱动是上游设计核心 |
| `pkg/ratelimit` + `pkg/trigger` | 保留 | 限流与 webhook 触发可复用 |
| `ui/embed.go`（内嵌静态资源 + SPA 回退） | 保留 | 新前端可复用同一内嵌模式 |
| `docs/` mkdocs 文档站 | 保留 | 新产品文档另起 `docs/researchos/` |
| MIT LICENSE | 保留 | 允许二次开发与商用，无需替换协议 |
| `Dockerfile` / `deploy/` / `deployments/` / `grafana/` | 保留 | 部署与监控资产可复用，新增 ResearchOS 编排不覆盖 |

## 2. 修改（在上游代码上做最小侵入改动）

| 项目 | 结论 | 依据 / 验收 |
|---|---|---|
| `pkg/server/events.go` `toInvocationMeta` | 修改：禁止 `metadata.user_id` 覆盖已验证 claims；未认证时不得采用客户端 user_id | 身份隔离修复；验收见 `docs/security-review.md` 与 `pkg/server/identity_isolation_test.go` 改造成「claims 胜出」断言 |
| `pkg/server/events.go` sessionID 来源 | 修改：sessionID 需与 userID 绑定校验，不能仅凭客户端 `context_id` | 防会话固定 / 越权续接 |
| `pkg/task/store.go` Get / List | 修改：查询条件补 `user_id`（或经服务层强制归属校验） | 消除跨用户按 ID 直读；验收：伪造 user_id 读他人任务必须 404/403 |
| `pkg/agent/remoteagent/a2a.go` `propagateMetadata` | 修改：身份相关键统一剥除（`user_id`、`tenant_id` 等），不只 `agent_id`/`author` | 防远端事件污染本地身份 |
| `pkg/auth` 共享密钥校验（`secret.go`） | 修改：共享密钥身份需可区分调用方，补 scope / 过期 / 吊销路径；或明确仅限单用户本地模式 | SECURITY_REVIEW 第 1 项「共享 Token 授权链」 |
| `pkg/server/admin_handler.go` 会话查询 | 修改：Bearer 比较改常量时间；`/admin/sessions?user_id=` 增加审计日志 | 管理面加固，不改变功能 |
| `pkg/config` 数据库方言分支 | 修改：ResearchOS 统一 PostgreSQL（pgx），清理 mysql 分支与新代码路径 | 与 ORM 决策一致（sqlc + PostgreSQL） |
| `pkg/session` 事件结构 | 修改：补全局 `event_id` 幂等键 + 「状态变更与事件同事务 / Outbox」 | 满足 EVENT_PROTOCOL 幂等与重放 |
| `pkg/checkpoint` 粒度 | 修改 + 扩展：在会话级之上加节点级 Checkpoint | 支撑 T2.5 单节点失败隔离与重试 |
| `pkg/execution/native` 队列 | 修改 + 扩展：增加任务级并发上限与暂停闸门 | 支撑 T2.3「单任务默认 4 Agent」与 T2.6 暂停 / 恢复 |

## 3. 新增（上游没有的，按研究域新建，边界独立）

| 项目 | 位置（建议） | 验收依据 |
|---|---|---|
| 研究任务生命周期状态机（Created→Planning→Awaiting Approval→Running→Synthesizing→Completed + Failed/Paused/Cancelled） | `internal/research` | T1.1：非法跃迁被拒绝并产生事件 |
| Research REST API + SSE 网关（`/api/v1/research/*`） | `internal/api` + `cmd/api` | T1.2 / T1.3：openapi 契约测试全过，断线可续接 |
| 独立 Worker 进程 | `cmd/worker`（复用 `pkg/execution/native`） | T2.3：api 与 worker 可独立启动 |
| DAG 编译与校验（环检测、孤节点、拓扑排序） | `internal/workflow` | T2.2：有环 DAG 被拒绝 |
| Planner 服务（结构化输出 + JSON Schema 校验 + 退回逻辑） | `internal/planner` | T2.1：非法 JSON 不进入执行 |
| 6 角色 Agent 注册与分配 | `internal/agent`（包装 `pkg/agent`） | T2.4：Agent 详情来自真实执行 |
| Search Provider 接口 + Mock + 真实 Provider | `internal/search` | T3.1：切换 Provider 不改业务代码 |
| 网页抓取 / 正文解析 / URL 规范化 / 内容哈希去重 / 快照 | `internal/search` | T3.2–T3.3：重复 URL 与重复内容只留一条 |
| Source / Evidence / Claim 模型与交叉验证 | `internal/evidence` | T3.5–T3.6：无来源只能 Unverified；冲突标 Conflicting |
| Report 生成（9 节）+ Markdown/PDF/HTML 导出 + 引用映射 | `internal/report` | T4.2–T4.4：报告 9 节齐全，PDF 非空 |
| 研究域表结构 + migration | `migrations/`（sqlc schema 同源） | T0.7 / T1.1：DATA_MODEL 与 migration 一致 |
| ResearchOS 产品前端（Next.js + React Flow） | `web/` | T1.4 / T1.5：画布状态与后端快照一致 |
| 研究事件映射层 | `web/lib/event-map.ts` | EVENT_PROTOCOL：禁止组件内散落硬编码 |
| ResearchOS 文档与部署 | `docs/researchos/` + `deployments/` | T5.6：全新机器 compose 可跑 |

## 4. 替换（明确不再使用）

| 项目 | 结论 | 依据 |
|---|---|---|
| 开发包自带的空骨架 `cmd/api/main.go`、`cmd/worker/main.go`（panic 占位） | 替换为真实实现，落进 `upstream/hector` 的同名路径 | 用户决策 2：不做与上游脱离的独立骨架 |
| `pkg/config` 中 MySQL 方言路径（仅新代码） | 替换为 PostgreSQL only | ORM 决策 sqlc + PostgreSQL |
| 开发包 `api/openapi.yaml` 模板 | 替换为与真实 handler 同步的契约（保留结构，补齐研究域接口） | AGENTS.md：改 API 必须同步 openapi |
| 开发包 `web/` 空目录 | 替换为真实 Next.js 应用 | T1.4 / T1.5 |
| `deployments/compose.yaml` 模板 | 替换为验证过的真实编排 | T5.6 从零部署验收 |

## 5. 技术债务（记录，不阻塞 Phase 0，排期处理）

| 项目 | 影响 | 处理时机 |
|---|---|---|
| 手写 SQL 双方言字符串切换（`?` vs `$1`） | 易错、难维护 | 迁移 sqlc 时一并收敛为 PostgreSQL |
| 无 migration 框架，schema 靠 `CREATE TABLE IF NOT EXISTS` 内联 | 版本不可追溯，T0.7 要求 migration v001 | Phase 1 引入 golang-migrate |
| `go.mod` 仍含 `go-sql-driver/mysql` | 冗余依赖、攻击面 | 清理 mysql 分支时移除 |
| `studio/` 仅提交源码，`ui/dist` 只有 `.gitkeep` | 源码构建默认无 UI，需 `make build-release` | 保留现状，产品前端独立构建 |
| 37 个测试文件 / 488 文件，身份与权限路径几乎无测试 | 安全回归无护栏 | Phase 0 已补 4 条证据测试；修复后补隔离回归测试 |
| 上游无 OpenAPI 规格（只有 A2A JSON-RPC） | 前后端契约缺失 | Phase 1 建立并纳入 verify.sh |
| admin key 明文常量比较、无轮换 | 管理面风险 | 与身份修复同批处理 |
| A2A `metadata` 命名空间无白名单 | 未来字段冲突与注入面 | 身份修复时同时加白名单 |
| 前端技术栈分歧（studio = Vite+React ≠ DECISIONS 的 Next.js） | 两套前端维护成本 | 见第 6 节，待人工确认 |

## 6. 开发包文件如何融入实际项目（关键决策，待确认）

开发基线是 `upstream/hector`，开发包文件按下列方式并入，**不覆盖上游同名文件**：

| 开发包路径 | 并入目标 | 方式 |
|---|---|---|
| `docs/*.md`（PRD / TASKS / 架构等） | `upstream/hector/docs/researchos/` | 整体搬入子目录，不碰 mkdocs 原站 |
| `docs/hector-audit.md`、`docs/mapping-table.md`、`docs/security-review.md` | 同上 | 同上 |
| `api/openapi.yaml` | `upstream/hector/api/openapi.yaml` | 新建目录，避免与上游冲突 |
| `cmd/api`、`cmd/worker` | `upstream/hector/cmd/api`、`cmd/worker` | 新增二进制，保留 `cmd/hector` |
| `internal/*` | `upstream/hector/internal/*` | 新产品域，独立于 `pkg/` |
| `web/` | `upstream/hector/web/` | 新产品前端 |
| `deployments/compose.yaml` | `upstream/hector/deployments/researchos.compose.yaml` | 不覆盖上游部署清单 |
| `scripts/*.sh` | `upstream/hector/scripts/researchos-*.sh` | 加前缀，避免与上游脚本同名 |
| `Makefile`（开发包） | `upstream/hector/Makefile.researchos` | 用 `make -f` 调用，不覆盖上游 Makefile |
| `AGENTS.md` / `CODEX.md` / `PROGRESS.md` | `upstream/hector/docs/researchos/` | 协作规范与进度账本随项目走 |

**待人工确认的技术选型分歧：**
1. 前端：上游 Studio 是 Vite + React；DECISIONS 锁定 Next.js + React Flow。建议 Studio 保留不动，ResearchOS 产品界面对齐 DECISIONS 新建。若改为直接扩展 Studio，需同步修订 DECISIONS 第 5 条。
2. 数据访问：已锁定 sqlc + PostgreSQL（用户决策 7），需同步修订 DECISIONS 第 4 条中的存储表述与新增决策条目。
