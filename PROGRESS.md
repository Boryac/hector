# PROGRESS.md — 进度账本（每次迭代必须更新）

## 当前状态
- 运行模式：**全自主迭代开发**（2026-11-04 起，见 `DECISIONS.md` 第 14 条），取消逐阶段人工审批
- 当前 Phase：Phase 5 收尾 + 补强任务（T6.x）
- 已完成：T0.1–T0.7、T1.1–T1.5、T2.1–T2.6、T3.1–T3.6、T4.1–T4.6、T5.1–T5.5、T5.7
- 部分完成：T4.4（缺 PDF）、T5.6（无 Docker，Compose 交付但未本机验证）
- 下一个任务：T6.1（PDF 导出）→ T6.2（覆盖率/基准）→ T6.6（Docker）→ T6.7（README/架构/部署）
- 待人工提供：LLM API Key、搜索 Provider Key（当前启发式降级，报告与 UI 均如实标注）、Docker 安装
- 已知环境限制：本机无法启动 GUI/无头浏览器（Chrome/Edge 均无输出），**截图需人工在普通桌面执行**

## 迭代记录（每轮追加一条，不覆盖历史）

### 2026-11-03 — [phase-0][T0.1] Fork 与仓库接入
- 做了什么：`git clone https://github.com/verikod/hector.git` 到 `upstream/hector/`（保留完整上游历史）；自 `main` 建 `feature/hector-researchos` 分支；记录基线 commit `abe472b6deb12b3dbba290dbec44cd3be27e163d`（2026-05-06，`fixed agent tool calling/streaming`）。
- 测试结果：`git clone` 成功；`git branch --show-current` → `feature/hector-researchos`；受跟踪文件 488 个。
- 遗留问题 / 假设：远程推送地址与凭据后补，不阻塞。
- 下一步：T0.2 模块映射。

### 2026-11-03 — [phase-0][T0.2] 原生结构与模块映射
- 做了什么：通读 `cmd/`、`pkg/`（33 个顶级包、277 文件）、`studio/`（122 文件 Vite+React）、`ui/embed.go`、`deploy*/`；产出 `docs/hector-audit.md`（目录树、模块能力映射、队列/Checkpoint 现状、环境基线）与 `docs/mapping-table.md`（保留 17 项 / 修改 10 项 / 新增 14 项 / 替换 5 项 / 技术债 9 项 + 开发包文件并入方案）。
- 测试结果：`go build ./...` 通过（exit 0）。
- 遗留问题 / 假设：上游前端技术栈（Vite+React）与 DECISIONS 第 5 条（Next.js + React Flow）冲突，建议 Studio 保留、产品前端另建，待确认。
- 下一步：T0.3 身份隔离审查。

### 2026-11-03 — [phase-0][T0.3] A2A 身份隔离审查（Issue #2）
- 做了什么：定位身份定稿点 `pkg/server/events.go:31-68`，确认 `metadata.user_id` 无条件覆盖已验证 JWT claims；核查共享密钥（固定 Subject `admin-via-secret`）、admin key 明文比较、task/session 读接口无 user 过滤、remoteagent metadata 透传；新增可运行证据测试 `pkg/server/identity_isolation_test.go`（4 条），结论写入 `docs/security-review.md`。
- 测试结果：`go test ./pkg/server/ -run TestToInvocationMeta -v` → 4/4 PASS，日志确认 `authenticated user "alice" is replaced by forged metadata user_id "bob"`、`unauthenticated request adopts arbitrary user_id "victim"`。
- 遗留问题 / 假设：结论为「隔离不成立」，修复方案 7 项已列入 security-review 第 1.6 节；修复前多用户功能禁用。
- 下一步：T0.4 队列与恢复审查。

### 2026-11-03 — [phase-0][T0.4] 队列 / Checkpoint / 恢复审查
- 做了什么：核查 `pkg/execution/native`（ack/nack、指数退避、死信、RecoverStale、WorkerPool 指标、ShutdownTimeout 30s、StaleRecoveryInterval 1m）与 `pkg/checkpoint`（RecoverPendingTasks、WORKING 自动续跑、过期置 FAILED）；列出 6 项相对 PRD 缺口写入 `docs/hector-audit.md` 第 4 节。
- 测试结果：`go test ./pkg/server/... ./pkg/auth/... ./pkg/session/... ./pkg/task/... ./pkg/checkpoint/...` → 5 包全 ok。
- 遗留问题 / 假设：缺节点级 Checkpoint、任务级并发上限、暂停闸门、全局 event_id 幂等键；均列入 T2.x 范围。
- 下一步：T0.5 技术选型锁定。

