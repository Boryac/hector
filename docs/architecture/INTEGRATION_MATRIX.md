# INTEGRATION_MATRIX — 前后端对接矩阵

> 生成于 2026-11-03 最终集成阶段。**每一条 PASS 都有可复现的验证依据**：
> 浏览器级证据来自 `upstream/hector/web/scripts/ui-functional.mjs`（CDP 抓真实网络请求）
> 与 `ui-audit.mjs`（控制台错误 + 溢出，1440px 与 390px 双视口）；
> 接口级证据来自 `scripts/verify.sh` 与文中标注的 curl 实测。
>
> 状态取值：PASS / PARTIAL / FAIL / BLOCKED / NOT_IMPLEMENTED。
> 「页面」按当前实际路由列出（设置中心为单页五分区，路由 `/settings/`，五个子路由 302 跳转到分区锚点）。

## 1. 页面 × 后端对接总表

| 页面 | 功能 | 前端实现 | API | 后端实现 | 实测 | 问题 | 状态 |
|---|---|---|---|---|---|---|---|
| Dashboard `/` | 系统状态 | 健康徽标来自轮询 | `GET /api/v1/health` | 鉴权免检探活端点 | ui-functional：请求 2xx；无 React 错误 | — | PASS |
| Dashboard | 任务统计 | 四张指标卡 | `GET /api/v1/stats` | SQL 全量按状态/来源/结论计数 | 同上；本轮新接入（此前前端未消费） | — | PASS |
| Dashboard | 研究活动 | 最近任务 + 事件流 | `GET /api/v1/research` + `GET /research/{id}/events`(ticket) | 列表 + SSE | 同上 | — | PASS |
| Dashboard | 模型配置摘要 | 读设置快照 | `GET /api/v1/settings` | 设置中心快照 | 同上 | — | PASS |
| Tasks `/tasks/` | 任务列表/筛选/搜索 | 表格 + 状态过滤 | `GET /api/v1/research` | 分页 + 过滤 | 同上 | — | PASS |
| Tasks | 创建研究 | 对话框（目标/标题/深度/并发/验证开关） | `POST /api/v1/research` | 校验 + 建任务 + plan_now | 实测：UI 提交后任务落库并出现在列表（审计脚本断言） | — | PASS |
| Workspace `/task/?id=` | 任务详情 + DAG | ReactFlow 画布 | `GET /api/v1/research/{id}` | 节点/边/状态 | ui-audit：画布 808x552、节点渲染正常 | — | PASS |
| Workspace | 实时进度 | SSE | `POST /research/{id}/events/ticket` → `GET /events?ticket=` | 一次性 ticket 短时凭据 | 审计确认 ticket 流建立；ticket 单测 11 条全绿 | — | PASS |
| Workspace | 来源 Tab | 表格 + 详情弹层 | `GET /research/{id}/sources` | 按 Tab 懒加载 | 审计逐 Tab 点击后断言 2xx | — | PASS |
| Workspace | 证据 Tab | 结论 + 引用 + 验证状态 | `GET /research/{id}/evidence` | 同上 | 同上 | — | PASS |
| Workspace | Agent Tab | 执行记录表 | `GET /research/{id}/agents` | 同上 | 同上 | — | PASS |
| Workspace | 报告 Tab | 在线阅读 + 导出 | `GET /research/{id}/report` | 版本化报告 | 同上 | — | PASS |
| Workspace | 操作（批准/暂停/恢复/取消/重规划/重试节点/删除） | 按钮组 + 二次确认 | `POST /research/{id}/{action}` 等 | 状态机校验 | 单测覆盖状态机；批准-执行-完成全链路实测通过 | — | PASS |
| Sources `/sources/` | 跨任务来源库 | 搜索 + Provider 过滤 | `GET /api/v1/sources` | 跨任务查询 | 审计 2xx；1083 条真实数据 | — | PASS |
| Evidence `/evidence/?id=` | 结论与验证状态 | 按任务查询 | `GET /research/{id}/evidence` | 结论 + 证据链 | 审计 2xx | — | PASS |
| Reports `/reports/` | 报告库 | 列表 + 在线阅读 | `GET /api/v1/reports` | 跨任务最新报告 | 审计 2xx；41 条真实数据 | — | PASS |
| Reports | 导出 | Markdown/HTML/PDF 按钮 | `GET /research/{id}/report.{md,html,pdf}` | 服务端渲染 | PDF 链路：HTTP 200、`%PDF` 头、内容与 Markdown 逐项一致（`verify_pdf_content.py`） | — | PASS |
| Settings `/settings/` | 五分区设置 | schema 驱动表单 | `GET /api/v1/settings` | 4 组字段 + 来源标注 | 审计：控件 >10 个、无 4xx | — | PASS |
| Settings | 保存/校验/恢复 | 脏字段批量保存 + 行级错误 | `PUT /settings/{section}`、`DELETE /settings/{section}/{key}` | 整批校验、密钥掩码、部署级 403 | 保存响应只回掩码、掩码回传不改写（API 单测）；部署级 403 实测 | — | PASS |
| Settings | 模型连接测试 | LlmTester | `POST /settings/llm/test` | 真实外呼 | 假 Key 得上游 401（诚实失败） | — | PASS |
| Settings | 检索连接测试 | SearchTester | `POST /settings/search/test` | 逐 Provider 实检 | 4 Provider 全通（619–3618ms，11 条结果） | — | PASS |
| Settings | MCP Hub | 登记/批准/发现/删除 | `POST/GET/DELETE /tools/servers`、`.../discover`、`.../approve` | 批准闸门 + stdio JSON-RPC | 未批准 409 → 批准 → 161ms 发现 2 工具（实测） | — | PASS |
| Settings | 系统分区运行环境 | SystemRuntime 卡片 | `GET /api/v1/system` | 版本/进程/连接池/迁移 | 本轮新接入；审计确认 ≥8 行真实数据 | 此前后端有接口、前端无入口（已修复） | PASS |
| 404 / 错误态 | — | ErrorState / EmptyState | — | — | 审计无 React 错误；空态组件在多个页面使用 | — | PASS |

