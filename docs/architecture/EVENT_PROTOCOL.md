# EVENT_PROTOCOL.md — 统一事件协议

## 事件模型（前后端唯一依据）

```json
{
  "event_id": "evt_001",
  "task_id": "task_001",
  "run_id": "run_001",
  "node_id": "research_a",
  "event_type": "node.started",
  "timestamp": "2026-11-03T12:00:00Z",
  "sequence": 42,
  "payload": {"agent_name": "Open Source Researcher"}
}
```

字段规则：event_id 全局唯一（UUID 或 evt_ + 随机串）；sequence 在同一 task 内单调递增；
timestamp 用 UTC ISO8601；payload 只放事件所需字段，不得放 API Key、Token、用户敏感信息。

## 事件类型（不得擅自重命名，新增需同步改本文件 + 前端映射 + 测试）

```
research.created / research.planning / research.started / research.paused / research.resumed
research.completed / research.failed
workflow.created / workflow.updated
node.queued / node.started / node.progress / node.completed / node.failed / node.retrying
agent.created / agent.started / agent.completed / agent.failed
tool.invoked / tool.completed / tool.failed
evidence.created / evidence.verified
report.started / report.completed
```

## 幂等与重放

- 消费端按 event_id 去重，重复事件不得重复改状态、重复计数 Token、重复写证据。
- 事件写入与状态变更应在同一事务或用 Outbox 模式保证“状态变了事件一定有，事件有了状态可重放”。
- 前端断线重连流程：先 GET 任务快照（含各节点最新状态 + 最新 sequence）→ 再 SSE 从 sequence+1 续接；
  若 SSE 序号跳号，重新拉快照，不得静默跳过。
- SSE 端点：`GET /api/v1/research/{task_id}/events`，Content-Type text/event-stream，
  支持 `Last-Event-ID` / `?after_sequence=` 两种续接方式（至少实现一种，并在 API 文档写明）。

## 前端映射规则

- node.* → 画布节点颜色/进度/日志；agent.* → Agent 执行中心；tool.* → 工具调用日志；
  evidence.* → 证据中心；report.* → 报告中心。映射表写在 web/lib/event-map.ts，禁止在组件里散落硬编码。
- 进度：node.progress 的 payload 必须含 `progress`（0-100）或可解释的阶段文本，不得用前端计时器伪造进度。