### 2026-11-03 — [phase-0][T0.5] 技术选型锁定
- 做了什么：按人工决策锁定 **sqlc + PostgreSQL**；`DECISIONS.md` 新增第 11（sqlc）、12（上游为基线）、13（身份未修复不出多用户）条。
- 测试结果：文档一致性检查（DECISIONS 与 mapping-table 第 5/6 节口径一致）。
- 遗留问题 / 假设：历史手写 SQL 双方言在迁移时收敛，MySQL 分支清理排期。
- 下一步：T0.6 测试基线。

### 2026-11-03 — [phase-0][T0.6] 测试与开发环境基线
- 做了什么：跑通上游构建与核心包测试；跑 `bash scripts/verify.sh`（开发包骨架）确认 openapi 解析正常、go/web 检查按预期跳过。
- 测试结果：`go build ./...` exit 0；核心 5 包 ok；`verify.sh` 输出 `ok: openapi yaml parses`。
- 遗留问题 / 假设：Docker 未安装（按决策推迟 Phase 5）；本地无 PostgreSQL，Phase 1 起用 Testcontainers 或远程实例。
- 下一步：T0.7（migration v001）建议并入 Phase 1 首任务，**Phase 0 到此暂停**。

## 风险与决策变更
- 2026-11-03：建议新增「上游前端技术栈与 DECISIONS 第 5 条冲突」的处理方案（Studio 保留 / 产品前端另建 Next.js），待人工确认后补入 DECISIONS。
- 2026-11-03：T0.7 未完成（migration v001 未建），不属于跳过验收，属范围调整提议，待人工确认。

### 2026-11-03 — [phase-1][T0.7] 数据库迁移与 sqlc 查询层
- 做了什么：新增 `sqlc.yaml`；写 `internal/researchos/db/migrations/0001_init.sql`（13 表，含 `research_tasks.latest_sequence`、`workflow_nodes.depends_on TEXT[]` / `lease_expires_at` / `attempts` / `next_attempt_at`）；写 `queries/{tasks,nodes,events,sources,claims,reports,misc}.sql`；`db/migrate.go` 用 `go:embed` 内嵌迁移并在 `schema_migrations` 记账。
- 测试结果：`go run ./cmd/researchos migrate` 在本机 PostgreSQL 16.2（`127.0.0.1:55432`）执行成功，`schema_migrations` 记录 `0001_init.sql`；`sqlc generate` 通过。
- 过程中的坑与修复：`nodes.sql` 的 `FailOrphanNodes` 报 `column reference "task_id" is ambiguous`（加表别名）；无名参数被推断成 `Column3/4/5`（全部改 `sqlc.arg('x')::type`）；`SetTaskStatus` 的 `CASE WHEN $2` 导致类型推断成 `interface{}` 更新失效（改具名参数）。
- 遗留问题 / 假设：本机无 Docker，用 Maven zonky 便携二进制在 `.devtools/` 内起库（端口 55432），不污染系统环境。
- 下一步：Phase 1 状态机与 API。

### 2026-11-03 — [phase-1] 任务生命周期 + REST + SSE + 工作台前端
- 做了什么：`domain`（模型/常量）、`store`（状态变更与事件同事务）、`api`（research CRUD、approve/pause/resume/cancel/plan/retry、SSE、sources/evidence/report/agents/tools/settings）、`worker`（tick 循环 + 认领 + 执行 + 收尾）。前端新建 `web/`（Next.js 14 + React 18 + TS + React Flow 11）：Dashboard、任务详情（DAG 画布、节点列表、5 个 Tab）。
- 测试结果：`curl` 建任务 → 规划 → 批准 → 8/8 节点完成 → 报告 v1（19 条引用、1284 字、7 章节）；`GET /events?format=json` 返回连续 `sequence`。
- 过程中的坑与修复：Next.js 构建在 Windows 沙箱下写 `.next/trace` 报 EPERM → 新增 `scripts/next-build.cjs`，用 `Module._load` 钩子把 `trace/report/to-json` 换成 no-op；`Markdown.tsx` 的 `flatMap` 返回类型不兼容 → 改显式 `push`。
- 遗留问题 / 假设：静态产物目录为 `web/.build`（`output: 'export'` + `distDir: '.build'`），由 `RESEARCHOS_WEB_DIR` 托管。
- 下一步：Phase 2 规划与调度。

