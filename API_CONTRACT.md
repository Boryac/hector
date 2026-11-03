# API_CONTRACT.md — REST + SSE 接口契约

Base: `/api/v1` · 认证：Phase 0 锁定（见 SECURITY_REVIEW.md，默认先做单用户本地模式 + 可插拔 Auth 中间件，
不得把 user_id 完全信任客户端传入）。所有接口返回结构化错误：`{code, message, details?}`。

## 研究任务

- `POST /research` — 创建任务。Body：goal（必填）、depth、scope、max_agents（默认 4）、
  max_duration、enable_verification（默认 true）。返回 task + 初始 workflow 预览占位（未规划前不得伪造节点）。
- `GET /research` — 列表（分页、按 status 过滤）。
- `GET /research/{id}` — 详情（含 workflow 定义、最新 Run、节点状态快照）。
- `POST /research/{id}/approve` — 批准规划，进入 Running（Awaiting Approval → Running）。
- `POST /research/{id}/pause` / `POST /research/{id}/resume` / `POST /research/{id}/cancel`
- `POST /research/{id}/nodes/{node_id}/retry` — 节点级重试。
- `GET /research/{id}/events` — SSE（见 EVENT_PROTOCOL.md）。
- `GET /research/{id}/events?after_sequence=N` — 事件重放/分页拉取（JSON 数组）。

## 规划 / Agent / 资料 / 证据 / 报告 / 工具

- `POST /research/{id}/plan` — 触发/重新规划，返回结构化 Planner 输出（DATA_MODEL.md Schema），校验失败返回 422。
- `GET /research/{id}/agents` / `GET /research/{id}/agents/{agent_id}` — Agent 执行详情（含工具调用、Token）。
- `GET /research/{id}/sources` — 资料列表（搜索、筛选、按可信度/站点过滤），`GET /research/{id}/sources/{source_id}`。
- `GET /research/{id}/evidence` / `GET /research/{id}/evidence/{claim_id}` — 证据与冲突。
- `GET /research/{id}/report` — 最新报告；`GET /research/{id}/report?version=N` 历史版本；
  `GET /research/{id}/report.md` / `.pdf` / `.html` — 导出（PDF 生成失败不得返回空文件冒充成功）。
- `GET /tools` / `POST /tools/servers` / `POST /tools/servers/{id}/start|stop` / `GET /tools/calls` — MCP 管理。
- `GET /health` — 健康检查（DB 连通性分级返回，不得只返回 ok 而掩盖 DB 断连）。
- `GET /settings` / `PUT /settings` — 配置（API Key 字段只返回掩码，如 `sk-****abcd`，绝不回显明文）。

完整字段见 `api/openapi.yaml`。Codex 改接口必须同步改 openapi.yaml + 本文件 + 前端 types，
并在 verify.sh 里跑契约检查（openapi 校验 + 关键接口冒烟）。

## 并发与超时约定

- Worker Pool 并发上限可配置（默认单任务 4 Agent，系统级可配置）；超限任务排队，不得无界创建 Goroutine。
- 节点超时、工具调用超时、搜索超时均可配置，超时必须产生 node.failed / tool.failed 事件，不得静默挂起。
- 大任务不得阻塞 Web API：API 只做入队与状态查询，重执行走 Worker。
