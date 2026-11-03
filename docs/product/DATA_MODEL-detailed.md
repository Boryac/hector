# DATA_MODEL.md — 数据模型与状态机

> 本文是唯一权威口径：`internal/researchos/db/migrations/*.sql`、`internal/researchos/domain/domain.go`、
> `api/openapi.yaml` 三者必须与本文一致。任何字段/状态变更，三处同步改。

- 数据库：PostgreSQL ≥ 14（本地验证版本 16.2）
- 迁移：`internal/researchos/db/migrations/0001_init.sql`，由 `researchos migrate` 顺序执行并记录到 `schema_migrations`
- 查询层：sqlc 生成（`sqlc.yaml` → `internal/researchos/db`），驱动 pgx/v5
- 时间：全部 `TIMESTAMPTZ`，应用层统一 UTC

---

## 1. 表清单

| 表 | 作用 | 关键约束 |
|---|---|---|
| `research_tasks` | 研究任务主表 | `status` CHECK 白名单 |
| `research_runs` | 执行轮次（重试产生新 run） | `UNIQUE(task_id, run_number)` |
| `workflow_nodes` | 工作流节点定义 + 当前执行状态 | `PRIMARY KEY(task_id, id)` |
| `node_executions` | 节点每次尝试的执行历史 | 追加写，不覆盖 |
| `agent_executions` | Agent 角色级执行记录 | 追加写 |
| `sources` | 检索/抓取到的资料 | `UNIQUE(task_id, normalized_url)` |
| `claims` | 结论（原子陈述） | `INDEX(task_id, normalized_key)` |
| `evidence` | 结论 ↔ 资料的证据边 | 唯一性由 `(claim_id, source_id, quote_hash)` 保证 |
| `reports` | 报告多版本 | `UNIQUE(task_id, version)` |
| `task_events` | 事件流（SSE 数据源） | `UNIQUE(task_id, sequence)`，`event_id` 主键 |
| `tool_invocations` | 工具/MCP 调用日志 | 追加写 |
| `mcp_servers` | MCP Server 注册表 | `name` 唯一 |
| `app_settings` | 运行期可改配置 | KV |

所有子表对 `research_tasks(id)` 外键 `ON DELETE CASCADE`：删除任务即清理派生数据，不留孤儿行。

---

## 2. 核心表字段

### 2.1 research_tasks

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | TEXT PK | UUID |
| `goal` | TEXT NOT NULL | 用户研究目标原文，Planner 的输入 |
| `title` | TEXT | 展示标题，缺省由 goal 或 Planner 回填 |
| `depth` | TEXT | `quick` / `standard` / `deep`，决定问题数与检索预算 |
| `scope` | JSONB | 范围约束（语言、站点白名单、时间窗） |
| `max_agents` | INT | 该任务并发 Agent 上限 |
| `max_duration_seconds` | INT | 任务墙钟上限，Worker 超时即失败 |
| `enable_verification` | BOOL | 是否要求 verify 节点（关闭则结论最高只能 partially_supported） |
| `status` | TEXT | 见第 3 节状态机 |
| `created_by` | TEXT NOT NULL | **服务端身份**，不接受客户端传入 |
| `planner_mode` | TEXT | `llm` / `heuristic`，如实标注是否真正调用模型 |
| `plan` / `workflow_definition` | JSONB | Planner 原始输出与编译后的工作流快照 |
| `latest_sequence` | BIGINT | 事件序列水位，`BumpSequence` 原子自增 |
| `stats` | JSONB | 完成时的统计（资料数/结论数/耗时） |

### 2.2 workflow_nodes

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | TEXT | 节点 ID（任务内唯一），同时是 DAG 依赖引用键 |
| `node_type` | TEXT | `search` / `fetch` / `analyze` / `verify` / `synthesize` |
| `agent_role` | TEXT | `planner` / `researcher` / `analyst` / `verifier` / `synthesizer` / `reporter` |
| `depends_on` | TEXT[] | 前置节点 ID 列表，**全部 completed 才可认领** |
| `position` | INT | 认领优先级（拓扑序，稳定） |
| `status` | TEXT | 见 3.2 |
| `attempts` / `max_attempts` | INT | 已尝试次数 / 上限（默认 3） |
| `next_attempt_at` | TIMESTAMPTZ | 退避重试的下次可认领时间 |
| `lease_expires_at` | TIMESTAMPTZ | 租约到期时间，过期即视为 Worker 失联 |
| `worker_id` | TEXT | 当前持有租约的 Worker |

### 2.3 sources