### 2026-11-03 — [phase-2] Planner / DAG / WorkerPool / 重试
- 做了什么：`planner`（Heuristic 模板 DAG + LLM 结构化输出 + Fallback 降级 + Normalize 校验）、`dag`（Validate/TopoSort/BuildEdges/Ready/Depth）、`worker`（并发上限、节点超时、退避 1/2/4s、`FailOrphanNodes` 级联、看门狗收敛）。
- 测试结果：`dag` 8 条、`planner` 14 条单测全绿（含环、重复 ID、自依赖、synthesize 收尾约束）；真实任务 8 节点全部完成，单节点失败可重试并产生新事件。
- 遗留问题 / 假设：未复用上游 `task_queue`（FIFO 无依赖语义），改用 `workflow_nodes` 依赖驱动队列，已写入 `DECISIONS.md` 第 15 条。
- 下一步：Phase 3 检索与证据。

### 2026-11-03 — [phase-3][phase-4] 真实检索 / 抓取 / 交叉验证 / 报告
- 做了什么：`search`（GitHub / Hacker News Algolia / Wikipedia / Bing / Tavily·SerpAPI·Brave，并发调用 + trace 降级）、`fetch`（robots.txt 解析与缓存、SSRF 防护、大小/超时限制、快照落盘）、`textutil`（URL 规范化、SHA256 指纹、正文抽取、中英文关键词）、`exec`（search→fetch→analyze→verify→synthesize，`quoteGrounded` 逐字校验，Jaccard 聚类 + 站点数 + 数值冲突）、`report`（结构化章节 + 引用编号稳定映射 + 越界引用剔除 + HTML 转义）。
- 测试结果：真实网络检索跑通，产出 28 条资料；报告含 19 条引用，每条都指向真实 URL。
- 暴露的质量问题（如实记录）：GitHub 匿名配额 403；Bing 返回百度经验/知乎等无关结果（robots 禁抓）；Wikipedia 仅 snippet；导致结论几乎全为 `partially_supported`。
- 下一步：质量修复 + 测试补齐。

### 2026-11-04 — [autonomous] 进入全自主模式 + 状态机落地
- 做了什么：`DECISIONS.md` 新增第 14–17 条（自主模式、不复用上游 FIFO 队列、自建 LLM 薄客户端、产品前端与 Studio 并存同源托管）；`TASKS.md` 重写为 `[x]/[~]/[ ]` 三态并新增 T6.x 补强任务；新增 `domain/transitions.go`（状态机白名单 + `ErrInvalidTransition` + `EventForTransition`）；`store.TransitionTask` 成为唯一状态变更入口（校验失败写 `research.transition_rejected` 审计事件，API 映射 409）；`api` 的 approve/pause/resume/cancel/retry/planAsync 全部改走状态机；worker 的 synthesizing/completed/failed 也走状态机。
- 测试结果：`go test ./internal/researchos/domain/...` 7 条用例全绿（白名单允许 15 组、拒绝 6 组、幂等、终态判定、事件映射）。
- 过程中的坑与修复：`planAsync` 原用 `from=""` 绕过状态机，改 `TransitionTask`；`failed` 任务重规划需要 `failed → planning`，据此扩白名单；已完成任务重试节点改为 409「请新建任务」。
- 遗留问题 / 假设：状态机表是硬编码常量，新增状态必须同步 `TaskStatuses()` 与文档。
- 下一步：前端托管与端到端可见。

