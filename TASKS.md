# TASKS.md — 可执行任务清单

> 格式：- [ ] Txx 任务 — 验收：…… — 依赖：……
> 状态标记：`[x]` 已完成且有可复现验证证据；`[~]` 已实现但验证/验收项未补齐；`[ ]` 未开始。
> **禁止**把 `[~]` 当 `[x]`：每项的验证命令与结果写在 `PROGRESS.md` 对应轮次。

## Phase 0 — 审计与初始化
- [x] T0.1 Fork verikod/hector，建立 feature/hector-researchos 分支，记录原仓库 commit 与目录树 — 验收：docs/hector-audit.md 含原目录树与 commit — 依赖：无
- [x] T0.2 梳理原有 Go Package、前端模块、数据模型、接口，输出映射表 — 验收：docs/mapping-table.md 逐项标 保留/修改/新增/替换 — 依赖：T0.1
- [x] T0.3 A2A 身份隔离审查（Issue #2）— 验收：SECURITY_REVIEW.md 第 1 节全填 + 伪造 user_id 测试 — 依赖：T0.1
- [x] T0.4 任务队列、Checkpoint、恢复逻辑审查 — 验收：审计文档写明现有恢复能力与缺口 — 依赖：T0.1
- [x] T0.5 锁定技术选型：ORM（二选一）、模型 Provider 接口、搜索 Provider 接口 — 验收：DECISIONS.md 更新 — 依赖：T0.2
- [x] T0.6 建立测试基线与开发环境：go test / 前端构建 / Docker Compose 空跑 — 验收：scripts/verify.sh 在空骨架上能跑通 — 依赖：T0.5
- [x] T0.7 确定数据库模型与 API 契约初版 — 验收：DATA_MODEL.md + api/openapi.yaml 与 migration v001 一致 — 依赖：T0.2, T0.5

## Phase 1 — 产品工作台
- [x] T1.1 ResearchTask 生命周期状态机 + 持久化 — 验收：状态非法跃迁被拒绝并有事件 — 依赖：T0.7
- [x] T1.2 REST API：创建/列表/详情/暂停/恢复/取消/批准 — 验收：契约与 openapi 一致（`/approve`、`/pause`、`/resume`、`/cancel`、`/retry`、`/plan`）— 依赖：T1.1
- [x] T1.3 事件总线 + SSE：事件写入、sequence、幂等、断线续接 — 验收：重复 event_id 不重复入库；`Last-Event-ID` 能续接 — 依赖：T1.1
- [x] T1.4 Dashboard + New Research + 任务详情页 — 验收：可创建任务并看到真实状态 — 依赖：T1.2, T1.3
- [x] T1.5 工作流画布（React Flow）：节点/边渲染、状态颜色、失败节点重试 — 验收：画布状态与 GET 详情快照一致 — 依赖：T1.3, T1.4

## Phase 2 — 自主研究执行
- [x] T2.1 Planner：结构化输出 + Schema 校验 + 失败降级 — 验收：非法 JSON 不进入执行 — 依赖：T1.1
- [x] T2.2 DAG 编译与校验：环检测、依赖拓扑排序、synthesize 必须收尾 — 验收：有环 DAG 被拒绝 — 依赖：T2.1
- [x] T2.3 Scheduler + Worker Pool：并发上限可配置、SKIP LOCKED 认领、租约回收 — 验收：并发上限生效，超限排队 — 依赖：T2.2
- [x] T2.4 Agent Runtime：角色注册、生命周期、Token/工具调用记录 — 验收：Agent 详情来自真实执行 — 依赖：T2.3
- [x] T2.5 节点级错误处理：退避重试、失败隔离、Checkpoint 写入 — 验收：单节点失败不拖垮整任务 — 依赖：T2.4
- [x] T2.6 暂停/恢复/取消全链路 — 验收：暂停后节点停止推进；恢复从已完成节点继续 — 依赖：T2.5