| 字段 | 说明 |
|---|---|
| `url` / `normalized_url` | 原始 URL / 去 utm、fragment、尾斜杠后的规范化 URL |
| `content_hash` | 正文 SHA256，用于跨 URL 的内容级去重 |
| `excerpt` | 抽取后的正文摘录（证据引文的来源） |
| `credibility` | `high` / `medium` / `low` / `downgraded`（抓取失败但用 snippet 降级） |
| `status` | `discovered` / `fetched` / `fetch_failed` / `robots_blocked` |

### 2.4 claims / evidence

- `claims.status`：`unverified` / `supported` / `partially_supported` / `conflicting` / `refuted`
- **硬规则**：没有任何 evidence 的 claim 只能是 `unverified`，不允许出现在「已验证结论」章节
- `evidence.quote` 是原文逐字片段（抽取时经 `quoteGrounded` 校验，长度 ≥6 字符）
- `evidence.stance`：`support` / `refute` / `unknown`

### 2.5 task_events

| 字段 | 说明 |
|---|---|
| `event_id` | PK，幂等键；重复投递被 `ON CONFLICT DO NOTHING` 丢弃 |
| `sequence` | 任务内单调递增，`UNIQUE(task_id, sequence)` |
| `type` | 事件类型，见第 4 节 |
| `payload` | JSONB，类型相关内容 |

SSE 续接：客户端发 `Last-Event-ID: <event_id>` 或 `?after_sequence=<n>`，服务端从该位置之后继续推送。

---

## 3. 状态机

### 3.1 任务状态（白名单见 `domain/transitions.go`）

```
                    ┌─────────────► cancelled ◄─────────┐
                    │                                  │
created ──► planning ──► awaiting_approval ──► running ──► synthesizing ──► completed
   │           │  ▲              │              │  ▲            │
   │           │  └──────────────┘              │  │            │
   │           │     (重新规划)                  │  │            │
   └───────────┴──────────► failed ◄────────────┴──┴────────────┘
                              │  ▲
                              │  └── paused ──┘（恢复）
                              └──► running / planning（人工重试）
```

| From \ To | planning | awaiting_approval | running | synthesizing | paused | completed | failed | cancelled |
|---|---|---|---|---|---|---|---|---|
| created | ✓ | | | | | | ✓ | ✓ |
| planning | | ✓ | ✓ | | | | ✓ | ✓ |
| awaiting_approval | ✓ | | ✓ | | | | ✓ | ✓ |
| running | | | | ✓ | ✓ | ✓ | ✓ | ✓ |
| synthesizing | | | | | ✓ | ✓ | ✓ | ✓ |
| paused | | | ✓ | | | | ✓ | ✓ |
| failed | ✓ | | ✓ | | | | | ✓ |
| completed | | | | | | | | |
| cancelled | | | | | | | | |

- 相同状态视为幂等成功（不写事件、不改时间戳）
- 非法跃迁：写 `research.transition_rejected` 审计事件 → 返回 `409 invalid_transition` → 状态不变
- `completed` / `cancelled` 是终态，不可复活（重跑请新建任务）

### 3.2 节点状态

`pending → (queued) → running → completed | failed | skipped | cancelled`

节点失败但 `attempts < max_attempts` 时回到 `pending` 并设置 `next_attempt_at`（退避 1s/2s/4s）。

---

## 4. 事件类型

| 分组 | 事件 |
|---|---|
| 任务 | `research.created` `research.planning` `research.planned` `research.started` `research.paused` `research.resumed` `research.synthesizing` `research.completed` `research.failed` `research.cancelled` `research.transition_rejected` |
| 工作流 | `workflow.created` `workflow.updated` |
| 节点 | `node.queued` `node.started` `node.progress` `node.completed` `node.failed` `node.retrying` |
| Agent | `agent.created` `agent.started` `agent.completed` `agent.failed` |
| 工具 | `tool.invoked` `tool.completed` `tool.failed` |
| 证据 | `evidence.created` `evidence.verified` |
| 报告 | `report.started` `report.completed` |

新增事件必须同时更新：`domain.go` 常量、`web/lib/event-map.ts` 映射、本文档。

---

## 5. 并发与恢复语义

### 5.1 依赖驱动队列（不复用上游 FIFO 队列）

认领节点的唯一入口是 `ClaimReadyNodes`：

