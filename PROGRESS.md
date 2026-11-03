# PROGRESS.md — 进度账本（每次迭代必须更新）

## 当前状态
- 运行模式：**全自主迭代开发**（2026-11-04 起，见 `DECISIONS.md` 第 14 条），取消逐阶段人工审批
- 当前 Phase：Phase 9 前端视觉系统重构（2026-11-03 人工指派）
- 已完成：T0.1–T0.7、T1.1–T1.5、T2.1–T2.6、T3.1–T3.6、T4.1–T4.6、T5.1–T5.5、T5.7、T6.2–T6.9、T7.1–T7.8、T8.1–T8.11、T9.1–T9.8
- 部分完成：T4.4（缺 PDF）、T5.6（无 Docker，Compose 交付但未本机验证）、T6.10（路由级浏览器审计已完成，全业务流 E2E 未做）
- 下一个任务：T6.1（PDF 导出）→ T6.10 余项（创建到导出的业务流 E2E）
- 待人工提供：LLM API Key、搜索 Provider Key（当前启发式降级，报告与 UI 均如实标注）、Docker 安装
- 浏览器验证：无头 Chrome + CDP（`127.0.0.1:9333`）可用；`web/scripts/ui-audit.mjs` 可重复跑 7 路由 × 桌面/移动，截图落 `docs/screenshots/`

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

### 2026-11-03 — [T7.8] 提交后复验：校验脚本与验证措辞的三处真缺陷
- 做了什么：双仓提交后再跑一遍端到端，用 LLM 模式任务（测试替身）复核冲突披露，连着抓出三个缺陷并修掉——① `verify-citations.py` 把状态名写成 `conflicted` / `verified`（真实值为 `conflicting` / `supported`），两个键都不存在，冲突检查恒真 PASS；② `detectConflict` 只数数值个数不看来源页数，同一站点两条结论数值相同也写「多来源数值一致」；③ 相对差异在最小值为 0 时把分母换成 `1e-9`，全零算出 `-100%`、`0 vs 23.6` 算出 `2359999999900%`。
- 测试结果：`TestDetectConflictSingleSourceWording` / `TestRelativeSpreadEdgeCases` 新增并通过；`go test ./internal/researchos/...` 全绿，`go vet` 干净。线上三连跑（`fd08c2b0` → `082a3def` → `aad4e77b`）逐次验证措辞收敛：最后一条为「不同来源页面上的数值差异显著（最小 0 / 最大 23.6，相对差异 100%，涉及 2 个来源页面）」，配置项状态同时给出「仅 1 个来源页面，数值无跨来源可比」。`verify-citations.py` 对 `aad4e77b` 判 PASS，对篡改副本判 FAIL（`--md-file` 负向验证）。
- 过程中的坑与修复：`b16455ba` 旧数据里 `source_count=1` 与 notes 的「多来源」自相矛盾，由此才顺线查到 ②③ 两处；`/tmp` 在 Windows Python 下不可写（Git Bash 的 `/tmp` 与 Windows Python 不互通），中间产物统一落 `.devtools/`。
- 遗留问题 / 假设：真实大模型质量仍未验证（无 Key），这三处修复本身不改变「测试替身不做推理」的事实。

### 2026-11-03 — [T8.1] 设置中心现状审查（先看清已有什么，再动手）
- 做了什么：把现有「配置」相关代码全量过一遍，结论是能复用的很薄、要补的很多——`GET/PUT /api/v1/settings` 只有 4 个键（addr / llm_provider / llm_model / llm_api_key 等），`default_depth` 只做校验不接受写入，MCP 仅支持登记与 start/stop，前端只有 2 个页面且无设置页。据此确认不存在重复开发风险，把工作拆成 T8.2–T8.11 写进 `TASKS.md` 第八阶段，并定下三层配置模型（`deploy` / `hot` / `restart`）作为整个实现的骨架。
- 测试结果：审查结论落表，无代码改动。这一条的价值在于防止「重写一遍已有的东西」——后来 13 个新端点里有 4 个是对旧端点的替换而非新增。