### 2026-11-04 — [phase-1] 前端同源托管打通
- 做了什么：`web/.build` 作为静态产物目录（`output:'export'` + `distDir:'.build'`）；`withAuth` 调整为「`/api/` 前缀必须鉴权，其余静态资源免鉴权」；`cmd/researchos` 增加 `.env.researchos` 自动加载（`config.LoadEnvFile`，shell 变量优先）；`lib/api.ts` 的导出改为带 Authorization 的 fetch + Blob 落盘，Token 不再进 URL。
- 测试结果：`curl /` → 200 5755B；`/task/` → 200 5417B；`/_next/static/...` → 200；`/api/v1/research` 无 token → 401；`/api/v1/health` → 200 且返回迁移列表与 pgxpool 统计。
- 过程中的坑与修复：静态页最初被鉴权中间件挡成 401（浏览器无法为页面附加 Authorization 头）→ 静态免鉴权 + 数据接口全部鉴权；`bearerToken` 的 query token 仅对 `/events` 放行，因此报告导出改用 fetch 下载。
- 遗留问题 / 假设：本机无法启动 Chrome/Edge 无头（`--version` 都无输出），**截图验证需人工执行** `scripts/screenshot.sh`。
- 下一步：单测补齐。

### 2026-11-04 — [phase-5][T5.1] 单测补齐与真实缺陷修复
- 做了什么：新增 `dag`(8)、`planner`(14)、`textutil`(11)、`report`(8)、`search`(7)、`exec`(8)、`fetch`(13)、`config`(9)、`api`(8)、`domain`(7) 共 10 个测试文件；测试过程中发现并修复 6 个真实缺陷（见下）。
- 测试结果：`go test ./...` 全仓通过（含上游 `pkg/...`）。
- 修复的真实缺陷：
  1. `Excerpt` 混用字节下标与 rune 长度，中文文本输出超长（82 > 80 rune）→ 全程 rune 下标 + 为省略号预留 2 rune。
  2. 维基模板 `{{flatlist|...}}` 未剥离，正文里残留 `{{`/`[[` → 新增 `wikiTplRe`/`wikiTblRe` 逐层剥离。
  3. `Sentence` 的 `len >= 10` 门槛让合法短中文句（「第二句话。」）返回空 → 放宽为 `>=6` 或「以句末标点结尾且 `>=4`」。
  4. `Dedupe` 放行 `javascript:` 等非 http(s) URL，会把可注入链接带进来源库与报告 → 规范化失败一律丢弃。
  5. `quoteGrounded("")` 因 `strings.Contains(x, "")` 恒真而返回 true → 引文去空白后 `<6` 字符判为无据。
  6. `parseRobots` 空行不重置分组，缺失 `User-agent` 声明的 `Disallow` 会继承上一组 → 空行重置 `active`。
- 另外加固：`fetch` 增加 `guardTransport`（Dial 阶段校验真实 IP，挡 DNS rebinding / `127.0.0.1.nip.io` / 云元数据地址）与 `guardResolved`；JSON 响应改为提取语义字段（`title/description/body/...`）而不是把原始 JSON 当正文。
- 上游遗留问题顺手修复：`pkg/rag/blob_source_test.go` 用 `"file://" + tmpDir` 在 Windows 下生成非法 URL（被解析成 host=C + 非法端口），改为 `fileURL()` 助手；`go test ./...` 因此全绿。
- 遗留问题 / 假设：覆盖率百分比尚未实测落 `docs/benchmark.md`（T6.2）。
- 下一步：Issue #2 修复与回归。

### 2026-11-04 — [phase-5][T5.5] Issue #2 身份伪造修复
- 做了什么：`pkg/server/events.go` 删除 `metadata["user_id"]` 覆盖 claims 的分支，改为「有 claims 只用 claims.Subject / 无 claims 归入常量 `anonymous` / metadata 出现身份键记审计日志」；`pkg/server/admin_handler.go` 的 admin key 比较改 `subtle.ConstantTimeCompare` 且空密钥直接拒绝；`identity_isolation_test.go` 从「证明漏洞」翻转为「锁定修复」，6 条用例。
- 测试结果：`go test ./pkg/server/ -run TestToInvocationMeta -v` → 6/6 PASS，日志可见 `ignored client-supplied user_id in message metadata`。
- 遗留问题 / 假设：会话归属校验（`context_id` → owner）与上游 `pkg/task` 的 user 过滤未做，因此**多用户模式仍禁用**，与 `DECISIONS.md` 第 13 条一致。状态已写入 `docs/security-review.md` 第 1.7 节。
- 下一步：契约与数据模型文档。