```sql
WITH ready AS (
    SELECT n.task_id, n.id
    FROM workflow_nodes n
    JOIN research_tasks t ON t.id = n.task_id
    WHERE t.status IN ('running','synthesizing')      -- 暂停闸门：非运行态一律不认领
      AND n.status = 'pending'
      AND (n.next_attempt_at IS NULL OR n.next_attempt_at <= now())
      AND NOT EXISTS (                                 -- 依赖必须全部 completed
          SELECT 1 FROM unnest(n.depends_on) AS dep(id)
          WHERE NOT EXISTS (
              SELECT 1 FROM workflow_nodes d
              WHERE d.task_id = n.task_id AND d.id = dep.id AND d.status = 'completed'
          )
      )
    ORDER BY n.position
    LIMIT $1
    FOR UPDATE OF n SKIP LOCKED                        -- 行锁认领，多 Worker 不重复执行
)
UPDATE workflow_nodes w SET status='running', attempts=attempts+1, worker_id=$2,
       lease_expires_at = now() + make_interval(secs => $3), ...
FROM ready WHERE w.task_id=ready.task_id AND w.id=ready.id
RETURNING w.*;
```

要点：
- **依赖语义**：`depends_on` 必须全部 `completed`；任一前置失败 → 该节点由 `FailOrphanNodes` 标失败，任务不会永久挂起
- **暂停闸门**：`t.status IN ('running','synthesizing')` 保证 `paused` 任务不被认领
- **SKIP LOCKED**：多 Worker 并发认领不互相阻塞，且同一节点不会被两个 Worker 同时拿到
- **租约**：`lease_expires_at` 到期视为 Worker 失联，`ResetStaleNodes` 把节点回退到 `pending`（保留 `attempts`，避免无限重建）
- **超时**：节点级 `RESEARCHOS_NODE_TIMEOUT`（默认 180s）、任务级 `max_duration_seconds`

### 5.2 事件顺序

`BumpSequence` 在事务内对 `latest_sequence` 自增并返回新值，与事件插入同事务，因此：
- 同一任务的事件 `sequence` 连续且唯一
- 状态变更与事件写入原子（`Store.Tx` + `EmitTx`），不会出现「状态变了但事件丢了」

### 5.3 恢复路径

| 故障 | 行为 |
|---|---|
| Worker 进程被杀 | 租约到期 → `ResetStaleNodes` → 其他 Worker 重新认领；已完成节点不重跑 |
| 节点执行超时 | 记 `node.failed`，按 `attempts < max_attempts` 退避重试 |
| 节点永久失败 | `FailNode` → `FailOrphanNodes` 级联 → `finalizeTask` 置任务 `failed`，保留已产出的资料与结论 |
| 任务长时间无进展 | Worker 看门狗 `ListRunningTaskIDs` → `finalizeTask` 收敛状态 |
| 服务重启 | 迁移幂等，`research_runs` 记录 run 边界，按节点状态继续 |

---

## 6. 索引设计理由

| 索引 | 服务的查询 |
|---|---|
| `idx_research_tasks_created_by (created_by, created_at DESC)` | 任务列表按身份 + 时间倒序 |
| `idx_research_tasks_status (status)` | 按状态过滤 |
| `idx_workflow_nodes_ready (task_id, status, next_attempt_at)` | 认领扫描的主路径 |
| `idx_sources_task (task_id, created_at)` | 资料面板 |
| `idx_sources_hash (task_id, content_hash)` | 内容级去重 |
| `idx_claims_key (task_id, normalized_key)` | 交叉验证聚类候选 |
| `idx_evidence_task (task_id, claim_id)` | 证据链展开 |
| `idx_task_events_seq (task_id, sequence)` | SSE 续接定位 |

---

## 7. 与 API/前端的映射

| 领域结构 | API | 前端 |
|---|---|---|
| `domain.Task` | `GET /research/{id}` 的 `task` | 任务详情页头部 |
| `domain.Node` | 同上 `nodes[]` | React Flow 画布节点 |
| `domain.Event` | `GET /research/{id}/events` | 事件流面板 |
| `domain.Source` | `GET /research/{id}/sources` | 资料 Tab |
| `domain.Claim` + `Evidence` | `GET /research/{id}/evidence` | 证据 Tab |
| `domain.Report` | `GET /research/{id}/report` | 报告 Tab |
| `domain.AgentExecution` | `GET /research/{id}/agents` | Agent Tab |

---

## 8. 已知限制

1. 尚无多租户：`created_by` 只有单值，跨用户隔离依赖部署边界（见 `docs/security-review.md`）。
2. `depends_on` 用 `TEXT[]` 而非关联表：查询简单，但无法对依赖做外键约束，环检测放在应用层（`dag.Validate`）。
3. 事件表无限增长，未做分区/归档；本地单用户量级可接受，长期需要按 `created_at` 分区。
4. 报告只存 Markdown 与结构化章节，PDF 渲染在导出时进行，不入库。