### 2026-11-03 — [T8.2/T8.3] 配置 schema、加密与掩码
- 做了什么：新增 `settings/schema.go`（4 组 27 个字段，每项带 `kind` / `scope` / `env` / 默认值 / 取值范围 / 中文帮助）与 `settings/manager.go`（`envBase` + `applied` 双层、`atomic.Pointer[Runtime]` 热更新、`Patch` 整批校验后落库、`__clear__` 清除覆盖）。新增 `secrets` 包：AES-256-GCM，密文前缀 `enc:v1:`，主密钥来自 `RESEARCHOS_SECRET_KEY` 或 `<DataDir>/secret.key`（0600，临时文件 + rename 落盘）。迁移 `0002_settings_center.sql` 给 `app_settings` 加 `section` / `encrypted` / `updated_by`，给 `mcp_servers` 加 `tools` / `last_error` / `updated_at`。
- 测试结果：`secrets` 9 条、`settings` 15 条全绿。过程中修掉三个真缺陷：① 保存响应把提交的明文原样放进 `applied`（会进浏览器控制台和网络面板），改为敏感项只回掩码；② 清除 `__clear__` 只在 secret 分支被识别，普通字段走到 `Validate` 报「需要一个整数」；③ 纯星号 `***` 不含 `****`，被当成新密钥存进去了。
- 实测：`python scripts/check-openapi.py` 通过；迁移实跑输出 `[0001_init.sql 0002_settings_center.sql]`，主密钥来源 `file:data\secret.key`。

### 2026-11-03 — [T8.4/T8.5] 热更新打通到执行链 + MCP 工具发现
- 做了什么：`settings.Manager` 的 `Runtime` 快照同时持有 `Cfg` / `LLM` / `Search` / `Planner`；`api.Server` 全部改为通过 `cfg()` / `llmClient()` / `searchEngine()` 访问器取值；`exec.Engine` 加 `SetRuntime(func)`，`analyze` / `synthesize` 每次调用取当时客户端——否则会出现「界面显示已切换、实际还在用旧配置」这种不报错但骗人的状态。MCP 侧新增 `mcp/discover.go`：真实 stdio JSON-RPC（`initialize` → `notifications/initialized` → `tools/list`），20s 硬超时、2MB 输出上限、只传显式 env、跳过穿插的通知行。
- 测试结果：`mcp` 6 条全绿，其中用测试二进制自身当 MCP Server 起真实子进程，含 `silent` 模式验证超时、以及在 `tools/list` 前插通知验证鲁棒性。
- 实测：登记 → 未批准 discover 返回 `409 not_approved` → 批准 → 真实拉起 Node 写的 MCP Server，161ms 拿回 2 个工具并回写库（状态 `ready`），列表里直接可读工具名与入参 schema。

### 2026-11-03 — [T8.6/T8.7] 设置 API、系统信息与统计
- 做了什么：新增 `api/settings_api.go`（分组读取 / 保存 / 清除 / LLM 试连 / 搜索试连 / `/system` / `/stats`）与 `api/hub_api.go`（MCP 列表、发现、批准、删除；跨任务的 sources / reports / agents 列表）。错误分三类映射：`ErrDeployField` → 403、`ErrUnknownSetting` → 422、校验失败 → 422。`/system` 返回 Go 版本、CPU、goroutine、连接池、迁移版本、主密钥来源；`/stats` 汇总任务 / 节点 / Agent / 来源 / 结论 / 报告与最近 8 个任务。
- 测试结果：`api` 包 10 条设置相关用例全绿（鉴权、响应不泄露 Token 与数据库密码、保存返掩码、deploy 403、校验 422、`requires_restart`、清除回落、LLM 试连诚实失败、搜索无 Provider、掩码回传不改写）。全量 `go test -count=1 ./internal/researchos/...` 17 个包全绿。
- 实测（真实服务 + 真实数据库）：保存 `sk-live-SECRET-abcdef1234567890` → 响应 `sk-l****7890`、`hot_applied=true`；GET 全文 6937 字节检索不到明文；库中该行 `encrypted=true`、值以 `enc:v1:` 开头、全文检索不到明文。改 `execution.workers=8` → `requires_restart=["execution.workers"]`、`restart_needed=true`。改 `system.addr` → 403；`max_agents=999` → 422「最大并发 Agent 必须在 1 到 8 之间」。停服务再启动 → `model.*` 四项 `source=web` 全部保留、`workers=8` 已生效、`restart_needed=[]`、`llm.key_set=true`（说明密文在主密钥不变时可正常解密）。
- 过程中的坑与修复：端到端跑之前先撞上 8080 被上一轮遗留进程占着，新二进制根本没起来——`health` 却有响应，误以为已就绪。查 PID 才发现是旧实例。这类「探活成功但服务是旧的」最容易被当成测试通过。