## Phase 3 — 搜索与证据
- [x] T3.1 Search Provider 接口 + 5 个真实 Provider（GitHub / HN / Wikipedia / Bing / Tavily·SerpAPI·Brave）— 验收：换 Provider 不改业务代码 — 依赖：T2.4
- [x] T3.2 网页抓取 + 正文解析 + 快照存储（robots.txt + SSRF 防护）— 验收：标题/正文/快照可查 — 依赖：T3.1
- [x] T3.3 URL 规范化 + 内容 SHA256 去重 — 验收：重复 URL/重复内容只留一条 — 依赖：T3.2
- [x] T3.4 来源中心：分类、可信度、按站点/可信度筛选 — 验收：`GET /sources?host=&min_trust=` 生效 — 依赖：T3.3
- [x] T3.5 Evidence：Claim 与 Source 关联、强度标记 — 验收：无来源 Claim 只能是 unverified — 依赖：T3.4
- [x] T3.6 Verifier：多源交叉验证、冲突识别 — 验收：构造冲突输入能被标 conflicting — 依赖：T3.5

## Phase 4 — 报告与交互
- [x] T4.1 Synthesizer：汇总多节点结论为统一分析 — 验收：汇总保留引用映射不丢失 — 依赖：T3.6
- [x] T4.2 Reporter：结构化报告生成（目标 9 节，当前 7 节 + 方法/局限）— 验收：引用编号连续、越界引用被剔除 — 依赖：T4.1
- [x] T4.3 Report Viewer：目录、引用跳转、来源弹窗 — 验收：点击引用能定位到来源 — 依赖：T4.2
- [~] T4.4 导出：Markdown / HTML / PDF — 验收：内容与报告一致、无路径穿越 — 依赖：T4.2
  - 已完成：`.md`、`.html`（转义 + 仅 http(s) 链接）。未完成：PDF（T6.1）。
- [x] T4.5 Tool Hub：MCP 注册/启停/健康检查/调用日志 — 验收：高风险工具默认需审批 — 依赖：T2.4
- [x] T4.6 节点详情、事件时间线、失败重试 UI — 验收：失败节点可在 UI 重试并产生新事件 — 依赖：T2.5, T4.3

## Phase 5 — 测试、部署与发布
- [x] T5.1 单元测试：dag / planner / textutil / report / search / exec / domain 状态机 — 验收：`go test ./internal/researchos/...` 全绿 — 依赖：T4.6
- [~] T5.2 集成 E2E：真实 PG + 真实 HTTP 全链路 — 验收：可重复跑通 — 依赖：T4.6
  - 已完成：`scripts/e2e.sh` 跑通一次完整任务（8/8 节点、报告生成）。未完成：前端浏览器 E2E（T6.10）。
- [x] T5.3 故障注入：kill Worker 后恢复 — 验收：租约超时回收、已完成节点不重跑 — 依赖：T2.6
- [x] T5.4 并发压测：5 任务并发，记录吞吐与错误率 — 验收：实测数据写入 benchmark — 依赖：T5.2
- [x] T5.5 权限隔离回归：Issue #2 修复 + 测试翻转 — 验收：伪造 `metadata.user_id` 不再生效 — 依赖：T0.3
- [~] T5.6 Docker Compose 从零部署 — 验收：`docker compose up` 能跑演示 — 依赖：T5.4
  - 本机无 Docker，按人工决策 Phase 5 再装；Compose/Dockerfile 交付但标注「未在本机验证」（T6.6）。
- [x] T5.7 最终演示脚本 + 验收清单 — 验收：清单逐条有证据或明确标注未验证 — 依赖：T5.6