## 2. API 覆盖核对（后端 41 个操作 vs 前端消费）

后端注册 41 个路由操作（`grep mux.HandleFunc`）。前端消费 24 个。其余为：

| 端点 | 无 UI 入口的原因 | 结论 |
|---|---|---|
| `GET /health` | Dashboard 已消费（免鉴权探活） | PASS |
| `POST /research/{id}/plan`（重新规划） | Workspace「重新规划」按钮消费 | PASS |
| `GET /research/{id}/sources/{sid}` | 来源详情弹层消费 | PASS |
| `GET /tools`、`GET /tools/calls` | **无 UI 入口**。工具执行审计流（工具调用记录）是 API-only，供运维脚本与后续 Agent 面板使用；openapi 已文档化 | PARTIAL（有意保留，见 FINAL_AUDIT §E） |
| `POST /tools/servers/{id}/{action}`（start/stop） | MCP Server 进程由发现时按需拉起；start/stop 供运维手工干预 | PARTIAL（同上） |
| `GET /agents`（跨任务） | API-only 聚合查询，运维用 | PARTIAL（同上） |

## 3. 本轮修复的对接缺陷

1. **`/api/v1/system` 无前端入口** → 设置中心「系统」分区新增 SystemRuntime 卡片（版本、运行时长、Go/平台、连接池、迁移、模型、检索源）。
2. **`/api/v1/stats` 无前端入口** → Dashboard 指标卡接入服务端全量计数，消除「客户端只统计最近 100 条」的口径失真。
3. **报告无法导出 PDF** → 新增服务端真实排版（见 `report/pdf.go`）+ 前端 PDF 按钮 + 内容一致性校验脚本。
4. **页面审计脚本只覆盖 5 条路由** → `ui-functional.mjs` 补齐 evidence/workspace/settings 三条路由 + 按 Tab 懒加载断言 + 真实创建任务交互。

## 4. 已知边界

- Chrome 无头在此环境只能以 `--remote-debugging-port` 方式运行（`--screenshot` 模式无输出），两个审计脚本均依赖 CDP 端口 9333；有桌面浏览器的机器可直接复跑。
- 设置中心的「保存后行为变化」验证：API 层已验证热更新生效（`workers` 需重启类、`model.*` 即时类），UI 层验证到保存成功反馈与掩码显示；「改配置 → 重跑任务 → 观察行为差异」的闭环需要真实 LLM Key（当前 BLOCKED）。