### 2026-11-03 — [T8.8/T8.9] 契约、前端与其他页面
- 做了什么：`api/openapi.yaml` 把 settings 段整体重写并新增 system / stats / sources / reports / agents / tools 系列，补齐 `SettingsSnapshot` / `SettingsPatchResponse` / `SettingsField` / `SystemInfo` / `StatsResponse` / `MCPDiscoverResult` 等 schema 与 `Limit` / `Offset` / `Forbidden` / `ServiceUnavailable` 组件。前端新增 `SettingsSection.tsx`（schema 驱动渲染，按 `kind` 选控件、按 `scope` 标生效方式、按 `source` 决定能否清除覆盖）、`/settings` 五个分页（路由即分页，可收藏可刷新停在原地）、`/sources`、`/reports`。
- 测试结果：新增 `scripts/check-openapi.py`，拒绝重复键 + 校验 `$ref` 不悬空 + 路径参数都声明，输出 `37 条路径 / 40 个操作 / 30 个 schema / 107 处引用，无重复键、无悬空引用`。`tsc --noEmit` 干净。
- 过程中的坑与修复：契约文件里 `/api/v1/tools/servers` 被定义了两次，YAML 解析器静默保留最后一个——这正是新脚本要拦的东西，写完当场就抓到一条。顺带修掉状态条上「最近刷新」用 `new Date()` 充数（永远显示「刚刚」，等于假指标），改为记录真实拉取时刻，并补上「N 项已保存待重启」徽标。

### 2026-11-03 — [T8.10/T8.11] 手册与决策落档
- 做了什么：`docs/USAGE.md` 按真实界面操作顺序重排：先认路（六个入口表）→ 设置中心五页逐页说明（模型 / 搜索 / MCP Hub / 执行 / 系统）→ 优先级与生效规则 → 密钥怎么存 → 才进入建任务、批准、看过程、拿报告 → 跨任务回看 → API。新增「在设置里点了保存但任务行为没变」「`secrets.available: false`」「改了主密钥后所有 Key 失效」等排查条目。`DECISIONS.md` 追加 29–35 七条（三层配置、优先级、加密与掩码、原子快照热更新、MCP 批准闸门、整批落库）。
- 测试结果：文档中的每条命令与返回都是本轮实测过的输出，不是照着代码想象写的。
- 遗留问题 / 假设：Dashboard 与任务详情页的 UI 由人工在改，本轮未触碰、也未截图验收；真实大模型质量的验收仍卡在缺可用 Key（用假 Key 试连得到 `401`，恰好证明了「密钥确实被解密并使用」与「失败如实上报」两件事）。

### 2026-11-03 — [phase-9][T9.1] 设计令牌与应用框架
- 做了什么：以 `hector-researchos-dashboard-prototype.html` 为视觉参考但不复制其青绿色品牌色，重写 `web/app/globals.css`：暖白 `#F6F6F4`、白卡、`#252725` 主文字、`#777D7B` 次文字、`#E9EAE7` 分隔线、`#526B7B` 唯一强调色；标题/分区/正文/辅助/关键数据五档字号，4/8/12/16/24/32 间距，8–12px 圆角，边框优先于阴影。重建 `AppShell`（232px 侧栏、68px 顶栏、分组导航、移动端抽屉 + 遮罩、`⌘K` 命令面板、健康状态与数据源说明）与 `ui.tsx`（Button / Badge / Field / Tabs / Modal / Drawer / EmptyState / ErrorState / Skeleton 等）；新增 `icon.svg` 修掉 `/favicon.ico` 404。
- 测试结果：`tsc --noEmit`、`next build` 通过；6 个路由均可从侧栏进入，移动端菜单开合正常；页面无 Emoji、无紫色/蓝色渐变、无霓虹/粒子/装饰光斑。
- 遗留问题 / 假设：参考稿的 3 列工作台在 1440px 下会把画布压到 570px，改为「画布优先 + 提纲默认收起」，用工具栏按钮显式展开。

