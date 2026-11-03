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
- [~] T6.8 演示脚本 + 真实运行截图素材 — 依赖：T6.7
  - 已完成：`DEMO_SCRIPT.md` 分镜、`docs/demo.md` 真实运行记录、`docs/demo/report.md|html` 真实产物、`scripts/screenshot.sh`
  - 未完成：截图与录屏需图形界面，本机 Chrome/Edge 无头模式无输出
- [x] T6.9 最终交付报告 + Git 历史整理（不推远程）— 依赖：全部
  - 见 `docs/final-report.md`；两个 Git 仓库均本地提交，未推送
- [ ] T6.10 前端浏览器 E2E（Playwright 或手工脚本化清单，需明确标注执行方式）— 依赖：T5.2
  - 未开始：本机无 Playwright，且无头浏览器不可用；`docs/demo.md` §3 给出手工核对清单

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