### 2026-11-04 — [phase-0][T0.7] 契约与数据模型文档补齐
- 做了什么：`api/openapi.yaml` 从 79 行占位重写为 OpenAPI 3.1（24 个 path、15 个 schema，覆盖 research/events/evidence/report/tools/settings，显式写明身份语义、状态机 409、SSE 续接两种方式、Token 不进 URL）；新增 `docs/DATA_MODEL.md`（13 表字段、状态机全矩阵、事件类型表、`ClaimReadyNodes` 的 SKIP LOCKED/租约/暂停闸门语义、恢复路径、索引理由、已知限制）。
- 测试结果：`python -c "yaml.safe_load(...)"` 解析通过（24 paths / 15 schemas）；文档中的状态矩阵与 `domain/transitions.go` 逐格核对一致。
- 过程中的坑与修复：`description: 报告中的 [n] 编号` 里的 `[` 被 YAML 当作流式序列起始导致解析失败 → 加引号。
- 遗留问题 / 假设：`scripts/verify.sh` 尚未接入 openapi 与 Go 路由的一致性检查（T6.x）。
- 下一步：PDF 导出、覆盖率、Docker、README。

### 2026-11-04 — [phase-5][T6.2] 基准与覆盖率实测落盘
- 做了什么：`docs/benchmark.md` 从 4 行占位重写为实测报告：5 轮并发压测对照表、两种失败模式的根因、本轮 7 项质量修复清单、10 个包的覆盖率实测值、未实测项与原因、已知性能限制、复现命令。同时补采集一次 6 并发压测与最终 5 并发压测。
- 测试结果：`bash scripts/load-test.sh 5 quick` → 4/5 成功（80%），P50 13s、P95 17s、平均 6 节点 / 67 事件；`go test -cover ./internal/researchos/...` → dag 96.2% / domain 90.5% / config 79.8% / report 76.7% / textutil 72.8% / planner 72.5% / fetch 37.8% / exec 28.9% / search 19.7% / api 3.7%。
- 过程中的坑与修复：`go test -cover` 对无测试文件的 4 个包报 `cmd/internal/cov is not in std`（Go 1.27.3 与 go.mod 声明不一致的工具链问题），`go test ./...` 正常，已在文档写明；第 3 轮 6 并发 P50 从 13s 退化到 52s，定位为外部检索配额而非本地调度，如实记录不调小数字。
- 遗留问题 / 假设：api / store / worker / db 无单测，语义靠压测与故障注入覆盖，已在文档说明不堆 sqlmock 的理由。
- 下一步：故障注入脚本化。

### 2026-11-04 — [phase-5][T6.4] 故障注入实测（PASS）与三处真实修复
- 做了什么：重写 `scripts/fault-injection.sh`（自读 `.env.researchos`、用 `ps -W` 的 WINPID 终止真实进程、每步校验 API 可达、验收不变量改为「停机前已完成的节点 attempts 不得变化」）。
- 测试结果：`RESEARCHOS_STALE_LEASE=40s bash scripts/fault-injection.sh` → **PASS**。过程：4 个节点完成 → 强杀 worker（WINPID 17376）→ 确认除 serve 外无残留进程 → 等租约过期 → 起 worker2 → t+15s 任务收敛 completed → 8/8 节点 completed，停机前已完成的 4 个节点 attempts 仍为 1，被打断的 `search_2` 重跑一次（attempts=2）。
- 过程中的坑与修复（全部是脚本/实现层的真问题）：
  1. 脚本没读 `.env.researchos`，Token 取默认值 → 所有 API 调用静默 401，整段在等一个拿不到状态的接口。
  2. `taskkill //PID $!` 用的是 MSYS pid 而非 Windows pid，杀错进程（实测把 serve 杀掉）；`kill -9 $!` 只杀 MSYS 外层壳，真正的 `researchos.exe` 变孤儿继续跑并继续续租 → 改用 `ps -W` 取 WINPID 后 `taskkill //F //PID`。
  3. `exec/exec.go` 的心跳续租硬编码 `3*time.Minute`，`RESEARCHOS_STALE_LEASE` 调小也不生效，崩溃恢复永远等 3 分钟 → 新增 `exec.Config.LeaseTTL` 并由 `cfg.StaleLease` 注入。
  4. 验收不变量写错：把「被打断节点的重跑」也算成回归 → 改为只校验停机前已完成的那批节点。
