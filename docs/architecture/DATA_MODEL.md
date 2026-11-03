# DATA_MODEL.md — 数据模型（先定义，再写执行链，再做前端）

## 实体关系（文字版 ERD）

ResearchTask (1) ── (1) WorkflowDefinition ── (n) WorkflowNode ── (n) NodeExecution
ResearchTask (1) ── (n) ResearchRun（每次执行/重试产生新 Run，历史可查）
NodeExecution (1) ── (1) AgentExecution（可为空，如纯系统节点）
ResearchTask (1) ── (n) Source ── (n) Evidence ── (n) Claim（结论）
ResearchTask (1) ── (0/1) Report（可多版本，保留历史版本）
ResearchTask (1) ── (n) TaskEvent（事件即审计日志，幂等）

## 核心表 / 结构（字段为 V1 最小必需，新增字段需同步改本文件 + migration）

### ResearchTask
id, title, goal（自然语言目标原文）, depth（quick/standard/deep）, scope（JSON：范围、排除项）,
max_agents, max_duration, enable_verification, status（见生命周期）, created_at, updated_at,
created_by（用户标识，Phase 0 身份审查后锁定其来源，不得信任客户端传入）, current_run_id

生命周期：Created → Planning → Awaiting Approval → Running → Synthesizing → Completed
异常：Failed / Paused / Cancelled。每次状态变更必须写事件 + 持久化，禁止只改内存。

### WorkflowDefinition vs WorkflowRun（必须分开）
- Definition：workflow_id, nodes[], edges[]，纯结构，无执行状态。
  节点示例：`{"id":"planner","type":"planner"}`；边：`{"source":"planner","target":"research_a"}`。
- Run / NodeExecution：run_id, node_id, execution_id, status, started_at, completed_at,
  retry_count, input_reference, output_reference, error, checkpoint_id。
  **运行状态必须由后端执行引擎维护，前端不得自造。**

### AgentExecution
id, task_id, run_id, node_id, role（planner/researcher/analyst/verifier/synthesizer/reporter）,
name, model, status（Pending/Running/Completed/Failed/Paused/Cancelled/Waiting for Approval）,
started_at, duration_ms, token_usage（prompt/completion/total）, tool_calls（JSON 数组）,
output_reference, error, retry_count

### Source（资料）
id, task_id, agent_id, url（规范化后）, title, site, fetched_at, summary,
content_hash（去重依据）, raw_snapshot_ref（对象存储引用）, credibility（high/medium/low/unknown）,
category。URL 规范化规则：去 utm/fragment、统一 scheme/host 小写、去尾斜杠（除根路径）。

### Evidence / Claim
claim_id, task_id, claim_text, status（Supported / Partially Supported / Conflicting / Unverified）,
confidence（0-1 + 判定依据）, source_ids[], verifier_notes, conflict_with[]。
规则：无 source_ids 的 Claim 只能是 Unverified；Verifier 不得把模型输出直接改写为 Supported。

### Report
id, task_id, version, markdown, html_ref, pdf_ref, sections（JSON：摘要/目标范围/核心发现/
分章节分析/对比表/关键结论/争议与不确定性/信息来源/方法与局限性）, citations[]（claim_id→source_id 映射）,
created_at。引用跳转：报告内每个引用必须能点开到 Source 详情。

### TaskEvent（见 EVENT_PROTOCOL.md）
event_id（唯一，幂等键）, task_id, run_id, node_id, event_type, timestamp, sequence（单调递增）,
payload。前端断线后凭 sequence 续接，必要时拉任务快照。

## Planner 输出 Schema（结构化，禁止纯文本）

```json
{
  "goal": "string",
  "research_questions": [{"id": "q1", "question": "string", "priority": "high|medium|low"}],
  "subtasks": [{"id": "research_a", "title": "string", "agent_role": "researcher",
    "inputs": ["string"], "outputs": ["string"], "depends_on": []}],
  "workflow": {"nodes": [], "edges": []},
  "risks_and_gaps": ["string"]
}
```

Planner 输出必须通过 JSON Schema 校验，校验失败不得进入执行，需重试或退回 Awaiting Approval
并展示可读错误，不得把未解析文本当作计划执行。