## 补强任务（自主模式新增）
- [ ] T6.1 PDF 导出：纯 Go 渲染（页眉页脚、目录、引用编号与 Markdown 一致）— 验收：导出 >20KB 且文本可提取 — 依赖：T4.2
- [x] T6.2 覆盖率实测 + 基准实测落 `docs/benchmark.md`（禁止把目标值当实测）— 依赖：T5.1
- [x] T6.3 govulncheck / 依赖清单扫描记录 — 依赖：T5.5
  - 见 `docs/dependency-scan.md`：研究域清零；全模块 20 → 2（剩余 2 条为上游未发版修复，附原因）
- [x] T6.4 故障注入脚本化：停 worker → 租约超时 → 自动接管 — 依赖：T5.3
  - 见 `scripts/fault-injection.sh`，实测 PASS：4 个已完成节点 attempts 未变，被打断节点重跑一次后任务收敛 completed
- [x] T6.5 压测脚本化：5 任务 × 4 Agent，输出 P50/P95/错误率 — 依赖：T5.4
  - 见 `scripts/load-test.sh`，5 轮实测数据入 `docs/benchmark.md` §1
- [x] T6.6 Dockerfile + docker-compose.yaml（postgres + app，web 构建入镜像）— 依赖：T5.6
  - 产物已交付；**运行验证未做**（本机无 Docker），见 T5.6
- [x] T6.7 README 首屏 + ARCHITECTURE + DEPLOYMENT — 依赖：T6.6
  - `README.md` / `ARCHITECTURE.md` / `DEPLOYMENT.md` / `docs/DATA_MODEL.md` / `api/openapi.yaml` 全部按实际落码重写
- [x] T6.8 演示脚本 + 真实运行截图素材 — 依赖：T6.7
  - 已完成：`DEMO_SCRIPT.md` 分镜、`docs/demo.md` 真实运行记录、`docs/demo/report.md|html` 真实产物、`scripts/screenshot.sh`
  - 2026-11-03 补齐：无头 Chrome + CDP 可稳定出图，`web/scripts/ui-audit.mjs --shots=...` 生成 `docs/screenshots/` 10 张真实页面截图；录屏仍未做
- [x] T6.9 最终交付报告 + Git 历史整理（不推远程）— 依赖：全部
  - 见 `docs/final-report.md`；两个 Git 仓库均本地提交，未推送
- [~] T6.10 前端浏览器 E2E（Playwright 或手工脚本化清单，需明确标注执行方式）— 依赖：T5.2
  - 已完成（2026-11-03）：`web/scripts/ui-audit.mjs` 用无头 Chrome + CDP 逐页检查 7 条路由 × 桌面/移动共 14 项，控制台错误 0、横向溢出 0；可重复执行
  - 未完成：创建 → 批准 → 暂停/恢复 → 完成 → 导出的全业务流自动 E2E；当前覆盖渲染与布局，不覆盖状态机

## 本轮迭代任务（2026-11-03，人工指派）
- [x] T7.1 全局统一 LLM 配置变量名（`RESEARCHOS_LLM_API_KEY` / `RESEARCHOS_LLM_PROVIDER`）
  - 修掉 `scripts/bootstrap.sh` 生成的错误模板（整套变量名不带前缀，服务根本读不到）；`/api/v1/health` 的提示文案从「未配置 LLM_API_KEY」改为正确变量名；README 配置表拆成逐项显式名称；`scripts/bootstrap.sh` 重写为「检测工具链 + 生成 `.env.researchos`」并实测可跑
  - 全仓复查：除抓取快照与上游 `pkg/`（另一子系统的 `OPENAI_API_KEY`）外，无残留错误引用
- [x] T7.2 SSE 鉴权从 `?token=` 改为一次性 ticket
  - 设计：短时（默认 60s，上限 10 分钟）、绑定 task_id、一次性、仅 events 端点可用、内存只存 sha256
  - 实现：`api/ticket.go` + `withAuth` 改造 + `POST /api/v1/research/{id}/events/ticket`；`?token=` 通道删除
  - 测试：`ticket_test.go` 13 条 + `auth_test.go` 11 条，`go test -race` 全绿
  - 实测：签发 200 → 建流 200 → 重放 401 → 跨任务 403 → 过期 401 → 当 Token 用 401
  - 前端：`eventsURL` 改为传 ticket，每次（重）连前重新签发；`tsc --noEmit` 与 `next build` 通过
