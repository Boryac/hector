# TASKS.md — 可执行任务清单（Codex 按顺序打勾，不得跳过验收）

> 格式：- [ ] Txx 任务 — 验收：…… — 依赖：……
> Codex 每完成一项，把 [ ] 改为 [x]，并在 PROGRESS.md 写测试结果。依赖未满足不得提前做。

## Phase 0 — 审计与初始化
- [x] T0.1 Fork verikod/hector，建立 feature/hector-researchos 分支，记录原仓库 commit 与目录树 — 验收：docs/hector-audit.md 含原目录树与 commit — 依赖：无
- [x] T0.2 梳理原有 Go Package、前端模块、数据模型、接口，输出映射表 — 验收：docs/mapping-table.md 逐项标 保留/修改/新增/替换 — 依赖：T0.1
- [x] T0.3 A2A 身份隔离审查（Issue #2）— 验收：SECURITY_REVIEW.md 第 1 节全填 + 伪造 user_id 测试 — 依赖：T0.1
- [x] T0.4 任务队列、Checkpoint、恢复逻辑审查 — 验收：审计文档写明现有恢复能力与缺口 — 依赖：T0.1
- [x] T0.5 锁定技术选型：ORM（二选一）、模型 Provider 接口、搜索 Provider 接口 — 验收：DECISIONS.md 更新 — 依赖：T0.2
- [x] T0.6 建立测试基线与开发环境：go test / 前端构建 / Docker Compose 空跑 — 验收：scripts/verify.sh 在空骨架上能跑通 — 依赖：T0.5
- [ ] T0.7 确定数据库模型与 API 契约初版 — 验收：DATA_MODEL.md + api/openapi.yaml 与 migration v001 一致 — 依赖：T0.2, T0.5

## Phase 1 — 产品工作台
- [ ] T1.1 ResearchTask CRUD + 生命周期状态机（Created→Planning→Awaiting Approval…）+ 持久化 — 验收：状态非法跃迁被拒绝并有事件 — 依赖：T0.7
- [ ] T1.2 REST API：创建/列表/详情/暂停/恢复/取消/批准 — 验收：openapi 契约测试全过 — 依赖：T1.1
- [ ] T1.3 事件总线 + SSE：事件写入、sequence、幂等、断线续接 — 验收：重复事件不重复改状态；重连后能续接 — 依赖：T1.1
- [ ] T1.4 Dashboard + New Research + 任务详情页（默认值齐全）— 验收：可创建任务并看到真实状态 — 依赖：T1.2, T1.3
- [ ] T1.5 工作流画布初版（React Flow）：节点/边渲染、状态颜色、节点详情抽屉 — 验收：画布状态与 GET 详情快照一致 — 依赖：T1.3, T1.4

## Phase 2 — 自主研究执行
- [ ] T2.1 Planner：结构化输出 + JSON Schema 校验 + 失败重试/退回逻辑 — 验收：非法 JSON 不进入执行；校验失败有可读错误 — 依赖：T1.1
- [ ] T2.2 DAG 编译与校验：环检测、孤节点警告、依赖拓扑排序 — 验收：有环 DAG 被拒绝 — 依赖：T2.1
- [ ] T2.3 Scheduler + Worker Pool：并发上限可配置、排队、超时 — 验收：并发上限生效（用测试证明超限排队）— 依赖：T2.2
- [ ] T2.4 Agent Runtime：6 角色注册、生命周期、Token/工具调用记录 — 验收：Agent 详情页数据来自真实执行 — 依赖：T2.3
- [ ] T2.5 节点级错误处理：重试策略、失败隔离、Checkpoint 写入 — 验收：单节点失败不拖垮整任务；可重试 — 依赖：T2.4
- [ ] T2.6 暂停/恢复/取消全链路 — 验收：暂停后节点停止推进；恢复从 Checkpoint 继续 — 依赖：T2.5

## Phase 3 — 搜索与证据
- [ ] T3.1 Search Provider 接口 + Mock Provider + 至少 1 真实 Provider — 验收：切换 Provider 不改业务代码 — 依赖：T2.4
- [ ] T3.2 网页抓取 + 正文解析 + 快照存储 — 验收：标题/正文/快照可查 — 依赖：T3.1
- [ ] T3.3 URL 规范化 + 内容哈希去重 — 验收：重复 URL/重复内容只留一条 — 依赖：T3.2
- [ ] T3.4 来源中心：分类、可信度、搜索筛选 — 验收：来源列表可按站点/可信度过滤 — 依赖：T3.3
- [ ] T3.5 Evidence：Claim 与 Source 关联、强度标记 — 验收：无来源 Claim 只能是 Unverified — 依赖：T3.4
- [ ] T3.6 Verifier：多源交叉验证、冲突识别 — 验收：构造冲突输入能被标 Conflicting — 依赖：T3.5

## Phase 4 — 报告与交互
- [ ] T4.1 Synthesizer：汇总多 Agent 结果为统一分析 — 验收：汇总保留来源映射不丢失 — 依赖：T3.6
- [ ] T4.2 Reporter：9 节结构化报告生成 — 验收：报告 9 节齐全、引用映射完整 — 依赖：T4.1
- [ ] T4.3 Report Viewer：目录、引用跳转、来源弹窗、表格 — 验收：点击引用能到 Source 详情 — 依赖：T4.2
- [ ] T4.4 导出：Markdown / PDF / HTML — 验收：PDF 非空、内容与 Markdown 一致、无路径穿越 — 依赖：T4.2
- [ ] T4.5 Tool Hub：MCP 注册/发现/启停/调用日志/健康检查/权限 — 验收：高风险工具默认需审批 — 依赖：T2.4
- [ ] T4.6 节点详情、日志时间线、失败重试 UI 完善 — 验收：失败节点可在 UI 重试并有新事件 — 依赖：T2.5, T4.3

## Phase 5 — 测试、部署与发布
- [ ] T5.1 单元测试补齐：关键模块 ≥80% 覆盖率目标（有断言，非凑数）— 验收：覆盖率报告附在 docs/benchmark.md — 依赖：T4.6
- [ ] T5.2 API 集成 + 前端 E2E（创建→规划→执行→证据→报告全链路）— 验收：E2E 在 CI/本地可重复跑通 — 依赖：T4.6
- [ ] T5.3 故障注入：kill Worker 后恢复 — 验收：任务从 Checkpoint 恢复，不重头、不丢已完成节点 — 依赖：T2.6
- [ ] T5.4 压力测试：≥5 并发任务、单任务默认 4 Agent、SSE P95 ≤2s（目标，实测为准）— 验收：实测数据写入 benchmark — 依赖：T5.2
- [ ] T5.5 权限隔离测试 + 依赖漏洞扫描 — 验收：隔离测试通过；高危漏洞有处理记录 — 依赖：T0.3, T5.2
- [ ] T5.6 Docker Compose 从零部署 + 文档 + README 首屏 — 验收：全新机器 `docker compose up` 能跑最终演示 — 依赖：T5.4
- [ ] T5.7 最终演示：真实任务全程 + 2–3 分钟视频脚本执行 — 验收：见 TESTING_AND_ACCEPTANCE.md 最终演示清单 — 依赖：T5.6
