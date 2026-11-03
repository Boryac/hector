// 与后端 API_CONTRACT.md 对齐的类型定义。
// 任何字段变更必须同步更新 api/openapi.yaml 与后端 handler。

export type TaskStatus =
  | 'created' | 'planning' | 'awaiting_approval' | 'running' | 'paused'
  | 'synthesizing' | 'completed' | 'failed' | 'cancelled';

export type NodeStatus =
  | 'pending' | 'queued' | 'running' | 'completed' | 'failed' | 'skipped' | 'cancelled';

export interface WorkflowNode {
  id: string;
  type: string;
  title: string;
  agent_role: string;
  depends_on: string[];
}

export interface Edge { source: string; target: string }

export interface Workflow { nodes: WorkflowNode[]; edges: Edge[] }

export interface Question { id: string; question: string; priority: string }

export interface Subtask {
  id: string; title: string; agent_role: string;
  inputs: string[]; outputs: string[]; depends_on: string[]; node_type?: string;
}

export interface Plan {
  goal: string;
  research_questions: Question[];
  subtasks: Subtask[];
  workflow: Workflow;
  risks_and_gaps: string[];
}

export interface Node {
  id: string;
  task_id: string;
  title: string;
  type: string;
  agent_role: string;
  agent_name: string;
  depends_on: string[];
  position: number;
  status: NodeStatus;
  attempts: number;
  max_attempts: number;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  error?: string;
  progress: number;
  progress_text?: string;
  tokens_total: number;
  started_at?: string;
  completed_at?: string;
}

export interface Task {
  id: string;
  title: string;
  goal: string;
  depth: string;
  scope: Record<string, unknown>;
  max_agents: number;
  max_duration_seconds: number;
  enable_verification: boolean;
  status: TaskStatus;
  created_by: string;
  planner_mode: string;
  plan?: Plan;
  plan_error?: string;
  workflow?: Workflow;
  stats: Record<string, unknown>;
  error?: string;
  created_at: string;
  updated_at: string;
  started_at?: string;
  finished_at?: string;
  nodes?: Node[];
  latest_sequence: number;
}

export interface TaskDetail {
  task: Task;
  nodes: Node[] | null;
  workflow: Workflow | null;
  node_status?: Record<string, number>;
  source_count?: number;
  report_summary?: {
    id: string; version: number; title: string;
    citation_count: number; word_count: number; created_at: string;
  };
}

export interface EventItem {
  event_id: string;
  task_id: string;
  run_id: string;
  node_id: string;
  event_type: string;
  timestamp: string;
  sequence: number;
  payload: Record<string, unknown>;
}

export interface Source {
  id: string;
  task_id: string;
  node_id: string;
  url: string;
  normalized_url: string;
  title: string;
  site: string;
  provider: string;
  snippet: string;
  excerpt: string;
  content_hash: string;
  http_status: number;
  credibility: string;
  status: string;
  error?: string;
  fetched_at?: string;
  created_at: string;
  evidence_count: number;
}

export interface Evidence {
  id: string;
  claim_id: string;
  source_id: string;
  quote: string;
  stance: string;
  confidence: number;
  source?: Source;
}

export interface Claim {
  id: string;
  task_id: string;
  node_id: string;
  statement: string;
  status: string;
  confidence: number;
  conflict_group?: string;
  verifier_notes?: string;
  source_count: number;
  numeric_value?: number;
  evidence?: Evidence[];
  created_at: string;
}

export interface Citation {
  index: number;
  claim_id: string;
  source_id: string;
  title: string;
  url: string;
  site: string;
  quote?: string;
}

export interface Report {
  id: string;
  task_id: string;
  version: number;
  title: string;
  summary: string;
  markdown: string;
  sections: { heading: string; body: string }[];
  citations: Citation[];
  citation_count: number;
  word_count: number;
  model: string;
  created_at: string;
}

export interface AgentExecution {
  id: string;
  task_id: string;
  node_id: string;
  role: string;
  name: string;
  model: string;
  status: string;
  started_at: string;
  duration_ms: number;
  token_usage: Record<string, number>;
  error?: string;
}

export interface Health {
  status: string;
  version: string;
  uptime_s: number;
  checks: {
    database: { ok: boolean; latency_ms?: number; migrations?: string[]; error?: string };
    llm: { enabled: boolean; provider: string; model: string; note?: string };
    search: { providers: string[]; note?: string };
    auth: { mode: string };
  };
}