- [x] T7.3 LLM 链路验证（无真实 Key，用本地 OpenAI 兼容测试替身）
  - `scripts/mock-llm.py` 提供 `/v1/chat/completions`，故意混入编造引文与伪造 `source_id`
  - 实测：任务 `planner_mode=llm`、3 个 analyze 节点 `llm_used=true`、每次 `dropped_ungrounded=2`、8/8 节点 completed、报告抬标 `mock-1`
  - 顺带修掉一个真缺陷：LLM 规划的 `query` 被 api 层丢弃（见 T7.4）
  - 补 `internal/researchos/llm/llm_test.go` 8 条（请求形状 / JSON 模式 / 围栏剥离 / 错误截断 / 上下文取消），覆盖率 94.5%
- [x] T7.4 复杂研究任务 + 引用可追溯性验收
  - 任务 `ee9d550d`（standard + 交叉验证）：8/8 节点、18 条结论、18 条证据
  - 新增 `scripts/verify-citations.py`：18/18 逐字落地、18 个编号闭合、10 条核心发现可回溯，PASS
  - 过程中修掉真缺陷：`textutil.Sentence` 在文本末尾补「。」，导致引文不再等于原文（旧样本 5/7 落地）
- [x] T7.5 交叉验证 / 冲突识别 / 未证实结论
  - 新增 `exec/verify_integration_test.go`（`RESEARCHOS_TEST_DSN` 门控，直连真实库跑 verify 节点）：clusters=3、supported=2、conflicting=2、unverified=1，冲突双方共享冲突组
  - 把内联的状态判定抽成 `verificationStatus`，7 条分支用例覆盖四种状态
- [x] T7.6 中断恢复 / 节点重试 / 事件续传
  - 故障注入 PASS（8/8 节点、停机前已完成节点 attempts 不变）；修掉脚本 PID 识别缺陷（`comm` 要求字典序，`sort -n` 满足不了）
  - 事件续接：`?after_sequence=10` → id 从 11 起；`Last-Event-ID: 50` → 从 51 起；116 条事件无空洞
  - 节点重试：发现 `synthesize` 的确定性失败被重试 3 次；新增 `exec.PermanentError`，attempts 3 → 1；人工 retry 后 attempts=2；对已完成节点 retry 返回 409
- [x] T7.7 手册与契约同步
  - `docs/USAGE.md`：ticket 用法与四条硬规则、LLM 开启后可见的三处变化、ticket 报错排查、验收脚本清单
  - `api/openapi.yaml`：新增 ticket 端点、events 鉴权说明、`401/403` 语义
  - `docs/security-review.md` §1.8、`ARCHITECTURE.md` §5、`README.md` 状态表与配置表同步

## 第八阶段：完整设置中心（2026-11-03 人工指派）

目标：普通用户不改源码、不编辑 `.env`、不重新编译，即可在 Web UI 管理日常运行配置。

### 现状审查结论（避免重复开发）
- 已有 `GET/PUT /api/v1/settings`，但只覆盖 4 个运行参数（`max_agents` / `require_approval` /
  `default_depth` / `search_results`）；`default_depth` 只校验不落配置，属无效分支。
- 凭据（LLM Key、各搜索 Key）只从环境变量读，接口仅返回掩码，**无法通过界面配置**。
- `app_settings` 表已存在但只被上述 4 个键使用，无加密、无来源标注、无重启语义。
- MCP 只有登记与 start/stop（start 置 `pending_approval`），无工具发现、无批准动作、无删除。
- 前端只有 Dashboard 与 Task 两个页面，**没有任何设置页**。
- 来源 / 报告只有「按任务」接口，缺跨任务管理视图。

