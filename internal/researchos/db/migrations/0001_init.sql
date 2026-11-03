-- Hector ResearchOS — 初始迁移
-- 目标数据库：PostgreSQL 14+
-- 约定：
--   * 所有时间戳为 timestamptz（UTC 存储）
--   * 所有 id 为 uuid 文本（应用层生成，便于跨进程/跨库迁移）
--   * 事件表 (task_events) 是唯一审计来源，状态变更必须与事件同事务写入

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- 研究任务
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS research_tasks (
    id                     TEXT PRIMARY KEY,
    title                  TEXT NOT NULL DEFAULT '',
    goal                   TEXT NOT NULL,
    depth                  TEXT NOT NULL DEFAULT 'standard',
    scope                  JSONB NOT NULL DEFAULT '{}'::jsonb,
    max_agents             INT  NOT NULL DEFAULT 4,
    max_duration_seconds   INT  NOT NULL DEFAULT 1800,
    enable_verification    BOOLEAN NOT NULL DEFAULT TRUE,
    status                 TEXT NOT NULL DEFAULT 'created',
    created_by             TEXT NOT NULL,
    planner_mode           TEXT NOT NULL DEFAULT '',
    plan                   JSONB,
    plan_error             TEXT NOT NULL DEFAULT '',
    workflow_definition    JSONB,
    stats                  JSONB NOT NULL DEFAULT '{}'::jsonb,
    latest_sequence        BIGINT NOT NULL DEFAULT 0,
    error                  TEXT NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at             TIMESTAMPTZ,
    finished_at            TIMESTAMPTZ,
    CONSTRAINT research_tasks_status_chk CHECK (status IN (
        'created','planning','awaiting_approval','running','paused','synthesizing',
        'completed','failed','cancelled'
    ))
);