- 遗留问题 / 假设：Windows 下必须用 WINPID，脚本已封装 `winpids()`/`kill_winpid()`，Linux 下走 `taskkill` 不存在时会退化为直接返回失败，未在 Linux 验证。
- 下一步：依赖漏洞扫描。

### 2026-11-04 — [phase-5][T6.3] govulncheck 扫描与依赖升级
- 做了什么：新增 `docs/dependency-scan.md`。研究域首扫 3 条可达漏洞（pgx SQL 注入 GO-2026-5004、x/text 死循环 GO-2026-5970/GO-2026-6629），升级 `pgx v5.9.2` + `x/text v0.41.0` 后清零；全模块首扫 20 条，批量抬依赖版本后降到 2 条。
- 测试结果：`govulncheck ./internal/researchos/...` → **No vulnerabilities found**；`go build ./...` 通过；`go test ./internal/researchos/...` 10 个包全绿；升级后重启服务做冒烟：health ok、quick 任务 completed、`report.md` 200。
- 过程中的坑与修复：`go get x/text@v0.39.0` 会把 `go` 指令从 1.24.4 抬到 1.25.0 并移除 `toolchain` 行 → 同步把 `Dockerfile.researchos` 改成 `golang:1.25-alpine` 并写进 `DECISIONS.md` 第 18 条；`go mod tidy` 因 `go.yaml.in/yaml/v3` 下载时 TLS 报错而留下 missing go.sum → 单独 `go get go.yaml.in/yaml/v3@v3.0.5` 后重跑 tidy 修复。
- 遗留问题 / 假设：剩余 2 条（otel OTLP 导出器日志泄露、aws-sdk EventStream panic）属上游未发版修复，且只经包 init 进入调用图、不在 researchos 运行路径上；已如实记录，未标成已修复。
- 下一步：架构与部署文档、演示脚本、最终交付报告。

### 2026-11-04 — [phase-5][T6.7/T6.8] 架构文档重写 + 演示实录
- 做了什么：`ARCHITECTURE.md` 从 112 行规划稿重写为「已实现架构」（实测行数的代码地图、一次研究的完整链路、节点类型与失败语义、状态机、事件与 SSE、并发与恢复参数、检索与抓取三层 SSRF 防护、前端结构、身份边界、与上游的关系）；新增 `docs/demo/`（真实报告 md + html 原样保存）；`docs/demo.md` 写成真实运行记录；`DEMO_SCRIPT.md` 重写为可执行分镜。
- 测试结果：演示任务 `aa1af51c-7b20-4578-9065-0dff96faebe3` → completed，8s，8/8 节点，39 条来源（18 条抓到正文，wikipedia 13 / github 11 / hackernews 10 / bing 5），110 条事件，报告 4895B / HTML 7270B。
- 过程中的坑与修复（报告质量的真实问题，逐条修完再复跑）：
  1. 结论是书名罗列 → `usableStatement` 增加《》成对、分隔符密集、多句拼接的拒绝规则。
  2. 结论是 GitHub 界面文案（"Notifications You must be signed in…"）与维基编者提示（"This article is about…" / "redirects here"）→ 加入界面文案黑名单。
  3. 结论以半个单词开头（"s AI agent deleted…"）→ 新增 `startsWithSentenceCase`，并把 `Excerpt` 的窗口边界对齐到句子边界（`nextBoundary`/`prevBoundary`）。
  4. 长度上限 120 字把英文正常长句全判掉（报告一度只剩 3 条结论）→ 改成「≤300 字且句末标点 ≤2」的语义判据。
- 遗留问题 / 假设：无 LLM 时规则抽取做不到「筛选 10 个项目并做竞争分析」，报告里 7 条结论多为单一来源简介、部分偏营销句；已在报告「方法与局限性」、`docs/demo.md` §2.1、`docs/benchmark.md` §4 三处写明。截图与录屏需图形界面，未产出。
- 下一步：最终交付报告与 Git 提交。