### 任务清单
- [x] T8.1 设置分层与优先级定稿：deploy（env 只读）/ hot（即时生效）/ restart（需重启）；Web 值与 env 的优先级写入 DECISIONS 与手册
- [x] T8.2 密钥保护：AES-256-GCM 加密存储，主密钥来自 `RESEARCHOS_SECRET_KEY` 或首次运行生成 `secret.key`(0600)；GET 只返回掩码 + `configured`
- [x] T8.3 持久化与热更新：设置读写走 `app_settings`；服务启动时把 DB 值叠加到环境变量之上；LLM / 搜索客户端重建并原子换快照
- [x] T8.4 设置 API：`GET /api/v1/settings`（分组 + 来源 + 可变性）、`PUT /api/v1/settings/{section}`（校验 + 保存 + 生效反馈 + `requires_restart`）
- [x] T8.5 连接测试：`POST /api/v1/settings/llm/test`、`POST /api/v1/settings/search/test`（真实外呼，失败返回可读原因）
- [x] T8.6 系统与统计 API：`GET /api/v1/system`（DB/服务/版本/运行环境）、`GET /api/v1/stats`（任务与 Agent 统计）
- [x] T8.7 MCP Hub 补全：工具发现、启用/禁用、权限审批、删除
- [x] T8.8 来源与报告跨任务接口：`GET /api/v1/sources`、`GET /api/v1/reports`
- [x] T8.9 前端九页：Dashboard / Research / Sources / Reports / Model / Search / MCP Hub / Execution / System
- [x] T8.10 测试：设置 API、持久化、密钥保护、重启行为、优先级、部署级参数拒绝
- [x] T8.11 文档：`docs/USAGE.md` 按真实界面顺序重写配置流程；`api/openapi.yaml` 同步；DECISIONS 追加

### 完成情况（2026-11-03 收口）

| 任务 | 证据 |
|---|---|
| T8.1–T8.3 | `settings` 15 条 + `secrets` 9 条测试全绿；迁移 `0002` 实跑输出 `[0001_init.sql 0002_settings_center.sql]` |
| T8.4–T8.5 | `api` 10 条设置用例全绿；真实外呼证据：搜索试连 4 个 Provider 全通（bing 619ms / wikipedia 1102ms / github 1463ms / hackernews 3618ms，共 11 条结果）；LLM 试连用假 Key 得到上游 `401`，证明 Key 确实被解密并发出 |
| T8.6–T8.7 | `/system` `/stats` `/sources`(1083 条) `/reports`(41 条) `/agents` 均返回真实数据；MCP 登记 → 409 拒绝 → 批准 → 161ms 发现 2 个工具 → 状态 `ready` |
| T8.8 | `scripts/check-openapi.py`：`37 条路径 / 40 个操作 / 30 个 schema / 107 处引用，无重复键、无悬空引用` |
| T8.9 | `/settings/{model,search,mcp,execution,system}` + `/sources` + `/reports` 完成；`tsc --noEmit` 干净。Dashboard 与任务详情页由人工在改，未触碰 |
| T8.10 | 端到端：保存后 GET 全文不含明文、库中 `encrypted=true` 且带 `enc:v1:` 前缀；`workers=8` 保存后 `requires_restart` 正确、重启后生效；`system.addr` → 403、`max_agents=999` → 422 |
| T8.11 | `docs/USAGE.md` 重排为「认路 → 设置五页 → 优先级 → 密钥 → 建任务 → 批准 → 过程 → 报告 → API」；DECISIONS 追加 29–35 |

未完成 / 待人工确认：Dashboard 与任务详情页 UI（人工在改）；真实大模型报告质量（缺可用 Key，仅验证到「链路通 + 失败如实上报」）。

## 第九阶段：前端视觉系统重构（2026-11-03 人工指派）