### 2026-11-03 — [phase-9][T9.2] 总览页：真实数据与 hydration 修复
- 做了什么：重做指标区（研究任务 / 已采集来源 / 近 7 天活动 / 完成率）、近 7 天真实活动图（无数据不画假柱）、运行时状态（规划方式、Agent 并发、检索条数、批准策略、robots、超时、数据库、模型）、最近任务、待处理项、最新报告；所有数值来自 `/api/v1/research`、`/api/v1/health`、`/api/v1/settings`、`/api/v1/reports`。加载态统一为骨架。
- 测试结果：实测指标 `62 / 899 / 62 / 73%`；活动图 7 组柱均有真实计数；运行时读数与 `GET /settings` 一致（`provider=none`、`max_agents=4`、`search.results=6`、`node_timeout=3m0s`、`task_timeout=30m0s`）。
- 过程中的坑与修复：静态导出把构建时刻写进 HTML，页脚「最后同步」服务端为 22:55、客户端为 22:57，React 报 4 次 #425（文本不匹配）并伴随 #418/#423；改为 `lastSync` 初始 `null`、数据到达后在 `useEffect` 里记录，服务端与首次客户端都渲染 `—`。修复后连续 4 次干净加载 0 错误，全站审计 Dashboard 也回到 0。
- 遗留问题 / 假设：活动图按浏览器本地时区分桶，跨时区查看同一天可能差 1 格，属预期。

### 2026-11-03 — [phase-9][T9.3] 研究工作台：竖向 DAG 与五页签
- 做了什么：`WorkflowCanvas` 改为自上而下的分层 DAG（`LAYER_GAP=138`、`ROW_GAP=216`、节点 196px），状态色沿用设计令牌；桌面 `fitView` 最大缩放 1，窄屏（≤620px）固定 `zoom=0.68` 保留可读节点并允许平移；工作台改为「提纲 / 画布 / 检查器」三栏，提纲在 1440px 默认收起、按钮可开；节点检查器展示角色、尝试次数、进度、Token、起止时间、输入与产物；五个页签（工作流 / 来源 / 证据 / 报告 / Agents）与批准、暂停、恢复、重规划、取消、导出全部保留原有 API 语义。
- 测试结果：真实任务 `aa1af51c` 渲染 8 节点；桌面节点 192×95、390px 下 133×66；提纲开关把网格从 `1fr/312` 切到 `226/570/312`，`aria-pressed` 同步；点击节点检查器出现完整详情；报告页签正文 3363 字符，Markdown 与 HTML 导出分别 8185 / 11051 字节；SSE ticket 请求成功，完成后自动回落为「定时刷新」。
- 过程中的坑与修复：早期在 860px 断点就强制 0.68 缩放，导致 818px 画布里的节点只有 133px；把窄屏阈值收到 620px 后，860px 节点恢复到 167px、720px 为 150px，520px 才是 133px。

### 2026-11-03 — [phase-9][T9.4] 来源 / 证据 / 报告 / 任务列表
- 做了什么：`/sources` 跨任务 1083 条来源、状态筛选、防抖搜索、50 条/页分页、详情抽屉；`/evidence` 按任务切换 Claim 与来源证据；`/reports` 41 份报告双栏阅读、搜索排序、Markdown/HTML 导出；`/tasks` 搜索、状态筛选、排序后进入工作台。空态、错误态、加载态均使用统一组件，不补假数据。
- 测试结果：来源页「共 1083 条来源 / 当前显示 50 条」；报告页 41 条；导出实测生成 `report-aad4e77b.md`（8185B）与 `report-aad4e77b.html`（11051B）；所有页面正文检索无 `undefined`、`NaN`、`[object Object]`。
- 遗留问题 / 假设：HTML 导出沿用后端转义与仅 http(s) 链接策略，未新增 PDF（T6.1）。