### 2026-11-03 — [docs] 新增使用手册 docs/USAGE.md
- 做了什么：按界面真实点击顺序写使用手册（确认在跑 → 打开界面 → 建任务 → 批准 → 看五个 Tab → 导出报告 → API 调用 → FAQ → 停止清理）。README 增加入口与文档索引条目。
- 事实核对：写完后逐条对照代码核了三个容易写错的点并改掉——端口变量是 `RESEARCHOS_ADDR`（不是 `RESEARCHOS_PORT`）、LLM 变量是 `RESEARCHOS_LLM_API_KEY` + `RESEARCHOS_LLM_PROVIDER`、来源状态是 `robots_blocked`（不是 `robots_denied`）；补充 `RESEARCHOS_REQUIRE_APPROVAL` 对批准闸门的控制，以及 `RESEARCHOS_ADDR` 改端口的方式。
- 遗留问题 / 假设：手册里的耗时区间（quick 10–20s、standard 15–40s）取自本机此前实测任务，非本轮重新采集；外部检索配额波动时会明显偏长，已在正文标注为「本机实测」。

### 2026-11-03 — [T7.1] 统一 LLM 配置变量名
- 做了什么：重写 `scripts/bootstrap.sh`（原来生成的模板是 `DATABASE_URL` / `LLM_API_KEY` / `SEARCH_PROVIDER` 一整套无前缀变量名，服务一个都读不到，等于给了份废配置）；健康检查文案改为 `RESEARCHOS_LLM_API_KEY`；README 配置表把 `_API_KEY` / `_MODEL` 的简写拆成显式全名。
- 测试结果：`bash scripts/bootstrap.sh` 实测输出 go 1.27.3 / node v22.22.2 / psql 缺失提示，`.env.researchos` 已存在时保持不变；全仓 grep 复查无残留（排除抓取快照与上游 `pkg/` 的 `OPENAI_API_KEY`）。
- 过程中的坑与修复：脚本第一版 `check go "--version"` 传参错误（go 用 `go version`），实测报 `flag provided but not defined: -version`，改为按命令全串执行。
- 下一步：SSE ticket。

### 2026-11-03 — [T7.2] SSE 一次性 ticket
- 做了什么：新增 `api/ticket.go`（进程内表、sha256 存 key、TTL、单任务上限 16、一次性、绑任务）；`withAuth` 改为方法并加入 ticket 通道，`bearerToken` 彻底不再读查询串；新增 `POST /api/v1/research/{id}/events/ticket`；配置项 `RESEARCHOS_SSE_TICKET_TTL`（默认 60s，强制上限 10 分钟）；前端 `eventsURL(taskId, after, ticket)`，每次（重）连前重新签发。
- 测试结果：`go test -race ./internal/researchos/api/...` 24 条全绿（ticket 13 + 鉴权 11）。线上一串实测：签发 200（43 字符）→ 建流 200 → 重放 401 `sse_ticket_invalid` → 跨任务 403 `sse_ticket_scope` → 2s TTL 实例等 3s 后 401 `sse_ticket_expired` → 当 Bearer 调普通接口 401 → `?token=` 401。
- 过程中的坑与修复：`auth_test.go` 原来断言「SSE 接受 query token」，与本轮目标相反——改写为「长期 Token 走查询串必须被拒」，并补「ticket 不能被非 events 接口消耗」的断言。
- 下一步：LLM 链路验证。

### 2026-11-03 — [T7.3] LLM 链路验证（无真实 Key）
- 做了什么：新增 `scripts/mock-llm.py` 作为本地 OpenAI 兼容测试替身，按提示词类型返回预置 JSON，并**故意**注入一条编造引文 + 一条不存在的 `source_id` 来检验引擎的校验是否真的生效。以 `RESEARCHOS_ADDR=127.0.0.1:8082 + LLM_PROVIDER=openai + LLM_BASE_URL=.../v1` 起第二个实例跑任务 `b16455ba`。
- 测试结果：`planner_mode=llm`；3 个 analyze 节点 `llm_used=true`、`dropped_ungrounded=2`/次（编造与伪造各 1 条被丢）、`kept=6/4/4`；verify 收 14 条、synthesize 出报告、报告 model 标注 `mock-1`；8/8 节点 completed，18s；报告中不含编造引文与伪造 source_id。补 `llm_test.go` 8 条，`llm` 覆盖率 0 → 94.5%。
- 过程中的坑与修复：发现 LLM 规划的 `query` 被 api 层丢弃（检索式永远来自模板），新增 `domain.Subtask.Query` + `planner.NodeSearchQuery` 修正，并补 2 条回归用例。
- 遗留问题 / 假设：没有真实 Key，**真实模型质量未验证**；测试替身只能证明链路通。
- 下一步：复杂任务与引用验收。