目标：以 `hector-researchos-dashboard-prototype.html` 为视觉与交互参考，把前端重做成可商业交付的统一设计系统；保留全部业务逻辑、API 契约、SSE、DAG 执行与鉴权，只改表现层。视觉关键词：Premium Enterprise、Editorial Minimalism、Swiss Typography、Quiet Luxury。

### 任务清单与验收
- [x] T9.1 设计令牌与应用框架 — 验收：一处定义颜色/字号/间距/圆角/动效；侧边栏 232px、顶栏 68px、内容最大 1440px；无 Emoji、无紫色渐变、无霓虹/粒子/装饰光斑；导航含总览/任务/来源/证据/报告/设置，移动端抽屉 + 遮罩，`⌘K` 命令面板可用
  - 证据：`web/app/globals.css` 的 `:root` 令牌与 `components/AppShell.tsx`；`web/app/icon.svg` 替换 404 favicon；浏览器实测 6 个入口均可达，移动端菜单可开合
- [x] T9.2 总览页重构 — 验收：指标、近 7 天活动图、运行时状态、最近任务、待处理项、最新报告全部来自真实 API；加载态为骨架而非假数据；时间显示不在服务端预渲染时被写死
  - 证据：指标实测 `62 / 899 / 62 / 73%`；活动图 7 组真实柱；运行时读取 `/settings` 与 `/health` 快照；修复 `new Date()` 造成的 hydration 文本不一致（#425）
- [x] T9.3 研究工作台（DAG 画布） — 验收：竖向分层 DAG、节点状态色、点击节点看详情、提纲开关、工作流/来源/证据/报告/Agents 五个页签、批准/暂停/恢复/重规划/取消/导出可用；窄屏不挤压到不可读
  - 证据：真实任务 `aa1af51c` 8 节点、39 来源、7 证据、1 报告；192×95 节点（1440）、133×66（390）；提纲开关切换 `226/1fr/312` 与 `1fr/312`；报告正文 3363 字符、Markdown/HTML 均下载成功
- [x] T9.4 来源 / 证据 / 报告 / 任务列表 — 验收：跨任务浏览、搜索、状态筛选、分页、详情抽屉或双栏阅读；空态/错误态/加载态齐全；不伪造数据
  - 证据：`/sources` 1083 条真实来源、50 条/页；`/reports` 41 份、双栏阅读；`/evidence` 按任务切换；`/tasks` 搜索与状态筛选可用
- [x] T9.5 设置中心 — 验收：按后端 schema 渲染 27 项配置，区分环境变量/界面保存/默认值/只读；枚举、数字、时长、多选校验；保存条固定在视口底部；骨架与错误提示完整
  - 证据：`GET /api/v1/settings` 驱动；保存 `search.results` 6→7→6 实测持久化与 toast；移动端保存条 `y=780`、按钮 `还原/保存更改`、`还原` 后恢复为 6 并隐藏；无 `[object Object]`
- [x] T9.6 响应式与动效 — 验收：1280/1024/860/720/520/390 无横向溢出；页面过渡 160–240ms；hover 克制；`prefers-reduced-motion` 下关闭动效；焦点可见
  - 证据：1280 / 1024 / 860 / 720 / 520 / 390 六档宽度全页面审计 `doc == viewport`；`route-fade` 220ms；`@media (prefers-reduced-motion: reduce)` 关闭动画；移除 `route-fade` 的 `animation-fill-mode` 修复固定保存条错位
- [x] T9.7 浏览器审计与截图 — 验收：可复现脚本逐页检查控制台错误与横向溢出，并输出真实截图
  - 证据：`web/scripts/ui-audit.mjs` 14 项检查全 `ok`（7 路由 × 桌面/移动）；`docs/screenshots/` 10 张真实运行截图
- [x] T9.8 文档与提交 — 验收：TASKS/PROGRESS/docs 与代码同步；小步提交、不推远程
  - 证据：本文件、`PROGRESS.md`、`docs/design-system.md`；`upstream/hector` 本地提交，未 push