### 2026-11-03 — [phase-9][T9.5] 设置中心：schema 驱动与固定保存条
- 做了什么：`/settings` 单页按 `GET /api/v1/settings` 的 sections/fields 渲染 27 项配置，按 `kind` 选控件、按 `scope` 标「环境变量 / 界面保存 / 默认值 / 只读」、按 `source` 决定是否可恢复默认；枚举、数字范围、时长、多选校验；密钥只显示「已配置（不回显）」；保存条汇总未保存项，支持还原与保存；LLM / 搜索试连与 MCP 登记、批准、发现、删除保留；旧 `/settings/{section}` 链接重定向到锚点。
- 测试结果：改「每 Provider 结果数」6→7，保存条出现，保存后 toast「设置已保存并即时生效」，重新读取为 7，再改回 6；移动端保存条位于视口底部（`y=780`，`390×844`），按钮「还原 / 保存更改」不溢出，点「还原」值回 6 且保存条消失；全程无 `[object Object]`。
- 过程中的坑与修复：保存条虽然 `position: fixed`，却被父级 `.route-fade` 的 `animation-fill-mode: both` 留下的 `transform` 变成「相对整页固定」，实测 `y=5242`，用户在页面顶部根本看不到；去掉 fill mode（让动画结束后 `transform` 回到 `none`）后恢复为视口底部。

### 2026-11-03 — [phase-9][T9.6] 响应式、动效与可访问性
- 做了什么：断点 1280 / 1120 / 1024 / 860 / 620 / 520；1024 以下侧栏变抽屉、860 以下工作台单列、620 以下 DAG 固定缩放；页面过渡 220ms、hover 160ms、图表一次性入场、`prefers-reduced-motion: reduce` 关闭动画；按钮与图标按钮补齐 `aria-label`，开关使用 `role="switch"`，焦点环使用强调色。
- 测试结果：1280 / 1024 / 860 / 720 / 520 / 390 六档全页面 `document.scrollWidth == innerWidth`；1024 起侧栏抽屉按钮出现；390 下所有页面无横向溢出，设置导航不截断；`ui-audit.mjs` 桌面 + 移动 14 项全过。
- 遗留问题 / 假设：移动端 DAG 采用「固定缩放 + 平移」而非全图自适应，全图自适应会把节点压到 100px 以下；这是可读性取舍，不是布局溢出。

### 2026-11-03 — [phase-9][T9.7] 浏览器审计脚本与真实截图
- 做了什么：新增 `web/scripts/ui-audit.mjs`（Node 22+ 内置 WebSocket，连 CDP `9333`），自动发现第一条研究任务，遍历 7 条路由 × 1440/390 两档，收集 `Runtime.exceptionThrown` / `console.error`，测量文档宽度、画布与节点尺寸，可 `--shots` 直接输出 PNG；截图落 `docs/screenshots/`。
- 测试结果：14 项全 `ok`，控制台错误 0、横向溢出 0；`workspace` 桌面画布 808×552、节点 192×95，移动画布 360×430、节点 133×66；生成 10 张截图（总览 / 任务 / 工作台 / 来源 / 证据 / 报告 / 设置 / 移动端总览、设置、工作台）。
- 遗留问题 / 假设：脚本只覆盖「渲染 + 布局 + 控制台」，不点击创建/批准/暂停等业务按钮；全业务流 E2E 仍留在 T6.10。