### 2026-11-03 — [T7.4] 复杂研究任务与引用可追溯性
- 做了什么：跑 `ee9d550d`（standard + 交叉验证，目标「对比 pgvector / Qdrant / Milvus 在千万级向量下的召回率与写入吞吐」）；新增 `scripts/verify-citations.py` 做五项检查（引文逐字落地、来源不悬空、编号闭合、冲突披露自洽、核心发现可回溯）。
- 测试结果：8/8 节点、18 条结论 / 18 条证据、18s；验收 PASS：18/18 逐字落地、18 个编号一一对应、核心发现 10/10 可回溯。
- 过程中的坑与修复（真缺陷）：`textutil.Sentence` 在文本末尾补「。」、截断补「…」，引文因此不再等于原文；旧样本 7 条证据里 2 条被判无据（Codex、Generative AI 词条）。去掉合成字符，改纯 rune 截断，补 `TestSentenceIsVerbatimSubstring`。
- 下一步：交叉验证集成测试。

### 2026-11-03 — [T7.5] 交叉验证与冲突识别（集成测试）
- 做了什么：真实网页数据几乎不会给出两条字面相近的结论，`supported` / `conflicting` 两条分支长期没有实际到达过。新增 `exec/verify_integration_test.go`：连真实库，造 4 个来源 + 5 条结论（双来源同事实、同指标冲突数值、无证据），直接跑 `runVerify` 再读回状态。`RESEARCHOS_TEST_DSN` 未设时跳过，不影响常规测试。
- 测试结果：`clusters=3 supported=2 conflicting=2 unverified=1`，冲突双方共享冲突组 `conflict:7733b8e641`；四条结论状态与预期一一对应。
- 过程中的坑与修复：`UpsertSource` / `InsertClaim` / `InsertEvidence` 都不自己生成主键，测试里没填 ID 直接撞 pkey；`Session` 的 `store` 字段未导出，构造字面量后 `Progress` 空指针 panic——改为同包注入真实 store。
- 下一步：重试语义与恢复。

### 2026-11-03 — [T7.6] 中断恢复、节点重试、事件续传
- 做了什么：重跑故障注入；修 `fault-injection.sh` 的 PID 识别；把验证状态判定抽成 `verificationStatus` 并补 7 条分支用例；新增 `exec.PermanentError` 让确定性失败不重试。
- 测试结果：故障注入 PASS（8/8 节点，杀前已完成的 `search_3`/`analyze_3` attempts 仍为 1，任务在 worker 重启后 5s 收敛 completed）。事件续接：`?after_sequence=10` → id 11 起、`Last-Event-ID: 50` → 51 起，116 条事件序列无空洞、`event_id` 唯一。重试：修复前 `synthesize` 确定性失败 attempts=3，修复后 attempts=1；人工 retry 202 → attempts=2 再次失败；对已完成任务节点 retry → 409 `invalid_state`。
- 过程中的坑与修复：`new_winpid()` 用 `comm -13` 做集合差，但 `comm` 要求字典序、`winpids` 用 `sort -n`，实测报 `comm: file 2 is not in sorted order` 并抓错 PID（worker2 抓到的竟然是 serve 的 PID）；改为直接遍历做集合差。
- 下一步：文档与提交。

### 2026-11-03 — [T7.7] 手册、契约与安全文档同步
- 做了什么：`docs/USAGE.md` 增加 ticket 四条硬规则表、ticket 报错排查、LLM 开启后三处可见变化、验收脚本清单（`verify-citations.py` / `load-test.sh` / `fault-injection.sh` / `mock-llm.py`）；`api/openapi.yaml` 加 ticket 端点与 401/403 语义；`docs/security-review.md` 新增 §1.8（泄露面、方案、验证表、剩余风险）；`ARCHITECTURE.md` §5 重写；README 状态表按本轮实测改（含「LLM 链路已验证 / 真实质量未验证」的区分）。
- 测试结果：`python -c "yaml.safe_load(openapi.yaml)"` 通过，25 条 path、ticket 端点与 events 参数可读；`tsc --noEmit` 与 `next build` 通过。
- 遗留问题 / 假设：真实大模型质量、Docker、浏览器截图仍是未验证项，已在 README 与 `docs/benchmark.md` §3.1 标注。