### 收口说明
- 未采用参考稿的青绿色 `#176d6b`，保持更克制的 `#526B7B` 作为唯一强调色；未引入任何装饰性插画或假图表。
- 仍有开放项：PDF 导出（T6.1）、完整业务流浏览器 E2E（T6.10 仍为 `[~]`，当前脚本覆盖路由渲染与布局，不覆盖创建→批准→完成全链路）。

## 第十阶段：最终集成、全项目审计与交付（2026-11-03 人工指派）

目标：接管项目收尾——建立前后端对接矩阵、修复遗留缺陷、补齐 PDF 导出、统一校验入口、全项目审计，形成可交付版本。UI 视觉成果（T9）保持不动，只做功能性补齐。

- [x] T10.1 全项目扫描：两仓 git 状态干净（UI Agent 工作已全部提交，T9.1–T9.8）；无 TODO/Mock/占位数据；后端 41 个路由操作与前端消费逐一对账
- [x] T10.2 PDF 报告导出：服务端真实排版（gopdf + 内嵌 Noto Sans SC Regular OFL 字体 10.1MB），A4、CJK 断行、表格跨页重复表头、页码；`GET /research/{id}/report.pdf` + 前端按钮；10 条单测
- [x] T10.3 集成缺陷修复：`/system` 接入设置页运行环境卡片；`/stats` 接入 Dashboard 指标卡（消除「只统计最近 100 条」口径失真）；html 与 pdf 共用 ParseBlocks
- [x] T10.4 verify.sh 重写：gofmt/vet/test/tsc/build/契约/compose/迁移/集成/PDF 内容一致性，负向自检证明校验器抓得到真错误；实跑 14 通过 / 0 失败 / 0 跳过
- [x] T10.5 部署资产修复：deployments/compose.yaml 从 phase-0 残骸重写；check-compose.py 静态校验；根 compose 补 healthcheck
- [x] T10.6 浏览器审计：ui-functional.mjs 37 项断言（逐页真实 API 调用、懒加载 Tab、SSE ticket、UI 创建任务落库）；ui-audit.mjs 双视口 14 项
- [x] T10.7 端到端真实任务：heuristic 模式全链路（3 researcher 并发 7.6–8.2s、7 来源全 ok、15 结论、PDF 3 页内容比对一致）
- [x] T10.8 文档与审计：INTEGRATION_MATRIX、FINAL_AUDIT、USAGE/README/DECISIONS/PROGRESS 同步

### 证据表

| 任务 | 证据 |
|---|---|
| T10.2 | `verify_pdf_content.py`：PDF 4 页；比对章节 7、URL 5、正文抽样 49 全部命中；渲染留证 `.devtools/e2e/pdf_*.png`；同输入字节级一致（单测） |
| T10.4 | `bash scripts/verify.sh` 输出 `14 通过 / 0 失败 / 0 跳过`，含负向自检 2 项 |
| T10.6 | `node scripts/ui-functional.mjs` 输出 `37 通过 / 0 失败`；`ui-audit.mjs` 14 项全 ok（7 路由 × 1440/390） |
| T10.7 | 任务 `497ba966` completed；agents 8 条（researcher×3 并发、analyst×3、verifier、synthesizer）；sources 7 条全 `ok`（bing 5 / github 1 / hackernews 1）；planner_mode=heuristic 如实标注 |

BLOCKED（外部条件，非代码问题）：
- 真实 LLM 研究质量：无可用 Key（`.env.researchos` 未配 `RESEARCHOS_LLM_*`）；链路已验证到「试连诚实返回上游 401、启发式全链路可交付」
- Docker 实机部署：本机无 Docker；静态校验（构建上下文/依赖关系/健康检查）已过，Dockerfile 多阶段构建逻辑与 compose 依赖已人工核对