### 2026-11-03 — [phase-9][T9.8] 账本与提交
- 做了什么：同步 `TASKS.md` 第九阶段与 T6.8/T6.10 状态，补 `PROGRESS.md` 本轮记录与 `docs/design-system.md`；按「设计系统 → 总览/工作台 → 资料页与设置 → 审计脚本与截图」拆小提交，均只提交到本地 `feature/hector-researchos`，不 push。
- 测试结果：提交前重跑 `tsc --noEmit`、`next build` 与 `node scripts/ui-audit.mjs --shots=...`，全部通过；`git status` 无未提交的前端源码。
- 遗留问题 / 假设：PDF 导出与全业务流 E2E 仍未完成，已在任务清单保留 `[~]`。

### 2026-11-03 — [T10.2] 真实 PDF 报告导出（从零实现）
- 做了什么：`report` 包新增 A4 排版器。字体决策是核心：gopdf 只认 glyf 型 TTF，而主流发行版的 CJK 字体几乎全是 TTC/OTF(CFF)，运行时扫系统字体等于把部署环境变成功能开关——所以内嵌 Noto Sans SC Regular 静态实例（google/fonts 官方 VF 经 fontTools 静态化 wght=400，OFL 授权全文随字体入库）。缺字行为是「报错」不是「静默丢字」，因此在排版前用生成的覆盖区间表把字体外的码点替换为 '□' 并计数。页码用两遍渲染（页脚占预留边距，不影响分页，两遍分页必然一致），同输入产出字节级相同的文件。
- 过程中的坑：① 端到端内容比对抓到真缺陷——plainText 把 `[标题](URL)` 剥成只剩标题，来源列表的 URL 在纸面上全没了，违反「引用可识别」；改为渲染成「文本（URL）」。② 测试还抓到 res.pages 从未同步渲染器计数、`MeasureTextWidth` 在未设字体时 panic 两个问题。③ 这份启发式报告本身没有对比表（无数值结论），表格渲染的视觉证据用 `RESEARCHOS_PDF_SAMPLE` 守卫的测试单独产出。
- 测试结果：report 包 19 条测试全绿；端到端 `verify_pdf_content.py` 比对章节 7、URL 5、正文抽样 49 全部命中；渲染留证 3 张 PNG。

### 2026-11-03 — [T10.3/T10.6] 前后端对接矩阵与集成缺陷
- 做了什么：写 `ui-functional.mjs`，用 CDP 抓每个页面真实发出的 API 请求逐个断言（不只看「页面能渲染」）。第一轮 37 项断言里抓出 7 个 FAIL，全部坐实：`/system` 和 `/stats` 是为前端建的但从来没有入口（stats 注释里就写着「供 Dashboard 使用」）；工作台五个 Tab 懒加载导致审计漏判（不是缺陷，审计脚本要切 Tab）；设置页字段数选择器写错。修复 = 接线 `/system`（设置页运行环境卡片）与 `/stats`（Dashboard 全量计数——原实现从最近 100 条任务客户端累加，任务多于 100 时卡片数字是错的）。
- 测试结果：第二轮 `37 通过 / 0 失败`，含「UI 真实提交创建任务 → 落库 → 出现在列表 → 清理」的完整交互闭环。`ui-audit.mjs` 双视口 14 项全 ok。截图全量重截（真实服务 + 已完成任务数据）。

### 2026-11-03 — [T10.4/T10.5] verify.sh 重写与部署资产修复
- 做了什么：旧 verify.sh 是 4 步骨架（大量 skip、无退出码语义、有「verify.sh passing on skeleton does NOT mean V1.0 done」的自供状注释）。重写为统一校验：gofmt / vet / test / tsc / next build / openapi / compose / 迁移幂等（跑两次）/ 起真实服务打 /health 与 report.pdf / PDF 内容比对，并内置负向自检——故意注入悬空 $ref 和坏格式文件，校验器抓不到就算校验器 FAIL。部署侧：`deployments/compose.yaml` 原是 phase-0 残骸（引用不存在的 Dockerfile、./api、./worker、../web/Dockerfile），重写为与实际镜像一致的生产编排；本机无 Docker，`check-compose.py` 提供静态校验并把实机部署如实标 BLOCKED。
- 测试结果：`bash scripts/verify.sh` 全量 14 通过 / 0 失败 / 0 跳过；负向自检 2 项通过（坏文件确实被抓到，exit 1）。