CREATE INDEX IF NOT EXISTS idx_research_tasks_created_by ON research_tasks (created_by, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_research_tasks_status ON research_tasks (status);

-- ---------------------------------------------------------------------------
-- 执行轮次（每次重试产生新 run，历史可查）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS research_runs (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    run_number  INT  NOT NULL,
    trigger     TEXT NOT NULL DEFAULT 'initial',
    status      TEXT NOT NULL DEFAULT 'running',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    UNIQUE (task_id, run_number)
);

-- ---------------------------------------------------------------------------
-- 工作流定义节点 + 当前执行状态（运行态由后端维护，前端不得自造）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflow_nodes (
    id               TEXT NOT NULL,
    task_id          TEXT NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    title            TEXT NOT NULL,
    node_type        TEXT NOT NULL,
    agent_role       TEXT NOT NULL DEFAULT 'researcher',
    agent_name       TEXT NOT NULL DEFAULT '',
    depends_on       TEXT[] NOT NULL DEFAULT '{}',
    position         INT NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT 'pending',
    attempts         INT NOT NULL DEFAULT 0,
    max_attempts     INT NOT NULL DEFAULT 3,
    input            JSONB NOT NULL DEFAULT '{}'::jsonb,
    output           JSONB,
    error            TEXT NOT NULL DEFAULT '',
    progress         INT NOT NULL DEFAULT 0,
    progress_text    TEXT NOT NULL DEFAULT '',
    next_attempt_at  TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ,
    worker_id        TEXT NOT NULL DEFAULT '',
    tokens_total     INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    PRIMARY KEY (task_id, id),
    CONSTRAINT workflow_nodes_status_chk CHECK (status IN (
        'pending','queued','running','completed','failed','skipped','cancelled'
    ))
);

CREATE INDEX IF NOT EXISTS idx_workflow_nodes_ready
    ON workflow_nodes (task_id, status, next_attempt_at);

-- ---------------------------------------------------------------------------
-- 节点执行历史（每次尝试一条）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS node_executions (
    id               TEXT PRIMARY KEY,
    task_id          TEXT NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    run_id           TEXT NOT NULL,
    node_id          TEXT NOT NULL,
    attempt          INT NOT NULL,
    status           TEXT NOT NULL,
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at     TIMESTAMPTZ,
    duration_ms      INT NOT NULL DEFAULT 0,
    error            TEXT NOT NULL DEFAULT '',
    output           JSONB,
    worker_id        TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_node_executions_task ON node_executions (task_id, node_id, started_at DESC);

-- ---------------------------------------------------------------------------
-- Agent 执行记录（角色、模型、Token、工具调用）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agent_executions (
    id           TEXT PRIMARY KEY,
    task_id      TEXT NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    run_id       TEXT NOT NULL DEFAULT '',
    node_id      TEXT NOT NULL DEFAULT '',
    role         TEXT NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    model        TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'running',
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms  INT NOT NULL DEFAULT 0,
    token_usage  JSONB NOT NULL DEFAULT '{}'::jsonb,
    tool_calls   JSONB NOT NULL DEFAULT '[]'::jsonb,
    error        TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_agent_executions_task ON agent_executions (task_id, started_at DESC);

-- ---------------------------------------------------------------------------
-- 资料（Source）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sources (
    id             TEXT PRIMARY KEY,
    task_id        TEXT NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    node_id        TEXT NOT NULL DEFAULT '',
    url            TEXT NOT NULL,
    normalized_url TEXT NOT NULL,
    title          TEXT NOT NULL DEFAULT '',
    site           TEXT NOT NULL DEFAULT '',
    provider       TEXT NOT NULL DEFAULT '',
    snippet        TEXT NOT NULL DEFAULT '',
    excerpt        TEXT NOT NULL DEFAULT '',
    content_hash   TEXT NOT NULL DEFAULT '',
    http_status    INT NOT NULL DEFAULT 0,
    credibility    TEXT NOT NULL DEFAULT 'unknown',
    status         TEXT NOT NULL DEFAULT 'ok',
    error          TEXT NOT NULL DEFAULT '',
    published_at   TIMESTAMPTZ,
    fetched_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, normalized_url)
);

CREATE INDEX IF NOT EXISTS idx_sources_task ON sources (task_id, created_at);
CREATE INDEX IF NOT EXISTS idx_sources_hash ON sources (task_id, content_hash);

-- ---------------------------------------------------------------------------
-- 结论（Claim）与证据（Evidence）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS claims (
    id             TEXT PRIMARY KEY,
    task_id        TEXT NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    node_id        TEXT NOT NULL DEFAULT '',
    statement      TEXT NOT NULL,
    normalized_key TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'unverified',
    confidence     REAL NOT NULL DEFAULT 0,
    conflict_group TEXT NOT NULL DEFAULT '',
    verifier_notes TEXT NOT NULL DEFAULT '',
    source_count   INT NOT NULL DEFAULT 0,
    numeric_value  DOUBLE PRECISION,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT claims_status_chk CHECK (status IN (
        'unverified','supported','partially_supported','conflicting','refuted'
    ))
);

CREATE INDEX IF NOT EXISTS idx_claims_task ON claims (task_id, created_at);
CREATE INDEX IF NOT EXISTS idx_claims_key ON claims (task_id, normalized_key);

CREATE TABLE IF NOT EXISTS evidence (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    claim_id    TEXT NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
    source_id   TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    node_id     TEXT NOT NULL DEFAULT '',
    quote       TEXT NOT NULL DEFAULT '',
    stance      TEXT NOT NULL DEFAULT 'support',
    confidence  REAL NOT NULL DEFAULT 0.5,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (claim_id, source_id),
    CONSTRAINT evidence_stance_chk CHECK (stance IN ('support','contradict','neutral'))
);

CREATE INDEX IF NOT EXISTS idx_evidence_task ON evidence (task_id, claim_id);

-- ---------------------------------------------------------------------------
-- 报告（多版本）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS reports (
    id              TEXT PRIMARY KEY,
    task_id         TEXT NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    version         INT NOT NULL DEFAULT 1,
    title           TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL DEFAULT '',
    markdown        TEXT NOT NULL DEFAULT '',
    sections        JSONB NOT NULL DEFAULT '[]'::jsonb,
    citations       JSONB NOT NULL DEFAULT '[]'::jsonb,
    citation_count  INT NOT NULL DEFAULT 0,
    word_count      INT NOT NULL DEFAULT 0,
    model           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, version)
);

-- ---------------------------------------------------------------------------
-- 事件（唯一审计来源，sequence 在任务内单调递增）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS task_events (
    event_id   TEXT PRIMARY KEY,
    task_id    TEXT NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    run_id     TEXT NOT NULL DEFAULT '',
    node_id    TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    sequence   BIGINT NOT NULL,
    payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_task_events_seq ON task_events (task_id, sequence);

-- ---------------------------------------------------------------------------
-- 工具调用（MCP）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tool_invocations (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL DEFAULT '',
    node_id     TEXT NOT NULL DEFAULT '',
    tool_name   TEXT NOT NULL,
    server_id   TEXT NOT NULL DEFAULT '',
    args        JSONB NOT NULL DEFAULT '{}'::jsonb,
    result      JSONB,
    status      TEXT NOT NULL DEFAULT 'running',
    error       TEXT NOT NULL DEFAULT '',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_tool_invocations_task ON tool_invocations (task_id, started_at DESC);

CREATE TABLE IF NOT EXISTS mcp_servers (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    command    TEXT NOT NULL,
    args       JSONB NOT NULL DEFAULT '[]'::jsonb,
    env        JSONB NOT NULL DEFAULT '{}'::jsonb,
    status     TEXT NOT NULL DEFAULT 'stopped',
    approved   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- 设置（单用户本地模式，键值 JSON）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS app_settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
